package main

import (
	job "github.com/bensyverson/jobs/internal/job"
	"strings"
	"testing"
)

func TestEdit_Title_Only(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := job.MustAddDesc(t, db, "", "Old", "kept")
	db.Close()

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "edit", id, "--title", "New"); err != nil {
		t.Fatalf("edit: %v", err)
	}

	db = openTestDB(t, dbFile)
	task := job.MustGet(t, db, id)
	if task.Title != "New" || task.Description != "kept" {
		t.Errorf("task: title=%q desc=%q", task.Title, task.Description)
	}
	detail, _ := job.GetLatestEventDetail(db, task.ID, "edited")
	if detail["new_title"] != "New" {
		t.Errorf("event new_title: %v", detail["new_title"])
	}
	if _, ok := detail["new_desc"]; ok {
		t.Errorf("event should omit desc fields when untouched: %v", detail)
	}
}

func TestEdit_Desc_Only(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := job.MustAdd(t, db, "", "Title")
	db.Close()

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "edit", id, "--desc", "body"); err != nil {
		t.Fatalf("edit: %v", err)
	}

	db = openTestDB(t, dbFile)
	task := job.MustGet(t, db, id)
	if task.Title != "Title" || task.Description != "body" {
		t.Errorf("task: title=%q desc=%q", task.Title, task.Description)
	}
	detail, _ := job.GetLatestEventDetail(db, task.ID, "edited")
	if _, ok := detail["new_title"]; ok {
		t.Errorf("event should omit title fields when untouched: %v", detail)
	}
}

func TestEdit_Both(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := job.MustAddDesc(t, db, "", "Old title", "old body")
	db.Close()

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "edit", id, "--title", "Nt", "--desc", "Nd"); err != nil {
		t.Fatalf("edit: %v", err)
	}

	db = openTestDB(t, dbFile)
	task := job.MustGet(t, db, id)
	if task.Title != "Nt" || task.Description != "Nd" {
		t.Errorf("task: %+v", task)
	}
	detail, _ := job.GetLatestEventDetail(db, task.ID, "edited")
	for _, k := range []string{"old_title", "new_title", "old_desc", "new_desc"} {
		if _, ok := detail[k]; !ok {
			t.Errorf("missing detail key %q in %v", k, detail)
		}
	}
}

func TestEdit_Neither_Error(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := job.MustAdd(t, db, "", "X")
	db.Close()

	_, _, err := runCLI(t, dbFile, "--as", "alice", "edit", id)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--title") || !strings.Contains(err.Error(), "--desc") {
		t.Errorf("err: %v", err)
	}
}

func TestEdit_ClearDesc(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := job.MustAddDesc(t, db, "", "T", "old")
	db.Close()

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "edit", id, "--desc", ""); err != nil {
		t.Fatalf("edit: %v", err)
	}

	db = openTestDB(t, dbFile)
	task := job.MustGet(t, db, id)
	if task.Description != "" {
		t.Errorf("desc should be cleared: %q", task.Description)
	}
}

func TestEdit_Positional_Gone(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := job.MustAdd(t, db, "", "X")
	db.Close()

	_, _, err := runCLI(t, dbFile, "--as", "alice", "edit", id, "New title")
	if err == nil {
		t.Fatal("expected error: positional title no longer accepted")
	}
}
