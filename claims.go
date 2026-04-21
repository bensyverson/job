package main

import (
	"database/sql"
	"fmt"
	"strconv"
)

const defaultClaimTTLSeconds int64 = 900

func parseDuration(s string) (int64, error) {
	if s == "" {
		return defaultClaimTTLSeconds, nil
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
	task, err := getTaskByShortID(tx, shortID)
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

	now := currentNowFunc().Unix()

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
		ago := now - claimedAt
		if ago < 0 {
			ago = 0
		}
		return fmt.Errorf("your claim on %s expired; it is now held by %s (claimed %s ago). Run 'claim %s' to take it back.",
			shortID, *task.ClaimedBy, formatDuration(ago), shortID)
	}

	expires := "0s"
	if task.ClaimExpiresAt != nil {
		left := *task.ClaimExpiresAt - now
		if left > 0 {
			expires = formatDuration(left)
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
	now := currentNowFunc().Unix()
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

func runClaim(db *sql.DB, shortID, duration, actor string, force bool) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := expireStaleClaimsInTx(tx, actor); err != nil {
		return err
	}

	task, err := getTaskByShortID(tx, shortID)
	if err != nil {
		return err
	}
	if task == nil {
		return fmt.Errorf("task %q not found", shortID)
	}
	if task.Status == "done" {
		return fmt.Errorf("task %s is done", shortID)
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
			left := *task.ClaimExpiresAt - currentNowFunc().Unix()
			if left > 0 {
				expires = formatDuration(left)
			}
		}
		return fmt.Errorf("task %s is claimed by %s (expires in %s). Wait for expiry, or ask %s to release.",
			shortID, holder, expires, holder)
	}

	seconds, err := parseDuration(duration)
	if err != nil {
		return err
	}

	now := currentNowFunc().Unix()
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

func runRelease(db *sql.DB, shortID, actor string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := expireStaleClaimsInTx(tx, actor); err != nil {
		return err
	}

	task, err := getTaskByShortID(tx, shortID)
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

	now := currentNowFunc().Unix()
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
func queryAvailableTasks(db *sql.DB, parentShortID string, limit int, labelName string) ([]*Task, error) {
	var parentFilter string
	var args []any
	if parentShortID != "" {
		parent, err := getTaskByShortID(db, parentShortID)
		if err != nil {
			return nil, err
		}
		if parent == nil {
			return nil, fmt.Errorf("task %q not found", parentShortID)
		}
		parentFilter = "AND t.parent_id = ?"
		args = append(args, parent.ID)
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

func runNext(db *sql.DB, parentShortID, actor string) (*Task, error) {
	return runNextFiltered(db, parentShortID, actor, "")
}

func runNextFiltered(db *sql.DB, parentShortID, actor, labelName string) (*Task, error) {
	if err := expireStaleClaims(db, actor); err != nil {
		return nil, err
	}
	tasks, err := queryAvailableTasks(db, parentShortID, 1, labelName)
	if err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		return nil, fmt.Errorf("No available tasks. Run 'list all' to see blocked or claimed work.")
	}
	return tasks[0], nil
}

func runNextAll(db *sql.DB, parentShortID, actor string) ([]*Task, error) {
	return runNextAllFiltered(db, parentShortID, actor, "")
}

func runNextAllFiltered(db *sql.DB, parentShortID, actor, labelName string) ([]*Task, error) {
	if err := expireStaleClaims(db, actor); err != nil {
		return nil, err
	}
	return queryAvailableTasks(db, parentShortID, 0, labelName)
}

func runClaimNext(db *sql.DB, parentShortID, duration, actor string, force bool) (*Task, error) {
	task, err := runNext(db, parentShortID, actor)
	if err != nil {
		return nil, err
	}

	if err := runClaim(db, task.ShortID, duration, actor, force); err != nil {
		return nil, err
	}

	task, err = getTaskByShortID(db, task.ShortID)
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
		remaining := *claimExpiresAt - currentNowFunc().Unix()
		if remaining > 0 {
			return fmt.Sprintf("claimed%s, expires in %s", by, formatDuration(remaining))
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

func isDuration(s string) bool {
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
	if isDuration(args[0]) {
		return "", args[0]
	}
	return args[0], ""
}
