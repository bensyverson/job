package job

import (
	"database/sql"
	"fmt"
	"strconv"
)

const DefaultClaimTTLSeconds int64 = 900

func ParseDuration(s string) (int64, error) {
	if s == "" {
		return DefaultClaimTTLSeconds, nil
	}

	last := s[len(s)-1]
	numStr := s[:len(s)-1]
	if len(numStr) == 0 {
		return 0, fmt.Errorf("invalid duration %q: missing number", s)
	}

	num, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: %w", s, err)
	}

	switch last {
	case 's':
		return num, nil
	case 'm':
		return num * 60, nil
	case 'h':
		return num * 3600, nil
	case 'd':
		return num * 86400, nil
	default:
		return 0, fmt.Errorf("invalid duration %q: unknown unit %q", s, string(last))
	}
}

func checkClaimOwnership(tx dbtx, shortID, caller string) error {
	task, err := GetTaskByShortID(tx, shortID)
	if err != nil {
		return err
	}
	if task == nil {
		return fmt.Errorf("task %q not found", shortID)
	}
	if task.Status != "claimed" || task.ClaimedBy == nil {
		return nil
	}
	if *task.ClaimedBy == caller {
		return nil
	}

	var callerOnceHeld bool
	err = tx.QueryRow(
		`SELECT EXISTS(
			SELECT 1 FROM events
			WHERE task_id = ? AND event_type = 'claimed' AND actor = ?
		)`, task.ID, caller,
	).Scan(&callerOnceHeld)
	if err != nil {
		return err
	}

	now := CurrentNowFunc().Unix()

	if callerOnceHeld {
		var claimedAt int64
		if err := tx.QueryRow(
			`SELECT created_at FROM events
			 WHERE task_id = ? AND event_type = 'claimed' AND actor = ?
			 ORDER BY created_at DESC, id DESC LIMIT 1`,
			task.ID, *task.ClaimedBy,
		).Scan(&claimedAt); err != nil && err != sql.ErrNoRows {
			return err
		}
		ago := max(now-claimedAt, 0)
		return fmt.Errorf("your claim on %s expired; it is now held by %s (claimed %s ago). Run 'claim %s' to take it back.",
			shortID, *task.ClaimedBy, FormatDuration(ago), shortID)
	}

	expires := "0s"
	if task.ClaimExpiresAt != nil {
		left := *task.ClaimExpiresAt - now
		if left > 0 {
			expires = FormatDuration(left)
		}
	}
	return fmt.Errorf("task %s is claimed by %s (expires in %s). Wait for expiry, or ask %s to release.",
		shortID, *task.ClaimedBy, expires, *task.ClaimedBy)
}

func expireStaleClaims(db *sql.DB, actor string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := expireStaleClaimsInTx(tx, actor); err != nil {
		return err
	}
	return tx.Commit()
}

func expireStaleClaimsInTx(tx dbtx, actor string) error {
	now := CurrentNowFunc().Unix()
	rows, err := tx.Query(
		"SELECT id, claimed_by FROM tasks WHERE status = 'claimed' AND claim_expires_at < ? AND deleted_at IS NULL",
		now,
	)
	if err != nil {
		return err
	}

	type expired struct {
		id        int64
		claimedBy *string
	}
	var expiredClaims []expired
	for rows.Next() {
		var e expired
		var claimedBy sql.NullString
		if err := rows.Scan(&e.id, &claimedBy); err != nil {
			rows.Close()
			return err
		}
		if claimedBy.Valid {
			cb := claimedBy.String
			e.claimedBy = &cb
		}
		expiredClaims = append(expiredClaims, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, e := range expiredClaims {
		if _, err := tx.Exec(
			"UPDATE tasks SET status = 'available', claimed_by = NULL, claim_expires_at = NULL, updated_at = ? WHERE id = ?",
			now, e.id,
		); err != nil {
			return err
		}
		var wasClaimedBy string
		if e.claimedBy != nil {
			wasClaimedBy = *e.claimedBy
		}
		if err := recordEvent(tx, e.id, "claim_expired", actor, map[string]any{
			"was_claimed_by": wasClaimedBy,
		}); err != nil {
			return err
		}
	}
	return nil
}

// openChildFilter returns the SQL fragment that selects rows counting as
// "open children" under leaf-frontier semantics: not yet done, not
// canceled, not soft-deleted. Shared by countOpenChildren,
// queryAvailableLeafFrontier, cascadeAutoCloseAncestors, and
// findOpenDescendants so the four sites can't drift.
//
// alias is the table alias the caller uses (e.g. "c" in a subtree join,
// or "" when querying the bare tasks table). The helper returns the
// fragment pre-prefixed so callers can concatenate it into a WHERE.
func openChildFilter(alias string) string {
	if alias == "" {
		return "status NOT IN ('done', 'canceled') AND deleted_at IS NULL"
	}
	return alias + ".status NOT IN ('done', 'canceled') AND " + alias + ".deleted_at IS NULL"
}

// countOpenChildren returns the number of direct children of taskID whose
// status is neither "done" nor "canceled". Used to enforce leaf-frontier
// claim semantics: a task with open children has no executable work of its
// own and should not be claimed directly.
func countOpenChildren(tx dbtx, taskID int64) (int, error) {
	var n int
	err := tx.QueryRow(
		`SELECT COUNT(*) FROM tasks WHERE parent_id = ? AND `+openChildFilter(""),
		taskID,
	).Scan(&n)
	return n, err
}

func RunClaim(db *sql.DB, shortID, duration, actor string, force bool) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := expireStaleClaimsInTx(tx, actor); err != nil {
		return err
	}

	task, err := GetTaskByShortID(tx, shortID)
	if err != nil {
		return err
	}
	if task == nil {
		return fmt.Errorf("task %q not found", shortID)
	}
	if task.Status == "done" {
		return fmt.Errorf("task %s is done", shortID)
	}
	openChildren, err := countOpenChildren(tx, task.ID)
	if err != nil {
		return err
	}
	if openChildren > 0 {
		return fmt.Errorf(
			"task %s has %d open children; claim a leaf instead, or run 'next %s all' to see them",
			shortID, openChildren, shortID,
		)
	}
	if task.Status == "claimed" && !force {
		holder := ""
		if task.ClaimedBy != nil {
			holder = *task.ClaimedBy
		}
		if holder == actor {
			return fmt.Errorf("task %s is already claimed by you. Use 'heartbeat' to refresh, or 'release' to stop.", shortID)
		}
		expires := "0s"
		if task.ClaimExpiresAt != nil {
			left := *task.ClaimExpiresAt - CurrentNowFunc().Unix()
			if left > 0 {
				expires = FormatDuration(left)
			}
		}
		return fmt.Errorf("task %s is claimed by %s (expires in %s). Wait for expiry, or ask %s to release.",
			shortID, holder, expires, holder)
	}

	seconds, err := ParseDuration(duration)
	if err != nil {
		return err
	}

	now := CurrentNowFunc().Unix()
	expiresAt := now + seconds

	if _, err := tx.Exec(
		"UPDATE tasks SET status = 'claimed', claimed_by = ?, claim_expires_at = ?, updated_at = ? WHERE id = ?",
		actor, expiresAt, now, task.ID,
	); err != nil {
		return err
	}

	if err := recordEvent(tx, task.ID, "claimed", actor, map[string]any{
		"duration":   duration,
		"expires_at": expiresAt,
	}); err != nil {
		return err
	}

	return tx.Commit()
}

func RunRelease(db *sql.DB, shortID, actor string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := expireStaleClaimsInTx(tx, actor); err != nil {
		return err
	}

	task, err := GetTaskByShortID(tx, shortID)
	if err != nil {
		return err
	}
	if task == nil {
		return fmt.Errorf("task %q not found", shortID)
	}
	if task.Status != "claimed" {
		return fmt.Errorf("task %s is not claimed (status: %s)", shortID, task.Status)
	}
	if task.ClaimedBy == nil || *task.ClaimedBy != actor {
		holder := ""
		if task.ClaimedBy != nil {
			holder = *task.ClaimedBy
		}
		return fmt.Errorf("task %s is claimed by %s, not you. 'release' operates only on your own claims.",
			shortID, holder)
	}

	now := CurrentNowFunc().Unix()
	if _, err := tx.Exec(
		"UPDATE tasks SET status = 'available', claimed_by = NULL, claim_expires_at = NULL, updated_at = ? WHERE id = ?",
		now, task.ID,
	); err != nil {
		return err
	}

	if err := recordEvent(tx, task.ID, "released", actor, map[string]any{}); err != nil {
		return err
	}

	return tx.Commit()
}

// queryAvailableTasks returns the available, unblocked, unclaimed tasks under
// the given parent (or root tasks when parentShortID is empty), in sort order.
// Used by both `next` (single) and `next all` (frontier). When labelName is
// non-empty, only tasks carrying that label are returned.
//
// Leaf-frontier semantics: by default, tasks with open children are excluded
// (they are not claimable work themselves — their children are). The search
// descends through such parents and surfaces their leaf descendants instead.
// Passing includeParents=true restores the pre-leaf-frontier behavior of
// returning direct children of the scope only.
//
// "Open children" means status NOT IN ('done', 'canceled'). A task whose
// children are all closed is itself treated as a leaf.
func queryAvailableTasks(db *sql.DB, parentShortID string, limit int, labelName string, includeParents bool) ([]*Task, error) {
	var parentID *int64
	if parentShortID != "" {
		parent, err := GetTaskByShortID(db, parentShortID)
		if err != nil {
			return nil, err
		}
		if parent == nil {
			return nil, fmt.Errorf("task %q not found", parentShortID)
		}
		parentID = &parent.ID
	}

	if includeParents {
		return queryAvailableDirectChildren(db, parentID, limit, labelName)
	}
	return queryAvailableLeafFrontier(db, parentID, limit, labelName)
}

// queryAvailableDirectChildren implements the legacy behavior used by
// --include-parents: return direct children of the scope (or root tasks),
// regardless of whether they have open children of their own.
func queryAvailableDirectChildren(db *sql.DB, parentID *int64, limit int, labelName string) ([]*Task, error) {
	var parentFilter string
	var args []any
	if parentID != nil {
		parentFilter = "AND t.parent_id = ?"
		args = append(args, *parentID)
	} else {
		parentFilter = "AND t.parent_id IS NULL"
	}

	labelFilter := ""
	if labelName != "" {
		labelFilter = "AND EXISTS (SELECT 1 FROM task_labels tl WHERE tl.task_id = t.id AND tl.name = ?)"
		args = append(args, labelName)
	}

	limitClause := ""
	if limit > 0 {
		limitClause = fmt.Sprintf(" LIMIT %d", limit)
	}

	query := fmt.Sprintf(`
		SELECT t.id, t.short_id, t.parent_id, t.title, t.description, t.status, t.sort_order,
		       t.claimed_by, t.claim_expires_at, t.completion_note, t.created_at, t.updated_at, t.deleted_at
		FROM tasks t
		WHERE t.status = 'available' AND t.deleted_at IS NULL %s %s
		  AND NOT EXISTS (
		    SELECT 1 FROM blocks b
		    JOIN tasks bt ON bt.id = b.blocker_id
		    WHERE b.blocked_id = t.id AND bt.status != 'done' AND bt.deleted_at IS NULL
		  )
		ORDER BY t.sort_order%s
	`, parentFilter, labelFilter, limitClause)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []*Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// queryAvailableLeafFrontier implements leaf-frontier semantics: descend
// through the subtree rooted at scope (or all roots) and return tasks that
// are available, unblocked, and have no open children. Results are ordered
// by depth-first sort_order traversal so sibling-declaration order is
// preserved.
func queryAvailableLeafFrontier(db *sql.DB, parentID *int64, limit int, labelName string) ([]*Task, error) {
	var anchorFilter string
	var args []any
	if parentID != nil {
		anchorFilter = "t.parent_id = ?"
		args = append(args, *parentID)
	} else {
		anchorFilter = "t.parent_id IS NULL"
	}

	labelFilter := ""
	if labelName != "" {
		labelFilter = "AND EXISTS (SELECT 1 FROM task_labels tl WHERE tl.task_id = t.id AND tl.name = ?)"
		args = append(args, labelName)
	}

	limitClause := ""
	if limit > 0 {
		limitClause = fmt.Sprintf(" LIMIT %d", limit)
	}

	// The recursive CTE builds a depth-first walk of the subtree. sort_path
	// concatenates zero-padded sort_order values at each level so that
	// lexicographic ordering matches preorder traversal. Six digits of padding
	// accommodates sort_order values up to 999999, well above any realistic
	// sibling count.
	query := fmt.Sprintf(`
		WITH RECURSIVE subtree(id, sort_path) AS (
			SELECT t.id, printf('%%06d', t.sort_order)
			FROM tasks t
			WHERE %s AND t.deleted_at IS NULL
			UNION ALL
			SELECT t.id, s.sort_path || '/' || printf('%%06d', t.sort_order)
			FROM tasks t JOIN subtree s ON t.parent_id = s.id
			WHERE t.deleted_at IS NULL
		)
		SELECT t.id, t.short_id, t.parent_id, t.title, t.description, t.status, t.sort_order,
		       t.claimed_by, t.claim_expires_at, t.completion_note, t.created_at, t.updated_at, t.deleted_at
		FROM tasks t JOIN subtree s ON s.id = t.id
		WHERE t.status = 'available' AND t.deleted_at IS NULL %s
		  AND NOT EXISTS (
		    SELECT 1 FROM blocks b
		    JOIN tasks bt ON bt.id = b.blocker_id
		    WHERE b.blocked_id = t.id AND bt.status != 'done' AND bt.deleted_at IS NULL
		  )
		  AND NOT EXISTS (
		    SELECT 1 FROM tasks c
		    WHERE c.parent_id = t.id
		      AND %s
		  )
		ORDER BY s.sort_path%s
	`, anchorFilter, labelFilter, openChildFilter("c"), limitClause)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []*Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

func RunNext(db *sql.DB, parentShortID, actor string) (*Task, error) {
	return RunNextFiltered(db, parentShortID, actor, "", false)
}

func RunNextFiltered(db *sql.DB, parentShortID, actor, labelName string, includeParents bool) (*Task, error) {
	if err := expireStaleClaims(db, actor); err != nil {
		return nil, err
	}
	tasks, err := queryAvailableTasks(db, parentShortID, 1, labelName, includeParents)
	if err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		return nil, fmt.Errorf("No available tasks. Run 'list all' to see blocked or claimed work.")
	}
	return tasks[0], nil
}

func runNextAll(db *sql.DB, parentShortID, actor string) ([]*Task, error) {
	return RunNextAllFiltered(db, parentShortID, actor, "", false)
}

func RunNextAllFiltered(db *sql.DB, parentShortID, actor, labelName string, includeParents bool) ([]*Task, error) {
	if err := expireStaleClaims(db, actor); err != nil {
		return nil, err
	}
	return queryAvailableTasks(db, parentShortID, 0, labelName, includeParents)
}

func RunClaimNext(db *sql.DB, parentShortID, duration, actor string, force bool) (*Task, error) {
	return RunClaimNextFiltered(db, parentShortID, duration, actor, force, false)
}

func RunClaimNextFiltered(db *sql.DB, parentShortID, duration, actor string, force, includeParents bool) (*Task, error) {
	task, err := RunNextFiltered(db, parentShortID, actor, "", includeParents)
	if err != nil {
		return nil, err
	}

	if err := RunClaim(db, task.ShortID, duration, actor, force); err != nil {
		return nil, err
	}

	task, err = GetTaskByShortID(db, task.ShortID)
	if err != nil {
		return nil, err
	}
	return task, nil
}

func formatClaimExpires(claimedBy *string, claimExpiresAt *int64) string {
	by := ""
	if claimedBy != nil {
		by = " by " + *claimedBy
	}
	if claimExpiresAt != nil {
		remaining := *claimExpiresAt - CurrentNowFunc().Unix()
		if remaining > 0 {
			return fmt.Sprintf("claimed%s, expires in %s", by, FormatDuration(remaining))
		}
		return fmt.Sprintf("claimed%s", by)
	}
	return fmt.Sprintf("claimed%s", by)
}

func parseDurationFromArgs(args []string) (duration string, who string) {
	duration = ""
	who = ""
	byIdx := -1
	for i, a := range args {
		if a == "by" {
			byIdx = i
			break
		}
	}
	if byIdx >= 0 {
		if byIdx > 0 {
			duration = args[0]
		}
		if len(args) > byIdx+1 {
			who = args[byIdx+1]
		}
	} else if len(args) > 0 {
		duration = args[0]
	}
	return
}

func IsDuration(s string) bool {
	if len(s) == 0 {
		return false
	}
	last := s[len(s)-1]
	if last != 's' && last != 'm' && last != 'h' && last != 'd' {
		return false
	}
	numStr := s[:len(s)-1]
	if len(numStr) == 0 {
		return false
	}
	_, err := strconv.ParseInt(numStr, 10, 64)
	return err == nil
}

func parseNextParentAndDuration(args []string) (parentShortID, duration string) {
	if len(args) == 0 {
		return "", ""
	}
	if IsDuration(args[0]) {
		return "", args[0]
	}
	return args[0], ""
}
