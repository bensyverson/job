package main

import (
	"strings"
	"testing"
)

func TestReopen_Plain_DoesNotTouchDescendants(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	p := mustAdd(t, db, "", "P")
	c := mustAdd(t, db, p, "C")
	if _, _, err := runDone(db, []string{p}, true, "", nil, testActor); err != nil {
		t.Fatalf("done: %v", err)
	}
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "reopen", p)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if strings.Contains(stdout, "subtasks") {
		t.Errorf("plain reopen should not mention subtasks:\n%s", stdout)
	}

	db = openTestDB(t, dbFile)
	parent := mustGet(t, db, p)
	if parent.Status != "available" {
		t.Errorf("parent: status=%q", parent.Status)
	}
	child := mustGet(t, db, c)
	if child.Status != "done" {
		t.Errorf("child: status=%q, want done (not reopened)", child.Status)
	}
}

func TestReopen_Cascade_ReopensAllDone(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	p := mustAdd(t, db, "", "P")
	c := mustAdd(t, db, p, "C")
	gc := mustAdd(t, db, c, "GC")
	if _, _, err := runDone(db, []string{p}, true, "", nil, testActor); err != nil {
		t.Fatalf("done: %v", err)
	}
	db.Close()

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "reopen", p, "--cascade"); err != nil {
		t.Fatalf("reopen: %v", err)
	}

	db = openTestDB(t, dbFile)
	for _, id := range []string{p, c, gc} {
		task := mustGet(t, db, id)
		if task.Status != "available" {
			t.Errorf("%s: status=%q, want available", id, task.Status)
		}
	}
}

func TestReopen_FromCanceled(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := mustAdd(t, db, "", "Task")
	db.Close()

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "cancel", id, "--reason", "x"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if _, _, err := runCLI(t, dbFile, "--as", "alice", "reopen", id); err != nil {
		t.Fatalf("reopen: %v", err)
	}

	db = openTestDB(t, dbFile)
	task := mustGet(t, db, id)
	if task.Status != "available" {
		t.Errorf("status: got %q, want available", task.Status)
	}
	detail, _ := getLatestEventDetail(db, task.ID, "reopened")
	if detail["from_status"] != "canceled" {
		t.Errorf("from_status: got %v, want canceled", detail["from_status"])
	}
}

func TestReopen_Cascade_IncludesCanceledDescendants(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	p := mustAdd(t, db, "", "P")
	c := mustAdd(t, db, p, "C")
	db.Close()

	// Cancel parent with cascade — closes both p and c as canceled.
	if _, _, err := runCLI(t, dbFile, "--as", "alice", "cancel", p, "--cascade", "--reason", "x"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if _, _, err := runCLI(t, dbFile, "--as", "alice", "reopen", p, "--cascade"); err != nil {
		t.Fatalf("reopen: %v", err)
	}

	db = openTestDB(t, dbFile)
	for _, id := range []string{p, c} {
		if task := mustGet(t, db, id); task.Status != "available" {
			t.Errorf("%s: status=%q, want available", id, task.Status)
		}
	}
}

func TestReopen_EventShape_Cascade(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	p := mustAdd(t, db, "", "P")
	c := mustAdd(t, db, p, "C")
	if _, _, err := runDone(db, []string{p}, true, "", nil, testActor); err != nil {
		t.Fatalf("done: %v", err)
	}
	db.Close()

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "reopen", p, "--cascade"); err != nil {
		t.Fatalf("reopen: %v", err)
	}

	db = openTestDB(t, dbFile)
	parent := mustGet(t, db, p)
	detail, _ := getLatestEventDetail(db, parent.ID, "reopened")
	if detail["cascade"] != true {
		t.Errorf("cascade: got %v, want true", detail["cascade"])
	}
	children, ok := detail["reopened_children"].([]any)
	if !ok || len(children) != 1 {
		t.Fatalf("reopened_children: got %v", detail["reopened_children"])
	}
	if children[0] != c {
		t.Errorf("child: got %v, want %s", children[0], c)
	}
}
