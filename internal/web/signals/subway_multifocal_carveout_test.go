package signals

import (
	"strings"
	"testing"
)

// ------------------------------------------------------------------
// Multi-focal tree-map: same-parent-siblings carve-out
//
// Reference: project/2026-04-27-graph-row-merging.md.
//
// Two focals that share the same parent ride a single row instead
// of triggering a fork. Non-focal siblings between focal stops
// render inline (≤2) or collapse to a mid-line broken-line elision
// (≥3). When the parent ALSO has a non-sibling focal-bearing child
// (a deeper subtree with its own focal), the parent is still a
// fork point — the sibling row and the deeper subtree row both
// branch from it.
// ------------------------------------------------------------------

// rowItemsSummary returns just the stops on a row in order.
func rowItemsSummary(items []LineItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		switch it.Kind {
		case LineItemStop:
			out = append(out, it.ShortID)
		case LineItemElision, LineItemElisionBroken:
			out = append(out, "…")
		case LineItemElisionTerminating:
			out = append(out, "⋯")
		}
	}
	return out
}

// rowSequence returns the row's anchor followed by its items as
// short IDs / elision glyphs. Used in tests where the anchor is a
// rendered node on the row (col 0) and the test asserts on the
// full visible sequence — anchor included.
func rowSequence(line Line) []string {
	out := []string{line.AnchorShortID}
	out = append(out, rowItemsSummary(line.Items)...)
	return out
}

func findRowByAnchor(rows []Line, anchor string) (int, bool) {
	for i, r := range rows {
		if r.AnchorShortID == anchor {
			return i, true
		}
	}
	return -1, false
}

// 2 focal siblings under the same parent → one row, B as anchor,
// both focals as adjacent stops. The parent is NOT a fork point
// under the carve-out rule.
func TestCarveOut_TwoFocalSiblings_OneRow(t *testing.T) {
	w := newTestWorld([]tt{
		{short: "Root", parent: "", status: "available"},
		{short: "B", parent: "Root", status: "available"},
		{short: "C", parent: "B", status: "claimed"},
		{short: "D", parent: "B", status: "claimed"},
	})
	focals := []*graphTask{mustTask(w, "C"), mustTask(w, "D")}

	rows := buildMultiFocalRows(w, focals, 2)

	if len(rows) != 1 {
		t.Fatalf("rows: got %d, want 1 (carve-out: same-parent siblings share a row)", len(rows))
	}
	want := []string{"B", "C", "D"}
	if got := rowSequence(rows[0]); !equalSubwayStrings(got, want) {
		t.Errorf("row sequence (anchor + items): got %v, want %v", got, want)
	}
}

// 3+ focal siblings under the same parent — all adjacent on the
// row.
func TestCarveOut_ThreeFocalSiblings_OneRow(t *testing.T) {
	w := newTestWorld([]tt{
		{short: "Root", parent: "", status: "available"},
		{short: "B", parent: "Root", status: "available"},
		{short: "C", parent: "B", status: "claimed"},
		{short: "D", parent: "B", status: "claimed"},
		{short: "E", parent: "B", status: "claimed"},
	})
	focals := []*graphTask{
		mustTask(w, "C"), mustTask(w, "D"), mustTask(w, "E"),
	}

	rows := buildMultiFocalRows(w, focals, 2)

	if len(rows) != 1 {
		t.Fatalf("rows: got %d, want 1 (carve-out extends to 3+ focal siblings)", len(rows))
	}
	want := []string{"B", "C", "D", "E"}
	if got := rowSequence(rows[0]); !equalSubwayStrings(got, want) {
		t.Errorf("row sequence: got %v, want %v", got, want)
	}
}

// 2 focal siblings with 1 non-focal sibling between them — inline.
func TestCarveOut_FocalsWithOneNonFocalBetween_Inline(t *testing.T) {
	w := newTestWorld([]tt{
		{short: "Root", parent: "", status: "available"},
		{short: "B", parent: "Root", status: "available"},
		{short: "C", parent: "B", status: "claimed"},
		{short: "M", parent: "B", status: "available"},
		{short: "D", parent: "B", status: "claimed"},
	})
	focals := []*graphTask{mustTask(w, "C"), mustTask(w, "D")}

	rows := buildMultiFocalRows(w, focals, 2)

	if len(rows) != 1 {
		t.Fatalf("rows: got %d, want 1", len(rows))
	}
	want := []string{"B", "C", "M", "D"}
	if got := rowSequence(rows[0]); !equalSubwayStrings(got, want) {
		t.Errorf("row sequence: got %v, want %v (one non-focal renders inline)", got, want)
	}
}

// 2 focal siblings with 2 non-focal siblings between them — inline
// (≤2 rule).
func TestCarveOut_FocalsWithTwoNonFocalBetween_Inline(t *testing.T) {
	w := newTestWorld([]tt{
		{short: "Root", parent: "", status: "available"},
		{short: "B", parent: "Root", status: "available"},
		{short: "C", parent: "B", status: "claimed"},
		{short: "M1", parent: "B", status: "available"},
		{short: "M2", parent: "B", status: "available"},
		{short: "D", parent: "B", status: "claimed"},
	})
	focals := []*graphTask{mustTask(w, "C"), mustTask(w, "D")}

	rows := buildMultiFocalRows(w, focals, 2)

	if len(rows) != 1 {
		t.Fatalf("rows: got %d, want 1", len(rows))
	}
	want := []string{"B", "C", "M1", "M2", "D"}
	if got := rowSequence(rows[0]); !equalSubwayStrings(got, want) {
		t.Errorf("row sequence: got %v, want %v (≤2 non-focals render inline)", got, want)
	}
}

// 2 focal siblings with 3+ non-focal siblings between them →
// mid-line broken-line elision between the focal stops.
func TestCarveOut_FocalsWithThreeNonFocalBetween_BrokenElision(t *testing.T) {
	w := newTestWorld([]tt{
		{short: "Root", parent: "", status: "available"},
		{short: "B", parent: "Root", status: "available"},
		{short: "C", parent: "B", status: "claimed"},
		{short: "M1", parent: "B", status: "available"},
		{short: "M2", parent: "B", status: "available"},
		{short: "M3", parent: "B", status: "available"},
		{short: "D", parent: "B", status: "claimed"},
	})
	focals := []*graphTask{mustTask(w, "C"), mustTask(w, "D")}

	rows := buildMultiFocalRows(w, focals, 2)

	if len(rows) != 1 {
		t.Fatalf("rows: got %d, want 1", len(rows))
	}
	want := []string{"B", "C", "…", "D"}
	if got := rowSequence(rows[0]); !equalSubwayStrings(got, want) {
		t.Errorf("row sequence: got %v, want %v (≥3 non-focals collapse to broken-line elision)",
			got, want)
	}
	for _, sid := range []string{"M1", "M2", "M3"} {
		for _, it := range rows[0].Items {
			if it.Kind == LineItemStop && it.ShortID == sid {
				t.Errorf("non-focal sibling %q should be elided, not rendered inline", sid)
			}
		}
	}
}

// Same-parent-siblings group whose parent ALSO has a non-sibling
// focal-bearing child (a deeper subtree with its own focal) — the
// parent IS a fork point. The sibling row carries the focal pair,
// and the deeper subtree gets its own row branching off the parent.
//
//	Root
//	└── B (parent of focal pair AND a deeper subtree)
//	    ├── C [focal]
//	    ├── D [focal]
//	    └── G
//	        └── H [focal]
//
// Expected rows:
//   - sibling-row: B → C → D (carve-out)
//   - deeper-row: G → H (branches off B)
func TestCarveOut_SiblingGroupPlusDeeperSubtree_ParentIsFork(t *testing.T) {
	w := newTestWorld([]tt{
		{short: "Root", parent: "", status: "available"},
		{short: "B", parent: "Root", status: "available"},
		{short: "C", parent: "B", status: "claimed"},
		{short: "D", parent: "B", status: "claimed"},
		{short: "G", parent: "B", status: "available"},
		{short: "H", parent: "G", status: "claimed"},
	})
	focals := []*graphTask{
		mustTask(w, "C"),
		mustTask(w, "D"),
		mustTask(w, "H"),
	}

	rows := buildMultiFocalRows(w, focals, 2)

	if len(rows) < 2 {
		t.Fatalf("rows: got %d, want ≥2 (sibling row + deeper-subtree row)", len(rows))
	}

	// One row carries focals C and D as adjacent stops.
	siblingRowIdx := -1
	for i, r := range rows {
		stops := rowItemsSummary(r.Items)
		hasC, hasD := false, false
		for _, s := range stops {
			if s == "C" {
				hasC = true
			}
			if s == "D" {
				hasD = true
			}
		}
		if hasC && hasD {
			siblingRowIdx = i
			break
		}
	}
	if siblingRowIdx < 0 {
		t.Fatalf("no row carries both C and D: rows=%v", rowsSummary(rows))
	}

	// A separate row carries focal H, and its ParentShortID is B
	// (the parent of the deeper subtree's branch leftmost).
	deeperRowIdx := -1
	for i, r := range rows {
		if i == siblingRowIdx {
			continue
		}
		for _, it := range r.Items {
			if it.Kind == LineItemStop && it.ShortID == "H" {
				deeperRowIdx = i
				break
			}
		}
	}
	if deeperRowIdx < 0 {
		t.Fatalf("no separate row carries focal H: rows=%v", rowsSummary(rows))
	}
	if rows[deeperRowIdx].ParentShortID != "B" {
		t.Errorf("deeper-subtree row ParentShortID: got %q, want %q (branches off B)",
			rows[deeperRowIdx].ParentShortID, "B")
	}
}

func rowsSummary(rows []Line) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		stops := rowItemsSummary(r.Items)
		parent := r.ParentShortID
		if parent == "" {
			parent = "-"
		}
		out[i] = parent + "→" + r.AnchorShortID + ":" + joinStrings(stops, ",")
	}
	return out
}

func joinStrings(parts []string, sep string) string {
	var out strings.Builder
	for i, p := range parts {
		if i > 0 {
			out.WriteString(sep)
		}
		out.WriteString(p)
	}
	return out.String()
}
