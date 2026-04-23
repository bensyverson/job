package main

import (
	"strings"
	"testing"

	job "github.com/bensyverson/job/internal/job"
)

// R2 — `job summary <id>` reads the target and prints rollup. No --as
// required (it's a read).

func TestSummaryCmd_Reads(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	root := job.MustAdd(t, db, "", "Project")
	job.MustAdd(t, db, root, "Phase A")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "summary", root)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if !strings.Contains(stdout, "Project") {
		t.Errorf("summary missing root title:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Phase A") {
		t.Errorf("summary missing direct child:\n%s", stdout)
	}
}
