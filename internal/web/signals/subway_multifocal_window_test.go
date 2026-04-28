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

// Row's structural leftmost is always rendered as the row's
// anchor (col 0). Each branch off a fork point becomes its own
// row at the data layer; the layout step (S3) handles visual
// inlining.
func TestRowWindow_LeftmostAlwaysRendered(t *testing.T) {
	// Tree:
	//   Root
	//   ├── B → C → D [focal]
	//   └── E → F [focal]
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

	// Three rows: top (Root), B's branch, E's branch.
	for _, want := range []string{"Root", "B", "E"} {
		idx, ok := findRowByAnchor(rows, want)
		if !ok {
			t.Errorf("no row anchored on %q (leftmost must always be rendered): rows=%v",
				want, rowsSummary(rows))
			continue
		}
		switch want {
		case "Root":
			if rows[idx].ParentShortID != "" {
				t.Errorf("Root (top row) ParentShortID: got %q, want empty",
					rows[idx].ParentShortID)
			}
		case "B", "E":
			if rows[idx].ParentShortID != "Root" {
				t.Errorf("%q sub-row ParentShortID: got %q, want %q",
					want, rows[idx].ParentShortID, "Root")
			}
		}
	}
}

// -N from the row's focal doesn't reach the leftmost's first
// child → broken-line elision between the leftmost (anchor) and
// the first content stop in the focal's window.
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

	// Branch sub-row: anchor=Branch, items=[…, s2, s3, s4].
	idx, ok := findRowByAnchor(rows, "Branch")
	if !ok {
		t.Fatalf("no sub-row anchored on Branch: rows=%v", rowsSummary(rows))
	}
	got := rowSequence(rows[idx])
	want := []string{"Branch", "…", "s2", "s3", "s4"}
	if !equalSubwayStrings(got, want) {
		t.Errorf("Branch row sequence: got %v, want %v", got, want)
	}
	if rows[idx].ParentShortID != "Root" {
		t.Errorf("Branch ParentShortID: got %q, want %q",
			rows[idx].ParentShortID, "Root")
	}
}

// +N from the row's focal continues past the row's branch →
// terminating ellipsis at the row's right edge.
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

	idx, ok := findRowByAnchor(rows, "Branch")
	if !ok {
		t.Fatalf("no sub-row anchored on Branch: rows=%v", rowsSummary(rows))
	}
	got := rowSequence(rows[idx])
	want := []string{"Branch", "s1", "s2", "⋯"}
	if !equalSubwayStrings(got, want) {
		t.Errorf("Branch row sequence: got %v, want %v", got, want)
	}
}

// Branch curve from parent's rendered position lands on each
// row's leftmost. With two sibling branches under fork point P,
// each becomes a sub-row whose ParentShortID = P at the data
// layer.
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

	for _, want := range []string{"A", "B"} {
		idx, ok := findRowByAnchor(rows, want)
		if !ok {
			t.Errorf("no sub-row anchored on %q: rows=%v",
				want, rowsSummary(rows))
			continue
		}
		if rows[idx].ParentShortID != "P" {
			t.Errorf("%q sub-row ParentShortID: got %q, want %q",
				want, rows[idx].ParentShortID, "P")
		}
	}
}

// Reproduces the "step 80/84 vs step 3/4 sibling branches" example
// from the design doc:
//
//	Parent
//	├── Branch1 → s1 → s2 → ... → s84 (focal at s80)
//	└── Branch2 → t1 → t2 → t3 → t4   (focal at t3)
//
// Branch1's row has leading broken-line elision (s78, s79 are the
// only -N=2 steps back from s80, far short of Branch1's first
// child s1) and trailing terminating elision (s81, s82, … past +N=2
// out of the row's continued branch). Branch2's row reaches all
// the way back to Branch2's first child t1 (no leading elision)
// and stops at t4 (no trailing elision).
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

	// Branch1 sub-row: anchor=Branch1, parent=Parent, items
	// reflect the ±2 window around s80 with leading broken and
	// trailing terminating elisions.
	idx1, ok := findRowByAnchor(rows, "Branch1")
	if !ok {
		t.Fatalf("no sub-row anchored on Branch1: rows=%v", rowsSummary(rows))
	}
	if rows[idx1].ParentShortID != "Parent" {
		t.Errorf("Branch1 sub-row ParentShortID: got %q, want %q",
			rows[idx1].ParentShortID, "Parent")
	}
	got1 := rowSequence(rows[idx1])
	want1 := []string{"Branch1", "…", "s78", "s79", "s80", "s81", "s82", "⋯"}
	if !equalSubwayStrings(got1, want1) {
		t.Errorf("Branch1 sub-row sequence: got %v, want %v", got1, want1)
	}

	// Branch2 sub-row: Branch2 anchor + all four steps, no elisions
	// (-N reaches t1, +N reaches t4). ParentShortID = Parent.
	idx2, ok := findRowByAnchor(rows, "Branch2")
	if !ok {
		t.Fatalf("no row for Branch2: rows=%v", rowsSummary(rows))
	}
	if rows[idx2].ParentShortID != "Parent" {
		t.Errorf("Branch2 sub-row ParentShortID: got %q, want %q",
			rows[idx2].ParentShortID, "Parent")
	}
	got2 := rowSequence(rows[idx2])
	want2 := []string{"Branch2", "t1", "t2", "t3", "t4"}
	if !equalSubwayStrings(got2, want2) {
		t.Errorf("Branch2 row sequence: got %v, want %v", got2, want2)
	}
}
