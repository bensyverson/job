package render

import (
	"fmt"
	"html/template"

	"github.com/bensyverson/jobs/internal/web/signals"
)

// SubwayView is the pre-laid-out, ready-to-render form of a
// signals.Subway. All positions are in CSS pixels; edge PathD
// strings are complete SVG path "d" attributes. The template
// iterates Nodes, Edges, and Elisions to emit the multi-line
// subway-system mini-graph (lines as horizontal tracks, an optional
// fork branching upward and downward, closure markers on blocked
// ingress edges, and `…` glyphs at elided spans).
type SubwayView struct {
	CanvasW, CanvasH int
	Empty            bool
	Lines            []SubwayLineView
	// Forks holds one entry per ancestor cluster; with cross-project
	// claims a single SubwayView can render multiple transfer
	// stations stacked vertically.
	Forks    []*SubwayForkView
	Nodes    []SubwayNodeView
	Edges    []SubwayEdgeView
	Elisions []SubwayElisionView
}

// SubwayLineView is the geometric record for one line: the row it
// occupies and the visible stop short IDs in order, anchored by the
// line's parent node. The template uses Row to lay out lines top-to-
// bottom; node positions themselves come from Nodes.
type SubwayLineView struct {
	AnchorShortID string
	Row           int
	StopShortIDs  []string
}

// SubwayForkView captures one cluster's transfer station — the LCA
// (or project root) shared by a group of lines. With cross-project
// claims a SubwayView holds multiple SubwayForkViews; each cluster's
// fork sits at col 0..k-1 (k = chain length) on the row of its
// inline line. AncestorShortIDs is the rendered chain (currently
// length 1 for shallow LCAs); BranchTargets lists the line anchor
// short IDs in render order so the template can draw fork-to-anchor
// connectors above and below the inline anchor.
type SubwayForkView struct {
	AncestorShortIDs []string
	BranchTargets    []string
}

// SubwayNodeView is a positioned subway stop or anchor. The template
// uses the boolean predicates (Go templates can't compare typed iota
// constants cleanly) and the precomputed Left/Top/Label coordinates
// to stamp anchors, optional actor bugs, and labels without extra
// lookups.
type SubwayNodeView struct {
	ShortID        string
	Title          string
	URL            string
	Left, Top      int
	StateClass     string
	IsActive       bool
	IsDone         bool
	IsLineAnchor   bool
	IsForkAncestor bool
	LabelLeft      int
	LabelTop       int
	Bug            *NodeBug
}

// SubwayEdgeView is one rendered connector. Boolean predicates
// classify the edge for the template (Go templates can't compare
// typed iota constants cleanly). IsClosure marks the `⊘` decoration
// on a sequence-blocked branch ingress; ClosureLeft/ClosureTop carry
// the glyph's pixel position when IsClosure is true (zero otherwise).
// The marker sits along the edge — never on the anchor node — so the
// reader sees "this connection is closed," not "this stop is broken."
type SubwayEdgeView struct {
	FromShortID string
	ToShortID   string
	PathD       string
	CSSClass    string
	IsFlow      bool
	IsBranch    bool
	IsClosure   bool
	IsBlocker   bool
	ClosureLeft int
	ClosureTop  int
}

// SubwayElisionView is a `…` marker positioned along a line between
// two non-adjacent visible windows (or between the line anchor and
// the first visible stop, or after the last visible stop).
type SubwayElisionView struct {
	Left, Top int
}

// Subway layout geometry. 32px node disc, 160px between column
// centers, 64px between row centers; 56px left/right and 14/40px
// top/bottom margins. The actor bug is a 20px avatar overhanging the
// node's bottom-right by 6px (matching the prototype's pattern). Row
// stride is 64 — wide enough that one row's label (sitting +40 below
// its disc to clear the bug overhang) doesn't collide with the next
// row's disc.
const (
	subwayNodeSize     = 32
	subwayColStep      = 160
	subwayRowStep      = 64
	subwayMarginLeft   = 56
	subwayMarginTop    = 14
	subwayMarginRight  = 56
	subwayMarginBottom = 40
	subwayNodeRadius   = subwayNodeSize / 2
	subwayBugSize      = 20
	subwayBugOverhang  = 6
)

// LayoutSubway turns a topological signals.Subway into a positioned
// SubwayView ready for the home-view template. Each line occupies a
// distinct row; a fork (when present) renders one line's anchor
// inline with the LCA and connects the others via /  \ branches with
// closure markers on sequence-blocked ingress edges.
//
// Inline-line rule (design doc): "typically the middle line for
// three lines, the top for two." Generalized as (n-1)/2 — top for 2,
// middle for 3, low-middle for 4+.
//
// Column layout: with a fork, the LCA chain occupies cols 0..k-1,
// each line's anchor sits at col k, and stops follow at cols k+1,
// k+2, .... Without a fork, the single anchor sits at col 0 and its
// stops at cols 1, 2, ....
func LayoutSubway(s signals.Subway) SubwayView {
	if len(s.Lines) == 0 {
		return SubwayView{Empty: true}
	}

	rows := map[string]int{}
	for i, line := range s.Lines {
		rows[line.AnchorShortID] = i
	}

	cols := map[string]int{}
	maxChain := 0
	for _, f := range s.Forks {
		if len(f.AncestorChain) > maxChain {
			maxChain = len(f.AncestorChain)
		}
	}
	anchorCol := maxChain
	// Each fork's ancestor chain sits at cols 0..k-1 on the row of
	// the cluster's inline line. The inline line within a cluster
	// follows the same (n-1)/2 rule as the global single-fork case —
	// top for 2, middle for 3, low-middle for 4+.
	for _, f := range s.Forks {
		if len(f.LineIndices) == 0 {
			continue
		}
		minRow := f.LineIndices[0]
		for _, idx := range f.LineIndices {
			if idx < minRow {
				minRow = idx
			}
		}
		inlineRow := minRow + (len(f.LineIndices)-1)/2
		for i, sid := range f.AncestorChain {
			cols[sid] = i
			rows[sid] = inlineRow
		}
	}

	view := SubwayView{
		Lines: make([]SubwayLineView, len(s.Lines)),
	}
	maxCol := anchorCol
	for i, line := range s.Lines {
		lineRow := i
		cols[line.AnchorShortID] = anchorCol
		lv := SubwayLineView{AnchorShortID: line.AnchorShortID, Row: lineRow}
		c := anchorCol + 1
		for _, item := range line.Items {
			if item.Kind == signals.LineItemStop {
				cols[item.ShortID] = c
				rows[item.ShortID] = lineRow
				lv.StopShortIDs = append(lv.StopShortIDs, item.ShortID)
			} else {
				view.Elisions = append(view.Elisions, SubwayElisionView{
					Left: subwayMarginLeft + c*subwayColStep + subwayNodeRadius,
					Top:  subwayMarginTop + lineRow*subwayRowStep + subwayNodeRadius,
				})
			}
			if c > maxCol {
				maxCol = c
			}
			c++
		}
		view.Lines[i] = lv
	}

	for _, f := range s.Forks {
		fv := &SubwayForkView{AncestorShortIDs: f.AncestorChain}
		for _, idx := range f.LineIndices {
			fv.BranchTargets = append(fv.BranchTargets, s.Lines[idx].AnchorShortID)
		}
		view.Forks = append(view.Forks, fv)
	}

	contentW := maxCol*subwayColStep + subwayNodeSize
	contentH := (len(s.Lines)-1)*subwayRowStep + subwayNodeSize
	view.CanvasW = subwayMarginLeft + contentW + subwayMarginRight
	view.CanvasH = subwayMarginTop + contentH + subwayMarginBottom

	anchorSet := map[string]bool{}
	for _, line := range s.Lines {
		anchorSet[line.AnchorShortID] = true
	}
	forkAncestorSet := map[string]bool{}
	for _, f := range s.Forks {
		for _, sid := range f.AncestorChain {
			forkAncestorSet[sid] = true
		}
	}

	for _, n := range s.Nodes {
		col, ok := cols[n.ShortID]
		if !ok {
			continue
		}
		row := rows[n.ShortID]
		left := subwayMarginLeft + col*subwayColStep
		top := subwayMarginTop + row*subwayRowStep
		nv := SubwayNodeView{
			ShortID:        n.ShortID,
			Title:          n.Title,
			URL:            n.URL,
			Left:           left,
			Top:            top,
			StateClass:     subwayStateClass(n.State),
			IsActive:       n.State == signals.SubwayNodeActive,
			IsDone:         n.State == signals.SubwayNodeDone,
			IsLineAnchor:   anchorSet[n.ShortID],
			IsForkAncestor: forkAncestorSet[n.ShortID],
			LabelLeft:      left + subwayNodeRadius,
			LabelTop:       top + subwayNodeSize + subwayBugOverhang + 2,
		}
		if n.State == signals.SubwayNodeActive && n.Actor != "" {
			nv.Bug = &NodeBug{
				Actor:    n.Actor,
				ActorURL: "/actors/" + n.Actor,
				Letter:   InitialOf(n.Actor),
				Color:    template.CSS(ActorColor(n.Actor)),
				Left:     left + subwayNodeSize - subwayBugSize + subwayBugOverhang,
				Top:      top + subwayNodeSize - subwayBugSize + subwayBugOverhang,
			}
		}
		view.Nodes = append(view.Nodes, nv)
	}

	nodeViewByShort := map[string]SubwayNodeView{}
	for _, nv := range view.Nodes {
		nodeViewByShort[nv.ShortID] = nv
	}
	for _, e := range s.Edges {
		from, fromOK := nodeViewByShort[e.FromShortID]
		to, toOK := nodeViewByShort[e.ToShortID]
		if !fromOK || !toOK {
			continue
		}
		view.Edges = append(view.Edges, buildSubwayEdgeView(e, from, to))
	}

	return view
}

func buildSubwayEdgeView(e signals.SubwayEdge, from, to SubwayNodeView) SubwayEdgeView {
	fromCX := from.Left + subwayNodeRadius
	fromCY := from.Top + subwayNodeRadius
	toCX := to.Left + subwayNodeRadius
	toCY := to.Top + subwayNodeRadius

	var d string
	if fromCY == toCY {
		x1, x2 := fromCX+subwayNodeRadius, toCX-subwayNodeRadius
		if fromCX > toCX {
			x1, x2 = fromCX-subwayNodeRadius, toCX+subwayNodeRadius
		}
		d = fmt.Sprintf("M%d %d L%d %d", x1, fromCY, x2, toCY)
	} else {
		x1 := fromCX + subwayNodeRadius
		x2 := toCX - subwayNodeRadius
		if fromCX > toCX {
			x1 = fromCX - subwayNodeRadius
			x2 = toCX + subwayNodeRadius
		}
		cx1 := (x1 + x2) / 2
		cx2 := cx1
		d = fmt.Sprintf("M%d %d C %d %d, %d %d, %d %d",
			x1, fromCY, cx1, fromCY, cx2, toCY, x2, toCY)
	}
	ev := SubwayEdgeView{
		FromShortID: e.FromShortID,
		ToShortID:   e.ToShortID,
		PathD:       d,
		CSSClass:    subwayEdgeClass(e.Kind),
		IsFlow:      e.Kind == signals.SubwayEdgeFlow,
		IsBranch:    e.Kind == signals.SubwayEdgeBranch || e.Kind == signals.SubwayEdgeBranchClosed,
		IsClosure:   e.Kind == signals.SubwayEdgeBranchClosed,
		IsBlocker:   e.Kind == signals.SubwayEdgeBlocker,
	}
	if ev.IsClosure {
		// Place the marker at the geometric midpoint between disc
		// centers — squarely on the edge, never on either anchor.
		ev.ClosureLeft = (fromCX + toCX) / 2
		ev.ClosureTop = (fromCY + toCY) / 2
	}
	return ev
}

func subwayStateClass(s signals.SubwayNodeState) string {
	switch s {
	case signals.SubwayNodeActive:
		return "c-graph-node--active"
	case signals.SubwayNodeDone:
		return "c-graph-node--done"
	}
	return "c-graph-node--todo"
}

func subwayEdgeClass(k signals.SubwayEdgeKind) string {
	switch k {
	case signals.SubwayEdgeBranch:
		return "c-graph-edge c-graph-edge--branch"
	case signals.SubwayEdgeBranchClosed:
		return "c-graph-edge c-graph-edge--branch c-graph-edge--closed"
	case signals.SubwayEdgeBlocker:
		return "c-graph-edge c-graph-edge--blocker"
	}
	return "c-graph-edge c-graph-edge--flow"
}
