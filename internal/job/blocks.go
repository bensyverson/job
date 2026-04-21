package job

import (
	"database/sql"
	"fmt"
	"strings"
)

func RunBlock(db *sql.DB, blockedShortID, blockerShortID, actor string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	blocked, err := GetTaskByShortID(tx, blockedShortID)
	if err != nil {
		return err
	}
	if blocked == nil {
		return fmt.Errorf("task %q not found", blockedShortID)
	}

	blocker, err := GetTaskByShortID(tx, blockerShortID)
	if err != nil {
		return err
	}
	if blocker == nil {
		return fmt.Errorf("task %q not found", blockerShortID)
	}

	if blocked.ID == blocker.ID {
		return fmt.Errorf("a task cannot block itself")
	}

	circular, err := wouldCreateCycle(tx, blocked.ID, blocker.ID)
	if err != nil {
		return err
	}
	if circular {
		return fmt.Errorf("cannot block %s by %s: would create a circular dependency", blockedShortID, blockerShortID)
	}

	if _, err := tx.Exec(
		"INSERT OR IGNORE INTO blocks (blocker_id, blocked_id) VALUES (?, ?)",
		blocker.ID, blocked.ID,
	); err != nil {
		return err
	}

	if err := recordEvent(tx, blocked.ID, "blocked", actor, map[string]any{
		"blocked_id": blockedShortID,
		"blocker_id": blockerShortID,
	}); err != nil {
		return err
	}

	return tx.Commit()
}

func wouldCreateCycle(tx dbtx, blockedID, blockerID int64) (bool, error) {
	visited := make(map[int64]bool)
	return walkBlockerChain(tx, blockerID, blockedID, visited)
}

func walkBlockerChain(tx dbtx, startID, targetID int64, visited map[int64]bool) (bool, error) {
	if startID == targetID {
		return true, nil
	}
	if visited[startID] {
		return false, nil
	}
	visited[startID] = true

	rows, err := tx.Query("SELECT blocker_id FROM blocks WHERE blocked_id = ?", startID)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var blockerID int64
		if err := rows.Scan(&blockerID); err != nil {
			return false, err
		}
		found, err := walkBlockerChain(tx, blockerID, targetID, visited)
		if err != nil {
			return false, err
		}
		if found {
			return true, nil
		}
	}
	return false, rows.Err()
}

func RunUnblock(db *sql.DB, blockedShortID, blockerShortID, actor string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	blocked, err := GetTaskByShortID(tx, blockedShortID)
	if err != nil {
		return err
	}
	if blocked == nil {
		return fmt.Errorf("task %q not found", blockedShortID)
	}

	blocker, err := GetTaskByShortID(tx, blockerShortID)
	if err != nil {
		return err
	}
	if blocker == nil {
		return fmt.Errorf("task %q not found", blockerShortID)
	}

	result, err := tx.Exec(
		"DELETE FROM blocks WHERE blocker_id = ? AND blocked_id = ?",
		blocker.ID, blocked.ID,
	)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("%s is not blocked by %s", blockedShortID, blockerShortID)
	}

	if err := recordEvent(tx, blocked.ID, "unblocked", actor, map[string]any{
		"blocked_id": blockedShortID,
		"blocker_id": blockerShortID,
		"reason":     "manual",
	}); err != nil {
		return err
	}

	return tx.Commit()
}

// GetBlockersForTaskIDs returns a map of blocked-task-id to the short IDs of
// its open blockers. One query covers the whole set, replacing N+1 walks.
func GetBlockersForTaskIDs(db *sql.DB, taskIDs []int64) (map[int64][]string, error) {
	out := make(map[int64][]string, len(taskIDs))
	if len(taskIDs) == 0 {
		return out, nil
	}
	placeholders := strings.Repeat("?,", len(taskIDs))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, 0, len(taskIDs))
	for _, id := range taskIDs {
		args = append(args, id)
	}
	rows, err := db.Query(`
		SELECT b.blocked_id, blocker.short_id
		FROM blocks b
		JOIN tasks blocker ON blocker.id = b.blocker_id
		WHERE b.blocked_id IN (`+placeholders+`)
		  AND blocker.status != 'done'
		  AND blocker.deleted_at IS NULL
		ORDER BY b.blocked_id, blocker.short_id
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var blockedID int64
		var blockerShort string
		if err := rows.Scan(&blockedID, &blockerShort); err != nil {
			return nil, err
		}
		out[blockedID] = append(out[blockedID], blockerShort)
	}
	return out, rows.Err()
}

func GetBlockers(db *sql.DB, shortID string) ([]*Task, error) {
	task, err := GetTaskByShortID(db, shortID)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, fmt.Errorf("task %q not found", shortID)
	}

	rows, err := db.Query(`
		SELECT t.id, t.short_id, t.parent_id, t.title, t.description, t.status, t.sort_order,
		       t.claimed_by, t.claim_expires_at, t.completion_note, t.created_at, t.updated_at, t.deleted_at
		FROM blocks b
		JOIN tasks t ON t.id = b.blocker_id
		WHERE b.blocked_id = ? AND t.status != 'done' AND t.deleted_at IS NULL
	`, task.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var blockers []*Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		blockers = append(blockers, t)
	}
	return blockers, rows.Err()
}
