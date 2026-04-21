package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type ClosedResult struct {
	ShortID             string
	Title               string
	CascadeClosed       []string
	AutoClosedAncestors []AutoClosedAncestor
}

// AutoClosedAncestor names an ancestor that was auto-closed by the
// leaf-frontier cascade (when its last open child closed). Walking from
// the closer upward; the first entry is the direct parent.
type AutoClosedAncestor struct {
	ShortID string
	Title   string
}

// AddResult carries the outcome of runAdd. ShortID is always set on
// success; AutoReleasedParent is set when the add triggered an auto-release
// of a claimed parent (leaf-frontier semantics — a parent with an open
// child has no executable work of its own).
type AddResult struct {
	ShortID             string
	AutoReleasedParent  string
	AutoReleasedByActor string
}

func runAdd(db *sql.DB, parentShortID, title, desc, beforeShortID, actor string) (*AddResult, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var parent *Task
	var parentID *int64
	if parentShortID != "" {
		p, err := getTaskByShortID(tx, parentShortID)
		if err != nil {
			return nil, err
		}
		if p == nil {
			return nil, fmt.Errorf("task %q not found", parentShortID)
		}
		parent = p
		parentID = &p.ID
	}

	shortID, err := generateShortID(tx)
	if err != nil {
		return nil, err
	}

	var sortOrder int
	if beforeShortID != "" {
		beforeTask, err := getTaskByShortID(tx, beforeShortID)
		if err != nil {
			return nil, err
		}
		if beforeTask == nil {
			return nil, fmt.Errorf("task %q not found", beforeShortID)
		}
		if (beforeTask.ParentID == nil) != (parentID == nil) {
			return nil, fmt.Errorf("task %q is not a sibling of the new task", beforeShortID)
		}
		if beforeTask.ParentID != nil && parentID != nil && *beforeTask.ParentID != *parentID {
			return nil, fmt.Errorf("task %q is not a sibling of the new task", beforeShortID)
		}
		sortOrder = beforeTask.SortOrder
		if parentID == nil {
			_, err = tx.Exec("UPDATE tasks SET sort_order = sort_order + 1 WHERE parent_id IS NULL AND sort_order >= ? AND deleted_at IS NULL", sortOrder)
		} else {
			_, err = tx.Exec("UPDATE tasks SET sort_order = sort_order + 1 WHERE parent_id = ? AND sort_order >= ? AND deleted_at IS NULL", *parentID, sortOrder)
		}
		if err != nil {
			return nil, err
		}
	} else {
		var maxSort sql.NullInt64
		if parentID == nil {
			err = tx.QueryRow("SELECT MAX(sort_order) FROM tasks WHERE parent_id IS NULL AND deleted_at IS NULL").Scan(&maxSort)
		} else {
			err = tx.QueryRow("SELECT MAX(sort_order) FROM tasks WHERE parent_id = ? AND deleted_at IS NULL", *parentID).Scan(&maxSort)
		}
		if err != nil {
			return nil, err
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
		return nil, err
	}

	eventParentID := ""
	if parentShortID != "" {
		eventParentID = parentShortID
	}
	if err := recordEvent(tx, taskID, "created", actor, map[string]any{
		"parent_id":   eventParentID,
		"title":       title,
		"description": desc,
		"sort_order":  sortOrder,
	}); err != nil {
		return nil, err
	}

	result := &AddResult{ShortID: shortID}

	// Leaf-frontier auto-release: adding an open child to a claimed parent
	// releases the parent's claim. The parent has no executable work of its
	// own — its work is in its children — so the lock has no referent.
	if parent != nil && parent.Status == "claimed" {
		prior := ""
		if parent.ClaimedBy != nil {
			prior = *parent.ClaimedBy
		}
		if _, err := tx.Exec(
			"UPDATE tasks SET status = 'available', claimed_by = NULL, claim_expires_at = NULL, updated_at = ? WHERE id = ?",
			now, parent.ID,
		); err != nil {
			return nil, err
		}
		if err := recordEvent(tx, parent.ID, "released", actor, map[string]any{
			"auto_released":      true,
			"triggered_by_child": shortID,
			"prior_claimant":     prior,
		}); err != nil {
			return nil, err
		}
		result.AutoReleasedParent = parent.ShortID
		result.AutoReleasedByActor = prior
	}

	return result, tx.Commit()
}

func runList(db *sql.DB, parentShortID, actor string, showAll bool) ([]*TaskNode, error) {
	return runListFiltered(db, parentShortID, actor, showAll, "")
}

func runListFiltered(db *sql.DB, parentShortID, actor string, showAll bool, labelName string) ([]*TaskNode, error) {
	if err := expireStaleClaims(db, actor); err != nil {
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

	filtered := filterTree(tree, showAll, blockedIDs)
	if labelName != "" {
		labeledIDs, err := taskIDsWithLabel(db, labelName)
		if err != nil {
			return nil, err
		}
		filtered = filterByLabel(filtered, labeledIDs)
	}
	return filtered, nil
}

// taskIDsWithLabel returns the set of task ids carrying the given label.
func taskIDsWithLabel(db *sql.DB, name string) (map[int64]bool, error) {
	rows, err := db.Query("SELECT task_id FROM task_labels WHERE name = ?", name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int64]bool)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// filterByLabel keeps only nodes whose task is in labeledIDs OR whose subtree
// contains a labeled task. Children that don't match (and have no labeled
// descendants) are pruned. This preserves the hierarchical context of the
// matching tasks rather than flattening the result.
func filterByLabel(nodes []*TaskNode, labeledIDs map[int64]bool) []*TaskNode {
	var out []*TaskNode
	for _, node := range nodes {
		filteredChildren := filterByLabel(node.Children, labeledIDs)
		if labeledIDs[node.Task.ID] || len(filteredChildren) > 0 {
			out = append(out, &TaskNode{Task: node.Task, Children: filteredChildren})
		}
	}
	return out
}

// cascadeAutoCloseAncestors walks the ancestor chain from taskID upward,
// auto-closing each ancestor whose open children have all been closed
// (status is now "done" or "canceled"). Emits a "done" event on each
// auto-closed ancestor with detail.auto_closed=true, attributing the close
// to actor (the agent who closed the last open descendant). Returns the
// ordered list of auto-closed ancestors, nearest-parent first.
func cascadeAutoCloseAncestors(tx dbtx, taskID int64, triggerShortID, actor string, now int64) ([]AutoClosedAncestor, error) {
	var result []AutoClosedAncestor
	cursorID := taskID
	for {
		var parentID *int64
		if err := tx.QueryRow(
			"SELECT parent_id FROM tasks WHERE id = ?", cursorID,
		).Scan(&parentID); err != nil {
			return nil, err
		}
		if parentID == nil {
			return result, nil
		}

		row := tx.QueryRow(`
			SELECT id, short_id, parent_id, title, description, status, sort_order,
			       claimed_by, claim_expires_at, completion_note, created_at, updated_at, deleted_at
			FROM tasks WHERE id = ?`, *parentID)
		p, err := scanTask(row)
		if err != nil {
			return nil, err
		}
		// If the parent is already done/canceled, stop the cascade — nothing
		// to do, and we shouldn't walk past it.
		if p.Status == "done" || p.Status == "canceled" {
			return result, nil
		}

		open, err := countOpenChildren(tx, p.ID)
		if err != nil {
			return nil, err
		}
		if open > 0 {
			return result, nil
		}

		if _, err := tx.Exec(
			"UPDATE tasks SET status = 'done', updated_at = ? WHERE id = ?",
			now, p.ID,
		); err != nil {
			return nil, err
		}
		if err := recordEvent(tx, p.ID, "done", actor, map[string]any{
			"auto_closed":  true,
			"triggered_by": triggerShortID,
		}); err != nil {
			return nil, err
		}
		if err := recordBlocksUnblockedOn(tx, p.ID, p.ShortID, actor); err != nil {
			return nil, err
		}

		result = append(result, AutoClosedAncestor{ShortID: p.ShortID, Title: p.Title})
		cursorID = p.ID
	}
}

// runDone closes one or more tasks atomically. If cascade is true, each target
// expands to include all open descendants. Returns per-target results, a list
// of already-done targets that were skipped, or an error (all-or-nothing).
func runDone(db *sql.DB, ids []string, cascade bool, note string, result json.RawMessage, actor string) (closed []*ClosedResult, alreadyDone []string, err error) {
	if len(ids) == 0 {
		return nil, nil, fmt.Errorf("done requires at least one task id")
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()

	if err := expireStaleClaimsInTx(tx, actor); err != nil {
		return nil, nil, err
	}

	// Phase A: validate every id and resolve to a task, partition already-done.
	type target struct {
		shortID string
		task    *Task
	}
	var targets []target
	seenExplicit := make(map[int64]bool)
	for _, id := range ids {
		if err := checkClaimOwnership(tx, id, actor); err != nil {
			return nil, nil, err
		}
		t, err := getTaskByShortID(tx, id)
		if err != nil {
			return nil, nil, err
		}
		if t == nil {
			return nil, nil, fmt.Errorf("task %q not found", id)
		}
		if t.Status == "done" {
			alreadyDone = append(alreadyDone, id)
			continue
		}
		if seenExplicit[t.ID] {
			continue
		}
		seenExplicit[t.ID] = true
		targets = append(targets, target{shortID: id, task: t})
	}

	// Phase A.2: for each target, validate or expand via cascade.
	type plan struct {
		target        target
		cascadeTasks  []*Task
		cascadeShorts []string
	}
	var plans []plan
	seenCascade := make(map[int64]bool)
	for _, tgt := range targets {
		incomplete, err := findIncompleteDescendants(tx, tgt.task.ID)
		if err != nil {
			return nil, nil, err
		}
		if len(incomplete) > 0 && !cascade {
			var descs []string
			for _, t := range incomplete {
				descs = append(descs, fmt.Sprintf("%s (%s)", t.ShortID, t.Title))
			}
			return nil, nil, fmt.Errorf("task %s has incomplete subtasks: %s (run 'job done --cascade %s' to close all).",
				tgt.shortID, strings.Join(descs, ", "), tgt.shortID)
		}
		var cTasks []*Task
		var cShorts []string
		if cascade {
			for _, d := range incomplete {
				if seenExplicit[d.ID] || seenCascade[d.ID] {
					continue
				}
				seenCascade[d.ID] = true
				cTasks = append(cTasks, d)
				cShorts = append(cShorts, d.ShortID)
			}
		}
		plans = append(plans, plan{target: tgt, cascadeTasks: cTasks, cascadeShorts: cShorts})
	}

	now := time.Now().Unix()

	var noteVal any
	if note != "" {
		noteVal = note
	}

	var resultVal any
	if len(result) > 0 {
		var parsed any
		if err := json.Unmarshal(result, &parsed); err != nil {
			return nil, nil, fmt.Errorf("--result: invalid JSON: %s", err)
		}
		resultVal = parsed
	}

	// Phase B: execute.
	for _, p := range plans {
		// Close cascaded descendants first.
		for _, child := range p.cascadeTasks {
			if _, err := tx.Exec(
				"UPDATE tasks SET status = 'done', updated_at = ? WHERE id = ?",
				now, child.ID,
			); err != nil {
				return nil, nil, err
			}
			if err := recordEvent(tx, child.ID, "done", actor, map[string]any{
				"cascade":                  true,
				"cascade_closed_by_parent": p.target.shortID,
			}); err != nil {
				return nil, nil, err
			}
			if err := recordBlocksUnblockedOn(tx, child.ID, child.ShortID, actor); err != nil {
				return nil, nil, err
			}
		}

		// Close the explicit target.
		if _, err := tx.Exec(
			"UPDATE tasks SET status = 'done', completion_note = ?, updated_at = ? WHERE id = ?",
			noteVal, now, p.target.task.ID,
		); err != nil {
			return nil, nil, err
		}
		detail := map[string]any{
			"note":           noteVal,
			"cascade":        cascade,
			"cascade_closed": p.cascadeShorts,
		}
		if resultVal != nil {
			detail["result"] = resultVal
		}
		if err := recordEvent(tx, p.target.task.ID, "done", actor, detail); err != nil {
			return nil, nil, err
		}
		if err := recordBlocksUnblockedOn(tx, p.target.task.ID, p.target.shortID, actor); err != nil {
			return nil, nil, err
		}

		// Leaf-frontier cascade: after closing this target, auto-close any
		// ancestors whose last open child has just been closed.
		autoClosed, err := cascadeAutoCloseAncestors(tx, p.target.task.ID, p.target.shortID, actor, now)
		if err != nil {
			return nil, nil, err
		}

		closed = append(closed, &ClosedResult{
			ShortID:             p.target.shortID,
			Title:               p.target.task.Title,
			CascadeClosed:       p.cascadeShorts,
			AutoClosedAncestors: autoClosed,
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	return closed, alreadyDone, nil
}

func recordBlocksUnblockedOn(tx dbtx, blockerID int64, blockerShortID, actor string) error {
	rows, err := tx.Query("SELECT blocked_id FROM blocks WHERE blocker_id = ?", blockerID)
	if err != nil {
		return err
	}
	var unblockedIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		unblockedIDs = append(unblockedIDs, id)
	}
	rows.Close()
	if len(unblockedIDs) == 0 {
		return nil
	}
	if _, err := tx.Exec("DELETE FROM blocks WHERE blocker_id = ?", blockerID); err != nil {
		return err
	}
	for _, id := range unblockedIDs {
		var blockedShortID string
		if err := tx.QueryRow("SELECT short_id FROM tasks WHERE id = ?", id).Scan(&blockedShortID); err != nil {
			return err
		}
		if err := recordEvent(tx, id, "unblocked", actor, map[string]any{
			"blocked_id": blockedShortID,
			"blocker_id": blockerShortID,
			"reason":     "blocker_done",
		}); err != nil {
			return err
		}
	}
	return nil
}

func runReopen(db *sql.DB, shortID string, cascade bool, actor string) ([]string, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if err := expireStaleClaimsInTx(tx, actor); err != nil {
		return nil, err
	}
	if err := checkClaimOwnership(tx, shortID, actor); err != nil {
		return nil, err
	}

	task, err := getTaskByShortID(tx, shortID)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, fmt.Errorf("task %q not found", shortID)
	}
	if task.Status != "done" && task.Status != "canceled" {
		return nil, fmt.Errorf("task %s is not done or canceled (status: %s)", shortID, task.Status)
	}
	fromStatus := task.Status

	now := time.Now().Unix()

	var reopenedChildren []string
	if cascade {
		descendants, err := findClosedDescendants(tx, task.ID)
		if err != nil {
			return nil, err
		}
		for _, d := range descendants {
			if _, err := tx.Exec(
				"UPDATE tasks SET status = 'available', completion_note = NULL, updated_at = ? WHERE id = ?",
				now, d.ID,
			); err != nil {
				return nil, err
			}
			if err := recordEvent(tx, d.ID, "reopened", actor, map[string]any{
				"cascade":           false,
				"reopened_children": []string{},
				"from_status":       d.Status,
			}); err != nil {
				return nil, err
			}
			reopenedChildren = append(reopenedChildren, d.ShortID)
		}
	}

	if _, err := tx.Exec(
		"UPDATE tasks SET status = 'available', completion_note = NULL, updated_at = ? WHERE id = ?",
		now, task.ID,
	); err != nil {
		return nil, err
	}

	if err := recordEvent(tx, task.ID, "reopened", actor, map[string]any{
		"cascade":           cascade,
		"reopened_children": reopenedChildren,
		"from_status":       fromStatus,
	}); err != nil {
		return nil, err
	}

	return reopenedChildren, tx.Commit()
}

func runEdit(db *sql.DB, shortID string, newTitle, newDesc *string, actor string) error {
	if newTitle == nil && newDesc == nil {
		return fmt.Errorf("edit requires --title and/or --desc")
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := expireStaleClaimsInTx(tx, actor); err != nil {
		return err
	}
	if err := checkClaimOwnership(tx, shortID, actor); err != nil {
		return err
	}

	task, err := getTaskByShortID(tx, shortID)
	if err != nil {
		return err
	}
	if task == nil {
		return fmt.Errorf("task %q not found", shortID)
	}

	now := time.Now().Unix()
	detail := map[string]any{}

	if newTitle != nil && *newTitle != task.Title {
		if _, err := tx.Exec(
			"UPDATE tasks SET title = ?, updated_at = ? WHERE id = ?",
			*newTitle, now, task.ID,
		); err != nil {
			return err
		}
		detail["old_title"] = task.Title
		detail["new_title"] = *newTitle
	} else if newTitle != nil {
		detail["old_title"] = task.Title
		detail["new_title"] = *newTitle
	}

	if newDesc != nil {
		if _, err := tx.Exec(
			"UPDATE tasks SET description = ?, updated_at = ? WHERE id = ?",
			*newDesc, now, task.ID,
		); err != nil {
			return err
		}
		detail["old_desc"] = task.Description
		detail["new_desc"] = *newDesc
	}

	if err := recordEvent(tx, task.ID, "edited", actor, detail); err != nil {
		return err
	}

	return tx.Commit()
}

func runNote(db *sql.DB, shortID, text string, result json.RawMessage, actor string) error {
	if text == "" {
		return fmt.Errorf("note text is empty")
	}

	var resultVal any
	if len(result) > 0 {
		var parsed any
		if err := json.Unmarshal(result, &parsed); err != nil {
			return fmt.Errorf("--result: invalid JSON: %s", err)
		}
		resultVal = parsed
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := expireStaleClaimsInTx(tx, actor); err != nil {
		return err
	}
	if err := checkClaimOwnership(tx, shortID, actor); err != nil {
		return err
	}

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

	detail := map[string]any{
		"text":              text,
		"description_after": newDesc,
	}
	if resultVal != nil {
		detail["result"] = resultVal
	}
	if err := recordEvent(tx, task.ID, "noted", actor, detail); err != nil {
		return err
	}

	return tx.Commit()
}

func runMove(db *sql.DB, shortID, direction, relativeToShortID, actor string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := expireStaleClaimsInTx(tx, actor); err != nil {
		return err
	}
	if err := checkClaimOwnership(tx, shortID, actor); err != nil {
		return err
	}

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

	if err := recordEvent(tx, task.ID, "moved", actor, map[string]any{
		"direction":      direction,
		"relative_to":    relativeToShortID,
		"old_sort_order": oldSortOrder,
		"new_sort_order": newSortOrder,
	}); err != nil {
		return err
	}

	return tx.Commit()
}

type DoneContext struct {
	ClosedID           string
	ClosedTitle        string
	NextSibling        *Task
	SkippedBlocked     *Task
	SkippedBlockedBy   string
	ParentID           string
	ParentTitle        string
	ParentWasDone      bool
	ParentDoneCount    int
	ParentTotalCount   int
	ParentAutoClosed   bool
	NextAfterParent    *Task
	WholeTreeComplete  bool
	WholeTreeDoneCount int
	WholeTreeRootID    string
}

// computeDoneContext computes the trailing-context block for a done ack.
// autoClosedSet names ancestors that were auto-closed by the leaf-frontier
// cascade in this same call — they are "done" now but were not done before,
// which we need to distinguish to compute ParentWasDone correctly.
func computeDoneContext(db *sql.DB, closedShortID string, autoClosedSet map[string]bool) (*DoneContext, error) {
	closed, err := getTaskByShortID(db, closedShortID)
	if err != nil {
		return nil, err
	}
	if closed == nil {
		return nil, fmt.Errorf("task %q not found", closedShortID)
	}

	ctx := &DoneContext{
		ClosedID:    closed.ShortID,
		ClosedTitle: closed.Title,
	}

	if closed.ParentID != nil {
		parent, err := getTaskByID(db, *closed.ParentID)
		if err != nil {
			return nil, err
		}
		if parent != nil {
			ctx.ParentID = parent.ShortID
			ctx.ParentTitle = parent.Title
			ctx.ParentAutoClosed = autoClosedSet[parent.ShortID]
			// ParentWasDone means "already done before this call." If the parent
			// just auto-closed in this call, it was NOT done before.
			ctx.ParentWasDone = parent.Status == "done" && !ctx.ParentAutoClosed

			siblings, err := getChildren(db, parent.ID)
			if err != nil {
				return nil, err
			}
			ctx.ParentTotalCount = len(siblings)
			for _, s := range siblings {
				if s.Status == "done" {
					ctx.ParentDoneCount++
				}
			}
			nextSib, skipped, skippedBy, err := findNextSibling(db, siblings, closed)
			if err != nil {
				return nil, err
			}
			ctx.NextSibling = nextSib
			ctx.SkippedBlocked = skipped
			ctx.SkippedBlockedBy = skippedBy

			// When the parent auto-closed, walk up through any additional
			// auto-closed ancestors to find the next work to surface.
			if ctx.ParentAutoClosed {
				topClosed := parent
				for topClosed.ParentID != nil {
					gp, err := getTaskByID(db, *topClosed.ParentID)
					if err != nil {
						return nil, err
					}
					if gp == nil || gp.Status != "done" {
						break
					}
					topClosed = gp
				}
				var grandSiblings []*Task
				if topClosed.ParentID != nil {
					grandSiblings, err = getChildren(db, *topClosed.ParentID)
				} else {
					grandSiblings, err = getRootTasks(db)
				}
				if err != nil {
					return nil, err
				}
				nextAfter, _, _, err := findNextSibling(db, grandSiblings, topClosed)
				if err != nil {
					return nil, err
				}
				ctx.NextAfterParent = nextAfter
			}
		}
	}

	root, err := findTopAncestor(db, closed)
	if err != nil {
		return nil, err
	}
	if root != nil {
		allDone, doneCount, err := subtreeCompleteness(db, root.ID)
		if err != nil {
			return nil, err
		}
		if allDone {
			ctx.WholeTreeComplete = true
			ctx.WholeTreeDoneCount = doneCount
			ctx.WholeTreeRootID = root.ShortID
		}
	}

	return ctx, nil
}

func getTaskByID(db *sql.DB, id int64) (*Task, error) {
	row := db.QueryRow(`
		SELECT id, short_id, parent_id, title, description, status, sort_order,
		       claimed_by, claim_expires_at, completion_note, created_at, updated_at, deleted_at
		FROM tasks WHERE id = ? AND deleted_at IS NULL
	`, id)
	t, err := scanTask(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

func getRootTasks(db *sql.DB) ([]*Task, error) {
	rows, err := db.Query(`
		SELECT id, short_id, parent_id, title, description, status, sort_order,
		       claimed_by, claim_expires_at, completion_note, created_at, updated_at, deleted_at
		FROM tasks WHERE parent_id IS NULL AND deleted_at IS NULL
		ORDER BY sort_order
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, rows.Err()
}

func getChildren(db *sql.DB, parentID int64) ([]*Task, error) {
	rows, err := db.Query(`
		SELECT id, short_id, parent_id, title, description, status, sort_order,
		       claimed_by, claim_expires_at, completion_note, created_at, updated_at, deleted_at
		FROM tasks WHERE parent_id = ? AND deleted_at IS NULL
		ORDER BY sort_order
	`, parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, rows.Err()
}

// findNextSibling returns the next open sibling after `closed` in sort order,
// skipping any sibling that is currently blocked. If the first candidate was
// blocked and we skipped over it, returns the blocked sibling and its first
// blocker via `skipped` / `skippedBy`.
func findNextSibling(db *sql.DB, siblings []*Task, closed *Task) (next *Task, skipped *Task, skippedBy string, err error) {
	var candidates []*Task
	for _, s := range siblings {
		if s.ID == closed.ID {
			continue
		}
		if s.SortOrder <= closed.SortOrder {
			continue
		}
		if s.Status == "done" {
			continue
		}
		candidates = append(candidates, s)
	}
	for i, c := range candidates {
		blockers, err := getBlockers(db, c.ShortID)
		if err != nil {
			return nil, nil, "", err
		}
		if len(blockers) == 0 {
			if i > 0 {
				skipped = candidates[0]
				bl, bErr := getBlockers(db, skipped.ShortID)
				if bErr == nil && len(bl) > 0 {
					skippedBy = bl[0].ShortID
				}
			}
			return c, skipped, skippedBy, nil
		}
	}
	return nil, nil, "", nil
}

func findTopAncestor(db *sql.DB, task *Task) (*Task, error) {
	current := task
	for current.ParentID != nil {
		parent, err := getTaskByID(db, *current.ParentID)
		if err != nil {
			return nil, err
		}
		if parent == nil {
			break
		}
		current = parent
	}
	return current, nil
}

// subtreeCompleteness returns whether every task under (and including) rootID is done, and the count of done tasks in that subtree.
func subtreeCompleteness(db *sql.DB, rootID int64) (allDone bool, doneCount int, err error) {
	rows, err := db.Query(`
		WITH RECURSIVE tree(id, status) AS (
			SELECT id, status FROM tasks WHERE id = ? AND deleted_at IS NULL
			UNION ALL
			SELECT t.id, t.status FROM tasks t JOIN tree ON t.parent_id = tree.id WHERE t.deleted_at IS NULL
		)
		SELECT status FROM tree
	`, rootID)
	if err != nil {
		return false, 0, err
	}
	defer rows.Close()
	total := 0
	allDone = true
	for rows.Next() {
		var status string
		if err := rows.Scan(&status); err != nil {
			return false, 0, err
		}
		total++
		if status == "done" {
			doneCount++
		} else {
			allDone = false
		}
	}
	if err := rows.Err(); err != nil {
		return false, 0, err
	}
	if total == 0 {
		return false, 0, nil
	}
	return allDone, doneCount, nil
}

type TaskInfo struct {
	Task     *Task
	Parent   *Task
	Children []*Task
	Blockers []*Task
	Labels   []string
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

	labels, err := getLabels(db, task.ID)
	if err != nil {
		return nil, err
	}

	return &TaskInfo{
		Task:     task,
		Parent:   parent,
		Children: children,
		Blockers: blockers,
		Labels:   labels,
	}, nil
}
