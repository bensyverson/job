package signals

import (
	"fmt"
	"testing"
)

// ------------------------------------------------------------------
// Multi-focal tree-map: per-row window + leftmost rendering
//
// Reference: project/2026-04-27-graph-row-merging.md.
//
// Each row computes its own ±N window around its focal(s), all
// within the row's branch only. The row's structural leftmost is
// always rendered as the curve target for any incoming branch
// curve. Backward broken-line elision sits between leftmost and
// the row's first content stop when -N doesn't reach the
// leftmost's first child; trailing terminating elision sits at
// the right edge when +N continues past the row's last visible
// stop.
// ------------------------------------------------------------------

// At a fork on the spine, the first in-subgraph child continues
// row 0 as a stop; remaining in-subgraph children become sub-rows
// anchored on their own leftmost. The cluster LCA is never alone
// on its row.
func TestRowWindow_LeftmostAlwaysRendered(t *testing.T) {
	// Tree:
	//   Root
	//   ├── B → C → D [focal]      (B continues the spine on row 0)
	//   └── E → F [focal]          (E becomes a sub-row, parent=Root)
	w := newTestWorld([]tt{
		{short: "Root", parent: "", status: "available"},
		{short: "B", parent: "Root", status: "available"},
		{short: "C", parent: "B", status: "available"},
		{short: "D", parent: "C", status: "claimed"},
		{short: "E", parent: "Root", status: "available"},
		{short: "F", parent: "E", status: "claimed"},
	})
	focals := []*graphTask{mustTask(w, "D"), mustTask(w, "F")}

	rows := buildMultiFocalRows(w, focals, 2)

	if len(rows) != 2 {
		t.Fatalf("rows: got %d, want 2 (Root spine + E sub-row): %v",
			len(rows), rowsSummary(rows))
	}
	// Row 0: Root spine, B is a stop (not a sub-row anchor).
	if got := rowSequence(rows[0]); !equalSubwayStrings(got, []string{"Root", "B", "C", "D"}) {
		t.Errorf("row 0 sequence: got %v, want [Root B C D]", got)
	}
	if rows[0].ParentShortID != "" {
		t.Errorf("row 0 ParentShortID: got %q, want empty", rows[0].ParentShortID)
	}
	// Row 1: E sub-row branching off Root.
	if got := rowSequence(rows[1]); !equalSubwayStrings(got, []string{"E", "F"}) {
		t.Errorf("row 1 sequence: got %v, want [E F]", got)
	}
	if rows[1].ParentShortID != "Root" {
		t.Errorf("row 1 ParentShortID: got %q, want Root", rows[1].ParentShortID)
	}
}

// -N from the row's focal doesn't reach the row's leftmost (the
// LCA on row 0) → broken-line elision between the leftmost and the
// first content stop in the focal's window. Branch is a stop on the
// spine, not a separate row anchor.
func TestRowWindow_LeadingBrokenElision(t *testing.T) {
	w := newTestWorld([]tt{
		{short: "Root", parent: "", status: "available"},
		{short: "Branch", parent: "Root", status: "available"},
		{short: "s1", parent: "Branch", status: "available"},
		{short: "s2", parent: "s1", status: "available"},
		{short: "s3", parent: "s2", status: "available"},
		{short: "s4", parent: "s3", status: "claimed"},
		{short: "Sib", parent: "Root", status: "available"},
		{short: "SibLeaf", parent: "Sib", status: "claimed"},
	})
	focals := []*graphTask{mustTask(w, "s4"), mustTask(w, "SibLeaf")}

	rows := buildMultiFocalRows(w, focals, 2)

	// Row 0 is the Root spine through Branch; ±2 around s4 leaves a
	// gap between Root (anchor) and s2 → leading broken elision.
	got := rowSequence(rows[0])
	want := []string{"Root", "…", "s2", "s3", "s4"}
	if !equalSubwayStrings(got, want) {
		t.Errorf("row 0 sequence: got %v, want %v", got, want)
	}
	if rows[0].ParentShortID != "" {
		t.Errorf("row 0 ParentShortID: got %q, want empty", rows[0].ParentShortID)
	}
}

// +N from the row's focal continues past the row's last visible
// preorder position → trailing terminating elision at the right
// edge. Under the LCA-spine model, Branch lives on row 0 as a stop;
// the trailing ⋯ marks the off-window non-focal siblings under Branch.
func TestRowWindow_TrailingTerminatingElision(t *testing.T) {
	w := newTestWorld([]tt{
		{short: "Root", parent: "", status: "available"},
		{short: "Branch", parent: "Root", status: "available"},
		{short: "s1", parent: "Branch", status: "claimed"},
		{short: "s2", parent: "Branch", status: "available"},
		{short: "s3", parent: "Branch", status: "available"},
		{short: "s4", parent: "Branch", status: "available"},
		{short: "s5", parent: "Branch", status: "available"},
		{short: "Sib", parent: "Root", status: "available"},
		{short: "SibLeaf", parent: "Sib", status: "claimed"},
	})
	focals := []*graphTask{mustTask(w, "s1"), mustTask(w, "SibLeaf")}

	rows := buildMultiFocalRows(w, focals, 1)

	got := rowSequence(rows[0])
	want := []string{"Root", "Branch", "s1", "s2", "⋯"}
	if !equalSubwayStrings(got, want) {
		t.Errorf("row 0 sequence: got %v, want %v", got, want)
	}
}

// Branch curve from the spine fork lands on the sub-row's leftmost.
// At fork P, the first child A continues the spine on row 0; the
// second child B becomes a sub-row anchored at B with ParentShortID=P.
func TestRowWindow_BranchCurveAnchorsOnLeftmost(t *testing.T) {
	w := newTestWorld([]tt{
		{short: "Root", parent: "", status: "available"},
		{short: "P", parent: "Root", status: "available"},
		{short: "A", parent: "P", status: "available"},
		{short: "AL", parent: "A", status: "claimed"},
		{short: "B", parent: "P", status: "available"},
		{short: "BL", parent: "B", status: "claimed"},
	})
	focals := []*graphTask{mustTask(w, "AL"), mustTask(w, "BL")}

	rows := buildMultiFocalRows(w, focals, 2)

	if len(rows) != 2 {
		t.Fatalf("rows: got %d, want 2 (P spine + B sub-row): %v",
			len(rows), rowsSummary(rows))
	}
	if got := rowSequence(rows[0]); !equalSubwayStrings(got, []string{"P", "A", "AL"}) {
		t.Errorf("row 0 sequence: got %v, want [P A AL]", got)
	}
	if rows[0].ParentShortID != "" {
		t.Errorf("row 0 ParentShortID: got %q, want empty", rows[0].ParentShortID)
	}
	if got := rowSequence(rows[1]); !equalSubwayStrings(got, []string{"B", "BL"}) {
		t.Errorf("row 1 sequence: got %v, want [B BL]", got)
	}
	if rows[1].ParentShortID != "P" {
		t.Errorf("row 1 ParentShortID: got %q, want P", rows[1].ParentShortID)
	}
}

// Reproduces the "step 80/84 vs step 3/4 sibling branches" example
// from the design doc:
//
//	Parent
//	├── Branch1 → s1, s2, … s84 (focal at s80)
//	└── Branch2 → t1, t2, t3, t4 (focal at t3)
//
// Under the LCA-spine model, Branch1 continues row 0 as a stop
// (Parent → Branch1 → … → s78, s79, s80, s81, s82, ⋯) with leading
// broken and trailing terminating elisions; Branch2 becomes a
// sub-row reaching all four steps without elision.
func TestRowWindow_DeepFocalVsShallowFocal_DesignDocExample(t *testing.T) {
	tasks := []tt{
		{short: "Parent", parent: "", status: "available"},
		{short: "Branch1", parent: "Parent", status: "available"},
	}
	for i := 1; i <= 84; i++ {
		st := "available"
		if i == 80 {
			st = "claimed"
		}
		tasks = append(tasks, tt{
			short:  fmt.Sprintf("s%d", i),
			parent: "Branch1",
			status: st,
		})
	}
	tasks = append(tasks, tt{short: "Branch2", parent: "Parent", status: "available"})
	for i := 1; i <= 4; i++ {
		st := "available"
		if i == 3 {
			st = "claimed"
		}
		tasks = append(tasks, tt{
			short:  fmt.Sprintf("t%d", i),
			parent: "Branch2",
			status: st,
		})
	}
	w := newTestWorld(tasks)
	focals := []*graphTask{mustTask(w, "s80"), mustTask(w, "t3")}

	rows := buildMultiFocalRows(w, focals, 2)

	if len(rows) != 2 {
		t.Fatalf("rows: got %d, want 2 (Parent spine + Branch2 sub-row): %v",
			len(rows), rowsSummary(rows))
	}
	// Row 0: Parent spine through Branch1, ±2 around s80 with
	// leading broken (gap to s78) and trailing terminating (s81+
	// past +2).
	got1 := rowSequence(rows[0])
	want1 := []string{"Parent", "…", "s78", "s79", "s80", "s81", "s82", "⋯"}
	if !equalSubwayStrings(got1, want1) {
		t.Errorf("row 0 sequence: got %v, want %v", got1, want1)
	}
	if rows[0].ParentShortID != "" {
		t.Errorf("row 0 ParentShortID: got %q, want empty", rows[0].ParentShortID)
	}
	// Row 1: Branch2 sub-row reaches all four steps.
	got2 := rowSequence(rows[1])
	want2 := []string{"Branch2", "t1", "t2", "t3", "t4"}
	if !equalSubwayStrings(got2, want2) {
		t.Errorf("row 1 sequence: got %v, want %v", got2, want2)
	}
	if rows[1].ParentShortID != "Parent" {
		t.Errorf("row 1 ParentShortID: got %q, want Parent",
			rows[1].ParentShortID)
	}
}
