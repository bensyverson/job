package main

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

func runAdd(db *sql.DB, parentShortID, title, desc, beforeShortID string) (string, error) {
	tx, err := db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	var parentID *int64
	if parentShortID != "" {
		parent, err := getTaskByShortID(tx, parentShortID)
		if err != nil {
			return "", err
		}
		if parent == nil {
			return "", fmt.Errorf("task %q not found", parentShortID)
		}
		parentID = &parent.ID
	}

	shortID, err := generateShortID(tx)
	if err != nil {
		return "", err
	}

	var sortOrder int
	if beforeShortID != "" {
		beforeTask, err := getTaskByShortID(tx, beforeShortID)
		if err != nil {
			return "", err
		}
		if beforeTask == nil {
			return "", fmt.Errorf("task %q not found", beforeShortID)
		}
		if (beforeTask.ParentID == nil) != (parentID == nil) {
			return "", fmt.Errorf("task %q is not a sibling of the new task", beforeShortID)
		}
		if beforeTask.ParentID != nil && parentID != nil && *beforeTask.ParentID != *parentID {
			return "", fmt.Errorf("task %q is not a sibling of the new task", beforeShortID)
		}
		sortOrder = beforeTask.SortOrder
		if parentID == nil {
			_, err = tx.Exec("UPDATE tasks SET sort_order = sort_order + 1 WHERE parent_id IS NULL AND sort_order >= ? AND deleted_at IS NULL", sortOrder)
		} else {
			_, err = tx.Exec("UPDATE tasks SET sort_order = sort_order + 1 WHERE parent_id = ? AND sort_order >= ? AND deleted_at IS NULL", *parentID, sortOrder)
		}
		if err != nil {
			return "", err
		}
	} else {
		var maxSort sql.NullInt64
		if parentID == nil {
			err = tx.QueryRow("SELECT MAX(sort_order) FROM tasks WHERE parent_id IS NULL AND deleted_at IS NULL").Scan(&maxSort)
		} else {
			err = tx.QueryRow("SELECT MAX(sort_order) FROM tasks WHERE parent_id = ? AND deleted_at IS NULL", *parentID).Scan(&maxSort)
		}
		if err != nil {
			return "", err
		}
		if maxSort.Valid {
			sortOrder = int(maxSort.Int64) + 1
		}
	}

	now := time.Now().Unix()
	var taskID int64
	err = tx.QueryRow(`
		INSERT INTO tasks (short_id, parent_id, title, description, status, sort_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'available', ?, ?, ?)
		RETURNING id
	`, shortID, parentID, title, desc, sortOrder, now, now).Scan(&taskID)
	if err != nil {
		return "", err
	}

	eventParentID := ""
	if parentShortID != "" {
		eventParentID = parentShortID
	}
	if err := recordEvent(tx, taskID, "created", map[string]any{
		"parent_id":   eventParentID,
		"title":       title,
		"description": desc,
		"sort_order":  sortOrder,
	}); err != nil {
		return "", err
	}

	return shortID, tx.Commit()
}

func runList(db *sql.DB, parentShortID string, showAll bool) ([]*TaskNode, error) {
	if err := expireStaleClaims(db); err != nil {
		return nil, err
	}

	tasks, err := loadAllTasks(db)
	if err != nil {
		return nil, err
	}

	tree := buildTree(tasks)

	if parentShortID != "" {
		parent := findNodeByShortID(tree, parentShortID)
		if parent == nil {
			return nil, fmt.Errorf("task %q not found", parentShortID)
		}
		tree = parent.Children
	}

	blockedIDs, err := getBlockedTaskIDs(db)
	if err != nil {
		return nil, err
	}

	return filterTree(tree, showAll, blockedIDs), nil
}

func runDone(db *sql.DB, shortID string, force bool, note string) ([]string, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	task, err := getTaskByShortID(tx, shortID)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, fmt.Errorf("task %q not found", shortID)
	}
	if task.Status == "done" {
		return nil, fmt.Errorf("task %s is already done", shortID)
	}

	var forceClosedShortIDs []string
	var forceClosedTasks []*Task

	incomplete, err := findIncompleteDescendants(tx, task.ID)
	if err != nil {
		return nil, err
	}

	if len(incomplete) > 0 {
		if !force {
			var descs []string
			for _, t := range incomplete {
				descs = append(descs, fmt.Sprintf("%s (%s)", t.ShortID, t.Title))
			}
			return nil, fmt.Errorf("task %s has incomplete subtasks: %s", shortID, strings.Join(descs, ", "))
		}
		forceClosedTasks = incomplete
		forceClosedShortIDs = make([]string, len(incomplete))
		for i, t := range incomplete {
			forceClosedShortIDs[i] = t.ShortID
		}
	}

	now := time.Now().Unix()

	for _, child := range forceClosedTasks {
		var noteVal any
		if _, err := tx.Exec(
			"UPDATE tasks SET status = 'done', completion_note = ?, updated_at = ? WHERE id = ?",
			noteVal, now, child.ID,
		); err != nil {
			return nil, err
		}
		if err := recordEvent(tx, child.ID, "done", map[string]any{
			"note":                   nil,
			"force":                  true,
			"force_closed_by_parent": shortID,
		}); err != nil {
			return nil, err
		}
	}

	var noteVal any
	if note != "" {
		noteVal = note
	}
	if _, err := tx.Exec(
		"UPDATE tasks SET status = 'done', completion_note = ?, updated_at = ? WHERE id = ?",
		noteVal, now, task.ID,
	); err != nil {
		return nil, err
	}

	if err := recordEvent(tx, task.ID, "done", map[string]any{
		"note":                  noteVal,
		"force":                 force,
		"force_closed_children": forceClosedShortIDs,
	}); err != nil {
		return nil, err
	}

	rows, err := tx.Query(
		"SELECT blocked_id FROM blocks WHERE blocker_id = ?", task.ID,
	)
	if err != nil {
		return nil, err
	}
	var unblockedIDs []int64
	for rows.Next() {
		var blockedID int64
		if err := rows.Scan(&blockedID); err != nil {
			rows.Close()
			return nil, err
		}
		unblockedIDs = append(unblockedIDs, blockedID)
	}
	rows.Close()

	if len(unblockedIDs) > 0 {
		if _, err := tx.Exec("DELETE FROM blocks WHERE blocker_id = ?", task.ID); err != nil {
			return nil, err
		}
		for _, blockedID := range unblockedIDs {
			var blockedShortID string
			if err := tx.QueryRow("SELECT short_id FROM tasks WHERE id = ?", blockedID).Scan(&blockedShortID); err != nil {
				return nil, err
			}
			if err := recordEvent(tx, blockedID, "unblocked", map[string]any{
				"blocked_id": blockedShortID,
				"blocker_id": shortID,
				"reason":     "blocker_done",
			}); err != nil {
				return nil, err
			}
		}
	}

	return forceClosedShortIDs, tx.Commit()
}

func runReopen(db *sql.DB, shortID string) ([]string, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	task, err := getTaskByShortID(tx, shortID)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, fmt.Errorf("task %q not found", shortID)
	}
	if task.Status != "done" {
		return nil, fmt.Errorf("task %s is not done (status: %s)", shortID, task.Status)
	}

	detail, err := getLatestEventDetail(tx, task.ID, "done")
	if err != nil {
		return nil, err
	}

	var reopenedChildren []string
	if detail != nil {
		if children, ok := detail["force_closed_children"].([]any); ok {
			now := time.Now().Unix()
			for _, c := range children {
				childShortID, ok := c.(string)
				if !ok {
					continue
				}
				child, err := getTaskByShortID(tx, childShortID)
				if err != nil || child == nil {
					continue
				}
				if _, err := tx.Exec(
					"UPDATE tasks SET status = 'available', completion_note = NULL, updated_at = ? WHERE id = ?",
					now, child.ID,
				); err != nil {
					return nil, err
				}
				if err := recordEvent(tx, child.ID, "reopened", map[string]any{
					"reopened_by_parent": shortID,
				}); err != nil {
					return nil, err
				}
				reopenedChildren = append(reopenedChildren, childShortID)
			}
		}
	}

	now := time.Now().Unix()
	if _, err := tx.Exec(
		"UPDATE tasks SET status = 'available', completion_note = NULL, updated_at = ? WHERE id = ?",
		now, task.ID,
	); err != nil {
		return nil, err
	}

	if err := recordEvent(tx, task.ID, "reopened", map[string]any{
		"reopened_children": reopenedChildren,
	}); err != nil {
		return nil, err
	}

	return reopenedChildren, tx.Commit()
}

func runEdit(db *sql.DB, shortID, newTitle string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	task, err := getTaskByShortID(tx, shortID)
	if err != nil {
		return err
	}
	if task == nil {
		return fmt.Errorf("task %q not found", shortID)
	}

	now := time.Now().Unix()
	if _, err := tx.Exec(
		"UPDATE tasks SET title = ?, updated_at = ? WHERE id = ?",
		newTitle, now, task.ID,
	); err != nil {
		return err
	}

	if err := recordEvent(tx, task.ID, "edited", map[string]any{
		"old_title": task.Title,
		"new_title": newTitle,
	}); err != nil {
		return err
	}

	return tx.Commit()
}

func runNote(db *sql.DB, shortID, text string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	task, err := getTaskByShortID(tx, shortID)
	if err != nil {
		return err
	}
	if task == nil {
		return fmt.Errorf("task %q not found", shortID)
	}

	var newDesc string
	timestamp := time.Now().Format("2006-01-02 15:04")
	if task.Description == "" {
		newDesc = text
	} else {
		newDesc = task.Description + "\n\n[" + timestamp + "] " + text
	}

	now := time.Now().Unix()
	if _, err := tx.Exec(
		"UPDATE tasks SET description = ?, updated_at = ? WHERE id = ?",
		newDesc, now, task.ID,
	); err != nil {
		return err
	}

	if err := recordEvent(tx, task.ID, "noted", map[string]any{
		"text":              text,
		"description_after": newDesc,
	}); err != nil {
		return err
	}

	return tx.Commit()
}

func runRemove(db *sql.DB, shortID string, removeAll, force bool) (int, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	task, err := getTaskByShortID(tx, shortID)
	if err != nil {
		return 0, err
	}
	if task == nil {
		return 0, fmt.Errorf("task %q not found", shortID)
	}

	var children []*Task
	rows, err := tx.Query(`
		SELECT id, short_id, parent_id, title, description, status, sort_order,
		       claimed_by, claim_expires_at, completion_note, created_at, updated_at, deleted_at
		FROM tasks WHERE parent_id = ? AND deleted_at IS NULL
	`, task.ID)
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		c, err := scanTask(rows)
		if err != nil {
			rows.Close()
			return 0, err
		}
		children = append(children, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	if len(children) > 0 && !removeAll {
		return 0, fmt.Errorf("task %s has %d subtasks. Use 'all' to remove them all, or remove subtasks first", shortID, len(children))
	}

	now := time.Now().Unix()

	var removedChildIDs []string
	if len(children) > 0 {
		ids, err := softDeleteDescendants(tx, task.ID, now)
		if err != nil {
			return 0, err
		}
		removedChildIDs = ids
	}

	if _, err := tx.Exec(
		"UPDATE tasks SET deleted_at = ?, updated_at = ? WHERE id = ?",
		now, now, task.ID,
	); err != nil {
		return 0, err
	}

	_, err = tx.Exec("DELETE FROM blocks WHERE blocker_id = ? OR blocked_id = ?", task.ID, task.ID)
	if err != nil {
		return 0, err
	}

	for _, child := range children {
		_, err = tx.Exec("DELETE FROM blocks WHERE blocker_id = ? OR blocked_id = ?", child.ID, child.ID)
		if err != nil {
			return 0, err
		}
	}

	if err := recordEvent(tx, task.ID, "removed", map[string]any{
		"title":            task.Title,
		"children_removed": removedChildIDs,
		"was_status":       task.Status,
	}); err != nil {
		return 0, err
	}

	return len(removedChildIDs), tx.Commit()
}

func softDeleteDescendants(tx dbtx, parentID int64, now int64) ([]string, error) {
	rows, err := tx.Query(`
		SELECT id, short_id, parent_id, title, description, status, sort_order,
		       claimed_by, claim_expires_at, completion_note, created_at, updated_at, deleted_at
		FROM tasks WHERE parent_id = ? AND deleted_at IS NULL
	`, parentID)
	if err != nil {
		return nil, err
	}
	var children []*Task
	for rows.Next() {
		c, err := scanTask(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		children = append(children, c)
	}
	rows.Close()

	var ids []string
	for _, child := range children {
		descIDs, err := softDeleteDescendants(tx, child.ID, now)
		if err != nil {
			return nil, err
		}
		ids = append(ids, descIDs...)

		if _, err := tx.Exec(
			"UPDATE tasks SET deleted_at = ?, updated_at = ? WHERE id = ?",
			now, now, child.ID,
		); err != nil {
			return nil, err
		}
		ids = append(ids, child.ShortID)
	}
	return ids, nil
}

func runMove(db *sql.DB, shortID, direction, relativeToShortID string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	task, err := getTaskByShortID(tx, shortID)
	if err != nil {
		return err
	}
	if task == nil {
		return fmt.Errorf("task %q not found", shortID)
	}

	relative, err := getTaskByShortID(tx, relativeToShortID)
	if err != nil {
		return err
	}
	if relative == nil {
		return fmt.Errorf("task %q not found", relativeToShortID)
	}

	if (task.ParentID == nil) != (relative.ParentID == nil) {
		return fmt.Errorf("%s and %s are not siblings (different parents)", shortID, relativeToShortID)
	}
	if task.ParentID != nil && relative.ParentID != nil && *task.ParentID != *relative.ParentID {
		return fmt.Errorf("%s and %s are not siblings (different parents)", shortID, relativeToShortID)
	}

	oldSortOrder := task.SortOrder
	var newSortOrder int

	if direction == "before" {
		newSortOrder = relative.SortOrder
		var parentFilter string
		var args []any
		if task.ParentID == nil {
			parentFilter = "parent_id IS NULL"
		} else {
			parentFilter = "parent_id = ?"
			args = append(args, *task.ParentID)
		}
		args = append(args, newSortOrder, task.ID)
		_, err = tx.Exec(
			"UPDATE tasks SET sort_order = sort_order + 1 WHERE "+parentFilter+" AND sort_order >= ? AND id != ? AND deleted_at IS NULL",
			args...,
		)
		if err != nil {
			return err
		}
	} else {
		newSortOrder = relative.SortOrder + 1
		var parentFilter string
		var args []any
		if task.ParentID == nil {
			parentFilter = "parent_id IS NULL"
		} else {
			parentFilter = "parent_id = ?"
			args = append(args, *task.ParentID)
		}
		args = append(args, relative.SortOrder, task.ID)
		_, err = tx.Exec(
			"UPDATE tasks SET sort_order = sort_order + 1 WHERE "+parentFilter+" AND sort_order > ? AND id != ? AND deleted_at IS NULL",
			args...,
		)
		if err != nil {
			return err
		}
	}

	now := time.Now().Unix()
	if _, err := tx.Exec(
		"UPDATE tasks SET sort_order = ?, updated_at = ? WHERE id = ?",
		newSortOrder, now, task.ID,
	); err != nil {
		return err
	}

	if err := recordEvent(tx, task.ID, "moved", map[string]any{
		"direction":      direction,
		"relative_to":    relativeToShortID,
		"old_sort_order": oldSortOrder,
		"new_sort_order": newSortOrder,
	}); err != nil {
		return err
	}

	return tx.Commit()
}

type TaskInfo struct {
	Task     *Task
	Parent   *Task
	Children []*Task
	Blockers []*Task
}

func runInfo(db *sql.DB, shortID string) (*TaskInfo, error) {
	task, err := getTaskByShortID(db, shortID)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, fmt.Errorf("task %q not found", shortID)
	}

	var parent *Task
	if task.ParentID != nil {
		row := db.QueryRow(`
			SELECT id, short_id, parent_id, title, description, status, sort_order,
			       claimed_by, claim_expires_at, completion_note, created_at, updated_at, deleted_at
			FROM tasks WHERE id = ? AND deleted_at IS NULL
		`, *task.ParentID)
		p, err := scanTask(row)
		if err != nil && err != sql.ErrNoRows {
			return nil, err
		}
		parent = p
	}

	rows, err := db.Query(`
		SELECT id, short_id, parent_id, title, description, status, sort_order,
		       claimed_by, claim_expires_at, completion_note, created_at, updated_at, deleted_at
		FROM tasks WHERE parent_id = ? AND deleted_at IS NULL
		ORDER BY sort_order
	`, task.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var children []*Task
	for rows.Next() {
		c, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		children = append(children, c)
	}

	blockers, err := getBlockers(db, shortID)
	if err != nil {
		return nil, err
	}

	return &TaskInfo{
		Task:     task,
		Parent:   parent,
		Children: children,
		Blockers: blockers,
	}, nil
}
