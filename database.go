package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const defaultDBName = ".jobs.db"
const base62Chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

type dbtx interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

func resolveDBPath(dbFlag string) string {
	if dbFlag != "" {
		return dbFlag
	}
	if env := os.Getenv("JOBS_DB"); env != "" {
		return env
	}
	return defaultDBName
}

func openDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("PRAGMA foreign_keys=ON")
	return db, nil
}

func createDB(path string) (*sql.DB, error) {
	db, err := openDB(path)
	if err != nil {
		return nil, err
	}
	if err := initSchema(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func initSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS tasks (
			id               INTEGER PRIMARY KEY AUTOINCREMENT,
			short_id         TEXT UNIQUE NOT NULL,
			parent_id        INTEGER REFERENCES tasks(id) ON DELETE CASCADE,
			title            TEXT NOT NULL,
			description      TEXT NOT NULL DEFAULT '',
			status           TEXT NOT NULL DEFAULT 'available',
			sort_order       INTEGER NOT NULL DEFAULT 0,
			claimed_by       TEXT,
			claim_expires_at INTEGER,
			completion_note  TEXT,
			created_at       INTEGER NOT NULL DEFAULT (strftime('%s','now')),
			updated_at       INTEGER NOT NULL DEFAULT (strftime('%s','now'))
		);
		CREATE INDEX IF NOT EXISTS idx_tasks_short_id ON tasks(short_id);
		CREATE INDEX IF NOT EXISTS idx_tasks_parent_id ON tasks(parent_id);
		CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);

		CREATE TABLE IF NOT EXISTS blocks (
			blocker_id  INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
			blocked_id  INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
			created_at  INTEGER NOT NULL DEFAULT (strftime('%s','now')),
			PRIMARY KEY (blocker_id, blocked_id)
		);
		CREATE INDEX IF NOT EXISTS idx_blocks_blocked_id ON blocks(blocked_id);

		CREATE TABLE IF NOT EXISTS events (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id     INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
			event_type  TEXT NOT NULL,
			detail      TEXT,
			created_at  INTEGER NOT NULL DEFAULT (strftime('%s','now'))
		);
		CREATE INDEX IF NOT EXISTS idx_events_task_id ON events(task_id);
		CREATE INDEX IF NOT EXISTS idx_events_created_at ON events(created_at);
	`)
	return err
}

func generateShortID(tx dbtx) (string, error) {
	for {
		id := make([]byte, 5)
		for i := range id {
			n, err := rand.Int(rand.Reader, big.NewInt(62))
			if err != nil {
				return "", fmt.Errorf("generate ID: %w", err)
			}
			id[i] = base62Chars[n.Int64()]
		}
		sid := string(id)
		var exists bool
		if err := tx.QueryRow("SELECT EXISTS(SELECT 1 FROM tasks WHERE short_id = ?)", sid).Scan(&exists); err != nil {
			return "", err
		}
		if !exists {
			return sid, nil
		}
	}
}

func recordEvent(tx dbtx, taskID int64, eventType string, detail any) error {
	var detailJSON string
	if detail != nil {
		b, err := json.Marshal(detail)
		if err != nil {
			return fmt.Errorf("marshal event detail: %w", err)
		}
		detailJSON = string(b)
	}
	_, err := tx.Exec(
		"INSERT INTO events (task_id, event_type, detail, created_at) VALUES (?, ?, ?, ?)",
		taskID, eventType, detailJSON, time.Now().Unix(),
	)
	return err
}

func getTaskByShortID(tx dbtx, shortID string) (*Task, error) {
	row := tx.QueryRow(`
		SELECT id, short_id, parent_id, title, description, status, sort_order,
		       claimed_by, claim_expires_at, completion_note, created_at, updated_at
		FROM tasks WHERE short_id = ?
	`, shortID)
	t, err := scanTask(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

func loadAllTasks(db *sql.DB) ([]*Task, error) {
	rows, err := db.Query(`
		SELECT id, short_id, parent_id, title, description, status, sort_order,
		       claimed_by, claim_expires_at, completion_note, created_at, updated_at
		FROM tasks ORDER BY parent_id, sort_order
	`)
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

func buildTree(tasks []*Task) []*TaskNode {
	byID := make(map[int64]*TaskNode)
	for _, t := range tasks {
		byID[t.ID] = &TaskNode{Task: t}
	}
	var roots []*TaskNode
	for _, t := range tasks {
		node := byID[t.ID]
		if t.ParentID == nil {
			roots = append(roots, node)
		} else if parent, ok := byID[*t.ParentID]; ok {
			parent.Children = append(parent.Children, node)
		}
	}
	return roots
}

func filterTree(nodes []*TaskNode, showAll bool) []*TaskNode {
	if showAll {
		return nodes
	}
	var result []*TaskNode
	for _, node := range nodes {
		if node.Task.Status != "available" {
			continue
		}
		result = append(result, &TaskNode{
			Task:     node.Task,
			Children: filterTree(node.Children, false),
		})
	}
	return result
}

func findNodeByShortID(nodes []*TaskNode, shortID string) *TaskNode {
	for _, node := range nodes {
		if node.Task.ShortID == shortID {
			return node
		}
		if found := findNodeByShortID(node.Children, shortID); found != nil {
			return found
		}
	}
	return nil
}

func getLatestEventDetail(tx dbtx, taskID int64, eventType string) (map[string]any, error) {
	var detail string
	err := tx.QueryRow(
		"SELECT detail FROM events WHERE task_id = ? AND event_type = ? ORDER BY created_at DESC, id DESC LIMIT 1",
		taskID, eventType,
	).Scan(&detail)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(detail), &result); err != nil {
		return nil, err
	}
	return result, nil
}

func findIncompleteDescendants(tx dbtx, taskID int64) ([]*Task, error) {
	rows, err := tx.Query(`
		SELECT id, short_id, parent_id, title, description, status, sort_order,
		       claimed_by, claim_expires_at, completion_note, created_at, updated_at
		FROM tasks WHERE parent_id = ? AND status != 'done'
	`, taskID)
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
		desc, err := findIncompleteDescendants(tx, t.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, desc...)
	}
	return result, rows.Err()
}

func childShortIDs(tx dbtx, parentID int64) ([]string, error) {
	rows, err := tx.Query("SELECT short_id FROM tasks WHERE parent_id = ? AND status != 'done'", parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

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
			_, err = tx.Exec("UPDATE tasks SET sort_order = sort_order + 1 WHERE parent_id IS NULL AND sort_order >= ?", sortOrder)
		} else {
			_, err = tx.Exec("UPDATE tasks SET sort_order = sort_order + 1 WHERE parent_id = ? AND sort_order >= ?", *parentID, sortOrder)
		}
		if err != nil {
			return "", err
		}
	} else {
		var maxSort sql.NullInt64
		if parentID == nil {
			err = tx.QueryRow("SELECT MAX(sort_order) FROM tasks WHERE parent_id IS NULL").Scan(&maxSort)
		} else {
			err = tx.QueryRow("SELECT MAX(sort_order) FROM tasks WHERE parent_id = ?", *parentID).Scan(&maxSort)
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

	return filterTree(tree, showAll), nil
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
