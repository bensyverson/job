package render

import (
	"fmt"
	"html/template"

	"github.com/bensyverson/jobs/internal/web/signals"
)

// MiniGraphView is the pre-laid-out, ready-to-render form of a
// signals.MiniGraph. All positions are in CSS pixels; edge PathD
// strings are complete SVG path "d" attributes. The template
// iterates Nodes and Edges and emits absolutely-positioned anchors
// plus an underlying SVG for edges.
type MiniGraphView struct {
	CanvasW, CanvasH int
	Empty            bool
	Nodes            []MiniGraphNodeView
	Edges            []MiniGraphEdgeView
}

// MiniGraphNodeView is a single positioned node with enough
// presentation state for the template to stamp an anchor, an
// optional actor "bug", and a label without any further lookups.
// Is{Done,Active,Blocked} are template-friendly glyph predicates
// (Go templates can't compare typed iota constants cleanly).
type MiniGraphNodeView struct {
	ShortID          string
	Title            string
	URL              string
	Left, Top        int // top-left of the 32px disc
	StateClass       string
	IsDone           bool
	IsActive         bool
	IsBlocked        bool
	ChildrenNotShown int
	LabelLeft        int
	LabelTop         int
	Bug              *MiniGraphBug
}

// MiniGraphBug is the small avatar overlaid on a claimed node.
// Color is typed as template.CSS because it's a known-safe HSL
// literal we generated ourselves from an actor-name hash; without
// that marker html/template's autoescape collapses it to
// "ZgotmplZ" when interpolated into a style attribute.
type MiniGraphBug struct {
	Actor    string
	ActorURL string
	Letter   string
	Color    template.CSS
	Left     int
	Top      int
}

// MiniGraphEdgeView carries the SVG path plus a semantic class the
// template uses to pick stroke/dash styling. IsBlocker suppresses
// the arrowhead — blocker arcs read as a constraint, not a flow
// direction.
type MiniGraphEdgeView struct {
	PathD     string
	CSSClass  string
	IsBlocker bool
}

// Layout geometry. Matches the prototype: a 32px node disc, 160px
// between column centers, 40px between row centers, with a 56px
// left inset and a 14px top inset. The actor bug is a 20px avatar
// overhanging the node's bottom-right corner by 6px (matches the
// prototype's right: -6px; bottom: -6px pattern).
const (
	miniGraphNodeSize     = 32
	miniGraphColStep      = 160
	miniGraphRowStep      = 40
	miniGraphMarginLeft   = 56
	miniGraphMarginTop    = 14
	miniGraphMarginRight  = 56
	miniGraphMarginBottom = 40
	miniGraphNodeRadius   = miniGraphNodeSize / 2
	miniGraphBugSize      = 20
	miniGraphBugOverhang  = 6
)

// LayoutMiniGraph positions every node in every lane and routes
// every edge to an SVG path string. Column numbering is derived
// from each lane's 5-slot spine with a reconciliation pass so
// nodes shared across lanes sit at a single consistent column.
// Rows stack lanes top-to-bottom with stacked siblings occupying
// rows immediately below their lane's focal.
func LayoutMiniGraph(mg signals.MiniGraph) MiniGraphView {
	if len(mg.Lanes) == 0 {
		return MiniGraphView{Empty: true}
	}

	cols := assignColumns(mg.Lanes)
	rows, maxRow := assignRows(mg.Lanes)

	// Canvas dimensions wrap the content tightly so the flex-
	// centered container renders a visually balanced graph.
	// Content width = (N-1) column steps + one node width; height
	// = (rows-1) row steps + one node height; margins surround it.
	contentW := 0
	if n := columnExtent(cols); n > 0 {
		contentW = (n-1)*miniGraphColStep + miniGraphNodeSize
	}
	contentH := maxRow*miniGraphRowStep + miniGraphNodeSize
	view := MiniGraphView{
		CanvasW: miniGraphMarginLeft + contentW + miniGraphMarginRight,
		CanvasH: miniGraphMarginTop + contentH + miniGraphMarginBottom,
	}
	for _, n := range mg.Nodes {
		col, okC := cols[n.ShortID]
		row, okR := rows[n.ShortID]
		if !okC || !okR {
			continue
		}
		view.Nodes = append(view.Nodes, buildNodeView(n, col, row))
	}
	for _, e := range mg.Edges {
		from, fromOK := view.nodeByShortID(e.FromShortID)
		to, toOK := view.nodeByShortID(e.ToShortID)
		if !fromOK || !toOK {
			continue
		}
		view.Edges = append(view.Edges, buildEdgeView(e, from, to))
	}
	return view
}

// nodeByShortID is a small lookup helper used during edge routing.
func (v MiniGraphView) nodeByShortID(id string) (MiniGraphNodeView, bool) {
	for _, n := range v.Nodes {
		if n.ShortID == id {
			return n, true
		}
	}
	return MiniGraphNodeView{}, false
}

// assignColumns walks the lanes in order, placing lane 0's focal
// at column 2 (matching the spine's SpineFocal index). Subsequent
// lanes align any already-positioned spine node to its existing
// column and derive the rest relative to that; if nothing is
// shared, the new lane's focal is also placed at column 2.
//
// Returns a ShortID → column map. Column indices are 0-based and
// may span wider than 5 once multi-lane alignment shifts things.
func assignColumns(lanes []signals.Lane) map[string]int {
	cols := map[string]int{}
	for _, lane := range lanes {
		offset, locked := 0, false
		// If any slot in this lane's spine is already placed,
		// lock our zero-offset so that slot lands where we found it.
		for idx, id := range lane.Spine {
			if id == "" {
				continue
			}
			if existing, ok := cols[id]; ok {
				offset = existing - idx
				locked = true
				break
			}
		}
		if !locked {
			offset = 0 // first lane: SpineL2 sits at column 0
		}
		for idx, id := range lane.Spine {
			if id == "" {
				continue
			}
			col := idx + offset
			if prev, ok := cols[id]; ok && prev != col {
				// Conflicting positions across lanes — keep the
				// earlier placement; a shared node can only have
				// one column.
				continue
			}
			cols[id] = col
		}
		// Stacked siblings sit at the focal's column.
		focalCol, ok := cols[lane.FocalShortID]
		if !ok {
			continue
		}
		for _, id := range lane.Stacked {
			if _, already := cols[id]; already {
				continue
			}
			cols[id] = focalCol
		}
	}
	return cols
}

// assignRows gives every lane a top row for the focal, followed
// by one row per stacked sibling immediately below. The second
// and later lanes start at (prev lane's last row + 1). Returns
// the per-ShortID row map and the maximum row index in use.
func assignRows(lanes []signals.Lane) (map[string]int, int) {
	rows := map[string]int{}
	next := 0
	maxRow := 0
	for _, lane := range lanes {
		focalRow := next
		for _, id := range lane.Spine {
			if id == "" {
				continue
			}
			if _, ok := rows[id]; !ok {
				rows[id] = focalRow
			}
		}
		stackRow := focalRow + 1
		for _, id := range lane.Stacked {
			if _, ok := rows[id]; !ok {
				rows[id] = stackRow
				stackRow++
			}
		}
		used := stackRow - 1
		if used < focalRow {
			used = focalRow
		}
		if used > maxRow {
			maxRow = used
		}
		next = used + 1
	}
	return rows, maxRow
}

// columnExtent returns the (max column + 1) across all placed
// nodes, which is the number of columns the canvas needs to span.
func columnExtent(cols map[string]int) int {
	max := 0
	for _, c := range cols {
		if c > max {
			max = c
		}
	}
	return max + 1
}

// buildNodeView converts a topological GraphNode at grid (col, row)
// into a positioned MiniGraphNodeView. The label sits 12px below
// the bottom of the node disc, centered horizontally.
func buildNodeView(n signals.GraphNode, col, row int) MiniGraphNodeView {
	left := miniGraphMarginLeft + col*miniGraphColStep
	top := miniGraphMarginTop + row*miniGraphRowStep
	v := MiniGraphNodeView{
		ShortID:          n.ShortID,
		Title:            n.Title,
		URL:              n.URL,
		Left:             left,
		Top:              top,
		StateClass:       stateClass(n.State),
		IsDone:           n.State == signals.GraphNodeDone,
		IsActive:         n.State == signals.GraphNodeActive,
		IsBlocked:        n.State == signals.GraphNodeBlocked,
		ChildrenNotShown: n.ChildrenNotShown,
		LabelLeft:        left + miniGraphNodeRadius,
		// Label sits below the node plus enough room for the
		// actor-bug overhang (present on focal nodes) so the
		// label never collides with the bug.
		LabelTop: top + miniGraphNodeSize + miniGraphBugOverhang + 2,
	}
	if n.State == signals.GraphNodeActive && n.Actor != "" {
		v.Bug = &MiniGraphBug{
			Actor:    n.Actor,
			ActorURL: "/actors/" + n.Actor,
			Letter:   InitialOf(n.Actor),
			Color:    template.CSS(ActorColor(n.Actor)),
			// Overhang pattern: bug's bottom-right sits overhang
			// pixels past the node's bottom-right. Derive the
			// top-left of the bug from there.
			Left: left + miniGraphNodeSize - miniGraphBugSize + miniGraphBugOverhang,
			Top:  top + miniGraphNodeSize - miniGraphBugSize + miniGraphBugOverhang,
		}
	}
	return v
}

func stateClass(s signals.GraphNodeState) string {
	switch s {
	case signals.GraphNodeActive:
		return "c-graph-node--active"
	case signals.GraphNodeBlocked:
		return "c-graph-node--blocked"
	case signals.GraphNodeDone:
		return "c-graph-node--done"
	}
	return "c-graph-node--todo"
}

// buildEdgeView routes a single edge. Same-row hops are straight
// horizontal lines between the right edge of the from-node and the
// left edge of the to-node; row-shifting hops use a tangent-matched
// cubic bezier so they leave each node horizontally (matching the
// prototype's curvature). Blocker edges use the same shape as flow
// edges — the dashed amber styling is applied by the template.
func buildEdgeView(e signals.GraphEdge, from, to MiniGraphNodeView) MiniGraphEdgeView {
	fromCX := from.Left + miniGraphNodeRadius
	fromCY := from.Top + miniGraphNodeRadius
	toCX := to.Left + miniGraphNodeRadius
	toCY := to.Top + miniGraphNodeRadius

	var d string
	if fromCY == toCY {
		// Straight hop. Trim the endpoints to the disc edges so
		// the stroke doesn't vanish under the node background.
		x1, x2 := fromCX+miniGraphNodeRadius, toCX-miniGraphNodeRadius
		if fromCX > toCX {
			x1, x2 = fromCX-miniGraphNodeRadius, toCX+miniGraphNodeRadius
		}
		d = fmt.Sprintf("M%d %d L%d %d", x1, fromCY, x2, toCY)
	} else {
		// Diagonal: exit the from-node horizontally, enter the
		// to-node horizontally. Control points sit halfway
		// between the nodes' x-coordinates, at each node's y.
		x1 := fromCX + miniGraphNodeRadius
		x2 := toCX - miniGraphNodeRadius
		if fromCX > toCX {
			x1 = fromCX - miniGraphNodeRadius
			x2 = toCX + miniGraphNodeRadius
		}
		cx1 := (x1 + x2) / 2
		cx2 := cx1
		d = fmt.Sprintf("M%d %d C %d %d, %d %d, %d %d",
			x1, fromCY, cx1, fromCY, cx2, toCY, x2, toCY)
	}
	return MiniGraphEdgeView{
		PathD:     d,
		CSSClass:  edgeClass(e.Kind),
		IsBlocker: e.Kind == signals.GraphEdgeBlocker,
	}
}

func edgeClass(k signals.GraphEdgeKind) string {
	switch k {
	case signals.GraphEdgeCousin:
		return "c-graph-edge c-graph-edge--cousin"
	case signals.GraphEdgeBlocker:
		return "c-graph-edge c-graph-edge--blocker"
	}
	return "c-graph-edge c-graph-edge--flow"
}
