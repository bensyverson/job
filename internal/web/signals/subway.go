package signals

import (
	"context"
	"database/sql"
	"sort"
	"time"
)

// Default windowing and lookahead values for the production home view.
// L was tightened from the original spec default of 2 to 1 once the
// dashboard was rendered against real data and the wider preview made
// rows feel cluttered. N stays at 2. The design doc's six reference
// scenarios are still authored against L=2 — tests that exercise those
// shapes pass 2 explicitly via buildSubwayWith.
const (
	subwayLookahead = 1
	subwayWindow    = 2
)

// BuildSubway loads the current task graph and produces the
// topological Subway model: lines, optional fork, deduplicated
// nodes, and typed edges. Snapshot-per-request; rendering and SVG
// layout live downstream in the render package.
//
// `now` is reserved for future use (e.g. filtering stale claims) and
// is currently unused, kept for signature symmetry with
// ComputeMiniGraph.
func BuildSubway(ctx context.Context, db *sql.DB, _ time.Time) (Subway, error) {
	w, err := loadGraphWorld(ctx, db)
	if err != nil {
		return Subway{}, err
	}
	return buildSubway(w), nil
}

// buildSubway is the pure, world-driven core of BuildSubway running
// at production defaults. Tests that depend on specific lookahead /
// window values call buildSubwayWith instead.
func buildSubway(w *graphWorld) Subway {
	return buildSubwayWith(w, subwayLookahead, subwayWindow)
}

// buildSubwayWith is the parameterized core. The public entry point
// (BuildSubway) and buildSubway both delegate here.
func buildSubwayWith(w *graphWorld, lookahead, window int) Subway {
	focals := pickFocals(w)
	if len(focals) == 0 {
		return Subway{}
	}
	seeds := collectLines(w, focals, lookahead)
	if len(seeds) == 0 {
		return Subway{}
	}
	forks := computeForks(seeds)

	lines := make([]Line, len(seeds))
	for i, seed := range seeds {
		lines[i] = applyWindow(seed, window)
	}

	// Render order: fork ancestors first, then for each line its
	// anchor followed by its visible stops. Deduplicated by ShortID.
	byShort := indexByShortID(w)
	var nodes []SubwayNode
	seen := map[string]bool{}
	pushNode := func(t *graphTask) {
		if t == nil || seen[t.shortID] {
			return
		}
		seen[t.shortID] = true
		nodes = append(nodes, SubwayNode{
			ShortID: t.shortID,
			Title:   t.title,
			State:   subwayState(t),
			Actor:   t.actor,
			URL:     "/tasks/" + t.shortID,
		})
	}
	for _, fork := range forks {
		for _, sid := range fork.AncestorChain {
			pushNode(byShort[sid])
		}
	}
	for i, seed := range seeds {
		pushNode(seed.parent)
		for _, item := range lines[i].Items {
			if item.Kind == LineItemStop {
				pushNode(byShort[item.ShortID])
			}
		}
	}

	// Edges. Order: branch edges (fork → each line anchor), then
	// flow edges line by line, then explicit blocker edges.
	var edges []SubwayEdge
	anchorSet := map[string]bool{}
	for _, seed := range seeds {
		anchorSet[seed.parent.shortID] = true
	}
	for _, fork := range forks {
		divergence := fork.AncestorChain[len(fork.AncestorChain)-1]
		for _, idx := range fork.LineIndices {
			anchor := seeds[idx].parent
			kind := SubwayEdgeBranch
			if anchor.openBlockers > 0 {
				kind = SubwayEdgeBranchClosed
			}
			edges = append(edges, SubwayEdge{
				FromShortID: divergence,
				ToShortID:   anchor.shortID,
				Kind:        kind,
			})
		}
	}
	// Same-line stop blockage renders on the immediate ingress edge,
	// not as a long span from the original blocker — the subway
	// metaphor is "this stop's ingress is closed," like BranchClosed
	// at the LCA but within a single line. For each rendered open
	// blocker on the same line as something it blocks, the *nearest*
	// blocked stop's ingress edge becomes a Blocker (replacing Flow);
	// subsequent blocks by the same blocker are transitive and don't
	// earn an extra marker. Cross-line blocks at stop level aren't
	// drawn here — the design covers cross-line blockage via
	// BranchClosed on the LCA ingress to the line anchor.
	candidates := map[string][]*graphTask{}
	var blockerOrder []string
	for _, n := range nodes {
		if anchorSet[n.ShortID] {
			continue
		}
		t := byShort[n.ShortID]
		if t == nil || t.parent == nil {
			continue
		}
		for _, blockerID := range t.blockerIDs {
			blocker := w.byID[blockerID]
			if blocker == nil || blocker.parent == nil {
				continue
			}
			if blocker.status == "done" || blocker.status == "canceled" {
				continue
			}
			if !seen[blocker.shortID] {
				continue
			}
			// Same-line check: blocker and blocked share a parent.
			if blocker.parent.id != t.parent.id {
				continue
			}
			if _, exists := candidates[blocker.shortID]; !exists {
				blockerOrder = append(blockerOrder, blocker.shortID)
			}
			candidates[blocker.shortID] = append(candidates[blocker.shortID], t)
		}
	}
	preorder := preorderAll(w)
	preorderPos := make(map[int64]int, len(preorder))
	for i, t := range preorder {
		preorderPos[t.id] = i
	}
	lineByAnchor := map[string]int{}
	for i, line := range lines {
		lineByAnchor[line.AnchorShortID] = i
	}
	type pair struct{ from, to string }
	ingressBlocked := map[pair]bool{}
	for _, blockerSID := range blockerOrder {
		blocked := candidates[blockerSID]
		sort.SliceStable(blocked, func(i, j int) bool {
			return preorderPos[blocked[i].id] < preorderPos[blocked[j].id]
		})
		nearest := blocked[0]
		lineIdx, ok := lineByAnchor[nearest.parent.shortID]
		if !ok {
			continue
		}
		line := lines[lineIdx]
		pred := line.AnchorShortID
		for _, item := range line.Items {
			if item.Kind != LineItemStop {
				continue
			}
			if item.ShortID == nearest.shortID {
				break
			}
			pred = item.ShortID
		}
		ingressBlocked[pair{pred, nearest.shortID}] = true
	}

	for _, line := range lines {
		prev := line.AnchorShortID
		for _, item := range line.Items {
			if item.Kind != LineItemStop {
				continue
			}
			kind := SubwayEdgeFlow
			if ingressBlocked[pair{prev, item.ShortID}] {
				kind = SubwayEdgeBlocker
			}
			edges = append(edges, SubwayEdge{
				FromShortID: prev,
				ToShortID:   item.ShortID,
				Kind:        kind,
			})
			prev = item.ShortID
		}
	}

	return Subway{
		Lines: lines,
		Forks: forks,
		Nodes: nodes,
		Edges: edges,
	}
}

// indexByShortID returns a lookup table from short ID to graphTask
// for every task in w. The Subway model addresses tasks by short ID,
// so this map is built once per BuildSubway call.
func indexByShortID(w *graphWorld) map[string]*graphTask {
	out := make(map[string]*graphTask, len(w.byID))
	for _, t := range w.byID {
		out[t.shortID] = t
	}
	return out
}

// subwayState maps a task's status to its rendered Subway state.
// Blocked is no longer a node state under the subway model: closure
// lives on the ingress edge (BranchClosed) or as a Blocker edge.
func subwayState(t *graphTask) SubwayNodeState {
	switch t.status {
	case "claimed":
		return SubwayNodeActive
	case "done", "canceled":
		return SubwayNodeDone
	}
	return SubwayNodeTodo
}

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
	// Forks holds one entry per cluster of lines that share a project
	// root. When all rendered lines share a single root the slice has
	// one Fork; with cross-project claims we emit one Fork per root so
	// each cluster's transfer station renders. A single isolated line
	// (one line, one root) leaves the slice empty — the LCA would be
	// chrome.
	Forks []*Fork
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
		for range L {
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

// applyWindow turns a lineSeed into a Line with ±N windowing
// applied. Each anchor (claimed or lookahead-touched stop) anchors a
// window of ±N siblings; the union of those windows becomes the set
// of visible stops. Elision markers (`…`) appear between the parent
// anchor and the first visible stop when the leading siblings are
// hidden, between two visible windows that aren't adjacent, and
// after the last visible stop when trailing siblings are hidden.
//
// Done siblings don't anchor windows of their own — they render only
// when they fall inside another anchor's window. Two close focals
// produce a single merged window (the line stays visually
// continuous); two distant focals produce two windows separated by
// a `…` (the multi-focal union case from the spec).
func applyWindow(seed *lineSeed, N int) Line {
	if seed == nil || seed.parent == nil {
		return Line{}
	}
	line := Line{AnchorShortID: seed.parent.shortID}
	children := seed.parent.children
	if len(children) == 0 {
		return line
	}

	indexOf := make(map[int64]int, len(children))
	for i, c := range children {
		indexOf[c.id] = i
	}

	visibleSet := map[int]bool{}
	for _, anchor := range seed.anchors {
		idx, ok := indexOf[anchor.id]
		if !ok {
			continue
		}
		lo := max(idx-N, 0)
		hi := min(idx+N, len(children)-1)
		for i := lo; i <= hi; i++ {
			visibleSet[i] = true
		}
	}
	if len(visibleSet) == 0 {
		return line
	}

	visible := make([]int, 0, len(visibleSet))
	for i := range visibleSet {
		visible = append(visible, i)
	}
	sort.Ints(visible)

	if visible[0] > 0 {
		line.Items = append(line.Items, LineItem{Kind: LineItemElision})
	}
	for i, idx := range visible {
		if i > 0 && idx > visible[i-1]+1 {
			line.Items = append(line.Items, LineItem{Kind: LineItemElision})
		}
		line.Items = append(line.Items, LineItem{
			Kind:    LineItemStop,
			ShortID: children[idx].shortID,
		})
	}
	if visible[len(visible)-1] < len(children)-1 {
		line.Items = append(line.Items, LineItem{Kind: LineItemElision})
	}
	return line
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

// computeForks groups seeds by project root and emits one Fork per
// cluster. Cross-project claims (lines under different roots) each
// get their own transfer station; a single isolated line yields no
// fork (the LCA would be chrome). Within a multi-line cluster the
// fork sits at the LCA of the cluster's seed parents (often the
// cluster root, sometimes deeper); a single-line cluster in a
// multi-cluster Subway anchors its fork on the cluster root so the
// project root still renders as the line's transfer station.
//
// LineIndices in each Fork are indices into the original seeds slice
// (preorder-stable from collectLines).
func computeForks(seeds []*lineSeed) []*Fork {
	if len(seeds) < 2 {
		return nil
	}
	type cluster struct {
		root    *graphTask
		seedIdx []int
	}
	var clusters []cluster
	clusterByRoot := map[int64]int{}
	for i, seed := range seeds {
		root := seed.parent
		for root.parent != nil {
			root = root.parent
		}
		if idx, ok := clusterByRoot[root.id]; ok {
			clusters[idx].seedIdx = append(clusters[idx].seedIdx, i)
			continue
		}
		clusterByRoot[root.id] = len(clusters)
		clusters = append(clusters, cluster{root: root, seedIdx: []int{i}})
	}
	if len(clusters) == 1 && len(clusters[0].seedIdx) < 2 {
		return nil
	}
	var forks []*Fork
	for _, c := range clusters {
		if len(c.seedIdx) >= 2 {
			subSeeds := make([]*lineSeed, len(c.seedIdx))
			for j, i := range c.seedIdx {
				subSeeds[j] = seeds[i]
			}
			f := computeFork(subSeeds)
			if f == nil {
				// Cluster shares a root, so an LCA must exist; fall
				// back defensively to the cluster root.
				f = &Fork{AncestorChain: []string{c.root.shortID}}
			}
			f.LineIndices = append([]int(nil), c.seedIdx...)
			forks = append(forks, f)
			continue
		}
		// Singleton in a multi-cluster Subway — anchor on the root.
		forks = append(forks, &Fork{
			AncestorChain: []string{c.root.shortID},
			LineIndices:   append([]int(nil), c.seedIdx...),
		})
	}
	return forks
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
