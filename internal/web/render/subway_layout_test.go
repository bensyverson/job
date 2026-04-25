package render

import (
	"strings"
	"testing"

	"github.com/bensyverson/jobs/internal/web/signals"
)

// ------------------------------------------------------------------
// Subway scenario fixtures
//
// Each helper builds the signals.Subway value that BuildSubway would
// produce for the corresponding scenario from
// project/2026-04-25-graph-clarification.md. Hand-constructed so the
// render tests don't depend on signals package internals.
// ------------------------------------------------------------------

func subwayNode(short string, state signals.SubwayNodeState) signals.SubwayNode {
	return signals.SubwayNode{
		ShortID: short,
		Title:   short,
		State:   state,
		URL:     "/tasks/" + short,
	}
}

func stopItem(short string) signals.LineItem {
	return signals.LineItem{Kind: signals.LineItemStop, ShortID: short}
}

// Scenario 1 — D claimed (C done). One line on B, no fork.
//
//	B → C✓ → [D] → E → F
func scenario1Subway() signals.Subway {
	return signals.Subway{
		Lines: []signals.Line{{
			AnchorShortID: "B",
			Items:         []signals.LineItem{stopItem("C"), stopItem("D"), stopItem("E"), stopItem("F")},
		}},
		Nodes: []signals.SubwayNode{
			subwayNode("B", signals.SubwayNodeTodo),
			subwayNode("C", signals.SubwayNodeDone),
			subwayNode("D", signals.SubwayNodeActive),
			subwayNode("E", signals.SubwayNodeTodo),
			subwayNode("F", signals.SubwayNodeTodo),
		},
		Edges: []signals.SubwayEdge{
			{FromShortID: "B", ToShortID: "C", Kind: signals.SubwayEdgeFlow},
			{FromShortID: "C", ToShortID: "D", Kind: signals.SubwayEdgeFlow},
			{FromShortID: "D", ToShortID: "E", Kind: signals.SubwayEdgeFlow},
			{FromShortID: "E", ToShortID: "F", Kind: signals.SubwayEdgeFlow},
		},
	}
}

// Scenario 2 — D, E claimed; G blocked by B.
//
//	  B → C✓ → [D] [E] → F
//	 /
//	A
//	 ⊘
//	  G → H → I
func scenario2Subway() signals.Subway {
	d := subwayNode("D", signals.SubwayNodeActive)
	d.Actor = "alice"
	e := subwayNode("E", signals.SubwayNodeActive)
	e.Actor = "bob"
	return signals.Subway{
		Lines: []signals.Line{
			{
				AnchorShortID: "B",
				Items:         []signals.LineItem{stopItem("C"), stopItem("D"), stopItem("E"), stopItem("F")},
			},
			{
				AnchorShortID: "G",
				Items:         []signals.LineItem{stopItem("H"), stopItem("I")},
			},
		},
		Fork: &signals.Fork{
			AncestorChain: []string{"A"},
			LineIndices:   []int{0, 1},
		},
		Nodes: []signals.SubwayNode{
			subwayNode("A", signals.SubwayNodeTodo),
			subwayNode("B", signals.SubwayNodeTodo),
			subwayNode("C", signals.SubwayNodeDone),
			d, e,
			subwayNode("F", signals.SubwayNodeTodo),
			subwayNode("G", signals.SubwayNodeTodo),
			subwayNode("H", signals.SubwayNodeTodo),
			subwayNode("I", signals.SubwayNodeTodo),
		},
		Edges: []signals.SubwayEdge{
			{FromShortID: "A", ToShortID: "B", Kind: signals.SubwayEdgeBranch},
			{FromShortID: "A", ToShortID: "G", Kind: signals.SubwayEdgeBranchClosed},
			{FromShortID: "B", ToShortID: "C", Kind: signals.SubwayEdgeFlow},
			{FromShortID: "C", ToShortID: "D", Kind: signals.SubwayEdgeFlow},
			{FromShortID: "D", ToShortID: "E", Kind: signals.SubwayEdgeFlow},
			{FromShortID: "E", ToShortID: "F", Kind: signals.SubwayEdgeFlow},
			{FromShortID: "G", ToShortID: "H", Kind: signals.SubwayEdgeFlow},
			{FromShortID: "H", ToShortID: "I", Kind: signals.SubwayEdgeFlow},
		},
	}
}

// Scenario 3 — D and F claimed, E done between (renders inline).
//
//	  B → C✓ → [D] → E✓ → [F]
//	 /
//	A
//	 ⊘
//	  G → H → I
func scenario3Subway() signals.Subway {
	return signals.Subway{
		Lines: []signals.Line{
			{
				AnchorShortID: "B",
				Items:         []signals.LineItem{stopItem("C"), stopItem("D"), stopItem("E"), stopItem("F")},
			},
			{
				AnchorShortID: "G",
				Items:         []signals.LineItem{stopItem("H"), stopItem("I")},
			},
		},
		Fork: &signals.Fork{AncestorChain: []string{"A"}, LineIndices: []int{0, 1}},
		Nodes: []signals.SubwayNode{
			subwayNode("A", signals.SubwayNodeTodo),
			subwayNode("B", signals.SubwayNodeTodo),
			subwayNode("C", signals.SubwayNodeDone),
			subwayNode("D", signals.SubwayNodeActive),
			subwayNode("E", signals.SubwayNodeDone),
			subwayNode("F", signals.SubwayNodeActive),
			subwayNode("G", signals.SubwayNodeTodo),
			subwayNode("H", signals.SubwayNodeTodo),
			subwayNode("I", signals.SubwayNodeTodo),
		},
		Edges: []signals.SubwayEdge{
			{FromShortID: "A", ToShortID: "B", Kind: signals.SubwayEdgeBranch},
			{FromShortID: "A", ToShortID: "G", Kind: signals.SubwayEdgeBranchClosed},
			{FromShortID: "B", ToShortID: "C", Kind: signals.SubwayEdgeFlow},
			{FromShortID: "C", ToShortID: "D", Kind: signals.SubwayEdgeFlow},
			{FromShortID: "D", ToShortID: "E", Kind: signals.SubwayEdgeFlow},
			{FromShortID: "E", ToShortID: "F", Kind: signals.SubwayEdgeFlow},
			{FromShortID: "G", ToShortID: "H", Kind: signals.SubwayEdgeFlow},
			{FromShortID: "H", ToShortID: "I", Kind: signals.SubwayEdgeFlow},
		},
	}
}

// Scenario 4 — D and H claimed (G unblocked). Three lines, fork at A,
// all branches open.
//
//	  B → C✓ → [D] → E → F
//	 /
//	A → G → [H] → I
//	 \
//	  J → K → L
func scenario4Subway() signals.Subway {
	return signals.Subway{
		Lines: []signals.Line{
			{
				AnchorShortID: "B",
				Items:         []signals.LineItem{stopItem("C"), stopItem("D"), stopItem("E"), stopItem("F")},
			},
			{
				AnchorShortID: "G",
				Items:         []signals.LineItem{stopItem("H"), stopItem("I")},
			},
			{
				AnchorShortID: "J",
				Items:         []signals.LineItem{stopItem("K"), stopItem("L")},
			},
		},
		Fork: &signals.Fork{AncestorChain: []string{"A"}, LineIndices: []int{0, 1, 2}},
		Nodes: []signals.SubwayNode{
			subwayNode("A", signals.SubwayNodeTodo),
			subwayNode("B", signals.SubwayNodeTodo),
			subwayNode("C", signals.SubwayNodeDone),
			subwayNode("D", signals.SubwayNodeActive),
			subwayNode("E", signals.SubwayNodeTodo),
			subwayNode("F", signals.SubwayNodeTodo),
			subwayNode("G", signals.SubwayNodeTodo),
			subwayNode("H", signals.SubwayNodeActive),
			subwayNode("I", signals.SubwayNodeTodo),
			subwayNode("J", signals.SubwayNodeTodo),
			subwayNode("K", signals.SubwayNodeTodo),
			subwayNode("L", signals.SubwayNodeTodo),
		},
		Edges: []signals.SubwayEdge{
			{FromShortID: "A", ToShortID: "B", Kind: signals.SubwayEdgeBranch},
			{FromShortID: "A", ToShortID: "G", Kind: signals.SubwayEdgeBranch},
			{FromShortID: "A", ToShortID: "J", Kind: signals.SubwayEdgeBranch},
			{FromShortID: "B", ToShortID: "C", Kind: signals.SubwayEdgeFlow},
			{FromShortID: "C", ToShortID: "D", Kind: signals.SubwayEdgeFlow},
			{FromShortID: "D", ToShortID: "E", Kind: signals.SubwayEdgeFlow},
			{FromShortID: "E", ToShortID: "F", Kind: signals.SubwayEdgeFlow},
			{FromShortID: "G", ToShortID: "H", Kind: signals.SubwayEdgeFlow},
			{FromShortID: "H", ToShortID: "I", Kind: signals.SubwayEdgeFlow},
			{FromShortID: "J", ToShortID: "K", Kind: signals.SubwayEdgeFlow},
			{FromShortID: "K", ToShortID: "L", Kind: signals.SubwayEdgeFlow},
		},
	}
}

// Scenario 5 — D and K claimed. G has no claim and lookahead doesn't
// reach it. Two lines (B and J), no G-line.
//
//	  B → C✓ → [D] → E → F
//	 /
//	A
//	 \
//	  J → [K] → L
func scenario5Subway() signals.Subway {
	return signals.Subway{
		Lines: []signals.Line{
			{
				AnchorShortID: "B",
				Items:         []signals.LineItem{stopItem("C"), stopItem("D"), stopItem("E"), stopItem("F")},
			},
			{
				AnchorShortID: "J",
				Items:         []signals.LineItem{stopItem("K"), stopItem("L")},
			},
		},
		Fork: &signals.Fork{AncestorChain: []string{"A"}, LineIndices: []int{0, 1}},
		Nodes: []signals.SubwayNode{
			subwayNode("A", signals.SubwayNodeTodo),
			subwayNode("B", signals.SubwayNodeTodo),
			subwayNode("C", signals.SubwayNodeDone),
			subwayNode("D", signals.SubwayNodeActive),
			subwayNode("E", signals.SubwayNodeTodo),
			subwayNode("F", signals.SubwayNodeTodo),
			subwayNode("J", signals.SubwayNodeTodo),
			subwayNode("K", signals.SubwayNodeActive),
			subwayNode("L", signals.SubwayNodeTodo),
		},
		Edges: []signals.SubwayEdge{
			{FromShortID: "A", ToShortID: "B", Kind: signals.SubwayEdgeBranch},
			{FromShortID: "A", ToShortID: "J", Kind: signals.SubwayEdgeBranch},
			{FromShortID: "B", ToShortID: "C", Kind: signals.SubwayEdgeFlow},
			{FromShortID: "C", ToShortID: "D", Kind: signals.SubwayEdgeFlow},
			{FromShortID: "D", ToShortID: "E", Kind: signals.SubwayEdgeFlow},
			{FromShortID: "E", ToShortID: "F", Kind: signals.SubwayEdgeFlow},
			{FromShortID: "J", ToShortID: "K", Kind: signals.SubwayEdgeFlow},
			{FromShortID: "K", ToShortID: "L", Kind: signals.SubwayEdgeFlow},
		},
	}
}

// Scenario 6 — H, K claimed, B's subtree fully done. B's line drops
// out. Fork at A, two open branches.
//
//	A → G → [H] → I
//	 \
//	  J → [K] → L
func scenario6Subway() signals.Subway {
	return signals.Subway{
		Lines: []signals.Line{
			{
				AnchorShortID: "G",
				Items:         []signals.LineItem{stopItem("H"), stopItem("I")},
			},
			{
				AnchorShortID: "J",
				Items:         []signals.LineItem{stopItem("K"), stopItem("L")},
			},
		},
		Fork: &signals.Fork{AncestorChain: []string{"A"}, LineIndices: []int{0, 1}},
		Nodes: []signals.SubwayNode{
			subwayNode("A", signals.SubwayNodeTodo),
			subwayNode("G", signals.SubwayNodeTodo),
			subwayNode("H", signals.SubwayNodeActive),
			subwayNode("I", signals.SubwayNodeTodo),
			subwayNode("J", signals.SubwayNodeTodo),
			subwayNode("K", signals.SubwayNodeActive),
			subwayNode("L", signals.SubwayNodeTodo),
		},
		Edges: []signals.SubwayEdge{
			{FromShortID: "A", ToShortID: "G", Kind: signals.SubwayEdgeBranch},
			{FromShortID: "A", ToShortID: "J", Kind: signals.SubwayEdgeBranch},
			{FromShortID: "G", ToShortID: "H", Kind: signals.SubwayEdgeFlow},
			{FromShortID: "H", ToShortID: "I", Kind: signals.SubwayEdgeFlow},
			{FromShortID: "J", ToShortID: "K", Kind: signals.SubwayEdgeFlow},
			{FromShortID: "K", ToShortID: "L", Kind: signals.SubwayEdgeFlow},
		},
	}
}

// ------------------------------------------------------------------
// Per-view assertion helpers
// ------------------------------------------------------------------

func findSubwayViewNode(v SubwayView, short string) (SubwayNodeView, bool) {
	for _, n := range v.Nodes {
		if n.ShortID == short {
			return n, true
		}
	}
	return SubwayNodeView{}, false
}

func subwayViewEdge(v SubwayView, from, to string) (SubwayEdgeView, bool) {
	for _, e := range v.Edges {
		if e.FromShortID == from && e.ToShortID == to {
			return e, true
		}
	}
	return SubwayEdgeView{}, false
}

func assertNonEmpty(t *testing.T, v SubwayView) {
	t.Helper()
	if v.Empty {
		t.Fatalf("expected non-empty SubwayView")
	}
}

func assertNodePositioned(t *testing.T, v SubwayView, short string) SubwayNodeView {
	t.Helper()
	n, ok := findSubwayViewNode(v, short)
	if !ok {
		t.Fatalf("missing node %q in view; got %v", short, viewNodeIDs(v))
	}
	return n
}

func viewNodeIDs(v SubwayView) []string {
	out := make([]string, len(v.Nodes))
	for i, n := range v.Nodes {
		out[i] = n.ShortID
	}
	return out
}

func assertEdgePresent(t *testing.T, v SubwayView, from, to string) SubwayEdgeView {
	t.Helper()
	e, ok := subwayViewEdge(v, from, to)
	if !ok {
		t.Fatalf("missing edge %s→%s", from, to)
	}
	if e.PathD == "" || !strings.HasPrefix(e.PathD, "M") {
		t.Errorf("edge %s→%s has invalid PathD: %q", from, to, e.PathD)
	}
	return e
}

// ------------------------------------------------------------------
// Empty scenario
// ------------------------------------------------------------------

func TestLayoutSubway_EmptyReturnsEmptyView(t *testing.T) {
	v := LayoutSubway(signals.Subway{})
	if !v.Empty {
		t.Errorf("Empty: got false, want true")
	}
	if len(v.Nodes) != 0 || len(v.Edges) != 0 || len(v.Lines) != 0 {
		t.Errorf("expected nothing in view, got %d nodes / %d edges / %d lines",
			len(v.Nodes), len(v.Edges), len(v.Lines))
	}
}

// ------------------------------------------------------------------
// Scenario 1 — single line, no fork
// ------------------------------------------------------------------

func TestLayoutSubway_Scenario1_SingleLineNoFork(t *testing.T) {
	v := LayoutSubway(scenario1Subway())
	assertNonEmpty(t, v)

	if len(v.Lines) != 1 {
		t.Fatalf("Lines: got %d, want 1", len(v.Lines))
	}
	if v.Fork != nil {
		t.Errorf("Fork: got %+v, want nil for single line", v.Fork)
	}
	if v.Lines[0].AnchorShortID != "B" {
		t.Errorf("anchor: got %q, want B", v.Lines[0].AnchorShortID)
	}

	// All five nodes (B + C/D/E/F) get positioned on the same row.
	rowB := assertNodePositioned(t, v, "B").Top
	for _, s := range []string{"C", "D", "E", "F"} {
		got := assertNodePositioned(t, v, s).Top
		if got != rowB {
			t.Errorf("%s top: got %d, want %d (same row as anchor)", s, got, rowB)
		}
	}

	// D is the lit (active) stop.
	d := assertNodePositioned(t, v, "D")
	if !d.IsActive {
		t.Errorf("D should render as active")
	}
	c := assertNodePositioned(t, v, "C")
	if !c.IsDone {
		t.Errorf("C should render as done")
	}

	// Line is monotonic left-to-right: B < C < D < E < F.
	prev := assertNodePositioned(t, v, "B").Left
	for _, s := range []string{"C", "D", "E", "F"} {
		got := assertNodePositioned(t, v, s).Left
		if got <= prev {
			t.Errorf("%s left: got %d, want > %d (monotonic)", s, got, prev)
		}
		prev = got
	}

	// Flow edges between adjacent stops, no branch / closure edges.
	for _, p := range [][2]string{{"B", "C"}, {"C", "D"}, {"D", "E"}, {"E", "F"}} {
		e := assertEdgePresent(t, v, p[0], p[1])
		if !e.IsFlow {
			t.Errorf("edge %s→%s should be Flow", p[0], p[1])
		}
		if e.IsBranch || e.IsClosure {
			t.Errorf("edge %s→%s should not be Branch/Closure", p[0], p[1])
		}
	}
}

// ------------------------------------------------------------------
// Scenario 2 — fork at A, BranchClosed ingress to G
// ------------------------------------------------------------------

func TestLayoutSubway_Scenario2_BranchClosedToG(t *testing.T) {
	v := LayoutSubway(scenario2Subway())
	assertNonEmpty(t, v)

	if len(v.Lines) != 2 {
		t.Fatalf("Lines: got %d, want 2", len(v.Lines))
	}
	if v.Fork == nil {
		t.Fatalf("Fork: got nil, want non-nil")
	}
	if len(v.Fork.AncestorShortIDs) == 0 || v.Fork.AncestorShortIDs[0] != "A" {
		t.Errorf("Fork ancestors: got %v, want [A...]", v.Fork.AncestorShortIDs)
	}

	a := assertNodePositioned(t, v, "A")
	if !a.IsForkAncestor {
		t.Errorf("A should be marked as fork ancestor")
	}

	// B and G sit on distinct rows.
	b := assertNodePositioned(t, v, "B")
	g := assertNodePositioned(t, v, "G")
	if b.Top == g.Top {
		t.Errorf("B and G should sit on distinct rows; both at top=%d", b.Top)
	}

	// Branch edges from A to each line anchor.
	branchAB := assertEdgePresent(t, v, "A", "B")
	if !branchAB.IsBranch {
		t.Errorf("A→B should be Branch")
	}
	if branchAB.IsClosure {
		t.Errorf("A→B should not carry a closure marker")
	}
	branchAG := assertEdgePresent(t, v, "A", "G")
	if !branchAG.IsBranch {
		t.Errorf("A→G should be Branch")
	}
	if !branchAG.IsClosure {
		t.Errorf("A→G should carry a closure marker (BranchClosed)")
	}

	// The active claims D and E render as lit stops on B's line.
	for _, s := range []string{"D", "E"} {
		n := assertNodePositioned(t, v, s)
		if !n.IsActive {
			t.Errorf("%s should render as active (lit stop)", s)
		}
	}
}

// ------------------------------------------------------------------
// Scenario 3 — done sibling between focals renders inline
// ------------------------------------------------------------------

func TestLayoutSubway_Scenario3_DoneInlineBetweenFocals(t *testing.T) {
	v := LayoutSubway(scenario3Subway())
	assertNonEmpty(t, v)

	if len(v.Lines) != 2 {
		t.Fatalf("Lines: got %d, want 2", len(v.Lines))
	}

	// E is positioned and marked done.
	e := assertNodePositioned(t, v, "E")
	if !e.IsDone {
		t.Errorf("E should render as done")
	}

	// E sits between D and F on the same row and in monotonic x order.
	d := assertNodePositioned(t, v, "D")
	f := assertNodePositioned(t, v, "F")
	if !(d.Top == e.Top && e.Top == f.Top) {
		t.Errorf("D, E, F should share a row; got %d, %d, %d", d.Top, e.Top, f.Top)
	}
	if !(d.Left < e.Left && e.Left < f.Left) {
		t.Errorf("D < E < F left positions expected; got %d, %d, %d", d.Left, e.Left, f.Left)
	}

	// Closure marker still on G ingress.
	branchAG := assertEdgePresent(t, v, "A", "G")
	if !branchAG.IsClosure {
		t.Errorf("A→G should carry a closure marker")
	}
}

// ------------------------------------------------------------------
// Scenario 4 — three lines, fork at A, all branches open
// ------------------------------------------------------------------

func TestLayoutSubway_Scenario4_ThreeLinesAllOpen(t *testing.T) {
	v := LayoutSubway(scenario4Subway())
	assertNonEmpty(t, v)

	if len(v.Lines) != 3 {
		t.Fatalf("Lines: got %d, want 3", len(v.Lines))
	}
	if v.Fork == nil {
		t.Fatalf("Fork: got nil, want non-nil")
	}

	// All three branch ingress edges open (no closure markers).
	for _, anchor := range []string{"B", "G", "J"} {
		e := assertEdgePresent(t, v, "A", anchor)
		if !e.IsBranch {
			t.Errorf("A→%s should be Branch", anchor)
		}
		if e.IsClosure {
			t.Errorf("A→%s should not carry a closure marker", anchor)
		}
	}

	// Each line anchor sits on a distinct row.
	rows := map[int]string{}
	for _, anchor := range []string{"B", "G", "J"} {
		n := assertNodePositioned(t, v, anchor)
		if other, dup := rows[n.Top]; dup {
			t.Errorf("anchors %s and %s share row at top=%d", other, anchor, n.Top)
		}
		rows[n.Top] = anchor
	}
}

// ------------------------------------------------------------------
// Scenario 5 — two lines, G absent
// ------------------------------------------------------------------

func TestLayoutSubway_Scenario5_TwoLinesGAbsent(t *testing.T) {
	v := LayoutSubway(scenario5Subway())
	assertNonEmpty(t, v)

	if len(v.Lines) != 2 {
		t.Fatalf("Lines: got %d, want 2", len(v.Lines))
	}
	if _, ok := findSubwayViewNode(v, "G"); ok {
		t.Errorf("G should not appear in view; got nodes %v", viewNodeIDs(v))
	}
	for _, anchor := range []string{"B", "J"} {
		e := assertEdgePresent(t, v, "A", anchor)
		if !e.IsBranch || e.IsClosure {
			t.Errorf("A→%s: want open Branch, got branch=%v closure=%v",
				anchor, e.IsBranch, e.IsClosure)
		}
	}
}

// ------------------------------------------------------------------
// Scenario 6 — B's subtree dropped out
// ------------------------------------------------------------------

func TestLayoutSubway_Scenario6_BSubtreeDropsOut(t *testing.T) {
	v := LayoutSubway(scenario6Subway())
	assertNonEmpty(t, v)

	if len(v.Lines) != 2 {
		t.Fatalf("Lines: got %d, want 2", len(v.Lines))
	}
	if _, ok := findSubwayViewNode(v, "B"); ok {
		t.Errorf("B should not appear (subtree fully done); got %v", viewNodeIDs(v))
	}
	for _, anchor := range []string{"G", "J"} {
		e := assertEdgePresent(t, v, "A", anchor)
		if !e.IsBranch || e.IsClosure {
			t.Errorf("A→%s: want open Branch", anchor)
		}
	}
}

// ------------------------------------------------------------------
// Canvas + URL plumbing
// ------------------------------------------------------------------

// CanvasW and CanvasH must be positive when there's anything to
// render, so the template can size the SVG container.
func TestLayoutSubway_CanvasDimensionsPositive(t *testing.T) {
	v := LayoutSubway(scenario4Subway())
	if v.CanvasW <= 0 || v.CanvasH <= 0 {
		t.Errorf("Canvas: got (%d, %d), want both positive", v.CanvasW, v.CanvasH)
	}
}

// Node URLs must round-trip from the underlying signals.Subway.
func TestLayoutSubway_NodeURLsPreserved(t *testing.T) {
	v := LayoutSubway(scenario1Subway())
	d := assertNodePositioned(t, v, "D")
	if d.URL != "/tasks/D" {
		t.Errorf("D URL: got %q, want %q", d.URL, "/tasks/D")
	}
}
