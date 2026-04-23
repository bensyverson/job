package job

import (
	"bytes"
	"strings"
	"testing"
)

// R2 — `job summary <parent>` fills the gap between `status` (whole DB,
// too broad) and `list … all` (full subtree, too wide).
//
// Two-level shape: rollup of the target plus one rollup line per direct
// child. Stops there — `list` covers deep trees.

func TestRunSummary_TargetWithMixedChildren(t *testing.T) {
	db := SetupTestDB(t)
	root := MustAdd(t, db, "", "Project")
	a := MustAdd(t, db, root, "Phase A")
	b := MustAdd(t, db, root, "Phase B")
	a1 := MustAdd(t, db, a, "A1")
	a2 := MustAdd(t, db, a, "A2")
	MustAdd(t, db, b, "B1")
	MustAdd(t, db, b, "B2")
	MustDone(t, db, a1)
	MustDone(t, db, a2)

	s, err := RunSummary(db, root)
	if err != nil {
		t.Fatalf("RunSummary: %v", err)
	}
	if s.Target.Done != 3 {
		t.Errorf("Target.Done: got %d, want 3 (a1, a2, and Phase A auto-closed)", s.Target.Done)
	}
	if s.Target.Open != 3 {
		t.Errorf("Target.Open: got %d, want 3 (Phase B + B1 + B2)", s.Target.Open)
	}
	if len(s.DirectChildren) != 2 {
		t.Fatalf("DirectChildren: got %d, want 2", len(s.DirectChildren))
	}
	if s.DirectChildren[0].Status != "done" {
		t.Errorf("Phase A status: got %q, want done", s.DirectChildren[0].Status)
	}
	if s.DirectChildren[1].Done != 0 || s.DirectChildren[1].Open != 2 {
		t.Errorf("Phase B counts: got %d done %d open, want 0,2", s.DirectChildren[1].Done, s.DirectChildren[1].Open)
	}
}

func TestRunSummary_TargetWithBlockedAndClaimed(t *testing.T) {
	db := SetupTestDB(t)
	root := MustAdd(t, db, "", "Root")
	a := MustAdd(t, db, root, "A")
	b := MustAdd(t, db, root, "B")
	c := MustAdd(t, db, root, "C")
	if err := RunBlock(db, b, a, TestActor); err != nil {
		t.Fatalf("block: %v", err)
	}
	if err := RunClaim(db, c, "1h", "alice", false); err != nil {
		t.Fatalf("claim: %v", err)
	}

	s, err := RunSummary(db, root)
	if err != nil {
		t.Fatalf("RunSummary: %v", err)
	}
	if s.Target.Blocked != 1 {
		t.Errorf("Blocked: got %d, want 1", s.Target.Blocked)
	}
	if s.Target.InFlight != 1 {
		t.Errorf("InFlight: got %d, want 1", s.Target.InFlight)
	}
	if s.Target.Available != 1 {
		t.Errorf("Available: got %d, want 1 (only A is claimable now)", s.Target.Available)
	}
	if s.Target.NextID != a {
		t.Errorf("NextID: got %q, want %q (A)", s.Target.NextID, a)
	}
}

func TestRunSummary_LeafTarget(t *testing.T) {
	db := SetupTestDB(t)
	id := MustAdd(t, db, "", "Lonely")

	s, err := RunSummary(db, id)
	if err != nil {
		t.Fatalf("RunSummary: %v", err)
	}
	if s.Target.Open != 0 || s.Target.Done != 0 {
		t.Errorf("leaf target should have 0 descendants: got %+v", s.Target)
	}
	if len(s.DirectChildren) != 0 {
		t.Errorf("leaf target should have no children: got %d", len(s.DirectChildren))
	}
}

func TestRunSummary_NotFound(t *testing.T) {
	db := SetupTestDB(t)
	_, err := RunSummary(db, "noExs")
	if err == nil {
		t.Fatal("expected error for missing task")
	}
}

func TestRenderSummary_FormatsRollup(t *testing.T) {
	db := SetupTestDB(t)
	root := MustAdd(t, db, "", "Project")
	a := MustAdd(t, db, root, "Phase A")
	b := MustAdd(t, db, root, "Phase B")
	a1 := MustAdd(t, db, a, "A1")
	MustAdd(t, db, b, "B1")
	MustDone(t, db, a1)

	s, err := RunSummary(db, root)
	if err != nil {
		t.Fatalf("RunSummary: %v", err)
	}
	var buf bytes.Buffer
	RenderSummary(&buf, s)
	got := buf.String()

	if !strings.Contains(got, "Project") || !strings.Contains(got, root) {
		t.Errorf("title/id missing:\n%s", got)
	}
	if !strings.Contains(got, "of") || !strings.Contains(got, "done") {
		t.Errorf("rollup line missing:\n%s", got)
	}
	if !strings.Contains(got, "Phase A") || !strings.Contains(got, "Phase B") {
		t.Errorf("direct children missing:\n%s", got)
	}
	// Phase A is fully done — should display its terminal state, not "0 of 0".
	if !strings.Contains(got, "Phase A") {
		t.Errorf("Phase A line missing")
	}
	// "Direct child line for Phase B should advertise its first available
	// task" — B1 is the only candidate.
	if !strings.Contains(got, b) {
		t.Errorf("Phase B id missing:\n%s", got)
	}
}
