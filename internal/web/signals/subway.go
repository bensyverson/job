package signals

import "sort"

// Subway is the topological model of the mini-graph under the
// subway-system metaphor. See
// project/2026-04-25-graph-clarification.md for the design.
//
// It describes one or more Lines (each anchored at a parent node,
// displaying that parent's children as stops in tree order with
// elision markers for skipped stops), an optional Fork (present only
// when two or more lines branch from a common ancestor), a
// deduplicated Nodes pool, and a typed Edges list.
//
// Grid/pixel positioning and SVG routing belong to the layout step in
// the render package; nothing here knows about columns, rows, or
// stroke styles.
type Subway struct {
	Lines []Line
	Fork  *Fork
	Nodes []SubwayNode
	Edges []SubwayEdge
}

// Line is a single subway line: a parent anchor followed by an
// ordered sequence of items (stops and elision markers). The anchor
// is the parent node whose children render as stops along this line.
//
// Tree order, not claim order: stops follow their underlying
// sort_order so the layout stays stable as claims churn.
type Line struct {
	AnchorShortID string
	Items         []LineItem
}

// LineItem is one element in a Line's sequence — either a stop (a
// child node referenced by ShortID) or an elision marker (rendered
// as `…` between non-adjacent visible windows or between the anchor
// and the first visible stop).
type LineItem struct {
	Kind    LineItemKind
	ShortID string
}

// LineItemKind tags a LineItem as a stop or an elision marker.
type LineItemKind int

const (
	// LineItemStop is a rendered stop on the line; ShortID is set.
	LineItemStop LineItemKind = iota
	// LineItemElision is a `…` marker; ShortID is empty.
	LineItemElision
)

// Fork is the transfer station where two or more lines diverge from a
// common ancestor. AncestorChain[0] is the topmost LCA; for shallow
// LCAs the chain has length 1. For a deep-LCA path
// (e.g. A → M → B vs A → M → G), the chain renders the full path
// from the LCA down to the actual divergence point so intermediate
// ancestors aren't silently collapsed.
//
// LineIndices references Subway.Lines in display order (top to
// bottom). A Subway with a single Line has Fork == nil — the LCA
// would be chrome.
type Fork struct {
	AncestorChain []string
	LineIndices   []int
}

// SubwayNode is a single task's presentation metadata, unique per
// Subway by ShortID.
type SubwayNode struct {
	ShortID string
	Title   string
	State   SubwayNodeState
	Actor   string
	URL     string
}

// SubwayNodeState drives the stop's visual treatment.
//
// Blocked-ness is no longer a node state in the subway model: a line
// blocked by a prior phase carries a closure marker on its ingress
// edge (SubwayEdgeBranchClosed), and an explicit blocks relationship
// between two rendered nodes carries a SubwayEdgeBlocker. Stops
// themselves render as Todo, Active, or Done.
type SubwayNodeState int

const (
	// SubwayNodeTodo is an available, unclaimed stop.
	SubwayNodeTodo SubwayNodeState = iota
	// SubwayNodeActive is a currently-claimed stop.
	SubwayNodeActive
	// SubwayNodeDone is a completed stop within a visible window.
	SubwayNodeDone
)

// SubwayEdge connects two rendered short IDs.
type SubwayEdge struct {
	FromShortID string
	ToShortID   string
	Kind        SubwayEdgeKind
}

// SubwayEdgeKind classifies an edge for rendering. The cousin-hop
// concept from the previous spine model goes away under Model D —
// lines never cross parent boundaries inline, so every flow edge sits
// between siblings on the same line or between a line anchor and its
// first/last visible stop.
type SubwayEdgeKind int

const (
	// SubwayEdgeFlow connects two adjacent items along a line, or a
	// line anchor to its first/last visible stop.
	SubwayEdgeFlow SubwayEdgeKind = iota
	// SubwayEdgeBranch connects the fork's divergence node to a
	// line's anchor when the line is open (no ingress block).
	SubwayEdgeBranch
	// SubwayEdgeBranchClosed connects the fork's divergence node to
	// a line's anchor when the line is sequence-blocked (a prior
	// sibling phase isn't done yet). Renders with a `⊘` decoration.
	SubwayEdgeBranchClosed
	// SubwayEdgeBlocker is an explicit `blocks` edge between two
	// rendered nodes, distinct from the sequential-phase ingress
	// block. The exact rendering glyph is an open question
	// (different glyph, dashed style, or color).
	SubwayEdgeBlocker
)

// ------------------------------------------------------------------
// Line collection (Phase 1)
// ------------------------------------------------------------------

// lineSeed is the intermediate result of line collection: a parent
// whose subtree contains an active focal or a lookahead-touched stop.
// LCA fork computation and windowing consume seeds in tree order.
//
// anchors is the union of claimed children of parent and
// lookahead-touched children of parent, deduplicated, in tree order.
// Both sets serve as window anchors for the ±N elision step.
type lineSeed struct {
	parent  *graphTask
	anchors []*graphTask
}

// collectLines returns the line seeds that should render given the
// set of focal tasks (currently-claimed or, when no claims exist, the
// globally-next available leaf). Each focal contributes its parent's
// line; from each focal we walk +L leaves in tree-traversal order,
// and any parent of a touched stop also gets a line.
//
// Output ordering is preorder over the project tree, so visually-
// adjacent lines sit next to each other in the rendered subway.
// Within each line, anchors are sorted by sort_order (tree order).
func collectLines(w *graphWorld, focals []*graphTask, L int) []*lineSeed {
	if len(focals) == 0 {
		return nil
	}

	type lineDraft struct {
		parent  *graphTask
		anchors map[int64]*graphTask
	}
	drafts := map[int64]*lineDraft{}

	addAnchor := func(stop *graphTask) {
		if stop == nil || stop.parent == nil {
			return
		}
		parent := stop.parent
		d, ok := drafts[parent.id]
		if !ok {
			d = &lineDraft{parent: parent, anchors: map[int64]*graphTask{}}
			drafts[parent.id] = d
		}
		d.anchors[stop.id] = stop
	}

	for _, focal := range focals {
		addAnchor(focal)
		cur := focal
		for i := 0; i < L; i++ {
			next := nextLeaf(w, cur)
			if next == nil {
				break
			}
			addAnchor(next)
			cur = next
		}
	}

	preorder := preorderAll(w)
	pos := make(map[int64]int, len(preorder))
	for i, t := range preorder {
		pos[t.id] = i
	}

	ordered := make([]*lineDraft, 0, len(drafts))
	for _, d := range drafts {
		ordered = append(ordered, d)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		return pos[ordered[i].parent.id] < pos[ordered[j].parent.id]
	})

	out := make([]*lineSeed, 0, len(ordered))
	for _, d := range ordered {
		anchors := make([]*graphTask, 0, len(d.anchors))
		for _, a := range d.anchors {
			anchors = append(anchors, a)
		}
		sort.SliceStable(anchors, func(i, j int) bool {
			return anchors[i].sortOrder < anchors[j].sortOrder
		})
		out = append(out, &lineSeed{parent: d.parent, anchors: anchors})
	}
	return out
}

// nextLeaf returns the next preorder leaf strictly after t in the
// tree, or nil if no such leaf exists. "Leaf" means a task with no
// children: lookahead skips over internal nodes, since stops along a
// line are always its parent's direct children.
func nextLeaf(w *graphWorld, t *graphTask) *graphTask {
	cur := t
	for {
		next := successorPreorder(w, cur)
		if next == nil {
			return nil
		}
		if len(next.children) == 0 {
			return next
		}
		cur = next
	}
}

// computeFork returns the Fork that connects two or more lines, or
// nil when there is fewer than two lines (the LCA would be chrome
// with only one line) or when the line parents have no common
// ancestor (disjoint trees).
//
// AncestorChain currently has length 1: the lowest common ancestor
// of all line parents. Extending the chain upward to render an
// only-child cascade above the divergence point ("deep-LCA path"
// in the spec) is a documented refinement and isn't applied here.
//
// LineIndices preserves the display order of seeds — collectLines
// emits them in preorder, so the fork keeps that ordering.
func computeFork(seeds []*lineSeed) *Fork {
	if len(seeds) < 2 {
		return nil
	}
	lca := seeds[0].parent
	for i := 1; i < len(seeds); i++ {
		lca = lcaPair(lca, seeds[i].parent)
		if lca == nil {
			return nil
		}
	}
	indices := make([]int, len(seeds))
	for i := range indices {
		indices[i] = i
	}
	return &Fork{
		AncestorChain: []string{lca.shortID},
		LineIndices:   indices,
	}
}

// lcaPair returns the lowest common ancestor of two tasks, or nil
// when they share no ancestor (disjoint trees). When one is an
// ancestor of the other, it is returned directly.
func lcaPair(a, b *graphTask) *graphTask {
	if a == nil || b == nil {
		return nil
	}
	seen := map[int64]*graphTask{}
	for cur := a; cur != nil; cur = cur.parent {
		seen[cur.id] = cur
	}
	for cur := b; cur != nil; cur = cur.parent {
		if anc, ok := seen[cur.id]; ok {
			return anc
		}
	}
	return nil
}

// successorPreorder returns the next task in DFS preorder strictly
// after t, or nil. The walk descends into t's first child when one
// exists; otherwise it ascends until it finds an ancestor with a
// following sibling.
func successorPreorder(w *graphWorld, t *graphTask) *graphTask {
	if len(t.children) > 0 {
		return t.children[0]
	}
	for cur := t; cur != nil; cur = cur.parent {
		sibs := siblingList(w, cur)
		for i, s := range sibs {
			if s.id == cur.id && i+1 < len(sibs) {
				return sibs[i+1]
			}
		}
	}
	return nil
}
