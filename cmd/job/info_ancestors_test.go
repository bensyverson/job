package main

import (
	job "github.com/bensyverson/jobs/internal/job"
	"strings"
	"testing"
)

// --ancestors on a root task is a no-op: only the task's own block is
// printed (no preamble), since there are no ancestors to prepend.
func TestInfo_Ancestors_RootTask_NoPreamble(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := job.MustAddDesc(t, db, "", "RootTask", "Root description body.")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "show", "--ancestors", id)
	if err != nil {
		t.Fatalf("show --ancestors: %v", err)
	}
	if !strings.Contains(stdout, "RootTask") {
		t.Errorf("expected root title in output:\n%s", stdout)
	}
	// The root task's own ID line must appear exactly once (no ancestor
	// preamble duplicating it).
	if got := strings.Count(stdout, "ID:           "+id); got != 1 {
		t.Errorf("expected exactly one ID line for %s, got %d:\n%s", id, got, stdout)
	}
}

// --ancestors on a child task prepends the parent's title and description
// before the child's own full block.
func TestInfo_Ancestors_ChildTask_PrependsParent(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	parent := job.MustAddDesc(t, db, "", "ParentTitle", "Parent description body.")
	child := job.MustAddDesc(t, db, parent, "ChildTitle", "Child description body.")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "show", "--ancestors", child)
	if err != nil {
		t.Fatalf("show --ancestors: %v", err)
	}
	// Parent identity, title, and description must appear.
	if !strings.Contains(stdout, parent) {
		t.Errorf("expected parent ID %s in output:\n%s", parent, stdout)
	}
	if !strings.Contains(stdout, "ParentTitle") {
		t.Errorf("expected parent title in output:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Parent description body.") {
		t.Errorf("expected parent description in output:\n%s", stdout)
	}
	// Child's own block still present.
	if !strings.Contains(stdout, "ChildTitle") {
		t.Errorf("expected child title in output:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Child description body.") {
		t.Errorf("expected child description in output:\n%s", stdout)
	}
	// Parent must appear before child in the output.
	pIdx := strings.Index(stdout, "ParentTitle")
	cIdx := strings.Index(stdout, "ChildTitle")
	if pIdx < 0 || cIdx < 0 || pIdx >= cIdx {
		t.Errorf("expected parent block before child block (pIdx=%d cIdx=%d):\n%s", pIdx, cIdx, stdout)
	}
}

// --ancestors on a grandchild prints grandparent → parent → leaf in order.
func TestInfo_Ancestors_Grandchild_RootToLeafOrder(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	gp := job.MustAddDesc(t, db, "", "Grandparent", "GP body.")
	p := job.MustAddDesc(t, db, gp, "Parent", "P body.")
	c := job.MustAddDesc(t, db, p, "Leaf", "Leaf body.")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "show", "--ancestors", c)
	if err != nil {
		t.Fatalf("show --ancestors: %v", err)
	}
	gpIdx := strings.Index(stdout, "Grandparent")
	pIdx := strings.Index(stdout, "Parent")
	cIdx := strings.Index(stdout, "Leaf")
	if gpIdx < 0 || pIdx < 0 || cIdx < 0 {
		t.Fatalf("missing one of GP/P/Leaf in output:\n%s", stdout)
	}
	if !(gpIdx < pIdx && pIdx < cIdx) {
		t.Errorf("expected order Grandparent < Parent < Leaf, got gp=%d p=%d c=%d:\n%s", gpIdx, pIdx, cIdx, stdout)
	}
}

// --ancestors prints a single header line above the preamble pinning the
// root → node convention. Header appears once, before the first ancestor
// block.
func TestInfo_Ancestors_HeaderPrecedesPreamble(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	parent := job.MustAddDesc(t, db, "", "ParentTitle", "Parent body.")
	child := job.MustAddDesc(t, db, parent, "ChildTitle", "Child body.")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "show", "--ancestors", child)
	if err != nil {
		t.Fatalf("show --ancestors: %v", err)
	}
	const header = "Ancestors (root → node):"
	if !strings.Contains(stdout, header) {
		t.Errorf("expected header %q in output:\n%s", header, stdout)
	}
	if got := strings.Count(stdout, header); got != 1 {
		t.Errorf("expected exactly one header line, got %d:\n%s", got, stdout)
	}
	hIdx := strings.Index(stdout, header)
	pIdx := strings.Index(stdout, "ParentTitle")
	if hIdx < 0 || pIdx < 0 || hIdx >= pIdx {
		t.Errorf("expected header before parent block (h=%d p=%d):\n%s", hIdx, pIdx, stdout)
	}
}

// Root tasks (no ancestors) get no header, since there's no preamble to
// label.
func TestInfo_Ancestors_RootTask_NoHeader(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := job.MustAddDesc(t, db, "", "RootOnly", "Root body.")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "show", "--ancestors", id)
	if err != nil {
		t.Fatalf("show --ancestors: %v", err)
	}
	if strings.Contains(stdout, "Ancestors (") {
		t.Errorf("root task must not show ancestors header:\n%s", stdout)
	}
}

// Without --ancestors, output is unchanged: no parent description leaks in.
func TestInfo_Ancestors_FlagOff_NoPreamble(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	parent := job.MustAddDesc(t, db, "", "ParentTitle", "Parent description body.")
	child := job.MustAddDesc(t, db, parent, "ChildTitle", "Child description body.")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "show", child)
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	if strings.Contains(stdout, "Parent description body.") {
		t.Errorf("flag-off output must not include parent description:\n%s", stdout)
	}
}
