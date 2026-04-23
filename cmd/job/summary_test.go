package main

import (
	"strings"
	"testing"

	job "github.com/bensyverson/job/internal/job"
)

// R2 — `job summary <id>` reads the target and prints rollup. No --as
// required (it's a read).

// u8 — `summary` is a deprecated alias for `status`. Same stdout as
// `status <id>`; a one-line stderr notice fires on every invocation,
// matching the ls→list / show→info / unblock→block-remove pattern.
func TestSummaryCmd_DeprecatedAlias_MatchesStatusAndEmitsNotice(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	root := job.MustAdd(t, db, "", "Root")
	phase := job.MustAdd(t, db, root, "Phase")
	job.MustAdd(t, db, phase, "leaf")
	db.Close()

	summaryOut, summaryErr, err := runCLI(t, dbFile, "summary", root)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	statusOut, statusErr, err := runCLI(t, dbFile, "status", root)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if summaryOut != statusOut {
		t.Errorf("summary stdout must match status stdout:\n--summary--\n%s\n--status--\n%s",
			summaryOut, statusOut)
	}
	if !strings.Contains(summaryErr, "deprecated") {
		t.Errorf("summary must emit a deprecation notice on stderr, got: %q", summaryErr)
	}
	if strings.Contains(statusErr, "deprecated") {
		t.Errorf("status must not emit a deprecation notice, got: %q", statusErr)
	}
}

func TestSummaryCmd_Reads(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	root := job.MustAdd(t, db, "", "Project")
	// Phase A carries a grandchild so it is NOT a leaf — keeps the
	// per-child block visible after the leaf-only-collapse rule (u3)
	// landed. The assertion below is about the summary verb reading
	// the target and rendering its rollup; the leaf-collapse shape is
	// covered by the unit tests in internal/job/summary_test.go.
	phaseA := job.MustAdd(t, db, root, "Phase A")
	job.MustAdd(t, db, phaseA, "A1")
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
