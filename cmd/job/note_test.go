package main

import (
	job "github.com/bensyverson/jobs/internal/job"
	"strings"
	"testing"
)

func TestNote_Flag_Happy(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := job.MustAdd(t, db, "", "job.Task")
	db.Close()

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "note", id, "-m", "progress"); err != nil {
		t.Fatalf("note: %v", err)
	}

	db = openTestDB(t, dbFile)
	task := job.MustGet(t, db, id)
	if !strings.Contains(task.Description, "progress") {
		t.Errorf("description: %q", task.Description)
	}
	detail, _ := job.GetLatestEventDetail(db, task.ID, "noted")
	if detail["text"] != "progress" {
		t.Errorf("event text: %v", detail["text"])
	}
}

func TestNote_Stdin_Form(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := job.MustAdd(t, db, "", "job.Task")
	db.Close()

	if _, _, err := runCLIWithStdin(t, dbFile, "from stdin\n", "--as", "alice", "note", id, "-"); err != nil {
		t.Fatalf("note: %v", err)
	}

	db = openTestDB(t, dbFile)
	task := job.MustGet(t, db, id)
	if !strings.Contains(task.Description, "from stdin") {
		t.Errorf("description: %q", task.Description)
	}
}

func TestNote_StdinAndFlag_Error(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := job.MustAdd(t, db, "", "job.Task")
	db.Close()

	_, _, err := runCLIWithStdin(t, dbFile, "x", "--as", "alice", "note", id, "-", "-m", "y")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("err: %v", err)
	}
}

func TestNote_Empty_FlagError(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := job.MustAdd(t, db, "", "job.Task")
	db.Close()

	_, _, err := runCLI(t, dbFile, "--as", "alice", "note", id, "-m", "")
	if err == nil {
		t.Fatal("expected empty-body error")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("err: %v", err)
	}
}

func TestNote_Empty_StdinError(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := job.MustAdd(t, db, "", "job.Task")
	db.Close()

	_, _, err := runCLIWithStdin(t, dbFile, "", "--as", "alice", "note", id, "-")
	if err == nil {
		t.Fatal("expected empty-body error")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("err: %v", err)
	}
}

func TestNote_Result(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := job.MustAdd(t, db, "", "job.Task")
	db.Close()

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "note", id, "-m", "hi", "--result", `{"p":true}`); err != nil {
		t.Fatalf("note: %v", err)
	}

	db = openTestDB(t, dbFile)
	task := job.MustGet(t, db, id)
	detail, _ := job.GetLatestEventDetail(db, task.ID, "noted")
	result, ok := detail["result"].(map[string]any)
	if !ok {
		t.Fatalf("result: got %T", detail["result"])
	}
	if result["p"] != true {
		t.Errorf("result[p]: %v", result["p"])
	}
}

func TestNote_Positional_Accepted(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := job.MustAdd(t, db, "", "job.Task")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "note", id, "some text")
	if err != nil {
		t.Fatalf("expected positional text to be accepted, got error: %v", err)
	}
	if !strings.Contains(stdout, "Noted:") {
		t.Errorf("stdout missing Noted ack: %q", stdout)
	}
	if !strings.Contains(stdout, "some text") {
		t.Errorf("stdout missing note preview: %q", stdout)
	}
}

func TestNote_MissingFlag_Error(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := job.MustAdd(t, db, "", "job.Task")
	db.Close()

	_, _, err := runCLI(t, dbFile, "--as", "alice", "note", id)
	if err == nil {
		t.Fatal("expected error when no text given (positional, -m, or stdin)")
	}
	if !strings.Contains(err.Error(), "requires text") {
		t.Errorf("err: %v", err)
	}
}
