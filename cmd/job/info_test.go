package main

import (
	"encoding/json"
	job "github.com/bensyverson/jobs/internal/job"
	"strings"
	"testing"
)

// R3 — single ID is unchanged behaviour.
func TestInfo_SingleID(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := job.MustAdd(t, db, "", "SingleTask")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "info", id)
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	if !strings.Contains(stdout, "SingleTask") {
		t.Errorf("single-ID output missing title:\n%s", stdout)
	}
}

// R3 — two IDs returns both tasks.
func TestInfo_TwoIDs_ReturnsBoth(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	a := job.MustAdd(t, db, "", "Alpha")
	b := job.MustAdd(t, db, "", "Beta")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "info", a, b)
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	if !strings.Contains(stdout, "Alpha") {
		t.Errorf("output missing Alpha:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Beta") {
		t.Errorf("output missing Beta:\n%s", stdout)
	}
}

// R3 — separator blank line appears between tasks, not after the last.
func TestInfo_TwoIDs_SeparatorBetween(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	a := job.MustAdd(t, db, "", "First")
	b := job.MustAdd(t, db, "", "Second")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "info", a, b)
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	// Must contain a blank line (two consecutive newlines) between tasks.
	if !strings.Contains(stdout, "\n\n") {
		t.Errorf("expected blank-line separator between tasks:\n%s", stdout)
	}
	// Must not end with two newlines (no trailing blank line).
	if strings.HasSuffix(stdout, "\n\n") {
		t.Errorf("must not have trailing blank line:\n%s", stdout)
	}
}

// R3 — valid + invalid ID fails on the invalid one (fail-fast).
func TestInfo_InvalidID_Fails(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	a := job.MustAdd(t, db, "", "Good")
	db.Close()

	_, _, err := runCLI(t, dbFile, "info", a, "XXXXX")
	if err == nil {
		t.Errorf("expected error for invalid ID, got nil")
	}
}

// R3 — JSON format produces an array when given multiple IDs.
func TestInfo_JSONFormat_MultipleIDs_Array(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	a := job.MustAdd(t, db, "", "Gamma")
	b := job.MustAdd(t, db, "", "Delta")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "info", "--format", "json", a, b)
	if err != nil {
		t.Fatalf("info --format json: %v", err)
	}
	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &arr); err != nil {
		t.Fatalf("output is not a JSON array: %v\n%s", err, stdout)
	}
	if len(arr) != 2 {
		t.Errorf("expected 2 elements, got %d", len(arr))
	}
}

// R3 — JSON format with a single ID is still a valid JSON array with 1 element.
func TestInfo_JSONFormat_SingleID_Array(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	a := job.MustAdd(t, db, "", "Epsilon")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "info", "--format", "json", a)
	if err != nil {
		t.Fatalf("info --format json: %v", err)
	}
	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &arr); err != nil {
		t.Fatalf("output is not a JSON array: %v\n%s", err, stdout)
	}
	if len(arr) != 1 {
		t.Errorf("expected 1 element, got %d", len(arr))
	}
}
