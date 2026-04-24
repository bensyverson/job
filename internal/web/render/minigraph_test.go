package render

import (
	"strings"
	"testing"

	"github.com/bensyverson/jobs/internal/web/signals"
)

func TestLayoutMiniGraph_EmptyReturnsEmptyView(t *testing.T) {
	v := LayoutMiniGraph(signals.MiniGraph{})
	if !v.Empty {
		t.Errorf("Empty: got false, want true")
	}
	if len(v.Nodes) != 0 || len(v.Edges) != 0 {
		t.Errorf("expected no nodes/edges, got %d/%d", len(v.Nodes), len(v.Edges))
	}
}

// Single-lane layout — five spine nodes all at row 0, evenly spaced
// along the x-axis. Edges are straight horizontal segments between
// adjacent disc edges. Matches Scenario A shape from the data layer.
func TestLayoutMiniGraph_SingleLaneGrid(t *testing.T) {
	mg := signals.MiniGraph{
		Lanes: []signals.Lane{{
			FocalShortID: "Step2_",
			Spine:        [5]string{"Phase3", "Step1_", "Step2_", "Step3_", "Phase4"},
		}},
		Nodes: []signals.GraphNode{
			{ShortID: "Phase3", IsParent: true, State: signals.GraphNodeTodo},
			{ShortID: "Step1_", State: signals.GraphNodeDone},
			{ShortID: "Step2_", State: signals.GraphNodeActive, Actor: "alice"},
			{ShortID: "Step3_", State: signals.GraphNodeBlocked},
			{ShortID: "Phase4", IsParent: true, ChildrenNotShown: 6, State: signals.GraphNodeTodo},
		},
		Edges: []signals.GraphEdge{
			{FromShortID: "Phase3", ToShortID: "Step1_", Kind: signals.GraphEdgeFlow},
			{FromShortID: "Step1_", ToShortID: "Step2_", Kind: signals.GraphEdgeFlow},
			{FromShortID: "Step2_", ToShortID: "Step3_", Kind: signals.GraphEdgeFlow},
			{FromShortID: "Step3_", ToShortID: "Phase4", Kind: signals.GraphEdgeCousin},
			{FromShortID: "Step2_", ToShortID: "Step3_", Kind: signals.GraphEdgeBlocker},
		},
	}
	v := LayoutMiniGraph(mg)

	if v.Empty {
		t.Errorf("Empty: got true, want false")
	}
	expect := map[string]struct{ left, top int }{
		"Phase3": {56, 14},
		"Step1_": {216, 14},
		"Step2_": {376, 14},
		"Step3_": {536, 14},
		"Phase4": {696, 14},
	}
	if len(v.Nodes) != len(expect) {
		t.Errorf("Nodes: got %d, want %d", len(v.Nodes), len(expect))
	}
	for _, n := range v.Nodes {
		want, ok := expect[n.ShortID]
		if !ok {
			t.Errorf("unexpected node %q", n.ShortID)
			continue
		}
		if n.Left != want.left || n.Top != want.top {
			t.Errorf("%s position: got (%d,%d), want (%d,%d)",
				n.ShortID, n.Left, n.Top, want.left, want.top)
		}
	}

	// Active node carries an actor bug with letter + HSL color.
	var step2 MiniGraphNodeView
	for _, n := range v.Nodes {
		if n.ShortID == "Step2_" {
			step2 = n
		}
	}
	if step2.Bug == nil {
		t.Fatalf("Step2_ should carry an actor bug")
	}
	if step2.Bug.Letter != "A" {
		t.Errorf("bug letter: got %q, want %q", step2.Bug.Letter, "A")
	}
	if !strings.HasPrefix(string(step2.Bug.Color), "hsl(") {
		t.Errorf("bug color: got %q, want HSL", step2.Bug.Color)
	}

	// State class drives the node styling.
	for _, n := range v.Nodes {
		switch n.ShortID {
		case "Step2_":
			if n.StateClass != "c-graph-node--active" {
				t.Errorf("Step2_ class: got %q", n.StateClass)
			}
		case "Step3_":
			if n.StateClass != "c-graph-node--blocked" {
				t.Errorf("Step3_ class: got %q", n.StateClass)
			}
		case "Step1_":
			if n.StateClass != "c-graph-node--done" {
				t.Errorf("Step1_ class: got %q", n.StateClass)
			}
		}
	}

	// Flow-kind edge between Step1_ and Step2_. Both nodes on the
	// same row means the path is a straight M…L… segment running
	// between their disc edges (centers at x=232 and x=392, minus
	// radius 16 on each side).
	for _, e := range v.Edges {
		if !strings.HasPrefix(e.PathD, "M") {
			t.Errorf("edge path not a valid SVG M-command: %q", e.PathD)
		}
	}
	var step12 MiniGraphEdgeView
	for _, e := range v.Edges {
		// Step1_'s right disc edge at (248, 30), Step2_'s left
		// disc edge at (376, 30).
		if strings.Contains(e.PathD, "M248 30") && strings.Contains(e.PathD, "L376 30") && strings.Contains(e.CSSClass, "flow") {
			step12 = e
		}
	}
	if step12.PathD == "" {
		t.Errorf("missing straight Step1_ -> Step2_ edge; got edges %+v", v.Edges)
	}
	if !strings.Contains(step12.CSSClass, "c-graph-edge--flow") {
		t.Errorf("Flow edge class: got %q", step12.CSSClass)
	}

	// Blocker edge carries the blocker CSS class.
	var blocker MiniGraphEdgeView
	for _, e := range v.Edges {
		if strings.Contains(e.CSSClass, "blocker") {
			blocker = e
		}
	}
	if blocker.PathD == "" {
		t.Errorf("missing blocker edge class; got edges %+v", v.Edges)
	}

	// Canvas dimensions wrap the content tightly: five columns at
	// 160px between centers span (5-1)*160 + 32 = 672 content
	// pixels, plus 56px of left + right margin = 784 total.
	if got, want := v.CanvasW, 56+(5-1)*160+32+56; got != want {
		t.Errorf("CanvasW: got %d, want %d", got, want)
	}
}

// Vertical stacking — the sole extra sibling in Lane.Stacked sits
// at the focal column on row 1.
func TestLayoutMiniGraph_StackedSiblingsBelowFocal(t *testing.T) {
	mg := signals.MiniGraph{
		Lanes: []signals.Lane{{
			FocalShortID: "S3____",
			Spine:        [5]string{"S1____", "S2____", "S3____", "S4____", "S5____"},
			Stacked:      []string{"S6____"},
		}},
		Nodes: []signals.GraphNode{
			{ShortID: "S1____", State: signals.GraphNodeDone},
			{ShortID: "S2____", State: signals.GraphNodeDone},
			{ShortID: "S3____", State: signals.GraphNodeActive, Actor: "alice"},
			{ShortID: "S4____"},
			{ShortID: "S5____"},
			{ShortID: "S6____"},
		},
	}
	v := LayoutMiniGraph(mg)

	var s3, s6 MiniGraphNodeView
	for _, n := range v.Nodes {
		switch n.ShortID {
		case "S3____":
			s3 = n
		case "S6____":
			s6 = n
		}
	}
	if s3.ShortID == "" || s6.ShortID == "" {
		t.Fatalf("missing focal or stacked sibling")
	}
	if s6.Left != s3.Left {
		t.Errorf("stacked S6 left: got %d, want %d (same col as focal)", s6.Left, s3.Left)
	}
	if s6.Top != s3.Top+miniGraphRowStep {
		t.Errorf("stacked S6 top: got %d, want %d (one row below focal)", s6.Top, s3.Top+miniGraphRowStep)
	}
}

// Multi-lane alignment — two lanes that share a node (Phase A) must
// place the shared node at a single column consistent across both
// lanes. Lane 2's focal and remaining spine shift to respect that.
func TestLayoutMiniGraph_MultiLaneSharedNodeAligned(t *testing.T) {
	// Lane 1: PhaseA -> StepA1 -> focal=StepA1 — hmm; let's build
	// a richer case: Lane 1 spine contains PhaseA at slot L2;
	// Lane 2 spine contains PhaseA at slot L1. The layout must
	// place PhaseA at a single column.
	mg := signals.MiniGraph{
		Lanes: []signals.Lane{
			{
				FocalShortID: "FocalA",
				Spine:        [5]string{"", "PhaseA", "FocalA", "NextA_", ""},
			},
			{
				FocalShortID: "FocalB",
				Spine:        [5]string{"", "PhaseA", "FocalB", "", ""},
			},
		},
		Nodes: []signals.GraphNode{
			{ShortID: "PhaseA", IsParent: true},
			{ShortID: "FocalA", State: signals.GraphNodeActive, Actor: "a"},
			{ShortID: "NextA_"},
			{ShortID: "FocalB", State: signals.GraphNodeActive, Actor: "b"},
		},
	}
	v := LayoutMiniGraph(mg)

	var phaseA, focalA, focalB MiniGraphNodeView
	for _, n := range v.Nodes {
		switch n.ShortID {
		case "PhaseA":
			phaseA = n
		case "FocalA":
			focalA = n
		case "FocalB":
			focalB = n
		}
	}
	if phaseA.ShortID == "" {
		t.Fatalf("PhaseA not placed")
	}
	// Lane 1 locks PhaseA at column 1 (SpineL1 index). Lane 2's
	// SpineL1 for PhaseA must therefore resolve to the same
	// column, and its focal (index 2) must land at column 2.
	if phaseA.Left != 56+1*160 {
		t.Errorf("PhaseA left: got %d, want %d", phaseA.Left, 56+160)
	}
	if focalA.Left != 56+2*160 {
		t.Errorf("FocalA left: got %d, want %d", focalA.Left, 56+2*160)
	}
	if focalB.Left != 56+2*160 {
		t.Errorf("FocalB left: got %d, want %d (should align with FocalA col via shared PhaseA)",
			focalB.Left, 56+2*160)
	}
	// Lanes on distinct rows.
	if focalA.Top == focalB.Top {
		t.Errorf("focals should sit on distinct rows; both at top=%d", focalA.Top)
	}
}
