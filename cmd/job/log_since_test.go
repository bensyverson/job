package main

import (
	job "github.com/bensyverson/job/internal/job"
	"strings"
	"testing"
	"time"
)

func TestLogSince_FiltersEvents(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := job.MustAdd(t, db, "", "X")
	if err := job.RunClaim(db, id, "1h", "alice", false); err != nil {
		t.Fatalf("claim: %v", err)
	}

	t0 := time.Date(2026, 4, 20, 9, 0, 0, 0, time.UTC)
	if _, err := db.Exec("UPDATE events SET created_at = ? WHERE event_type = 'created'", t0.Unix()); err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, err := db.Exec("UPDATE events SET created_at = ? WHERE event_type = 'claimed'", t0.Add(2*time.Hour).Unix()); err != nil {
		t.Fatalf("update: %v", err)
	}
	db.Close()

	cutoff := t0.Add(1 * time.Hour).Format(time.RFC3339)
	stdout, _, err := runCLI(t, dbFile, "log", id, "--since", cutoff)
	if err != nil {
		t.Fatalf("log --since: %v", err)
	}
	if strings.Contains(stdout, "created:") {
		t.Errorf("created event should be filtered out:\n%s", stdout)
	}
	if !strings.Contains(stdout, "claimed") {
		t.Errorf("claimed event should be present:\n%s", stdout)
	}
}

func TestLogSince_BoundaryInclusive(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := job.MustAdd(t, db, "", "X")

	t0 := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	if _, err := db.Exec("UPDATE events SET created_at = ? WHERE event_type = 'created'", t0.Unix()); err != nil {
		t.Fatalf("update: %v", err)
	}
	db.Close()

	cutoff := t0.Format(time.RFC3339)
	stdout, _, err := runCLI(t, dbFile, "log", id, "--since", cutoff)
	if err != nil {
		t.Fatalf("log --since: %v", err)
	}
	if !strings.Contains(stdout, "created:") {
		t.Errorf("event at exact cutoff should be included:\n%s", stdout)
	}
}

func TestLogSince_InvalidRFC3339_Errors(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := job.MustAdd(t, db, "", "X")
	db.Close()

	_, _, err := runCLI(t, dbFile, "log", id, "--since", "yesterday")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid RFC3339 timestamp") {
		t.Errorf("err: %v", err)
	}
}

func TestLogSince_WithTaskSubtree(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	parent := job.MustAdd(t, db, "", "Parent")
	_ = job.MustAdd(t, db, parent, "Child")

	t0 := time.Date(2026, 4, 20, 9, 0, 0, 0, time.UTC)
	// All current events: pre-cutoff.
	if _, err := db.Exec("UPDATE events SET created_at = ?", t0.Unix()); err != nil {
		t.Fatalf("update: %v", err)
	}

	gcID := job.MustAdd(t, db, parent, "GC")
	// Move the GC's created event past the cutoff.
	if _, err := db.Exec(
		"UPDATE events SET created_at = ? WHERE event_type = 'created' AND task_id = (SELECT id FROM tasks WHERE short_id = ?)",
		t0.Add(2*time.Hour).Unix(), gcID,
	); err != nil {
		t.Fatalf("update: %v", err)
	}
	db.Close()

	cutoff := t0.Add(1 * time.Hour).Format(time.RFC3339)
	stdout, _, err := runCLI(t, dbFile, "log", parent, "--since", cutoff)
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	if !strings.Contains(stdout, "GC") {
		t.Errorf("post-cutoff descendant event should appear:\n%s", stdout)
	}
	if strings.Contains(stdout, "\"Parent\"") {
		t.Errorf("pre-cutoff parent created event should not appear:\n%s", stdout)
	}
}
