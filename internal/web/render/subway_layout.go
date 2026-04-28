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
// ingress edges, and in-gap dots at elided spans).
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

// SubwayForkView captures one cluster's transfer station — the
// group of lines that share a branch parent. BranchTargets lists
// the line anchor short IDs in render order so the template can
// draw fork-to-anchor connectors above and below the parent's
// rendered position; the parent identity itself lives on each
// Line's ParentShortID (project/2026-04-27-graph-row-merging.md,
// S3). With cross-project claims a SubwayView holds multiple
// SubwayForkViews stacked vertically.
type SubwayForkView struct {
	BranchTargets []string
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

// SubwayElisionView is an in-gap dots marker positioned at the
// midpoint between two surrounding items on a line — anchor↔first
// visible stop, or stop↔stop across a gap. It does not consume a
// column slot.
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
// distinct row; a fork (when present) connects each row's leftmost
// to its branch parent's rendered position.
//
// Column layout (project/2026-04-27-graph-row-merging.md, S3):
// depth-aligned leftmost. The topmost row's leftmost sits at col 0;
// each non-top row's leftmost sits at col(parent) + 1, where parent
// is the row's branch parent (Line.ParentShortID under the new
// model, or Fork.AncestorChain's last entry for legacy fixtures
// that haven't migrated yet). Stops on a row advance one col each
// from the row's leftmost. The cluster LCA renders as the topmost
// row's leftmost; fork-ancestor chains carved off the chain (legacy
// only) sit at cols 0..k-1 inline with the cluster's middle row,
// preserving the old "transfer station" rendering until the
// AncestorChain field is retired in S3d.
func LayoutSubway(s signals.Subway) SubwayView {
	if len(s.Lines) == 0 {
		return SubwayView{Empty: true}
	}

	rows := map[string]int{}
	cols := map[string]int{}

	anchorSet := map[string]bool{}
	stopSet := map[string]bool{}
	for _, line := range s.Lines {
		anchorSet[line.AnchorShortID] = true
		for _, item := range line.Items {
			if item.Kind == signals.LineItemStop {
				stopSet[item.ShortID] = true
			}
		}
	}

	// Pre-seed any branch parent that isn't otherwise rendered as a
	// line anchor or stop. This covers legacy fixtures where the
	// cluster LCA exists only as fork chrome (no leftmost-only top
	// row); the parent gets placed at col 0 inline with the
	// cluster's middle row so the lines loop below can resolve
	// `cols[parent] + 1` to col 1 for each branching row.
	for _, f := range s.Forks {
		if len(f.LineIndices) == 0 {
			continue
		}
		firstIdx := f.LineIndices[0]
		if firstIdx < 0 || firstIdx >= len(s.Lines) {
			continue
		}
		parent := s.Lines[firstIdx].ParentShortID
		if parent == "" || anchorSet[parent] || stopSet[parent] {
			continue
		}
		if _, alreadyPositioned := cols[parent]; alreadyPositioned {
			continue
		}
		minRow := f.LineIndices[0]
		for _, idx := range f.LineIndices {
			if idx < minRow {
				minRow = idx
			}
		}
		inlineRow := minRow + (len(f.LineIndices)-1)/2
		cols[parent] = 0
		rows[parent] = inlineRow
	}

	view := SubwayView{
		Lines: make([]SubwayLineView, len(s.Lines)),
	}
	maxCol := 0
	for _, c := range cols {
		if c > maxCol {
			maxCol = c
		}
	}
	for i, line := range s.Lines {
		lineRow := i
		// Each row's leftmost sits at col(parent) + 1, or col 0 for
		// the topmost row. When the parent is a stop on an earlier
		// row, its col was set during that row's items walk; when
		// the parent is an earlier line's anchor, its col was set
		// when that line's leftmost was placed.
		anchorCol := 0
		if parent := line.ParentShortID; parent != "" {
			if pc, ok := cols[parent]; ok {
				anchorCol = pc + 1
			}
		}
		cols[line.AnchorShortID] = anchorCol
		rows[line.AnchorShortID] = lineRow
		if anchorCol > maxCol {
			maxCol = anchorCol
		}
		lv := SubwayLineView{AnchorShortID: line.AnchorShortID, Row: lineRow}
		// prevCol tracks the column of the most recent item that
		// occupied a slot — the line anchor by default, then each
		// stop as we walk Items. It exists so an in-gap elision
		// (which doesn't take a slot) can be positioned at the
		// midpoint of the surrounding pair.
		prevCol := anchorCol
		pendingElision := false
		c := anchorCol + 1
		emplaceElision := func(nextCol int) {
			prevLeft := subwayMarginLeft + prevCol*subwayColStep
			nextLeft := subwayMarginLeft + nextCol*subwayColStep
			view.Elisions = append(view.Elisions, SubwayElisionView{
				Left: (prevLeft+nextLeft)/2 + subwayNodeRadius,
				Top:  subwayMarginTop + lineRow*subwayRowStep + subwayNodeRadius,
			})
		}
		for _, item := range line.Items {
			switch item.Kind {
			case signals.LineItemStop:
				cols[item.ShortID] = c
				rows[item.ShortID] = lineRow
				lv.StopShortIDs = append(lv.StopShortIDs, item.ShortID)
				if pendingElision {
					emplaceElision(c)
					pendingElision = false
				}
				prevCol = c
				if c > maxCol {
					maxCol = c
				}
				c++
			case signals.LineItemElision:
				pendingElision = true
			}
		}
		view.Lines[i] = lv
	}

	for _, f := range s.Forks {
		fv := &SubwayForkView{}
		for _, idx := range f.LineIndices {
			fv.BranchTargets = append(fv.BranchTargets, s.Lines[idx].AnchorShortID)
		}
		view.Forks = append(view.Forks, fv)
	}

	contentH := (len(s.Lines)-1)*subwayRowStep + subwayNodeSize
	view.CanvasH = subwayMarginTop + contentH + subwayMarginBottom

	// IsForkAncestor used to mark the legacy LCA chain for templates.
	// Under the per-row parent-edge model (project/2026-04-27-graph-
	// row-merging.md, S3) every parent is on a line and there is no
	// distinct "fork chrome" set; flag the row's ParentShortID so
	// templates that still consume IsForkAncestor see the same nodes
	// they did before, but pulled from the lines themselves.
	forkAncestorSet := map[string]bool{}
	for _, line := range s.Lines {
		if line.ParentShortID != "" {
			forkAncestorSet[line.ParentShortID] = true
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
			LabelTop:       top + subwayNodeSize + subwayBugOverhang,
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

	// Centering pass (project/2026-04-27-graph-row-merging.md, S3c):
	// compute the rendered bounding box from disc positions and shift
	// the whole rendering horizontally so the extent sits centered in
	// the canvas with symmetric left/right margins. Under the current
	// layout (top row leftmost at col 0, content extending right) the
	// pre-shift extent already starts at subwayMarginLeft and the
	// shift is zero; the pass exists so absolute-depth interpretations
	// or multi-cluster renderings can rely on a single source of
	// truth for centering. CanvasW is sized from the actual extent
	// rather than maxCol so an unoccupied col 0 (future absolute-
	// depth case) doesn't pad the canvas asymmetrically.
	minLeft, maxRight := 0, 0
	if len(view.Nodes) > 0 {
		minLeft = view.Nodes[0].Left
		maxRight = view.Nodes[0].Left + subwayNodeSize
		for _, n := range view.Nodes {
			if n.Left < minLeft {
				minLeft = n.Left
			}
			if n.Left+subwayNodeSize > maxRight {
				maxRight = n.Left + subwayNodeSize
			}
		}
	} else {
		// No discs to render — fall back to the col-derived extent
		// so the canvas still has a non-zero width.
		minLeft = subwayMarginLeft
		maxRight = subwayMarginLeft + maxCol*subwayColStep + subwayNodeSize
	}
	extentW := maxRight - minLeft
	view.CanvasW = subwayMarginLeft + extentW + subwayMarginRight
	shift := subwayMarginLeft - minLeft
	if shift != 0 {
		for i := range view.Nodes {
			view.Nodes[i].Left += shift
			view.Nodes[i].LabelLeft += shift
			if view.Nodes[i].Bug != nil {
				view.Nodes[i].Bug.Left += shift
			}
		}
		for i := range view.Elisions {
			view.Elisions[i].Left += shift
		}
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
	switch {
	case fromCY == toCY:
		x1, x2 := fromCX+subwayNodeRadius, toCX-subwayNodeRadius
		if fromCX > toCX {
			x1, x2 = fromCX-subwayNodeRadius, toCX+subwayNodeRadius
		}
		d = fmt.Sprintf("M%d %d L%d %d", x1, fromCY, x2, toCY)
	default:
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
