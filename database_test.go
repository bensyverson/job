package main

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := createDB(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func mustAdd(t *testing.T, db *sql.DB, parentShortID, title string) string {
	t.Helper()
	id, err := runAdd(db, parentShortID, title, "", "")
	if err != nil {
		t.Fatalf("add task %q: %v", title, err)
	}
	return id
}

func mustAddDesc(t *testing.T, db *sql.DB, parentShortID, title, desc string) string {
	t.Helper()
	id, err := runAdd(db, parentShortID, title, desc, "")
	if err != nil {
		t.Fatalf("add task %q: %v", title, err)
	}
	return id
}

func mustDone(t *testing.T, db *sql.DB, shortID string) {
	t.Helper()
	if _, err := runDone(db, shortID, false, ""); err != nil {
		t.Fatalf("done task %s: %v", shortID, err)
	}
}

func mustGet(t *testing.T, db *sql.DB, shortID string) *Task {
	t.Helper()
	task, err := getTaskByShortID(db, shortID)
	if err != nil {
		t.Fatalf("get task %s: %v", shortID, err)
	}
	if task == nil {
		t.Fatalf("task %s not found", shortID)
	}
	return task
}

// --- Add ---

func TestRunAdd_RootTask(t *testing.T) {
	db := setupTestDB(t)
	id, err := runAdd(db, "", "Root task", "", "")
	if err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	if len(id) != 5 {
		t.Fatalf("expected 5-char ID, got %q", id)
	}

	task := mustGet(t, db, id)
	if task.Title != "Root task" {
		t.Errorf("title: got %q, want %q", task.Title, "Root task")
	}
	if task.ParentID != nil {
		t.Errorf("parent_id: got %d, want nil", *task.ParentID)
	}
	if task.Status != "available" {
		t.Errorf("status: got %q, want %q", task.Status, "available")
	}
	if task.SortOrder != 0 {
		t.Errorf("sort_order: got %d, want 0", task.SortOrder)
	}
}

func TestRunAdd_Subtask(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	cid, err := runAdd(db, pid, "Child", "", "")
	if err != nil {
		t.Fatalf("runAdd: %v", err)
	}

	child := mustGet(t, db, cid)
	if child.ParentID == nil {
		t.Fatal("parent_id: got nil, want non-nil")
	}
	parent := mustGet(t, db, pid)
	if *child.ParentID != parent.ID {
		t.Errorf("parent_id: got %d, want %d", *child.ParentID, parent.ID)
	}
}

func TestRunAdd_WithDescription(t *testing.T) {
	db := setupTestDB(t)
	id := mustAddDesc(t, db, "", "Task", "Some description")
	task := mustGet(t, db, id)
	if task.Description != "Some description" {
		t.Errorf("description: got %q, want %q", task.Description, "Some description")
	}
}

func TestRunAdd_SortOrder(t *testing.T) {
	db := setupTestDB(t)
	id1 := mustAdd(t, db, "", "First")
	id2 := mustAdd(t, db, "", "Second")
	id3 := mustAdd(t, db, "", "Third")

	t1 := mustGet(t, db, id1)
	t2 := mustGet(t, db, id2)
	t3 := mustGet(t, db, id3)

	if t1.SortOrder >= t2.SortOrder {
		t.Errorf("First sort_order %d >= Second %d", t1.SortOrder, t2.SortOrder)
	}
	if t2.SortOrder >= t3.SortOrder {
		t.Errorf("Second sort_order %d >= Third %d", t2.SortOrder, t3.SortOrder)
	}
}

func TestRunAdd_Before(t *testing.T) {
	db := setupTestDB(t)
	id1 := mustAdd(t, db, "", "First")
	id2 := mustAdd(t, db, "", "Last")
	id3, err := runAdd(db, "", "Middle", "", id2)
	if err != nil {
		t.Fatalf("runAdd before: %v", err)
	}

	t1 := mustGet(t, db, id1)
	t2 := mustGet(t, db, id2)
	t3 := mustGet(t, db, id3)

	if t3.SortOrder >= t2.SortOrder {
		t.Errorf("Middle sort_order %d should be < Last %d", t3.SortOrder, t2.SortOrder)
	}
	if t1.SortOrder >= t3.SortOrder {
		t.Errorf("First sort_order %d should be < Middle %d", t1.SortOrder, t3.SortOrder)
	}
}

func TestRunAdd_ParentNotFound(t *testing.T) {
	db := setupTestDB(t)
	_, err := runAdd(db, "noExs", "Task", "", "")
	if err == nil {
		t.Fatal("expected error for non-existent parent")
	}
}

func TestRunAdd_BeforeNotFound(t *testing.T) {
	db := setupTestDB(t)
	_, err := runAdd(db, "", "Task", "", "noExs")
	if err == nil {
		t.Fatal("expected error for non-existent before target")
	}
}

func TestRunAdd_BeforeNotSibling(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	otherID := mustAdd(t, db, "", "Other root")
	_, err := runAdd(db, pid, "Child", "", otherID)
	if err == nil {
		t.Fatal("expected error when before target is not a sibling")
	}
}

func TestRunAdd_CreatedEvent(t *testing.T) {
	db := setupTestDB(t)
	id := mustAddDesc(t, db, "", "My task", "my desc")
	task := mustGet(t, db, id)

	detail, err := getLatestEventDetail(db, task.ID, "created")
	if err != nil {
		t.Fatalf("getLatestEventDetail: %v", err)
	}
	if detail == nil {
		t.Fatal("expected created event, got nil")
	}
	if detail["title"] != "My task" {
		t.Errorf("title: got %v, want %q", detail["title"], "My task")
	}
	if detail["description"] != "my desc" {
		t.Errorf("description: got %v, want %q", detail["description"], "my desc")
	}
}

// --- List ---

func TestRunList_Empty(t *testing.T) {
	db := setupTestDB(t)
	nodes, err := runList(db, "", false)
	if err != nil {
		t.Fatalf("runList: %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(nodes))
	}
}

func TestRunList_RootTasks(t *testing.T) {
	db := setupTestDB(t)
	mustAdd(t, db, "", "Task A")
	mustAdd(t, db, "", "Task B")

	nodes, err := runList(db, "", false)
	if err != nil {
		t.Fatalf("runList: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 roots, got %d", len(nodes))
	}
	if nodes[0].Task.Title != "Task A" {
		t.Errorf("first: got %q, want %q", nodes[0].Task.Title, "Task A")
	}
	if nodes[1].Task.Title != "Task B" {
		t.Errorf("second: got %q, want %q", nodes[1].Task.Title, "Task B")
	}
}

func TestRunList_Subtasks(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	mustAdd(t, db, pid, "Child A")
	mustAdd(t, db, pid, "Child B")

	nodes, err := runList(db, pid, false)
	if err != nil {
		t.Fatalf("runList: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 children, got %d", len(nodes))
	}
}

func TestRunList_TreeStructure(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	cid := mustAdd(t, db, pid, "Child")
	mustAdd(t, db, cid, "Grandchild")

	nodes, err := runList(db, "", false)
	if err != nil {
		t.Fatalf("runList: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("roots: got %d, want 1", len(nodes))
	}
	if len(nodes[0].Children) != 1 {
		t.Fatalf("children: got %d, want 1", len(nodes[0].Children))
	}
	if len(nodes[0].Children[0].Children) != 1 {
		t.Fatalf("grandchildren: got %d, want 1", len(nodes[0].Children[0].Children))
	}
}

func TestRunList_DefaultExcludesDone(t *testing.T) {
	db := setupTestDB(t)
	idAvail := mustAdd(t, db, "", "Available")
	idDone := mustAdd(t, db, "", "Done")
	mustDone(t, db, idDone)

	nodes, err := runList(db, "", false)
	if err != nil {
		t.Fatalf("runList: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if nodes[0].Task.ShortID != idAvail {
		t.Errorf("got %s, want %s", nodes[0].Task.ShortID, idAvail)
	}
}

func TestRunList_AllIncludesDone(t *testing.T) {
	db := setupTestDB(t)
	mustAdd(t, db, "", "Available")
	idDone := mustAdd(t, db, "", "Done")
	mustDone(t, db, idDone)

	nodes, err := runList(db, "", true)
	if err != nil {
		t.Fatalf("runList: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}
}

func TestRunList_DoneChildHidden(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	cidDone := mustAdd(t, db, pid, "Done child")
	mustAdd(t, db, pid, "Available child")
	mustDone(t, db, cidDone)

	nodes, err := runList(db, "", false)
	if err != nil {
		t.Fatalf("runList: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("roots: got %d, want 1", len(nodes))
	}
	if len(nodes[0].Children) != 1 {
		t.Fatalf("visible children: got %d, want 1", len(nodes[0].Children))
	}
	if nodes[0].Children[0].Task.Title != "Available child" {
		t.Errorf("got %q, want %q", nodes[0].Children[0].Task.Title, "Available child")
	}
}

func TestRunList_ParentNotFound(t *testing.T) {
	db := setupTestDB(t)
	_, err := runList(db, "noExs", false)
	if err == nil {
		t.Fatal("expected error for non-existent parent")
	}
}

// --- Done ---

func TestRunDone_LeafTask(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	forced, err := runDone(db, id, false, "")
	if err != nil {
		t.Fatalf("runDone: %v", err)
	}
	if len(forced) != 0 {
		t.Errorf("forced: got %d, want 0", len(forced))
	}

	task := mustGet(t, db, id)
	if task.Status != "done" {
		t.Errorf("status: got %q, want %q", task.Status, "done")
	}
}

func TestRunDone_WithNote(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	if _, err := runDone(db, id, false, "abc1234"); err != nil {
		t.Fatalf("runDone: %v", err)
	}

	task := mustGet(t, db, id)
	if task.CompletionNote == nil || *task.CompletionNote != "abc1234" {
		t.Errorf("note: got %v, want %q", task.CompletionNote, "abc1234")
	}
}

func TestRunDone_IncompleteChildren(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	mustAdd(t, db, pid, "Incomplete child")

	_, err := runDone(db, pid, false, "")
	if err == nil {
		t.Fatal("expected error for incomplete children")
	}
	if !strings.Contains(err.Error(), "Incomplete child") || !strings.Contains(err.Error(), "incomplete") {
		t.Errorf("error should mention incomplete children: %v", err)
	}
}

func TestRunDone_ForceClosesChildren(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	cid := mustAdd(t, db, pid, "Child")

	forced, err := runDone(db, pid, true, "")
	if err != nil {
		t.Fatalf("runDone --force: %v", err)
	}
	if len(forced) != 1 || forced[0] != cid {
		t.Errorf("forced: got %v, want [%s]", forced, cid)
	}

	child := mustGet(t, db, cid)
	if child.Status != "done" {
		t.Errorf("child status: got %q, want %q", child.Status, "done")
	}
}

func TestRunDone_ForceNested(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	cid := mustAdd(t, db, pid, "Child")
	gcid := mustAdd(t, db, cid, "Grandchild")

	forced, err := runDone(db, pid, true, "")
	if err != nil {
		t.Fatalf("runDone --force: %v", err)
	}
	if len(forced) != 2 {
		t.Errorf("forced: got %d, want 2", len(forced))
	}

	for _, id := range []string{cid, gcid} {
		task := mustGet(t, db, id)
		if task.Status != "done" {
			t.Errorf("%s status: got %q, want %q", id, task.Status, "done")
		}
	}
}

func TestRunDone_AlreadyDone(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")
	mustDone(t, db, id)

	_, err := runDone(db, id, false, "")
	if err == nil {
		t.Fatal("expected error for already-done task")
	}
}

func TestRunDone_NotFound(t *testing.T) {
	db := setupTestDB(t)
	_, err := runDone(db, "noExs", false, "")
	if err == nil {
		t.Fatal("expected error for non-existent task")
	}
}

func TestRunDone_DoneEvent(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")
	if _, err := runDone(db, id, false, "abc1234"); err != nil {
		t.Fatalf("runDone: %v", err)
	}

	task := mustGet(t, db, id)
	detail, err := getLatestEventDetail(db, task.ID, "done")
	if err != nil {
		t.Fatalf("getLatestEventDetail: %v", err)
	}
	if detail == nil {
		t.Fatal("expected done event")
	}
	if detail["note"] != "abc1234" {
		t.Errorf("note: got %v, want %q", detail["note"], "abc1234")
	}
	if detail["force"] != false {
		t.Errorf("force: got %v, want false", detail["force"])
	}
}

func TestRunDone_ForceEventRecordsChildren(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	cid := mustAdd(t, db, pid, "Child")

	if _, err := runDone(db, pid, true, ""); err != nil {
		t.Fatalf("runDone: %v", err)
	}

	parent := mustGet(t, db, pid)
	detail, err := getLatestEventDetail(db, parent.ID, "done")
	if err != nil {
		t.Fatalf("getLatestEventDetail: %v", err)
	}
	children, ok := detail["force_closed_children"].([]any)
	if !ok {
		t.Fatalf("force_closed_children: got %T, want []any", detail["force_closed_children"])
	}
	if len(children) != 1 {
		t.Fatalf("force_closed_children: got %d, want 1", len(children))
	}
	if children[0] != cid {
		t.Errorf("force_closed_children[0]: got %v, want %s", children[0], cid)
	}
}

// --- Reopen ---

func TestRunReopen_DoneTask(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")
	mustDone(t, db, id)

	reopened, err := runReopen(db, id)
	if err != nil {
		t.Fatalf("runReopen: %v", err)
	}
	if len(reopened) != 0 {
		t.Errorf("reopened children: got %d, want 0", len(reopened))
	}

	task := mustGet(t, db, id)
	if task.Status != "available" {
		t.Errorf("status: got %q, want %q", task.Status, "available")
	}
	if task.CompletionNote != nil {
		t.Errorf("completion_note: got %v, want nil", task.CompletionNote)
	}
}

func TestRunReopen_ForceClosedChildren(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	cid := mustAdd(t, db, pid, "Child")

	if _, err := runDone(db, pid, true, ""); err != nil {
		t.Fatalf("runDone --force: %v", err)
	}

	reopened, err := runReopen(db, pid)
	if err != nil {
		t.Fatalf("runReopen: %v", err)
	}
	if len(reopened) != 1 || reopened[0] != cid {
		t.Errorf("reopened: got %v, want [%s]", reopened, cid)
	}

	child := mustGet(t, db, cid)
	if child.Status != "available" {
		t.Errorf("child status: got %q, want %q", child.Status, "available")
	}
}

func TestRunReopen_AvailableTask(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	_, err := runReopen(db, id)
	if err == nil {
		t.Fatal("expected error when reopening available task")
	}
}

func TestRunReopen_NotFound(t *testing.T) {
	db := setupTestDB(t)
	_, err := runReopen(db, "noExs")
	if err == nil {
		t.Fatal("expected error for non-existent task")
	}
}

func TestRunReopen_ReopenedEvent(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")
	mustDone(t, db, id)

	if _, err := runReopen(db, id); err != nil {
		t.Fatalf("runReopen: %v", err)
	}

	task := mustGet(t, db, id)
	detail, err := getLatestEventDetail(db, task.ID, "reopened")
	if err != nil {
		t.Fatalf("getLatestEventDetail: %v", err)
	}
	if detail == nil {
		t.Fatal("expected reopened event")
	}
}

func TestRunReopen_ForceClosedNested(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	cid := mustAdd(t, db, pid, "Child")
	gcid := mustAdd(t, db, cid, "Grandchild")

	if _, err := runDone(db, pid, true, ""); err != nil {
		t.Fatalf("runDone --force: %v", err)
	}

	reopened, err := runReopen(db, pid)
	if err != nil {
		t.Fatalf("runReopen: %v", err)
	}
	if len(reopened) != 2 {
		t.Errorf("reopened: got %d, want 2", len(reopened))
	}

	for _, id := range []string{cid, gcid} {
		task := mustGet(t, db, id)
		if task.Status != "available" {
			t.Errorf("%s status: got %q, want %q", id, task.Status, "available")
		}
	}
}
