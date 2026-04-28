package render

import (
	"fmt"
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
		Forks: []*signals.Fork{{
			AncestorChain: []string{"A"},
			LineIndices:   []int{0, 1},
		}},
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
		Forks: []*signals.Fork{{AncestorChain: []string{"A"}, LineIndices: []int{0, 1}}},
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
		Forks: []*signals.Fork{{AncestorChain: []string{"A"}, LineIndices: []int{0, 1, 2}}},
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
		Forks: []*signals.Fork{{AncestorChain: []string{"A"}, LineIndices: []int{0, 1}}},
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
		Forks: []*signals.Fork{{AncestorChain: []string{"A"}, LineIndices: []int{0, 1}}},
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
	if len(v.Forks) != 0 {
		t.Errorf("Forks: got %d, want 0 for single line", len(v.Forks))
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
	if len(v.Forks) == 0 {
		t.Fatalf("Forks: got 0, want non-empty")
	}
	if len(v.Forks[0].AncestorShortIDs) == 0 || v.Forks[0].AncestorShortIDs[0] != "A" {
		t.Errorf("Fork ancestors: got %v, want [A...]", v.Forks[0].AncestorShortIDs)
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
	if len(v.Forks) == 0 {
		t.Fatalf("Forks: got 0, want non-empty")
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

// ------------------------------------------------------------------
// Closure-marker placement (covers Scenarios 2 and 3)
//
// Per the design doc: "The closure marker `⊘` lives on the connector
// edge from the fork to the blocked line's parent, not on the parent
// node itself." Tests pin the marker's geometry so it can never drift
// onto the anchor node.
// ------------------------------------------------------------------

// closurePosition strictly between two endpoints (exclusive). Used to
// assert the marker sits along the ingress edge geometry rather than
// at either endpoint.
func assertBetween(t *testing.T, label string, got, lo, hi int) {
	t.Helper()
	if lo > hi {
		lo, hi = hi, lo
	}
	if got <= lo || got >= hi {
		t.Errorf("%s: got %d, want strictly within (%d, %d)", label, got, lo, hi)
	}
}

// Scenario 2 — A → G is BranchClosed. Closure marker must sit on the
// edge geometry between A and G, never on G's anchor disc.
func TestLayoutSubway_Scenario2_ClosureMarkerOnIngressEdge(t *testing.T) {
	v := LayoutSubway(scenario2Subway())

	a := assertNodePositioned(t, v, "A")
	g := assertNodePositioned(t, v, "G")

	e := assertEdgePresent(t, v, "A", "G")
	if !e.IsClosure {
		t.Fatalf("A→G should be a closure edge")
	}

	// The marker must be positioned (non-zero coordinates).
	if e.ClosureLeft == 0 && e.ClosureTop == 0 {
		t.Errorf("closure marker not positioned: ClosureLeft=%d ClosureTop=%d",
			e.ClosureLeft, e.ClosureTop)
	}

	// The marker must sit *between* A and G — not on either anchor's
	// disc center. "Strictly between" rules out a stray placement on
	// the line's parent node itself, which the spec forbids.
	aCX, aCY := a.Left+subwayNodeRadius, a.Top+subwayNodeRadius
	gCX, gCY := g.Left+subwayNodeRadius, g.Top+subwayNodeRadius
	assertBetween(t, "A→G ClosureLeft", e.ClosureLeft, aCX, gCX)
	assertBetween(t, "A→G ClosureTop", e.ClosureTop, aCY, gCY)

	// Negative: the marker must not coincide with G's disc.
	if e.ClosureLeft == gCX && e.ClosureTop == gCY {
		t.Errorf("closure marker collided with G anchor center (%d,%d)", gCX, gCY)
	}

	// Negative: open Branch edges must not carry closure coordinates.
	openAB := assertEdgePresent(t, v, "A", "B")
	if openAB.IsClosure {
		t.Errorf("A→B should not be a closure edge")
	}
	if openAB.ClosureLeft != 0 || openAB.ClosureTop != 0 {
		t.Errorf("open Branch A→B should have zero closure coords; got (%d,%d)",
			openAB.ClosureLeft, openAB.ClosureTop)
	}
}

// Scenario 3 — same ingress block; the inline E✓ done sibling on B's
// line must not affect the closure-marker placement on A→G.
func TestLayoutSubway_Scenario3_ClosureMarkerStillOnIngress(t *testing.T) {
	v := LayoutSubway(scenario3Subway())

	a := assertNodePositioned(t, v, "A")
	g := assertNodePositioned(t, v, "G")

	e := assertEdgePresent(t, v, "A", "G")
	if !e.IsClosure {
		t.Fatalf("A→G should be a closure edge")
	}
	if e.ClosureLeft == 0 && e.ClosureTop == 0 {
		t.Errorf("closure marker not positioned: ClosureLeft=%d ClosureTop=%d",
			e.ClosureLeft, e.ClosureTop)
	}

	aCX, aCY := a.Left+subwayNodeRadius, a.Top+subwayNodeRadius
	gCX, gCY := g.Left+subwayNodeRadius, g.Top+subwayNodeRadius
	assertBetween(t, "A→G ClosureLeft", e.ClosureLeft, aCX, gCX)
	assertBetween(t, "A→G ClosureTop", e.ClosureTop, aCY, gCY)
}

// ------------------------------------------------------------------
// Elision & More-pill positioning
//
// In-gap elision sits in the gap between two surrounding items
// (anchor↔stop, stop↔stop) without consuming a column slot. The
// trailing (+N) "More" pill consumes a slot at the end of the line
// and carries the count of hidden trailing siblings. The connector
// from the last stop terminates at the More pill — addressed via
// the synthetic short ID returned by signals.MoreShortID.
// ------------------------------------------------------------------

// elisionScenarioSubway builds a single-line scenario with a leading
// elision, a mid-line elision, and a trailing (+N) pill, reflecting
// the structure applyWindow emits when two distant focals leave a
// gap and trailing siblings remain hidden.
//
//	B  ·  c02 c03  ·  c07 c08 c09  (+5)
//
// (`·` denotes the in-gap elision dots.)
func elisionScenarioSubway() signals.Subway {
	return signals.Subway{
		Lines: []signals.Line{{
			AnchorShortID: "B",
			Items: []signals.LineItem{
				{Kind: signals.LineItemElision},
				stopItem("c02"), stopItem("c03"),
				{Kind: signals.LineItemElision},
				stopItem("c07"), stopItem("c08"), stopItem("c09"),
				{Kind: signals.LineItemMore, MoreCount: 5},
			},
		}},
		Nodes: []signals.SubwayNode{
			subwayNode("B", signals.SubwayNodeTodo),
			subwayNode("c02", signals.SubwayNodeTodo),
			subwayNode("c03", signals.SubwayNodeActive),
			subwayNode("c07", signals.SubwayNodeActive),
			subwayNode("c08", signals.SubwayNodeTodo),
			subwayNode("c09", signals.SubwayNodeTodo),
		},
		Edges: []signals.SubwayEdge{
			{FromShortID: "B", ToShortID: "c02", Kind: signals.SubwayEdgeFlow},
			{FromShortID: "c02", ToShortID: "c03", Kind: signals.SubwayEdgeFlow},
			{FromShortID: "c03", ToShortID: "c07", Kind: signals.SubwayEdgeFlow},
			{FromShortID: "c07", ToShortID: "c08", Kind: signals.SubwayEdgeFlow},
			{FromShortID: "c08", ToShortID: "c09", Kind: signals.SubwayEdgeFlow},
			{FromShortID: "c09", ToShortID: signals.MoreShortID("B"), Kind: signals.SubwayEdgeFlow},
		},
	}
}

// In-gap elisions don't consume column slots: the first visible stop
// after a leading elision sits one column past the anchor, not two.
func TestLayoutSubway_LeadingElision_DoesNotConsumeSlot(t *testing.T) {
	v := LayoutSubway(elisionScenarioSubway())
	assertNonEmpty(t, v)

	b := assertNodePositioned(t, v, "B")
	c02 := assertNodePositioned(t, v, "c02")
	if c02.Left-b.Left != subwayColStep {
		t.Errorf("c02 should sit one column past B; got Left diff %d, want %d",
			c02.Left-b.Left, subwayColStep)
	}
}

// The leading-elision view sits at the midpoint between the anchor
// disc center and the first visible stop's disc center, on the
// anchor's row.
func TestLayoutSubway_LeadingElision_PositionedBetweenAnchorAndFirstStop(t *testing.T) {
	v := LayoutSubway(elisionScenarioSubway())

	if len(v.Elisions) < 1 {
		t.Fatalf("Elisions: got 0, want at least 1 (leading)")
	}
	b := assertNodePositioned(t, v, "B")
	c02 := assertNodePositioned(t, v, "c02")
	wantLeft := (b.Left + c02.Left) / 2
	wantLeft += subwayNodeRadius
	wantTop := b.Top + subwayNodeRadius

	leading := v.Elisions[0]
	if leading.Left != wantLeft {
		t.Errorf("leading elision Left: got %d, want %d (midpoint of B↔c02 centers)",
			leading.Left, wantLeft)
	}
	if leading.Top != wantTop {
		t.Errorf("leading elision Top: got %d, want %d (anchor row center)",
			leading.Top, wantTop)
	}
}

// The mid-line elision sits at the midpoint between the two stops it
// separates — c03 and c07 in the fixture.
func TestLayoutSubway_MidLineElision_PositionedBetweenStops(t *testing.T) {
	v := LayoutSubway(elisionScenarioSubway())

	if len(v.Elisions) < 2 {
		t.Fatalf("Elisions: got %d, want 2 (leading + mid-line)", len(v.Elisions))
	}
	c03 := assertNodePositioned(t, v, "c03")
	c07 := assertNodePositioned(t, v, "c07")
	wantLeft := (c03.Left+c07.Left)/2 + subwayNodeRadius
	wantTop := c03.Top + subwayNodeRadius

	mid := v.Elisions[1]
	if mid.Left != wantLeft {
		t.Errorf("mid-line elision Left: got %d, want %d (midpoint of c03↔c07 centers)",
			mid.Left, wantLeft)
	}
	if mid.Top != wantTop {
		t.Errorf("mid-line elision Top: got %d, want %d", mid.Top, wantTop)
	}
}

// The trailing (+N) pill consumes one column slot past the last
// visible stop and carries the hidden-sibling count.
func TestLayoutSubway_TrailingMore_TakesSlotAndCarriesCount(t *testing.T) {
	v := LayoutSubway(elisionScenarioSubway())

	if len(v.Mores) != 1 {
		t.Fatalf("Mores: got %d, want 1", len(v.Mores))
	}
	more := v.Mores[0]
	if more.Count != 5 {
		t.Errorf("More count: got %d, want 5", more.Count)
	}
	c09 := assertNodePositioned(t, v, "c09")
	if more.Left-c09.Left != subwayColStep {
		t.Errorf("More pill should sit one column past c09; got Left diff %d, want %d",
			more.Left-c09.Left, subwayColStep)
	}
	if more.Top != c09.Top {
		t.Errorf("More pill should share the line's row; got Top %d, want %d",
			more.Top, c09.Top)
	}
}

// The Flow edge from the last visible stop to the trailing More
// pill is positioned and routed (non-empty PathD).
func TestLayoutSubway_TrailingMore_EdgeFromLastStopArrives(t *testing.T) {
	v := LayoutSubway(elisionScenarioSubway())

	moreID := signals.MoreShortID("B")
	e := assertEdgePresent(t, v, "c09", moreID)
	if !e.IsFlow {
		t.Errorf("c09 → %q should be Flow; got IsBranch=%v IsBlocker=%v",
			moreID, e.IsBranch, e.IsBlocker)
	}
}

// Regression for ?at=1288: when line 1's anchor is also a stop on
// line 0 (line 1's parent is a child of the LCA, which IS line 0's
// anchor), the layout must place line 1's anchor on line 1's row.
// Before the fix, the line loop wrote cols[anchor] but never wrote
// rows[anchor], so line 1's anchor inherited row 0 from the line 0
// pass — piling its disc on top of line 0's anchor and producing a
// zero-length, visually-backward edge to its first stop.
func scenarioLineAnchorIsStopOnPriorLineSubway() signals.Subway {
	return signals.Subway{
		Lines: []signals.Line{
			{
				AnchorShortID: "A",
				Items:         []signals.LineItem{stopItem("B"), stopItem("X")},
			},
			{
				AnchorShortID: "B",
				Items:         []signals.LineItem{stopItem("Y")},
			},
		},
		Forks: []*signals.Fork{{
			AncestorChain: []string{"A"},
			LineIndices:   []int{0, 1},
		}},
		Nodes: []signals.SubwayNode{
			subwayNode("A", signals.SubwayNodeTodo),
			subwayNode("B", signals.SubwayNodeTodo),
			subwayNode("X", signals.SubwayNodeActive),
			subwayNode("Y", signals.SubwayNodeActive),
		},
		Edges: []signals.SubwayEdge{
			{FromShortID: "A", ToShortID: "B", Kind: signals.SubwayEdgeFlow},
			{FromShortID: "B", ToShortID: "X", Kind: signals.SubwayEdgeFlow},
			{FromShortID: "B", ToShortID: "Y", Kind: signals.SubwayEdgeFlow},
		},
	}
}

func TestLayoutSubway_LineAnchorThatIsAlsoStop_RendersOnItsOwnRow(t *testing.T) {
	v := LayoutSubway(scenarioLineAnchorIsStopOnPriorLineSubway())
	a := assertNodePositioned(t, v, "A")
	b := assertNodePositioned(t, v, "B")

	if b.Top == a.Top {
		t.Errorf("B (line 1 anchor) should not share line 0 anchor's row; both at top=%d", b.Top)
	}
	// The line loop assigns rows monotonically — line 1 sits on
	// line 0's row + one row stride.
	wantBTop := a.Top + subwayRowStep
	if b.Top != wantBTop {
		t.Errorf("B Top: got %d, want %d (one row stride below A)", b.Top, wantBTop)
	}
}

// Defensive companion: no two distinct nodes may share the same
// (Left, Top) coordinates. Today only the line-anchor row bug
// triggers this; the fence catches future regressions broadly.
func TestLayoutSubway_NoTwoNodesShareACoordinate(t *testing.T) {
	v := LayoutSubway(scenarioLineAnchorIsStopOnPriorLineSubway())
	type pos struct{ Left, Top int }
	seen := map[pos]string{}
	for _, n := range v.Nodes {
		p := pos{n.Left, n.Top}
		if other, dup := seen[p]; dup {
			t.Errorf("nodes %q and %q share position (%d, %d)", other, n.ShortID, p.Left, p.Top)
		}
		seen[p] = n.ShortID
	}
}

// Regression for the second face of ?at=1288: a fork branch from a
// shared-LCA-line-anchor down to a child line anchor sits at the
// same column on both rows (the LCA's line and the child's line
// share anchorCol). Pre-fix the edge fell into the L/R-anchored
// curve branch with x1=fromCX+r, x2=fromCX-r — drawing an S-shape
// that exits the "from" node's right side and enters the "to"
// node's left side, leaving the marker-end arrowhead pointing
// backward into the child. The fix routes such same-column
// different-row edges vertically: exit the bottom of "from",
// enter the top of "to", arrow points down.
func TestLayoutSubway_SameColumnDifferentRow_RoutedVertically(t *testing.T) {
	s := scenarioLineAnchorIsStopOnPriorLineSubway()
	v := LayoutSubway(s)

	a := assertNodePositioned(t, v, "A")
	b := assertNodePositioned(t, v, "B")

	aCX := a.Left + subwayNodeRadius
	bCX := b.Left + subwayNodeRadius
	if aCX != bCX {
		t.Fatalf("test precondition: expected A and B at same column; got cx %d vs %d", aCX, bCX)
	}

	e := assertEdgePresent(t, v, "A", "B")
	// Bad case: path begins at fromCX+radius — horizontal exit from
	// the right side of "from", which combined with toCX-radius on
	// the receiving end is the backward S-curve.
	badPrefix := fmt.Sprintf("M%d ", aCX+subwayNodeRadius)
	if strings.HasPrefix(e.PathD, badPrefix) {
		t.Errorf("A→B path uses horizontal-exit S-curve (arrow points backward); got d=%q",
			e.PathD)
	}
	// Good case: vertical routing exits the bottom of "from", so
	// the path's first M coordinate sits at the shared column
	// center, not its left/right edge.
	goodPrefix := fmt.Sprintf("M%d ", aCX)
	if !strings.HasPrefix(e.PathD, goodPrefix) {
		t.Errorf("A→B path should begin at column center x=%d (vertical routing); got d=%q",
			aCX, e.PathD)
	}
}

// Closure markers are reserved for edges, not nodes — no node carries
// closure-related state. The model deliberately removed Blocked from
// SubwayNodeState; this fence keeps that property tested.
func TestLayoutSubway_Scenario2_AnchorNodeCarriesNoClosureState(t *testing.T) {
	v := LayoutSubway(scenario2Subway())
	g := assertNodePositioned(t, v, "G")
	// G must render as Todo, not as a "blocked" or special state
	// adjacent to the closure marker.
	if g.IsActive || g.IsDone {
		t.Errorf("G should render as Todo (not Active/Done); got Active=%v Done=%v",
			g.IsActive, g.IsDone)
	}
	if strings.Contains(g.StateClass, "closed") || strings.Contains(g.StateClass, "block") {
		t.Errorf("G StateClass should not encode closure/block state; got %q", g.StateClass)
	}
}
