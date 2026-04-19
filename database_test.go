package main

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// --- Edit ---

func TestRunEdit_ChangesTitle(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Old title")

	if err := runEdit(db, id, "New title"); err != nil {
		t.Fatalf("runEdit: %v", err)
	}

	task := mustGet(t, db, id)
	if task.Title != "New title" {
		t.Errorf("title: got %q, want %q", task.Title, "New title")
	}
}

func TestRunEdit_RecordsEvent(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Old title")

	if err := runEdit(db, id, "New title"); err != nil {
		t.Fatalf("runEdit: %v", err)
	}

	task := mustGet(t, db, id)
	detail, err := getLatestEventDetail(db, task.ID, "edited")
	if err != nil {
		t.Fatalf("getLatestEventDetail: %v", err)
	}
	if detail == nil {
		t.Fatal("expected edited event")
	}
	if detail["old_title"] != "Old title" {
		t.Errorf("old_title: got %v, want %q", detail["old_title"], "Old title")
	}
	if detail["new_title"] != "New title" {
		t.Errorf("new_title: got %v, want %q", detail["new_title"], "New title")
	}
}

func TestRunEdit_NotFound(t *testing.T) {
	db := setupTestDB(t)
	err := runEdit(db, "noExs", "New title")
	if err == nil {
		t.Fatal("expected error for non-existent task")
	}
}

// --- Note ---

func TestRunNote_AppendsToEmptyDescription(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	if err := runNote(db, id, "First note"); err != nil {
		t.Fatalf("runNote: %v", err)
	}

	task := mustGet(t, db, id)
	if task.Description != "First note" {
		t.Errorf("description: got %q, want %q", task.Description, "First note")
	}
}

func TestRunNote_AppendsToExistingDescription(t *testing.T) {
	db := setupTestDB(t)
	id := mustAddDesc(t, db, "", "Task", "Original desc")

	if err := runNote(db, id, "Added note"); err != nil {
		t.Fatalf("runNote: %v", err)
	}

	task := mustGet(t, db, id)
	if !strings.Contains(task.Description, "Original desc") {
		t.Errorf("description should contain original: %q", task.Description)
	}
	if !strings.Contains(task.Description, "Added note") {
		t.Errorf("description should contain note: %q", task.Description)
	}
	if !strings.Contains(task.Description, "[20") {
		t.Errorf("description should contain timestamp: %q", task.Description)
	}
}

func TestRunNote_RecordsEvent(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	if err := runNote(db, id, "A note"); err != nil {
		t.Fatalf("runNote: %v", err)
	}

	task := mustGet(t, db, id)
	detail, err := getLatestEventDetail(db, task.ID, "noted")
	if err != nil {
		t.Fatalf("getLatestEventDetail: %v", err)
	}
	if detail == nil {
		t.Fatal("expected noted event")
	}
	if detail["text"] != "A note" {
		t.Errorf("text: got %v, want %q", detail["text"], "A note")
	}
}

func TestRunNote_NotFound(t *testing.T) {
	db := setupTestDB(t)
	err := runNote(db, "noExs", "A note")
	if err == nil {
		t.Fatal("expected error for non-existent task")
	}
}

// --- Remove (soft delete) ---

func TestRunRemove_LeafTask(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	count, err := runRemove(db, id, false, false)
	if err != nil {
		t.Fatalf("runRemove: %v", err)
	}
	if count != 0 {
		t.Errorf("removed children: got %d, want 0", count)
	}

	task, err := getTaskByShortID(db, id)
	if err != nil {
		t.Fatalf("getTaskByShortID: %v", err)
	}
	if task != nil {
		t.Error("task should not be visible after removal")
	}

	deletedTask, err := getTaskByShortIDIncludeDeleted(db, id)
	if err != nil {
		t.Fatalf("getTaskByShortIDIncludeDeleted: %v", err)
	}
	if deletedTask == nil {
		t.Fatal("task should still exist in DB (soft delete)")
	}
	if deletedTask.DeletedAt == nil {
		t.Error("deleted_at should be set")
	}
}

func TestRunRemove_HasChildren(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	mustAdd(t, db, pid, "Child")

	_, err := runRemove(db, pid, false, false)
	if err == nil {
		t.Fatal("expected error when removing task with children without 'all'")
	}
}

func TestRunRemove_WithAll(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	cid := mustAdd(t, db, pid, "Child")

	count, err := runRemove(db, pid, true, false)
	if err != nil {
		t.Fatalf("runRemove: %v", err)
	}
	if count != 1 {
		t.Errorf("removed children: got %d, want 1", count)
	}

	for _, id := range []string{pid, cid} {
		task, _ := getTaskByShortID(db, id)
		if task != nil {
			t.Errorf("%s should not be visible after removal", id)
		}
		deletedTask, _ := getTaskByShortIDIncludeDeleted(db, id)
		if deletedTask == nil {
			t.Errorf("%s should still exist in DB (soft delete)", id)
		}
	}
}

func TestRunRemove_RecordsEvent(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task to remove")
	task := mustGet(t, db, id)

	if _, err := runRemove(db, id, false, false); err != nil {
		t.Fatalf("runRemove: %v", err)
	}

	detail, err := getLatestEventDetail(db, task.ID, "removed")
	if err != nil {
		t.Fatalf("getLatestEventDetail: %v", err)
	}
	if detail == nil {
		t.Fatal("expected removed event")
	}
	if detail["title"] != "Task to remove" {
		t.Errorf("title: got %v, want %q", detail["title"], "Task to remove")
	}
}

func TestRunRemove_NotFound(t *testing.T) {
	db := setupTestDB(t)
	_, err := runRemove(db, "noExs", false, false)
	if err == nil {
		t.Fatal("expected error for non-existent task")
	}
}

func TestRunRemove_RemovedTaskNotInList(t *testing.T) {
	db := setupTestDB(t)
	id1 := mustAdd(t, db, "", "Keep")
	id2 := mustAdd(t, db, "", "Remove")

	if _, err := runRemove(db, id2, false, false); err != nil {
		t.Fatalf("runRemove: %v", err)
	}

	nodes, err := runList(db, "", false)
	if err != nil {
		t.Fatalf("runList: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if nodes[0].Task.ShortID != id1 {
		t.Errorf("got %s, want %s", nodes[0].Task.ShortID, id1)
	}
}

func TestRunRemove_RemovesBlockingRelationships(t *testing.T) {
	db := setupTestDB(t)
	blocker := mustAdd(t, db, "", "Blocker")
	blocked := mustAdd(t, db, "", "Blocked")
	if err := runBlock(db, blocked, blocker); err != nil {
		t.Fatalf("runBlock: %v", err)
	}

	if _, err := runRemove(db, blocker, false, false); err != nil {
		t.Fatalf("runRemove: %v", err)
	}

	blocks, err := getBlockers(db, blocked)
	if err != nil {
		t.Fatalf("getBlockers: %v", err)
	}
	if len(blocks) != 0 {
		t.Errorf("expected no blockers after removal, got %d", len(blocks))
	}
}

// --- Move ---

func TestRunMove_Before(t *testing.T) {
	db := setupTestDB(t)
	id1 := mustAdd(t, db, "", "First")
	id2 := mustAdd(t, db, "", "Second")

	if err := runMove(db, id2, "before", id1); err != nil {
		t.Fatalf("runMove: %v", err)
	}

	t1 := mustGet(t, db, id1)
	t2 := mustGet(t, db, id2)
	if t2.SortOrder >= t1.SortOrder {
		t.Errorf("Second (sort %d) should be before First (sort %d)", t2.SortOrder, t1.SortOrder)
	}
}

func TestRunMove_After(t *testing.T) {
	db := setupTestDB(t)
	id1 := mustAdd(t, db, "", "First")
	id2 := mustAdd(t, db, "", "Second")

	if err := runMove(db, id1, "after", id2); err != nil {
		t.Fatalf("runMove: %v", err)
	}

	t1 := mustGet(t, db, id1)
	t2 := mustGet(t, db, id2)
	if t1.SortOrder <= t2.SortOrder {
		t.Errorf("First (sort %d) should be after Second (sort %d)", t1.SortOrder, t2.SortOrder)
	}
}

func TestRunMove_NotSiblings(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	cid := mustAdd(t, db, pid, "Child")
	other := mustAdd(t, db, "", "Other root")

	err := runMove(db, cid, "before", other)
	if err == nil {
		t.Fatal("expected error when moving non-siblings")
	}
}

func TestRunMove_RecordsEvent(t *testing.T) {
	db := setupTestDB(t)
	id1 := mustAdd(t, db, "", "First")
	id2 := mustAdd(t, db, "", "Second")
	task2 := mustGet(t, db, id2)

	if err := runMove(db, id2, "before", id1); err != nil {
		t.Fatalf("runMove: %v", err)
	}

	detail, err := getLatestEventDetail(db, task2.ID, "moved")
	if err != nil {
		t.Fatalf("getLatestEventDetail: %v", err)
	}
	if detail == nil {
		t.Fatal("expected moved event")
	}
	if detail["direction"] != "before" {
		t.Errorf("direction: got %v, want %q", detail["direction"], "before")
	}
	if detail["relative_to"] != id1 {
		t.Errorf("relative_to: got %v, want %s", detail["relative_to"], id1)
	}
}

func TestRunMove_NotFound(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")
	err := runMove(db, id, "before", "noExs")
	if err == nil {
		t.Fatal("expected error for non-existent target")
	}
}

// --- Block ---

func TestRunBlock_CreatesRelationship(t *testing.T) {
	db := setupTestDB(t)
	blocker := mustAdd(t, db, "", "Blocker")
	blocked := mustAdd(t, db, "", "Blocked")

	if err := runBlock(db, blocked, blocker); err != nil {
		t.Fatalf("runBlock: %v", err)
	}

	blocks, err := getBlockers(db, blocked)
	if err != nil {
		t.Fatalf("getBlockers: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 blocker, got %d", len(blocks))
	}
	if blocks[0].ShortID != blocker {
		t.Errorf("blocker: got %s, want %s", blocks[0].ShortID, blocker)
	}
}

func TestRunBlock_RecordsEvent(t *testing.T) {
	db := setupTestDB(t)
	blocker := mustAdd(t, db, "", "Blocker")
	blocked := mustAdd(t, db, "", "Blocked")

	if err := runBlock(db, blocked, blocker); err != nil {
		t.Fatalf("runBlock: %v", err)
	}

	blockedTask := mustGet(t, db, blocked)
	detail, err := getLatestEventDetail(db, blockedTask.ID, "blocked")
	if err != nil {
		t.Fatalf("getLatestEventDetail: %v", err)
	}
	if detail == nil {
		t.Fatal("expected blocked event")
	}
}

func TestRunBlock_CircularDependency(t *testing.T) {
	db := setupTestDB(t)
	a := mustAdd(t, db, "", "A")
	b := mustAdd(t, db, "", "B")

	if err := runBlock(db, b, a); err != nil {
		t.Fatalf("runBlock: %v", err)
	}
	err := runBlock(db, a, b)
	if err == nil {
		t.Fatal("expected error for circular dependency")
	}
}

func TestRunBlock_NotFound(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")
	err := runBlock(db, id, "noExs")
	if err == nil {
		t.Fatal("expected error for non-existent blocker")
	}
}

// --- Unblock ---

func TestRunUnblock_RemovesRelationship(t *testing.T) {
	db := setupTestDB(t)
	blocker := mustAdd(t, db, "", "Blocker")
	blocked := mustAdd(t, db, "", "Blocked")

	if err := runBlock(db, blocked, blocker); err != nil {
		t.Fatalf("runBlock: %v", err)
	}
	if err := runUnblock(db, blocked, blocker); err != nil {
		t.Fatalf("runUnblock: %v", err)
	}

	blocks, err := getBlockers(db, blocked)
	if err != nil {
		t.Fatalf("getBlockers: %v", err)
	}
	if len(blocks) != 0 {
		t.Errorf("expected 0 blockers after unblock, got %d", len(blocks))
	}
}

func TestRunUnblock_RecordsEvent(t *testing.T) {
	db := setupTestDB(t)
	blocker := mustAdd(t, db, "", "Blocker")
	blocked := mustAdd(t, db, "", "Blocked")

	if err := runBlock(db, blocked, blocker); err != nil {
		t.Fatalf("runBlock: %v", err)
	}
	if err := runUnblock(db, blocked, blocker); err != nil {
		t.Fatalf("runUnblock: %v", err)
	}

	blockedTask := mustGet(t, db, blocked)
	detail, err := getLatestEventDetail(db, blockedTask.ID, "unblocked")
	if err != nil {
		t.Fatalf("getLatestEventDetail: %v", err)
	}
	if detail == nil {
		t.Fatal("expected unblocked event")
	}
}

func TestRunUnblock_NotBlocked(t *testing.T) {
	db := setupTestDB(t)
	a := mustAdd(t, db, "", "A")
	b := mustAdd(t, db, "", "B")
	err := runUnblock(db, a, b)
	if err == nil {
		t.Fatal("expected error when unblocking non-blocked task")
	}
}

// --- JSON format ---

func TestFormatJSON_ListOutput(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	mustAdd(t, db, pid, "Child")

	nodes, err := runList(db, "", true)
	if err != nil {
		t.Fatalf("runList: %v", err)
	}

	jsonBytes, err := formatTaskNodesJSON(nodes)
	if err != nil {
		t.Fatalf("formatTaskNodesJSON: %v", err)
	}

	var result []map[string]any
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 root, got %d", len(result))
	}
	if result[0]["title"] != "Parent" {
		t.Errorf("title: got %v, want %q", result[0]["title"], "Parent")
	}
	children, ok := result[0]["children"].([]any)
	if !ok {
		t.Fatalf("children: got %T", result[0]["children"])
	}
	if len(children) != 1 {
		t.Errorf("children: got %d, want 1", len(children))
	}
}

// --- Info ---

func TestRunInfo_RootTask(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "My task")

	info, err := runInfo(db, id)
	if err != nil {
		t.Fatalf("runInfo: %v", err)
	}
	if info.Task.ShortID != id {
		t.Errorf("short_id: got %s, want %s", info.Task.ShortID, id)
	}
	if info.Task.Title != "My task" {
		t.Errorf("title: got %s, want %q", info.Task.Title, "My task")
	}
	if info.Parent != nil {
		t.Errorf("parent: got %v, want nil", info.Parent)
	}
}

func TestRunInfo_ChildTask(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	cid := mustAdd(t, db, pid, "Child")

	info, err := runInfo(db, cid)
	if err != nil {
		t.Fatalf("runInfo: %v", err)
	}
	if info.Parent == nil {
		t.Fatal("parent: got nil, want non-nil")
	}
	if info.Parent.ShortID != pid {
		t.Errorf("parent short_id: got %s, want %s", info.Parent.ShortID, pid)
	}
}

func TestRunInfo_NotFound(t *testing.T) {
	db := setupTestDB(t)
	_, err := runInfo(db, "noExs")
	if err == nil {
		t.Fatal("expected error for non-existent task")
	}
}

// --- Duration parsing ---

func TestParseDuration_Seconds(t *testing.T) {
	got, err := parseDuration("45s")
	if err != nil {
		t.Fatalf("parseDuration: %v", err)
	}
	if got != 45 {
		t.Errorf("got %d, want 45", got)
	}
}

func TestParseDuration_Minutes(t *testing.T) {
	got, err := parseDuration("30m")
	if err != nil {
		t.Fatalf("parseDuration: %v", err)
	}
	if got != 1800 {
		t.Errorf("got %d, want 1800", got)
	}
}

func TestParseDuration_Hours(t *testing.T) {
	got, err := parseDuration("4h")
	if err != nil {
		t.Fatalf("parseDuration: %v", err)
	}
	if got != 14400 {
		t.Errorf("got %d, want 14400", got)
	}
}

func TestParseDuration_Days(t *testing.T) {
	got, err := parseDuration("2d")
	if err != nil {
		t.Fatalf("parseDuration: %v", err)
	}
	if got != 172800 {
		t.Errorf("got %d, want 172800", got)
	}
}

func TestParseDuration_Default(t *testing.T) {
	got, err := parseDuration("")
	if err != nil {
		t.Fatalf("parseDuration: %v", err)
	}
	if got != 3600 {
		t.Errorf("got %d, want 3600 (1h default)", got)
	}
}

func TestParseDuration_InvalidUnit(t *testing.T) {
	_, err := parseDuration("5x")
	if err == nil {
		t.Fatal("expected error for invalid unit")
	}
}

func TestParseDuration_NoNumber(t *testing.T) {
	_, err := parseDuration("h")
	if err == nil {
		t.Fatal("expected error for missing number")
	}
}

// --- Claim ---

func mustClaim(t *testing.T, db *sql.DB, shortID, duration, who string) {
	t.Helper()
	if err := runClaim(db, shortID, duration, who, false); err != nil {
		t.Fatalf("claim %s: %v", shortID, err)
	}
}

func TestRunClaim_Basic(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	if err := runClaim(db, id, "", "", false); err != nil {
		t.Fatalf("runClaim: %v", err)
	}

	task := mustGet(t, db, id)
	if task.Status != "claimed" {
		t.Errorf("status: got %q, want %q", task.Status, "claimed")
	}
	if task.ClaimedBy != nil {
		t.Errorf("claimed_by: got %q, want nil", *task.ClaimedBy)
	}
	if task.ClaimExpiresAt == nil {
		t.Fatal("claim_expires_at: got nil, want non-nil")
	}
}

func TestRunClaim_WithDuration(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	if err := runClaim(db, id, "4h", "", false); err != nil {
		t.Fatalf("runClaim: %v", err)
	}

	task := mustGet(t, db, id)
	if task.Status != "claimed" {
		t.Errorf("status: got %q, want %q", task.Status, "claimed")
	}
}

func TestRunClaim_WithWho(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	if err := runClaim(db, id, "", "Jesse", false); err != nil {
		t.Fatalf("runClaim: %v", err)
	}

	task := mustGet(t, db, id)
	if task.ClaimedBy == nil || *task.ClaimedBy != "Jesse" {
		t.Errorf("claimed_by: got %v, want %q", task.ClaimedBy, "Jesse")
	}
}

func TestRunClaim_WithDurationAndWho(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	if err := runClaim(db, id, "4h", "Jesse", false); err != nil {
		t.Fatalf("runClaim: %v", err)
	}

	task := mustGet(t, db, id)
	if task.ClaimedBy == nil || *task.ClaimedBy != "Jesse" {
		t.Errorf("claimed_by: got %v, want %q", task.ClaimedBy, "Jesse")
	}
}

func TestRunClaim_AlreadyClaimed(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")
	mustClaim(t, db, id, "", "Jesse")

	err := runClaim(db, id, "", "", false)
	if err == nil {
		t.Fatal("expected error when claiming already-claimed task")
	}
	if !strings.Contains(err.Error(), "already claimed") {
		t.Errorf("error should mention already claimed: %v", err)
	}
}

func TestRunClaim_ForceOverride(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")
	mustClaim(t, db, id, "", "Jesse")

	if err := runClaim(db, id, "1h", "Agent-1", true); err != nil {
		t.Fatalf("runClaim --force: %v", err)
	}

	task := mustGet(t, db, id)
	if task.ClaimedBy == nil || *task.ClaimedBy != "Agent-1" {
		t.Errorf("claimed_by: got %v, want %q", task.ClaimedBy, "Agent-1")
	}
}

func TestRunClaim_DoneTask(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")
	mustDone(t, db, id)

	err := runClaim(db, id, "", "", false)
	if err == nil {
		t.Fatal("expected error when claiming done task")
	}
}

func TestRunClaim_NotFound(t *testing.T) {
	db := setupTestDB(t)
	err := runClaim(db, "noExs", "", "", false)
	if err == nil {
		t.Fatal("expected error for non-existent task")
	}
}

func TestRunClaim_RecordsEvent(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	if err := runClaim(db, id, "4h", "Jesse", false); err != nil {
		t.Fatalf("runClaim: %v", err)
	}

	task := mustGet(t, db, id)
	detail, err := getLatestEventDetail(db, task.ID, "claimed")
	if err != nil {
		t.Fatalf("getLatestEventDetail: %v", err)
	}
	if detail == nil {
		t.Fatal("expected claimed event")
	}
	if detail["by"] != "Jesse" {
		t.Errorf("by: got %v, want %q", detail["by"], "Jesse")
	}
	if detail["duration"] != "4h" {
		t.Errorf("duration: got %v, want %q", detail["duration"], "4h")
	}
	if detail["expires_at"] == nil {
		t.Error("expires_at should not be nil")
	}
}

func TestRunClaim_ExpiredClaimCanBeReclaimed(t *testing.T) {
	origNow := currentNowFunc
	defer func() { currentNowFunc = origNow }()

	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	baseTime := time.Now()
	currentNowFunc = func() time.Time { return baseTime }
	mustClaim(t, db, id, "1h", "Jesse")

	currentNowFunc = func() time.Time { return baseTime.Add(2 * time.Hour) }

	if err := runClaim(db, id, "1h", "Agent-1", false); err != nil {
		t.Fatalf("runClaim after expiry: %v", err)
	}

	task := mustGet(t, db, id)
	if task.ClaimedBy == nil || *task.ClaimedBy != "Agent-1" {
		t.Errorf("claimed_by: got %v, want %q", task.ClaimedBy, "Agent-1")
	}
}

// --- Release ---

func TestRunRelease_ClaimedTask(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")
	mustClaim(t, db, id, "", "Jesse")

	if err := runRelease(db, id); err != nil {
		t.Fatalf("runRelease: %v", err)
	}

	task := mustGet(t, db, id)
	if task.Status != "available" {
		t.Errorf("status: got %q, want %q", task.Status, "available")
	}
	if task.ClaimedBy != nil {
		t.Errorf("claimed_by: got %q, want nil", *task.ClaimedBy)
	}
	if task.ClaimExpiresAt != nil {
		t.Errorf("claim_expires_at: got %d, want nil", *task.ClaimExpiresAt)
	}
}

func TestRunRelease_RecordsEvent(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")
	mustClaim(t, db, id, "", "Jesse")

	if err := runRelease(db, id); err != nil {
		t.Fatalf("runRelease: %v", err)
	}

	task := mustGet(t, db, id)
	detail, err := getLatestEventDetail(db, task.ID, "released")
	if err != nil {
		t.Fatalf("getLatestEventDetail: %v", err)
	}
	if detail == nil {
		t.Fatal("expected released event")
	}
	if detail["was_claimed_by"] != "Jesse" {
		t.Errorf("was_claimed_by: got %v, want %q", detail["was_claimed_by"], "Jesse")
	}
}

func TestRunRelease_NotClaimed(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	err := runRelease(db, id)
	if err == nil {
		t.Fatal("expected error when releasing unclaimed task")
	}
}

func TestRunRelease_NotFound(t *testing.T) {
	db := setupTestDB(t)
	err := runRelease(db, "noExs")
	if err == nil {
		t.Fatal("expected error for non-existent task")
	}
}

// --- Claim expiry ---

func TestExpireStaleClaims_ExpiredClaimReset(t *testing.T) {
	origNow := currentNowFunc
	defer func() { currentNowFunc = origNow }()

	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	baseTime := time.Now()
	currentNowFunc = func() time.Time { return baseTime }
	mustClaim(t, db, id, "1h", "Jesse")

	currentNowFunc = func() time.Time { return baseTime.Add(2 * time.Hour) }
	if err := expireStaleClaims(db); err != nil {
		t.Fatalf("expireStaleClaims: %v", err)
	}

	task := mustGet(t, db, id)
	if task.Status != "available" {
		t.Errorf("status after expiry: got %q, want %q", task.Status, "available")
	}
	if task.ClaimedBy != nil {
		t.Errorf("claimed_by after expiry: got %q, want nil", *task.ClaimedBy)
	}
}

func TestExpireStaleClaims_RecordsEvent(t *testing.T) {
	origNow := currentNowFunc
	defer func() { currentNowFunc = origNow }()

	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	baseTime := time.Now()
	currentNowFunc = func() time.Time { return baseTime }
	mustClaim(t, db, id, "1h", "Jesse")

	currentNowFunc = func() time.Time { return baseTime.Add(2 * time.Hour) }
	if err := expireStaleClaims(db); err != nil {
		t.Fatalf("expireStaleClaims: %v", err)
	}

	task := mustGet(t, db, id)
	detail, err := getLatestEventDetail(db, task.ID, "claim_expired")
	if err != nil {
		t.Fatalf("getLatestEventDetail: %v", err)
	}
	if detail == nil {
		t.Fatal("expected claim_expired event")
	}
	if detail["was_claimed_by"] != "Jesse" {
		t.Errorf("was_claimed_by: got %v, want %q", detail["was_claimed_by"], "Jesse")
	}
}

func TestExpireStaleClaims_ActiveClaimNotExpired(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")
	mustClaim(t, db, id, "4h", "Jesse")

	if err := expireStaleClaims(db); err != nil {
		t.Fatalf("expireStaleClaims: %v", err)
	}

	task := mustGet(t, db, id)
	if task.Status != "claimed" {
		t.Errorf("status: got %q, want %q", task.Status, "claimed")
	}
}

// --- Next ---

func TestRunNext_WithParent(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	cid1 := mustAdd(t, db, pid, "First child")
	mustAdd(t, db, pid, "Second child")

	task, err := runNext(db, pid)
	if err != nil {
		t.Fatalf("runNext: %v", err)
	}
	if task.ShortID != cid1 {
		t.Errorf("got %s, want %s (lowest sort_order)", task.ShortID, cid1)
	}
}

func TestRunNext_NoParent(t *testing.T) {
	db := setupTestDB(t)
	id1 := mustAdd(t, db, "", "First root")
	mustAdd(t, db, "", "Second root")

	task, err := runNext(db, "")
	if err != nil {
		t.Fatalf("runNext: %v", err)
	}
	if task.ShortID != id1 {
		t.Errorf("got %s, want %s", task.ShortID, id1)
	}
}

func TestRunNext_NoAvailable(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	cid := mustAdd(t, db, pid, "Child")
	mustDone(t, db, cid)

	_, err := runNext(db, pid)
	if err == nil {
		t.Fatal("expected error when no tasks available")
	}
}

func TestRunNext_SkipsBlocked(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	cid1 := mustAdd(t, db, pid, "Blocked child")
	cid2 := mustAdd(t, db, pid, "Available child")
	blocker := mustAdd(t, db, "", "Blocker")
	if err := runBlock(db, cid1, blocker); err != nil {
		t.Fatalf("runBlock: %v", err)
	}

	task, err := runNext(db, pid)
	if err != nil {
		t.Fatalf("runNext: %v", err)
	}
	if task.ShortID != cid2 {
		t.Errorf("got %s, want %s (should skip blocked)", task.ShortID, cid2)
	}
}

func TestRunNext_SkipsClaimed(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	cid1 := mustAdd(t, db, pid, "Claimed child")
	cid2 := mustAdd(t, db, pid, "Available child")
	mustClaim(t, db, cid1, "", "Jesse")

	task, err := runNext(db, pid)
	if err != nil {
		t.Fatalf("runNext: %v", err)
	}
	if task.ShortID != cid2 {
		t.Errorf("got %s, want %s (should skip claimed)", task.ShortID, cid2)
	}
}

func TestRunNext_SkipsDone(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	cid1 := mustAdd(t, db, pid, "Done child")
	cid2 := mustAdd(t, db, pid, "Available child")
	mustDone(t, db, cid1)

	task, err := runNext(db, pid)
	if err != nil {
		t.Fatalf("runNext: %v", err)
	}
	if task.ShortID != cid2 {
		t.Errorf("got %s, want %s (should skip done)", task.ShortID, cid2)
	}
}

func TestRunNext_ParentNotFound(t *testing.T) {
	db := setupTestDB(t)
	_, err := runNext(db, "noExs")
	if err == nil {
		t.Fatal("expected error for non-existent parent")
	}
}

func TestRunNext_NoAvailableAtRoot(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Done root")
	mustDone(t, db, id)

	_, err := runNext(db, "")
	if err == nil {
		t.Fatal("expected error when no root tasks available")
	}
}

// --- ClaimNext ---

func TestRunClaimNext_WithParent(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	cid := mustAdd(t, db, pid, "Available child")

	task, err := runClaimNext(db, pid, "", "", false)
	if err != nil {
		t.Fatalf("runClaimNext: %v", err)
	}
	if task.ShortID != cid {
		t.Errorf("got %s, want %s", task.ShortID, cid)
	}

	updated := mustGet(t, db, cid)
	if updated.Status != "claimed" {
		t.Errorf("status: got %q, want %q", updated.Status, "claimed")
	}
}

func TestRunClaimNext_NoParent(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Root task")

	task, err := runClaimNext(db, "", "", "", false)
	if err != nil {
		t.Fatalf("runClaimNext: %v", err)
	}
	if task.ShortID != id {
		t.Errorf("got %s, want %s", task.ShortID, id)
	}
}

func TestRunClaimNext_WithDurationAndWho(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	cid := mustAdd(t, db, pid, "Available child")

	task, err := runClaimNext(db, pid, "4h", "Jesse", false)
	if err != nil {
		t.Fatalf("runClaimNext: %v", err)
	}
	if task.ShortID != cid {
		t.Errorf("got %s, want %s", task.ShortID, cid)
	}

	updated := mustGet(t, db, cid)
	if updated.ClaimedBy == nil || *updated.ClaimedBy != "Jesse" {
		t.Errorf("claimed_by: got %v, want %q", updated.ClaimedBy, "Jesse")
	}
}

func TestRunClaimNext_NoAvailable(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	cid := mustAdd(t, db, pid, "Done child")
	mustDone(t, db, cid)

	_, err := runClaimNext(db, pid, "", "", false)
	if err == nil {
		t.Fatal("expected error when no tasks available")
	}
}

func TestRunClaimNext_SkipsBlocked(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	cid1 := mustAdd(t, db, pid, "Blocked child")
	cid2 := mustAdd(t, db, pid, "Available child")
	blocker := mustAdd(t, db, "", "Blocker")
	if err := runBlock(db, cid1, blocker); err != nil {
		t.Fatalf("runBlock: %v", err)
	}

	task, err := runClaimNext(db, pid, "", "", false)
	if err != nil {
		t.Fatalf("runClaimNext: %v", err)
	}
	if task.ShortID != cid2 {
		t.Errorf("got %s, want %s (should skip blocked)", task.ShortID, cid2)
	}
}

func TestRunClaimNext_RecordsClaimedEvent(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	cid := mustAdd(t, db, pid, "Available child")

	if _, err := runClaimNext(db, pid, "2h", "Jesse", false); err != nil {
		t.Fatalf("runClaimNext: %v", err)
	}

	task := mustGet(t, db, cid)
	detail, err := getLatestEventDetail(db, task.ID, "claimed")
	if err != nil {
		t.Fatalf("getLatestEventDetail: %v", err)
	}
	if detail == nil {
		t.Fatal("expected claimed event")
	}
	if detail["by"] != "Jesse" {
		t.Errorf("by: got %v, want %q", detail["by"], "Jesse")
	}
}

func TestRunClaimNext_ParentNotFound(t *testing.T) {
	db := setupTestDB(t)
	_, err := runClaimNext(db, "noExs", "", "", false)
	if err == nil {
		t.Fatal("expected error for non-existent parent")
	}
}

// --- Auto-unblock on done ---

func TestRunDone_AutoUnblocks(t *testing.T) {
	db := setupTestDB(t)
	blocker := mustAdd(t, db, "", "Blocker")
	blocked := mustAdd(t, db, "", "Blocked")
	if err := runBlock(db, blocked, blocker); err != nil {
		t.Fatalf("runBlock: %v", err)
	}

	mustDone(t, db, blocker)

	blockers, err := getBlockers(db, blocked)
	if err != nil {
		t.Fatalf("getBlockers: %v", err)
	}
	if len(blockers) != 0 {
		t.Errorf("expected no blockers after blocker done, got %d", len(blockers))
	}
}

func TestRunDone_RecordsUnblockedEvent(t *testing.T) {
	db := setupTestDB(t)
	blocker := mustAdd(t, db, "", "Blocker")
	blocked := mustAdd(t, db, "", "Blocked")
	if err := runBlock(db, blocked, blocker); err != nil {
		t.Fatalf("runBlock: %v", err)
	}

	mustDone(t, db, blocker)

	blockedTask := mustGet(t, db, blocked)
	detail, err := getLatestEventDetail(db, blockedTask.ID, "unblocked")
	if err != nil {
		t.Fatalf("getLatestEventDetail: %v", err)
	}
	if detail == nil {
		t.Fatal("expected unblocked event")
	}
	if detail["reason"] != "blocker_done" {
		t.Errorf("reason: got %v, want %q", detail["reason"], "blocker_done")
	}
}

// --- List excludes blocked/claimed ---

func TestRunList_ExcludesBlockedTasks(t *testing.T) {
	db := setupTestDB(t)
	blocker := mustAdd(t, db, "", "Blocker")
	blocked := mustAdd(t, db, "", "Blocked")
	if err := runBlock(db, blocked, blocker); err != nil {
		t.Fatalf("runBlock: %v", err)
	}

	nodes, err := runList(db, "", false)
	if err != nil {
		t.Fatalf("runList: %v", err)
	}

	for _, node := range nodes {
		if node.Task.ShortID == blocked {
			t.Error("blocked task should not appear in default list")
		}
	}
}

func TestRunList_AllIncludesBlocked(t *testing.T) {
	db := setupTestDB(t)
	blocker := mustAdd(t, db, "", "Blocker")
	blocked := mustAdd(t, db, "", "Blocked")
	if err := runBlock(db, blocked, blocker); err != nil {
		t.Fatalf("runBlock: %v", err)
	}

	nodes, err := runList(db, "", true)
	if err != nil {
		t.Fatalf("runList: %v", err)
	}

	found := false
	for _, node := range nodes {
		if node.Task.ShortID == blocked {
			found = true
		}
	}
	if !found {
		t.Error("blocked task should appear in 'list all'")
	}
}

func TestRunList_ExcludesClaimedTasks(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Claimed task")
	mustClaim(t, db, id, "4h", "Jesse")

	nodes, err := runList(db, "", false)
	if err != nil {
		t.Fatalf("runList: %v", err)
	}

	for _, node := range nodes {
		if node.Task.ShortID == id {
			t.Error("claimed task should not appear in default list")
		}
	}
}

func TestRunList_ExpiredClaimShowsAsAvailable(t *testing.T) {
	origNow := currentNowFunc
	defer func() { currentNowFunc = origNow }()

	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	baseTime := time.Now()
	currentNowFunc = func() time.Time { return baseTime }
	mustClaim(t, db, id, "1h", "Jesse")

	currentNowFunc = func() time.Time { return baseTime.Add(2 * time.Hour) }

	nodes, err := runList(db, "", false)
	if err != nil {
		t.Fatalf("runList: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node after expiry, got %d", len(nodes))
	}
	if nodes[0].Task.ShortID != id {
		t.Errorf("got %s, want %s", nodes[0].Task.ShortID, id)
	}
	if nodes[0].Task.Status != "available" {
		t.Errorf("status: got %q, want %q", nodes[0].Task.Status, "available")
	}
}
