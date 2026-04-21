package job

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

var CurrentNowFunc = time.Now

const defaultDBName = ".jobs.db"
const base62Chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

type dbtx interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

func ResolveDBPath(dbFlag string) string {
	if dbFlag != "" {
		return dbFlag
	}
	if env := os.Getenv("JOBS_DB"); env != "" {
		return env
	}
	if found := findAncestorDB(); found != "" {
		return found
	}
	return defaultDBName
}

// ResolveDBPathForInit is used by `job init` to pick a destination path. It
// intentionally does NOT walk up looking for an ancestor database: running
// `job init` in a subfolder of an existing project creates a new db in cwd,
// the same way `git init` inside a git repo creates a new repo.
func ResolveDBPathForInit(dbFlag string) string {
	if dbFlag != "" {
		return dbFlag
	}
	if env := os.Getenv("JOBS_DB"); env != "" {
		return env
	}
	return defaultDBName
}

func findAncestorDB() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	dir := cwd
	for {
		candidate := filepath.Join(dir, defaultDBName)
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func OpenDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("PRAGMA foreign_keys=ON")
	if err := InitSchema(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func CreateDB(path string) (*sql.DB, error) {
	db, err := OpenDB(path)
	if err != nil {
		return nil, err
	}
	if err := InitSchema(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func InitSchema(db *sql.DB) error {
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
			updated_at       INTEGER NOT NULL DEFAULT (strftime('%s','now')),
			deleted_at       INTEGER
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

		CREATE TABLE IF NOT EXISTS task_labels (
			task_id    INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
			name       TEXT NOT NULL,
			created_at INTEGER NOT NULL DEFAULT (strftime('%s','now')),
			PRIMARY KEY (task_id, name)
		);
		CREATE INDEX IF NOT EXISTS idx_task_labels_name ON task_labels(name);

		CREATE TABLE IF NOT EXISTS events (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id     INTEGER REFERENCES tasks(id),
			event_type  TEXT NOT NULL,
			actor       TEXT NOT NULL DEFAULT '',
			detail      TEXT,
			created_at  INTEGER NOT NULL DEFAULT (strftime('%s','now'))
		);
		CREATE INDEX IF NOT EXISTS idx_events_task_id ON events(task_id);
		CREATE INDEX IF NOT EXISTS idx_events_created_at ON events(created_at);

		CREATE TABLE IF NOT EXISTS users (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			name       TEXT UNIQUE NOT NULL,
			created_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
		)
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

func recordEvent(tx dbtx, taskID int64, eventType, actor string, detail any) error {
	var detailJSON string
	if detail != nil {
		b, err := json.Marshal(detail)
		if err != nil {
			return fmt.Errorf("marshal event detail: %w", err)
		}
		detailJSON = string(b)
	}
	_, err := tx.Exec(
		"INSERT INTO events (task_id, event_type, actor, detail, created_at) VALUES (?, ?, ?, ?, ?)",
		taskID, eventType, actor, detailJSON, time.Now().Unix(),
	)
	return err
}

// recordOrphanEvent records an event with task_id = NULL. Used for events that
// outlive their subject task (e.g. a `purged` event on a root task whose row
// is being erased in the same transaction).
func recordOrphanEvent(tx dbtx, eventType, actor string, detail any) error {
	var detailJSON string
	if detail != nil {
		b, err := json.Marshal(detail)
		if err != nil {
			return fmt.Errorf("marshal event detail: %w", err)
		}
		detailJSON = string(b)
	}
	_, err := tx.Exec(
		"INSERT INTO events (task_id, event_type, actor, detail, created_at) VALUES (NULL, ?, ?, ?, ?)",
		eventType, actor, detailJSON, time.Now().Unix(),
	)
	return err
}

func GetTaskByShortID(tx dbtx, shortID string) (*Task, error) {
	return getTaskByShortIDFilter(tx, shortID, true)
}

func getTaskByShortIDIncludeDeleted(tx dbtx, shortID string) (*Task, error) {
	return getTaskByShortIDFilter(tx, shortID, false)
}

func getTaskByShortIDFilter(tx dbtx, shortID string, excludeDeleted bool) (*Task, error) {
	q := `
		SELECT id, short_id, parent_id, title, description, status, sort_order,
		       claimed_by, claim_expires_at, completion_note, created_at, updated_at, deleted_at
		FROM tasks WHERE short_id = ?`
	if excludeDeleted {
		q += " AND deleted_at IS NULL"
	}
	row := tx.QueryRow(q, shortID)
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
		       claimed_by, claim_expires_at, completion_note, created_at, updated_at, deleted_at
		FROM tasks WHERE deleted_at IS NULL ORDER BY parent_id, sort_order
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

func filterTree(nodes []*TaskNode, showAll bool, blockedIDs map[int64]bool) []*TaskNode {
	if showAll {
		return nodes
	}
	var result []*TaskNode
	for _, node := range nodes {
		// Default `list` shows only actionable work — open + unblocked + unclaimed.
		// Done and canceled tasks are explicitly hidden; pass `all` to surface them.
		if node.Task.Status != "available" || blockedIDs[node.Task.ID] {
			continue
		}
		result = append(result, &TaskNode{
			Task:     node.Task,
			Children: filterTree(node.Children, false, blockedIDs),
		})
	}
	return result
}

func getBlockedTaskIDs(db *sql.DB) (map[int64]bool, error) {
	rows, err := db.Query(`
		SELECT DISTINCT b.blocked_id
		FROM blocks b
		JOIN tasks t ON t.id = b.blocker_id
		WHERE t.status != 'done' AND t.deleted_at IS NULL
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int64]bool)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result[id] = true
	}
	return result, rows.Err()
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

func GetLatestEventDetail(tx dbtx, taskID int64, eventType string) (map[string]any, error) {
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

// findClosedDescendants returns every descendant whose status is either
// "done" or "canceled". Used by `reopen --cascade` to revive a closed subtree.
func findClosedDescendants(tx dbtx, taskID int64) ([]*Task, error) {
	rows, err := tx.Query(`
		SELECT id, short_id, parent_id, title, description, status, sort_order,
		       claimed_by, claim_expires_at, completion_note, created_at, updated_at, deleted_at
		FROM tasks WHERE parent_id = ? AND deleted_at IS NULL
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
		if t.Status == "done" || t.Status == "canceled" {
			result = append(result, t)
		}
		desc, err := findClosedDescendants(tx, t.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, desc...)
	}
	return result, rows.Err()
}

func findDoneDescendants(tx dbtx, taskID int64) ([]*Task, error) {
	rows, err := tx.Query(`
		SELECT id, short_id, parent_id, title, description, status, sort_order,
		       claimed_by, claim_expires_at, completion_note, created_at, updated_at, deleted_at
		FROM tasks WHERE parent_id = ? AND deleted_at IS NULL
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
		if t.Status == "done" {
			result = append(result, t)
		}
		desc, err := findDoneDescendants(tx, t.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, desc...)
	}
	return result, rows.Err()
}

// findOpenDescendants returns every descendant of taskID whose status is
// neither "done" nor "canceled". Used by `cancel --cascade` to walk the live
// subtree under a task being canceled.
func findOpenDescendants(tx dbtx, taskID int64) ([]*Task, error) {
	rows, err := tx.Query(`
		SELECT id, short_id, parent_id, title, description, status, sort_order,
		       claimed_by, claim_expires_at, completion_note, created_at, updated_at, deleted_at
		FROM tasks WHERE parent_id = ? AND status != 'done' AND status != 'canceled' AND deleted_at IS NULL
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
		desc, err := findOpenDescendants(tx, t.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, desc...)
	}
	return result, rows.Err()
}

// findAllDescendants returns every descendant of taskID regardless of status.
// Used by `cancel --purge --cascade` which erases the entire subtree.
func findAllDescendants(tx dbtx, taskID int64) ([]*Task, error) {
	rows, err := tx.Query(`
		SELECT id, short_id, parent_id, title, description, status, sort_order,
		       claimed_by, claim_expires_at, completion_note, created_at, updated_at, deleted_at
		FROM tasks WHERE parent_id = ? AND deleted_at IS NULL
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
		desc, err := findAllDescendants(tx, t.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, desc...)
	}
	return result, rows.Err()
}

func findIncompleteDescendants(tx dbtx, taskID int64) ([]*Task, error) {
	rows, err := tx.Query(`
		SELECT id, short_id, parent_id, title, description, status, sort_order,
		       claimed_by, claim_expires_at, completion_note, created_at, updated_at, deleted_at
		FROM tasks WHERE parent_id = ? AND status != 'done' AND deleted_at IS NULL
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
	rows, err := tx.Query("SELECT short_id FROM tasks WHERE parent_id = ? AND status != 'done' AND deleted_at IS NULL", parentID)
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

func GetEventsForTaskTree(db *sql.DB, shortID string) ([]EventEntry, error) {
	rows, err := db.Query(`
		WITH RECURSIVE tree AS (
			SELECT id FROM tasks WHERE short_id = ? AND deleted_at IS NULL
			UNION ALL
			SELECT t.id FROM tasks t JOIN tree ON t.parent_id = tree.id WHERE t.deleted_at IS NULL
		)
		SELECT e.id, e.task_id, t.short_id, e.event_type, e.actor, e.detail, e.created_at
		FROM events e
		JOIN tasks t ON t.id = e.task_id
		WHERE e.task_id IN (SELECT id FROM tree)
		ORDER BY e.created_at, e.id
	`, shortID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []EventEntry
	for rows.Next() {
		var e EventEntry
		if err := rows.Scan(&e.ID, &e.TaskID, &e.ShortID, &e.EventType, &e.Actor, &e.Detail, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func getEventsForTaskTreeSince(db *sql.DB, shortID string, since int64) ([]EventEntry, error) {
	rows, err := db.Query(`
		WITH RECURSIVE tree AS (
			SELECT id FROM tasks WHERE short_id = ? AND deleted_at IS NULL
			UNION ALL
			SELECT t.id FROM tasks t JOIN tree ON t.parent_id = tree.id WHERE t.deleted_at IS NULL
		)
		SELECT e.id, e.task_id, t.short_id, e.event_type, e.actor, e.detail, e.created_at
		FROM events e
		JOIN tasks t ON t.id = e.task_id
		WHERE e.task_id IN (SELECT id FROM tree) AND e.created_at >= ?
		ORDER BY e.created_at, e.id
	`, shortID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []EventEntry
	for rows.Next() {
		var e EventEntry
		if err := rows.Scan(&e.ID, &e.TaskID, &e.ShortID, &e.EventType, &e.Actor, &e.Detail, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func getEventsAfterID(db *sql.DB, shortID string, afterID int64) ([]EventEntry, error) {
	rows, err := db.Query(`
		WITH RECURSIVE tree AS (
			SELECT id FROM tasks WHERE short_id = ? AND deleted_at IS NULL
			UNION ALL
			SELECT t.id FROM tasks t JOIN tree ON t.parent_id = tree.id WHERE t.deleted_at IS NULL
		)
		SELECT e.id, e.task_id, t.short_id, e.event_type, e.actor, e.detail, e.created_at
		FROM events e
		JOIN tasks t ON t.id = e.task_id
		WHERE e.task_id IN (SELECT id FROM tree) AND e.id > ?
		ORDER BY e.created_at, e.id
	`, shortID, afterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []EventEntry
	for rows.Next() {
		var e EventEntry
		if err := rows.Scan(&e.ID, &e.TaskID, &e.ShortID, &e.EventType, &e.Actor, &e.Detail, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
