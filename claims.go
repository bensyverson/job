package main

import (
	"database/sql"
	"fmt"
	"strconv"
)

func parseDuration(s string) (int64, error) {
	if s == "" {
		return 3600, nil
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

func expireStaleClaims(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := expireStaleClaimsInTx(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func expireStaleClaimsInTx(tx dbtx) error {
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
		if err := recordEvent(tx, e.id, "claim_expired", map[string]any{
			"was_claimed_by": wasClaimedBy,
		}); err != nil {
			return err
		}
	}
	return nil
}

func runClaim(db *sql.DB, shortID, duration, who string, force bool) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := expireStaleClaimsInTx(tx); err != nil {
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
		by := ""
		if task.ClaimedBy != nil {
			by = " by " + *task.ClaimedBy
		}
		remaining := ""
		if task.ClaimExpiresAt != nil {
			left := *task.ClaimExpiresAt - currentNowFunc().Unix()
			if left > 0 {
				remaining = fmt.Sprintf(" (expires in %s)", formatDuration(left))
			}
		}
		return fmt.Errorf("task %s is already claimed%s%s", shortID, by, remaining)
	}

	seconds, err := parseDuration(duration)
	if err != nil {
		return err
	}

	now := currentNowFunc().Unix()
	expiresAt := now + seconds

	var whoVal any
	if who != "" {
		whoVal = who
	}
	if _, err := tx.Exec(
		"UPDATE tasks SET status = 'claimed', claimed_by = ?, claim_expires_at = ?, updated_at = ? WHERE id = ?",
		whoVal, expiresAt, now, task.ID,
	); err != nil {
		return err
	}

	if err := recordEvent(tx, task.ID, "claimed", map[string]any{
		"by":         whoVal,
		"duration":   duration,
		"expires_at": expiresAt,
	}); err != nil {
		return err
	}

	return tx.Commit()
}

func runRelease(db *sql.DB, shortID string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := expireStaleClaimsInTx(tx); err != nil {
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

	wasClaimedBy := ""
	if task.ClaimedBy != nil {
		wasClaimedBy = *task.ClaimedBy
	}

	now := currentNowFunc().Unix()
	if _, err := tx.Exec(
		"UPDATE tasks SET status = 'available', claimed_by = NULL, claim_expires_at = NULL, updated_at = ? WHERE id = ?",
		now, task.ID,
	); err != nil {
		return err
	}

	if err := recordEvent(tx, task.ID, "released", map[string]any{
		"was_claimed_by": wasClaimedBy,
	}); err != nil {
		return err
	}

	return tx.Commit()
}

func runNext(db *sql.DB, parentShortID string) (*Task, error) {
	if err := expireStaleClaims(db); err != nil {
		return nil, err
	}

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

	query := fmt.Sprintf(`
		SELECT t.id, t.short_id, t.parent_id, t.title, t.description, t.status, t.sort_order,
		       t.claimed_by, t.claim_expires_at, t.completion_note, t.created_at, t.updated_at, t.deleted_at
		FROM tasks t
		WHERE t.status = 'available' AND t.deleted_at IS NULL %s
		  AND NOT EXISTS (
		    SELECT 1 FROM blocks b
		    JOIN tasks bt ON bt.id = b.blocker_id
		    WHERE b.blocked_id = t.id AND bt.status != 'done' AND bt.deleted_at IS NULL
		  )
		ORDER BY t.sort_order LIMIT 1
	`, parentFilter)

	row := db.QueryRow(query, args...)
	task, err := scanTask(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no available tasks found")
	}
	if err != nil {
		return nil, err
	}
	return task, nil
}

func runClaimNext(db *sql.DB, parentShortID, duration, who string, force bool) (*Task, error) {
	task, err := runNext(db, parentShortID)
	if err != nil {
		return nil, err
	}

	if err := runClaim(db, task.ShortID, duration, who, force); err != nil {
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
