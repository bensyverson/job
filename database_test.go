package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestResolveDBPath_FlagWins(t *testing.T) {
	t.Setenv("JOBS_DB", "/should/be/ignored")
	got := resolveDBPath("/tmp/x.db")
	if got != "/tmp/x.db" {
		t.Errorf("resolveDBPath(%q) = %q, want %q", "/tmp/x.db", got, "/tmp/x.db")
	}
}

func TestResolveDBPath_EnvWins(t *testing.T) {
	t.Setenv("JOBS_DB", "/tmp/env-chosen.db")
	got := resolveDBPath("")
	if got != "/tmp/env-chosen.db" {
		t.Errorf("resolveDBPath(\"\") = %q, want %q", got, "/tmp/env-chosen.db")
	}
}

func TestResolveDBPath_WalksUp(t *testing.T) {
	t.Setenv("JOBS_DB", "")
	root := t.TempDir()
	a := filepath.Join(root, "a")
	c := filepath.Join(a, "b", "c")
	if err := os.MkdirAll(c, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	dbFile := filepath.Join(a, ".jobs.db")
	if err := os.WriteFile(dbFile, []byte(""), 0o644); err != nil {
		t.Fatalf("write db: %v", err)
	}
	t.Chdir(c)

	got := resolveDBPath("")
	// Resolve symlinks for compare; macOS tempdirs often involve /var -> /private/var.
	wantAbs, _ := filepath.EvalSymlinks(dbFile)
	gotAbs, _ := filepath.EvalSymlinks(got)
	if gotAbs != wantAbs {
		t.Errorf("resolveDBPath = %q, want %q", got, dbFile)
	}
}

func TestResolveDBPath_NoAncestor_FallsBackToCwd(t *testing.T) {
	t.Setenv("JOBS_DB", "")
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Chdir(sub)

	got := resolveDBPath("")
	if got != defaultDBName {
		t.Errorf("resolveDBPath = %q, want literal %q", got, defaultDBName)
	}
}

const testActor = "TestActor"

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
	res, err := runAdd(db, parentShortID, title, "", "", testActor)
	if err != nil {
		t.Fatalf("add task %q: %v", title, err)
	}
	return res.ShortID
}

func mustAddDesc(t *testing.T, db *sql.DB, parentShortID, title, desc string) string {
	t.Helper()
	res, err := runAdd(db, parentShortID, title, desc, "", testActor)
	if err != nil {
		t.Fatalf("add task %q: %v", title, err)
	}
	return res.ShortID
}

func mustDone(t *testing.T, db *sql.DB, shortID string) {
	t.Helper()
	if _, _, err := runDone(db, []string{shortID}, false, "", nil, testActor); err != nil {
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
	res, err := runAdd(db, "", "Root task", "", "", testActor)
	if err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	id := res.ShortID
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
	cres, err := runAdd(db, pid, "Child", "", "", testActor)
	if err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	cid := cres.ShortID

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
	id3res, err := runAdd(db, "", "Middle", "", id2, testActor)
	if err != nil {
		t.Fatalf("runAdd before: %v", err)
	}
	id3 := id3res.ShortID

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
	_, err := runAdd(db, "noExs", "Task", "", "", testActor)
	if err == nil {
		t.Fatal("expected error for non-existent parent")
	}
}

func TestRunAdd_BeforeNotFound(t *testing.T) {
	db := setupTestDB(t)
	_, err := runAdd(db, "", "Task", "", "noExs", testActor)
	if err == nil {
		t.Fatal("expected error for non-existent before target")
	}
}

func TestRunAdd_BeforeNotSibling(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	otherID := mustAdd(t, db, "", "Other root")
	_, err := runAdd(db, pid, "Child", "", otherID, testActor)
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
	nodes, err := runList(db, "", testActor, false)
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

	nodes, err := runList(db, "", testActor, false)
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

	nodes, err := runList(db, pid, testActor, false)
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

	nodes, err := runList(db, "", testActor, false)
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

	nodes, err := runList(db, "", testActor, false)
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

	nodes, err := runList(db, "", testActor, true)
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

	nodes, err := runList(db, "", testActor, false)
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
	_, err := runList(db, "noExs", testActor, false)
	if err == nil {
		t.Fatal("expected error for non-existent parent")
	}
}

// --- Done ---

func TestRunDone_LeafTask(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	closed, _, err := runDone(db, []string{id}, false, "", nil, testActor)
	if err != nil {
		t.Fatalf("runDone: %v", err)
	}
	if len(closed) != 1 || len(closed[0].CascadeClosed) != 0 {
		t.Errorf("closed: got %+v, want 1 target with no cascade", closed)
	}

	task := mustGet(t, db, id)
	if task.Status != "done" {
		t.Errorf("status: got %q, want %q", task.Status, "done")
	}
}

func TestRunDone_WithNote(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	if _, _, err := runDone(db, []string{id}, false, "abc1234", nil, testActor); err != nil {
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

	_, _, err := runDone(db, []string{pid}, false, "", nil, testActor)
	if err == nil {
		t.Fatal("expected error for incomplete children")
	}
	if !strings.Contains(err.Error(), "Incomplete child") || !strings.Contains(err.Error(), "incomplete") {
		t.Errorf("error should mention incomplete children: %v", err)
	}
	if !strings.Contains(err.Error(), "--cascade") {
		t.Errorf("error should suggest --cascade: %v", err)
	}
}

func TestRunDone_CascadeClosesChildren(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	cid := mustAdd(t, db, pid, "Child")

	closed, _, err := runDone(db, []string{pid}, true, "", nil, testActor)
	if err != nil {
		t.Fatalf("runDone --cascade: %v", err)
	}
	if len(closed) != 1 || len(closed[0].CascadeClosed) != 1 || closed[0].CascadeClosed[0] != cid {
		t.Errorf("closed: got %+v, want 1 target cascading [%s]", closed, cid)
	}

	child := mustGet(t, db, cid)
	if child.Status != "done" {
		t.Errorf("child status: got %q, want %q", child.Status, "done")
	}
}

func TestRunDone_CascadeNested(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	cid := mustAdd(t, db, pid, "Child")
	gcid := mustAdd(t, db, cid, "Grandchild")

	closed, _, err := runDone(db, []string{pid}, true, "", nil, testActor)
	if err != nil {
		t.Fatalf("runDone --cascade: %v", err)
	}
	if len(closed) != 1 || len(closed[0].CascadeClosed) != 2 {
		t.Errorf("closed cascade count: got %+v, want 2", closed)
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

	closed, alreadyDone, err := runDone(db, []string{id}, false, "", nil, testActor)
	if err != nil {
		t.Fatalf("runDone on already-done should be idempotent: %v", err)
	}
	if len(alreadyDone) != 1 || alreadyDone[0] != id {
		t.Errorf("alreadyDone: got %v, want [%s]", alreadyDone, id)
	}
	if len(closed) != 0 {
		t.Errorf("closed: got %+v, want 0", closed)
	}
}

func TestRunDone_BlockedTaskSucceeds(t *testing.T) {
	db := setupTestDB(t)
	blocker := mustAdd(t, db, "", "Blocker")
	blocked := mustAdd(t, db, "", "Blocked")
	if err := runBlock(db, blocked, blocker, testActor); err != nil {
		t.Fatalf("runBlock: %v", err)
	}

	closed, _, err := runDone(db, []string{blocked}, false, "", nil, testActor)
	if err != nil {
		t.Fatalf("done on blocked task should succeed: %v", err)
	}
	if len(closed) != 1 {
		t.Errorf("closed: got %+v, want 1", closed)
	}
}

func TestRunDone_NotFound(t *testing.T) {
	db := setupTestDB(t)
	_, _, err := runDone(db, []string{"noExs"}, false, "", nil, testActor)
	if err == nil {
		t.Fatal("expected error for non-existent task")
	}
}

func TestRunDone_DoneEvent(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")
	if _, _, err := runDone(db, []string{id}, false, "abc1234", nil, testActor); err != nil {
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
	if detail["cascade"] != false {
		t.Errorf("cascade: got %v, want false", detail["cascade"])
	}
}

func TestRunDone_CascadeEventRecordsChildren(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	cid := mustAdd(t, db, pid, "Child")

	if _, _, err := runDone(db, []string{pid}, true, "", nil, testActor); err != nil {
		t.Fatalf("runDone: %v", err)
	}

	parent := mustGet(t, db, pid)
	detail, err := getLatestEventDetail(db, parent.ID, "done")
	if err != nil {
		t.Fatalf("getLatestEventDetail: %v", err)
	}
	children, ok := detail["cascade_closed"].([]any)
	if !ok {
		t.Fatalf("cascade_closed: got %T, want []any", detail["cascade_closed"])
	}
	if len(children) != 1 {
		t.Fatalf("cascade_closed: got %d, want 1", len(children))
	}
	if children[0] != cid {
		t.Errorf("cascade_closed[0]: got %v, want %s", children[0], cid)
	}
}

// --- Reopen ---

func TestRunReopen_DoneTask(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")
	mustDone(t, db, id)

	reopened, err := runReopen(db, id, false, testActor)
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

func TestRunReopen_CascadeReopensChildren(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	cid := mustAdd(t, db, pid, "Child")

	if _, _, err := runDone(db, []string{pid}, true, "", nil, testActor); err != nil {
		t.Fatalf("runDone --cascade: %v", err)
	}

	reopened, err := runReopen(db, pid, true, testActor)
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

	_, err := runReopen(db, id, false, testActor)
	if err == nil {
		t.Fatal("expected error when reopening available task")
	}
}

func TestRunReopen_NotFound(t *testing.T) {
	db := setupTestDB(t)
	_, err := runReopen(db, "noExs", false, testActor)
	if err == nil {
		t.Fatal("expected error for non-existent task")
	}
}

func TestRunReopen_ReopenedEvent(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")
	mustDone(t, db, id)

	if _, err := runReopen(db, id, false, testActor); err != nil {
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

func TestRunReopen_CascadeNested(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	cid := mustAdd(t, db, pid, "Child")
	gcid := mustAdd(t, db, cid, "Grandchild")

	if _, _, err := runDone(db, []string{pid}, true, "", nil, testActor); err != nil {
		t.Fatalf("runDone --cascade: %v", err)
	}

	reopened, err := runReopen(db, pid, true, testActor)
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

	nt := "New title"
	if err := runEdit(db, id, &nt, nil, testActor); err != nil {
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

	nt := "New title"
	if err := runEdit(db, id, &nt, nil, testActor); err != nil {
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
	nt := "New title"
	err := runEdit(db, "noExs", &nt, nil, testActor)
	if err == nil {
		t.Fatal("expected error for non-existent task")
	}
}

// --- Note ---

func TestRunNote_AppendsToEmptyDescription(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	if err := runNote(db, id, "First note", nil, testActor); err != nil {
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

	if err := runNote(db, id, "Added note", nil, testActor); err != nil {
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

	if err := runNote(db, id, "A note", nil, testActor); err != nil {
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
	err := runNote(db, "noExs", "A note", nil, testActor)
	if err == nil {
		t.Fatal("expected error for non-existent task")
	}
}

// --- Move ---

func TestRunMove_Before(t *testing.T) {
	db := setupTestDB(t)
	id1 := mustAdd(t, db, "", "First")
	id2 := mustAdd(t, db, "", "Second")

	if err := runMove(db, id2, "before", id1, testActor); err != nil {
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

	if err := runMove(db, id1, "after", id2, testActor); err != nil {
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

	err := runMove(db, cid, "before", other, testActor)
	if err == nil {
		t.Fatal("expected error when moving non-siblings")
	}
}

func TestRunMove_RecordsEvent(t *testing.T) {
	db := setupTestDB(t)
	id1 := mustAdd(t, db, "", "First")
	id2 := mustAdd(t, db, "", "Second")
	task2 := mustGet(t, db, id2)

	if err := runMove(db, id2, "before", id1, testActor); err != nil {
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
	err := runMove(db, id, "before", "noExs", testActor)
	if err == nil {
		t.Fatal("expected error for non-existent target")
	}
}

// --- Block ---

func TestRunBlock_CreatesRelationship(t *testing.T) {
	db := setupTestDB(t)
	blocker := mustAdd(t, db, "", "Blocker")
	blocked := mustAdd(t, db, "", "Blocked")

	if err := runBlock(db, blocked, blocker, testActor); err != nil {
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

	if err := runBlock(db, blocked, blocker, testActor); err != nil {
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

	if err := runBlock(db, b, a, testActor); err != nil {
		t.Fatalf("runBlock: %v", err)
	}
	err := runBlock(db, a, b, testActor)
	if err == nil {
		t.Fatal("expected error for circular dependency")
	}
}

func TestRunBlock_NotFound(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")
	err := runBlock(db, id, "noExs", testActor)
	if err == nil {
		t.Fatal("expected error for non-existent blocker")
	}
}

// --- Unblock ---

func TestRunUnblock_RemovesRelationship(t *testing.T) {
	db := setupTestDB(t)
	blocker := mustAdd(t, db, "", "Blocker")
	blocked := mustAdd(t, db, "", "Blocked")

	if err := runBlock(db, blocked, blocker, testActor); err != nil {
		t.Fatalf("runBlock: %v", err)
	}
	if err := runUnblock(db, blocked, blocker, testActor); err != nil {
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

	if err := runBlock(db, blocked, blocker, testActor); err != nil {
		t.Fatalf("runBlock: %v", err)
	}
	if err := runUnblock(db, blocked, blocker, testActor); err != nil {
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
	err := runUnblock(db, a, b, testActor)
	if err == nil {
		t.Fatal("expected error when unblocking non-blocked task")
	}
}

// --- JSON format ---

func TestFormatJSON_ListOutput(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	mustAdd(t, db, pid, "Child")

	nodes, err := runList(db, "", testActor, true)
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
	if got != defaultClaimTTLSeconds {
		t.Errorf("got %d, want %d (15m default)", got, defaultClaimTTLSeconds)
	}
}

func TestParseDuration_DefaultIs15m(t *testing.T) {
	got, err := parseDuration("")
	if err != nil {
		t.Fatalf("parseDuration: %v", err)
	}
	if got != 900 {
		t.Errorf("default TTL: got %d, want 900", got)
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

func mustClaim(t *testing.T, db *sql.DB, shortID, duration string) {
	t.Helper()
	if err := runClaim(db, shortID, duration, testActor, false); err != nil {
		t.Fatalf("claim %s: %v", shortID, err)
	}
}

func mustRelease(t *testing.T, db *sql.DB, shortID string) {
	t.Helper()
	if err := runRelease(db, shortID, testActor); err != nil {
		t.Fatalf("release %s: %v", shortID, err)
	}
}

func TestRunClaim_Basic(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	if err := runClaim(db, id, "", testActor, false); err != nil {
		t.Fatalf("runClaim: %v", err)
	}

	task := mustGet(t, db, id)
	if task.Status != "claimed" {
		t.Errorf("status: got %q, want %q", task.Status, "claimed")
	}
	if task.ClaimedBy == nil || *task.ClaimedBy != testActor {
		t.Errorf("claimed_by: got %v, want %q", task.ClaimedBy, testActor)
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
	mustClaim(t, db, id, "")

	err := runClaim(db, id, "", "", false)
	if err == nil {
		t.Fatal("expected error when claiming already-claimed task")
	}
	if !strings.Contains(err.Error(), "claimed by") {
		t.Errorf("error should mention who holds the claim: %v", err)
	}
}

func TestRunClaim_ForceOverride(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")
	mustClaim(t, db, id, "")

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
	mustClaim(t, db, id, "1h")

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
	mustClaim(t, db, id, "")

	if err := runRelease(db, id, testActor); err != nil {
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
	mustClaim(t, db, id, "")

	if err := runRelease(db, id, testActor); err != nil {
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

}

func TestRunRelease_NotClaimed(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	err := runRelease(db, id, testActor)
	if err == nil {
		t.Fatal("expected error when releasing unclaimed task")
	}
}

func TestRunRelease_NotFound(t *testing.T) {
	db := setupTestDB(t)
	err := runRelease(db, "noExs", testActor)
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
	mustClaim(t, db, id, "1h")

	currentNowFunc = func() time.Time { return baseTime.Add(2 * time.Hour) }
	if err := expireStaleClaims(db, testActor); err != nil {
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
	mustClaim(t, db, id, "1h")

	currentNowFunc = func() time.Time { return baseTime.Add(2 * time.Hour) }
	if err := expireStaleClaims(db, testActor); err != nil {
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

}

func TestExpireStaleClaims_ActiveClaimNotExpired(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")
	mustClaim(t, db, id, "4h")

	if err := expireStaleClaims(db, testActor); err != nil {
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

	task, err := runNext(db, pid, testActor)
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

	task, err := runNext(db, "", testActor)
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

	_, err := runNext(db, pid, testActor)
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
	if err := runBlock(db, cid1, blocker, testActor); err != nil {
		t.Fatalf("runBlock: %v", err)
	}

	task, err := runNext(db, pid, testActor)
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
	mustClaim(t, db, cid1, "")

	task, err := runNext(db, pid, testActor)
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

	task, err := runNext(db, pid, testActor)
	if err != nil {
		t.Fatalf("runNext: %v", err)
	}
	if task.ShortID != cid2 {
		t.Errorf("got %s, want %s (should skip done)", task.ShortID, cid2)
	}
}

func TestRunNext_ParentNotFound(t *testing.T) {
	db := setupTestDB(t)
	_, err := runNext(db, "noExs", testActor)
	if err == nil {
		t.Fatal("expected error for non-existent parent")
	}
}

func TestRunNext_NoAvailableAtRoot(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Done root")
	mustDone(t, db, id)

	_, err := runNext(db, "", testActor)
	if err == nil {
		t.Fatal("expected error when no root tasks available")
	}
}

// --- ClaimNext ---

func TestRunClaimNext_WithParent(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	cid := mustAdd(t, db, pid, "Available child")

	task, err := runClaimNext(db, pid, "", testActor, false)
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

	task, err := runClaimNext(db, "", "", testActor, false)
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

	task, err := runClaimNext(db, pid, "4h", testActor, false)
	if err != nil {
		t.Fatalf("runClaimNext: %v", err)
	}
	if task.ShortID != cid {
		t.Errorf("got %s, want %s", task.ShortID, cid)
	}

	updated := mustGet(t, db, cid)
	if updated.ClaimedBy == nil || *updated.ClaimedBy != testActor {
		t.Errorf("claimed_by: got %v, want %q", updated.ClaimedBy, testActor)
	}
}

func TestRunClaimNext_NoAvailable(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	cid := mustAdd(t, db, pid, "Done child")
	mustDone(t, db, cid)

	_, err := runClaimNext(db, pid, "", testActor, false)
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
	if err := runBlock(db, cid1, blocker, testActor); err != nil {
		t.Fatalf("runBlock: %v", err)
	}

	task, err := runClaimNext(db, pid, "", testActor, false)
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

	if _, err := runClaimNext(db, pid, "2h", testActor, false); err != nil {
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

}

func TestRunClaimNext_ParentNotFound(t *testing.T) {
	db := setupTestDB(t)
	_, err := runClaimNext(db, "noExs", "", testActor, false)
	if err == nil {
		t.Fatal("expected error for non-existent parent")
	}
}

// --- Auto-unblock on done ---

func TestRunDone_AutoUnblocks(t *testing.T) {
	db := setupTestDB(t)
	blocker := mustAdd(t, db, "", "Blocker")
	blocked := mustAdd(t, db, "", "Blocked")
	if err := runBlock(db, blocked, blocker, testActor); err != nil {
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
	if err := runBlock(db, blocked, blocker, testActor); err != nil {
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
	if err := runBlock(db, blocked, blocker, testActor); err != nil {
		t.Fatalf("runBlock: %v", err)
	}

	nodes, err := runList(db, "", testActor, false)
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
	if err := runBlock(db, blocked, blocker, testActor); err != nil {
		t.Fatalf("runBlock: %v", err)
	}

	nodes, err := runList(db, "", testActor, true)
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
	mustClaim(t, db, id, "4h")

	nodes, err := runList(db, "", testActor, false)
	if err != nil {
		t.Fatalf("runList: %v", err)
	}

	for _, node := range nodes {
		if node.Task.ShortID == id {
			t.Error("claimed task should not appear in default list")
		}
	}
}

// --- Log ---

func TestRunLog_SingleTask(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "My task")

	events, err := runLog(db, id, nil)
	if err != nil {
		t.Fatalf("runLog: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventType != "created" {
		t.Errorf("event type: got %q, want %q", events[0].EventType, "created")
	}
	if events[0].ShortID != id {
		t.Errorf("short_id: got %q, want %q", events[0].ShortID, id)
	}
}

func TestRunLog_WithDescendants(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	cid := mustAdd(t, db, pid, "Child")

	events, err := runLog(db, pid, nil)
	if err != nil {
		t.Fatalf("runLog: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].EventType != "created" || events[0].ShortID != pid {
		t.Errorf("first event: got %q/%q, want created/%q", events[0].EventType, events[0].ShortID, pid)
	}
	if events[1].EventType != "created" || events[1].ShortID != cid {
		t.Errorf("second event: got %q/%q, want created/%q", events[1].EventType, events[1].ShortID, cid)
	}
}

func TestRunLog_VariousEventTypes(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")
	mustClaim(t, db, id, "1h")
	mustRelease(t, db, id)

	events, err := runLog(db, id, nil)
	if err != nil {
		t.Fatalf("runLog: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events (created, claimed, released), got %d", len(events))
	}
	if events[0].EventType != "created" {
		t.Errorf("event 0: got %q, want created", events[0].EventType)
	}
	if events[1].EventType != "claimed" {
		t.Errorf("event 1: got %q, want claimed", events[1].EventType)
	}
	if events[2].EventType != "released" {
		t.Errorf("event 2: got %q, want released", events[2].EventType)
	}
}

func TestRunLog_EventsOrderedByTime(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	mustAdd(t, db, pid, "Child A")
	mustAdd(t, db, pid, "Child B")

	events, err := runLog(db, pid, nil)
	if err != nil {
		t.Fatalf("runLog: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	for i := 1; i < len(events); i++ {
		if events[i].CreatedAt < events[i-1].CreatedAt {
			t.Errorf("event %d (ts %d) < event %d (ts %d)", i, events[i].CreatedAt, i-1, events[i-1].CreatedAt)
		}
	}
}

func TestRunLog_DoesNotIncludeOtherBranches(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent")
	mustAdd(t, db, pid, "Included child")
	otherID := mustAdd(t, db, "", "Other root")
	mustAdd(t, db, otherID, "Other child")

	events, err := runLog(db, pid, nil)
	if err != nil {
		t.Fatalf("runLog: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events (parent + 1 child), got %d", len(events))
	}
	for _, e := range events {
		if e.ShortID == otherID {
			t.Error("should not include events from other roots")
		}
	}
}

func TestRunLog_TaskNotFound(t *testing.T) {
	db := setupTestDB(t)
	_, err := runLog(db, "noExs", nil)
	if err == nil {
		t.Fatal("expected error for non-existent task")
	}
}

func TestRunLog_FormattedMarkdown(t *testing.T) {
	db := setupTestDB(t)
	pid := mustAdd(t, db, "", "Parent task")
	cid := mustAdd(t, db, pid, "Child task")
	mustDone(t, db, cid)

	events, err := runLog(db, pid, nil)
	if err != nil {
		t.Fatalf("runLog: %v", err)
	}

	var buf strings.Builder
	renderEventLogMarkdown(&buf, events)
	output := buf.String()

	if !strings.Contains(output, "[") {
		t.Error("expected timestamp brackets in output")
	}
	if !strings.Contains(output, pid) {
		t.Error("expected parent short_id in output")
	}
	if !strings.Contains(output, cid) {
		t.Error("expected child short_id in output")
	}
	if !strings.Contains(output, "created") {
		t.Error("expected 'created' in output")
	}
}

func TestRunLog_FormattedJSON(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")
	mustClaim(t, db, id, "1h")

	events, err := runLog(db, id, nil)
	if err != nil {
		t.Fatalf("runLog: %v", err)
	}

	jsonBytes, err := formatEventLogJSON(events)
	if err != nil {
		t.Fatalf("formatEventLogJSON: %v", err)
	}

	var result []map[string]any
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 events, got %d", len(result))
	}
	if result[0]["event_type"] != "created" {
		t.Errorf("event 0 type: got %v, want created", result[0]["event_type"])
	}
	if result[1]["event_type"] != "claimed" {
		t.Errorf("event 1 type: got %v, want claimed", result[1]["event_type"])
	}
	if result[0]["short_id"] != id {
		t.Errorf("event 0 short_id: got %v, want %s", result[0]["short_id"], id)
	}
}

// --- Tail ---

func TestGetEventsAfterID_ReturnsNewEvents(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	allEvents, err := getEventsForTaskTree(db, id)
	if err != nil {
		t.Fatalf("getEventsForTaskTree: %v", err)
	}
	if len(allEvents) != 1 {
		t.Fatalf("expected 1 initial event, got %d", len(allEvents))
	}

	mustClaim(t, db, id, "1h")

	newEvents, err := getEventsAfterID(db, id, allEvents[0].ID)
	if err != nil {
		t.Fatalf("getEventsAfterID: %v", err)
	}
	if len(newEvents) != 1 {
		t.Fatalf("expected 1 new event, got %d", len(newEvents))
	}
	if newEvents[0].EventType != "claimed" {
		t.Errorf("event type: got %q, want claimed", newEvents[0].EventType)
	}
}

func TestGetEventsAfterID_NoNewEvents(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	allEvents, err := getEventsForTaskTree(db, id)
	if err != nil {
		t.Fatalf("getEventsForTaskTree: %v", err)
	}

	newEvents, err := getEventsAfterID(db, id, allEvents[0].ID)
	if err != nil {
		t.Fatalf("getEventsAfterID: %v", err)
	}
	if len(newEvents) != 0 {
		t.Errorf("expected 0 new events, got %d", len(newEvents))
	}
}

func TestRunTail_PicksUpNewEvents(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var mu sync.Mutex
	var collected []EventEntry
	initialDrained := make(chan struct{})
	claimSeen := make(chan struct{})
	done := make(chan struct{})

	go func() {
		sawInitial := false
		runTail(ctx, db, id, 10*time.Millisecond, func(events []EventEntry) error {
			mu.Lock()
			collected = append(collected, events...)
			hasClaim := false
			for _, e := range events {
				if e.EventType == "claimed" {
					hasClaim = true
				}
			}
			mu.Unlock()
			if !sawInitial {
				sawInitial = true
				close(initialDrained)
			}
			if hasClaim {
				select {
				case <-claimSeen:
				default:
					close(claimSeen)
				}
				cancel()
			}
			return nil
		})
		close(done)
	}()

	// Wait for the tail to drain the pre-existing `created` event before
	// firing mustClaim — otherwise the first poll might include `created`
	// only, the callback cancels, and mustClaim's event never arrives.
	<-initialDrained
	mustClaim(t, db, id, "1h")
	<-claimSeen
	<-done

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, e := range collected {
		if e.EventType == "claimed" {
			found = true
		}
	}
	if !found {
		t.Error("expected to find a claimed event")
	}
}

func TestRunTail_TaskNotFound(t *testing.T) {
	db := setupTestDB(t)
	ctx := t.Context()

	err := runTail(ctx, db, "noExs", 10*time.Millisecond, func(events []EventEntry) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected error for non-existent task")
	}
}

func TestRunList_ExpiredClaimShowsAsAvailable(t *testing.T) {
	origNow := currentNowFunc
	defer func() { currentNowFunc = origNow }()

	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	baseTime := time.Now()
	currentNowFunc = func() time.Time { return baseTime }
	mustClaim(t, db, id, "1h")

	currentNowFunc = func() time.Time { return baseTime.Add(2 * time.Hour) }

	nodes, err := runList(db, "", testActor, false)
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
