package main

import (
	"strings"
	"testing"

	job "github.com/bensyverson/jobs/internal/job"
)

func TestListAll_ShowsRecentlyClosedFooter(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	a := job.MustAdd(t, db, "", "alpha")
	job.MustAdd(t, db, "", "open one")
	job.MustDone(t, db, a)
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "list", "--all")
	if err != nil {
		t.Fatalf("ls --all: %v", err)
	}
	if !strings.Contains(stdout, "Recently closed (1 of 1)") {
		t.Errorf("expected 'Recently closed (1 of 1)' header, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "alpha") {
		t.Errorf("expected closed task title 'alpha' in tail, got:\n%s", stdout)
	}
}

func TestListAll_ClosedChildRendersInTreeWhenParentVisible(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	parent := job.MustAdd(t, db, "", "Parent")
	job.MustAdd(t, db, parent, "open sibling")
	child := job.MustAdd(t, db, parent, "child task")
	job.MustDone(t, db, child)
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "list", "--all")
	if err != nil {
		t.Fatalf("ls --all: %v", err)
	}
	// Parent is still open (it has another open child), so the closed child
	// renders inline under it; the footer should NOT also list it.
	if !strings.Contains(stdout, "[x] `"+child+"` child task") {
		t.Errorf("expected closed child rendered inline in tree, got:\n%s", stdout)
	}
	// Hybrid rule: footer skips closed tasks whose parent is being rendered
	// in the open tree.
	if strings.Contains(stdout, "Recently closed") {
		t.Errorf("expected no Recently closed footer when parent is in tree, got:\n%s", stdout)
	}
}

func TestListAll_OrphanedClosedTaskGoesToFooter(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	root := job.MustAdd(t, db, "", "shipped feature")
	job.MustAdd(t, db, "", "still open")
	job.MustDone(t, db, root)
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "list", "--all")
	if err != nil {
		t.Fatalf("ls --all: %v", err)
	}
	if !strings.Contains(stdout, "Recently closed") {
		t.Errorf("expected Recently closed footer, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "shipped feature") {
		t.Errorf("expected closed root in tail, got:\n%s", stdout)
	}
}

func TestListAll_BreadcrumbForOrphanWithClosedParent(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	parent := job.MustAdd(t, db, "", "Parent")
	child := job.MustAdd(t, db, parent, "child task")
	job.MustDone(t, db, child) // cascades parent closed too
	job.MustAdd(t, db, "", "keep tree non-empty")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "list", "--all")
	if err != nil {
		t.Fatalf("ls --all: %v", err)
	}
	// Parent auto-closed → not in open tree → child is an orphan from the
	// tree's perspective and renders in the footer with breadcrumb.
	if !strings.Contains(stdout, "(in `"+parent+"` Parent)") {
		t.Errorf("expected breadcrumb '(in `%s` Parent)' for orphaned child, got:\n%s", parent, stdout)
	}
}

func TestListAll_NoBreadcrumbForSubtreeScope(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	parent := job.MustAdd(t, db, "", "Parent")
	child := job.MustAdd(t, db, parent, "child task")
	job.MustDone(t, db, child)
	db.Close()

	// Scoping to <parent>: the parent itself is not rendered (we render its
	// subtree), so the closed child becomes an orphan in the rendered scope
	// and lands in the footer without a breadcrumb (parent is implicit).
	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "list", parent, "--all")
	if err != nil {
		t.Fatalf("ls <parent> --all: %v", err)
	}
	if strings.Contains(stdout, "(in `") {
		t.Errorf("expected no breadcrumb in subtree-scoped tail, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Recently closed") {
		t.Errorf("expected 'Recently closed' section, got:\n%s", stdout)
	}
}

func TestListAll_FooterOmittedWhenNoClosed(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	job.MustAdd(t, db, "", "only open")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "list", "--all")
	if err != nil {
		t.Fatalf("ls --all: %v", err)
	}
	if strings.Contains(stdout, "Recently closed") {
		t.Errorf("expected no 'Recently closed' section when nothing is closed, got:\n%s", stdout)
	}
}

func TestList_NoAllNoTail(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	a := job.MustAdd(t, db, "", "alpha")
	job.MustAdd(t, db, "", "open one")
	job.MustDone(t, db, a)
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "list")
	if err != nil {
		t.Fatalf("ls: %v", err)
	}
	if strings.Contains(stdout, "Recently closed") {
		t.Errorf("default ls should not render Recently closed footer, got:\n%s", stdout)
	}
}
