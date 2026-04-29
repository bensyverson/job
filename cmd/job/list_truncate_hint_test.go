package main

import (
	"strings"
	"testing"

	job "github.com/bensyverson/jobs/internal/job"
)

func TestListAll_TruncationFooter_FiresWhenCapped(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	for range 12 {
		id := job.MustAdd(t, db, "", "closed task")
		job.MustDone(t, db, id)
	}
	job.MustAdd(t, db, "", "open one")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "list", "--all")
	if err != nil {
		t.Fatalf("ls --all: %v", err)
	}
	if !strings.Contains(stdout, "of 12 recent closures") &&
		!strings.Contains(stdout, "of 12 ") {
		t.Errorf("expected truncation hint mentioning total, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "--since") {
		t.Errorf("expected hint to name --since, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "--no-truncate") {
		t.Errorf("expected hint to name --no-truncate, got:\n%s", stdout)
	}
}

func TestListAll_TruncationFooter_SuppressedWhenAllShown(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	for range 5 {
		id := job.MustAdd(t, db, "", "closed task")
		job.MustDone(t, db, id)
	}
	job.MustAdd(t, db, "", "open one")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "list", "--all")
	if err != nil {
		t.Fatalf("ls --all: %v", err)
	}
	if strings.Contains(stdout, "--no-truncate") {
		t.Errorf("expected NO truncation hint when nothing was truncated, got:\n%s", stdout)
	}
}

func TestListAll_TruncationFooter_SuppressedForJSON(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	for range 12 {
		id := job.MustAdd(t, db, "", "closed task")
		job.MustDone(t, db, id)
	}
	job.MustAdd(t, db, "", "open one")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "list", "--all", "--format=json")
	if err != nil {
		t.Fatalf("ls --all --format=json: %v", err)
	}
	if strings.Contains(stdout, "--no-truncate") {
		t.Errorf("JSON output should not include truncation hint, got:\n%s", stdout)
	}
	if strings.Contains(stdout, "recent closures") {
		t.Errorf("JSON output should not include truncation hint, got:\n%s", stdout)
	}
}
