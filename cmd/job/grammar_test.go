package main

import (
	"strings"
	"testing"

	job "github.com/bensyverson/jobs/internal/job"
)

// R3 — Consistent verb grammar + typo aliases with helper warnings.
//
// Canonical forms after R3:
//   - block add <blocked> by <blocker>
//   - block remove <blocked> by <blocker>
//
// Legacy aliases that still work but emit a one-line stderr deprecation
// notice on every invocation:
//   - block <blocked> by <blocker>           → block add ... by ...
//   - unblock <blocked> from <blocker>       → block remove ... by ...
//   - ls                                     → list
//   - show <id>                              → info <id>

func TestBlockAdd_Canonical(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	a := job.MustAdd(t, db, "", "A")
	b := job.MustAdd(t, db, "", "B")
	db.Close()

	stdout, stderr, err := runCLI(t, dbFile, "--as", "alice", "block", "add", a, "by", b)
	if err != nil {
		t.Fatalf("block add: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, "Blocked: "+a) {
		t.Errorf("stdout missing 'Blocked: %s':\n%s", a, stdout)
	}
	if stderr != "" {
		t.Errorf("canonical form should emit no stderr notice, got:\n%s", stderr)
	}
}

func TestBlockRemove_Canonical(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	a := job.MustAdd(t, db, "", "A")
	b := job.MustAdd(t, db, "", "B")
	if err := job.RunBlock(db, a, b, "alice"); err != nil {
		t.Fatalf("seed block: %v", err)
	}
	db.Close()

	stdout, stderr, err := runCLI(t, dbFile, "--as", "alice", "block", "remove", a, "by", b)
	if err != nil {
		t.Fatalf("block remove: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, "Unblocked: "+a) {
		t.Errorf("stdout missing 'Unblocked: %s':\n%s", a, stdout)
	}
	if stderr != "" {
		t.Errorf("canonical form should emit no stderr notice, got:\n%s", stderr)
	}
}

func TestBlock_LegacyAlias_Works(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	a := job.MustAdd(t, db, "", "A")
	b := job.MustAdd(t, db, "", "B")
	db.Close()

	stdout, stderr, err := runCLI(t, dbFile, "--as", "alice", "block", a, "by", b)
	if err != nil {
		t.Fatalf("legacy block: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, "Blocked: "+a) {
		t.Errorf("stdout missing 'Blocked: %s':\n%s", a, stdout)
	}
	if !strings.Contains(stderr, "block add") {
		t.Errorf("stderr should warn about canonical 'block add' form:\n%s", stderr)
	}
	if !strings.Contains(stderr, "alias") {
		t.Errorf("stderr should mention alias status:\n%s", stderr)
	}
}

func TestUnblock_LegacyAlias_Works(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	a := job.MustAdd(t, db, "", "A")
	b := job.MustAdd(t, db, "", "B")
	if err := job.RunBlock(db, a, b, "alice"); err != nil {
		t.Fatalf("seed block: %v", err)
	}
	db.Close()

	stdout, stderr, err := runCLI(t, dbFile, "--as", "alice", "unblock", a, "from", b)
	if err != nil {
		t.Fatalf("legacy unblock: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, "Unblocked: "+a) {
		t.Errorf("stdout missing 'Unblocked: %s':\n%s", a, stdout)
	}
	if !strings.Contains(stderr, "block remove") {
		t.Errorf("stderr should warn about canonical 'block remove' form:\n%s", stderr)
	}
}

func TestLs_AliasForList(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	job.MustAdd(t, db, "", "Alpha")
	db.Close()

	stdout, stderr, err := runCLI(t, dbFile, "ls")
	if err != nil {
		t.Fatalf("ls: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, "Alpha") {
		t.Errorf("ls should list tasks like list:\n%s", stdout)
	}
	if !strings.Contains(stderr, "list") {
		t.Errorf("stderr should warn that 'ls' is an alias for 'list':\n%s", stderr)
	}
	if !strings.Contains(stderr, "alias") {
		t.Errorf("stderr should mention alias status:\n%s", stderr)
	}
}

func TestList_NoAliasNotice(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	job.MustAdd(t, db, "", "Alpha")
	db.Close()

	_, stderr, err := runCLI(t, dbFile, "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if stderr != "" {
		t.Errorf("canonical 'list' should emit no stderr notice, got:\n%s", stderr)
	}
}

func TestShow_AliasForInfo(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := job.MustAdd(t, db, "", "Hello")
	db.Close()

	stdout, stderr, err := runCLI(t, dbFile, "show", id)
	if err != nil {
		t.Fatalf("show: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, "Hello") {
		t.Errorf("show should render info output:\n%s", stdout)
	}
	if !strings.Contains(stderr, "info") {
		t.Errorf("stderr should warn that 'show' is an alias for 'info':\n%s", stderr)
	}
	if !strings.Contains(stderr, "alias") {
		t.Errorf("stderr should mention alias status:\n%s", stderr)
	}
}

func TestInfo_NoAliasNotice(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := job.MustAdd(t, db, "", "Hello")
	db.Close()

	_, stderr, err := runCLI(t, dbFile, "info", id)
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	if stderr != "" {
		t.Errorf("canonical 'info' should emit no stderr notice, got:\n%s", stderr)
	}
}

// R1 — multi-blocker forms on the canonical surface.

func TestBlockAdd_MultipleBlockers(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	gate := job.MustAdd(t, db, "", "Gate")
	a := job.MustAdd(t, db, "", "A")
	b := job.MustAdd(t, db, "", "B")
	c := job.MustAdd(t, db, "", "C")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "block", "add", gate, "by", a, b, c)
	if err != nil {
		t.Fatalf("block add multi: %v", err)
	}
	for _, id := range []string{a, b, c} {
		if !strings.Contains(stdout, id) {
			t.Errorf("stdout missing blocker %s:\n%s", id, stdout)
		}
	}

	db = openTestDB(t, dbFile)
	defer db.Close()
	bls, err := job.GetBlockers(db, gate)
	if err != nil {
		t.Fatalf("GetBlockers: %v", err)
	}
	if len(bls) != 3 {
		t.Errorf("expected 3 blockers, got %d", len(bls))
	}
}

func TestBlockRemove_MultipleBlockers(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	gate := job.MustAdd(t, db, "", "Gate")
	a := job.MustAdd(t, db, "", "A")
	b := job.MustAdd(t, db, "", "B")
	if err := job.RunBlockMany(db, gate, []string{a, b}, "alice"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	db.Close()

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "block", "remove", gate, "by", a, b); err != nil {
		t.Fatalf("block remove multi: %v", err)
	}

	db = openTestDB(t, dbFile)
	defer db.Close()
	bls, err := job.GetBlockers(db, gate)
	if err != nil {
		t.Fatalf("GetBlockers: %v", err)
	}
	if len(bls) != 0 {
		t.Errorf("expected 0 blockers after multi-remove, got %d", len(bls))
	}
}

func TestRootHelp_GrammarPreamble(t *testing.T) {
	resetFlags()
	t.Cleanup(resetFlags)
	root := newRootCmd()
	help := root.Long
	if !strings.Contains(help, "Multi-operation verbs") {
		t.Errorf("root help should mention 'Multi-operation verbs':\n%s", help)
	}
	if !strings.Contains(help, "Single-operation verbs") {
		t.Errorf("root help should mention 'Single-operation verbs':\n%s", help)
	}
}
