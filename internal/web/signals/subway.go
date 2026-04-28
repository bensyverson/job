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
//
// Mode dispatch is per-cluster (focals grouped by project root):
// single-focal clusters render via buildSingleFocalLine (project/
// 2026-04-27-graph-row-merging.md, single-focal preorder window
// mode); multi-focal clusters render via buildMultiFocalRows (same
// doc, multi-focal tree-map mode), with rows grouped by parent
// short ID into Fork entries so the existing layout/branch-curve
// loop in render/subway_layout.go keeps working unchanged. The
// layout pivot to depth-aligned columns and retiring AncestorChain
// in favor of per-row ParentShortID lands in S3.
//
// The lookahead parameter is unused under the new modes; it stays
// on the signature so tests authored against L=2 still parameterize
// the call. The legacy collectLines/applyWindow/computeForks
// helpers remain in this file with their own unit tests until a
// follow-up retires them.
func buildSubwayWith(w *graphWorld, _ /* lookahead */, window int) Subway {
	focals := pickFocals(w)
	if len(focals) == 0 {
		return Subway{}
	}

	preorder := preorderAll(w)
	preorderPos := make(map[int64]int, len(preorder))
	for i, t := range preorder {
		preorderPos[t.id] = i
	}

	clusters := map[int64][]*graphTask{}
	var rootOrder []*graphTask
	for _, f := range focals {
		root := f
		for root.parent != nil {
			root = root.parent
		}
		if _, ok := clusters[root.id]; !ok {
			rootOrder = append(rootOrder, root)
		}
		clusters[root.id] = append(clusters[root.id], f)
	}
	sort.SliceStable(rootOrder, func(i, j int) bool {
		return preorderPos[rootOrder[i].id] < preorderPos[rootOrder[j].id]
	})

	var lines []Line
	var forks []*Fork
	for _, root := range rootOrder {
		clusterFocals := clusters[root.id]
		if len(clusterFocals) == 1 {
			lines = append(lines, buildSingleFocalLine(w, clusterFocals[0], window))
			continue
		}
		// Multi-focal cluster: render via the tree-map mode
		// (project/2026-04-27-graph-row-merging.md). Each row in
		// rows carries a ParentShortID identifying its branch
		// parent in the focal-path subgraph (empty for the
		// topmost row, whose leftmost is the cluster LCA). For
		// the S2 Fork shim we group sub-rows by ParentShortID
		// and emit one Fork per group with the parent as a
		// single-element AncestorChain — that keeps the
		// existing layout/branch-curve loop in
		// render/subway_layout.go drawing each row's curve from
		// its parent to its leftmost. The layout pivot to
		// depth-aligned columns + retiring AncestorChain in
		// favor of per-row ParentShortID lands in S3 (Ai3De).
		baseIdx := len(lines)
		rows := buildMultiFocalRows(w, clusterFocals, window)
		if len(rows) == 0 {
			continue
		}
		lines = append(lines, rows...)
		type forkGroup struct {
			parent  string
			indices []int
		}
		var groups []*forkGroup
		groupByParent := map[string]*forkGroup{}
		for i, row := range rows {
			if row.ParentShortID == "" {
				continue
			}
			g, ok := groupByParent[row.ParentShortID]
			if !ok {
				g = &forkGroup{parent: row.ParentShortID}
				groupByParent[row.ParentShortID] = g
				groups = append(groups, g)
			}
			g.indices = append(g.indices, baseIdx+i)
		}
		for _, g := range groups {
			forks = append(forks, &Fork{
				AncestorChain: []string{g.parent},
				LineIndices:   g.indices,
			})
		}
	}
	if len(lines) == 0 {
		return Subway{}
	}

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
	for _, line := range lines {
		pushNode(byShort[line.AnchorShortID])
		for _, item := range line.Items {
			if item.Kind == LineItemStop {
				pushNode(byShort[item.ShortID])
			}
		}
	}

	var edges []SubwayEdge
	for _, fork := range forks {
		divergence := fork.AncestorChain[len(fork.AncestorChain)-1]
		for _, idx := range fork.LineIndices {
			anchor := byShort[lines[idx].AnchorShortID]
			if anchor == nil {
				continue
			}
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
	// Same-row stop blockage renders on the immediate ingress edge.
	// "Same row" means both stops appear on the same Line (under the
	// new single-focal preorder mode this is "same preorder window";
	// under the legacy parent-rooted model it's "same parent," which
	// happens to be a stricter version of the same predicate). The
	// nearest blocked stop's ingress edge becomes a Blocker
	// (replacing Flow); subsequent blocks by the same blocker are
	// transitive and don't earn an extra marker.
	lineByStop := map[string]int{}
	for i, line := range lines {
		for _, item := range line.Items {
			if item.Kind == LineItemStop {
				lineByStop[item.ShortID] = i
			}
		}
	}
	candidates := map[string][]*graphTask{}
	var blockerOrder []string
	anchorSet := map[string]bool{}
	for _, line := range lines {
		anchorSet[line.AnchorShortID] = true
	}
	for _, n := range nodes {
		if anchorSet[n.ShortID] {
			continue
		}
		t := byShort[n.ShortID]
		if t == nil {
			continue
		}
		for _, blockerID := range t.blockerIDs {
			blocker := w.byID[blockerID]
			if blocker == nil {
				continue
			}
			if blocker.status == "done" || blocker.status == "canceled" {
				continue
			}
			if !seen[blocker.shortID] {
				continue
			}
			bLine, bOK := lineByStop[blocker.shortID]
			tLine, tOK := lineByStop[t.shortID]
			if !bOK || !tOK || bLine != tLine {
				continue
			}
			if _, exists := candidates[blocker.shortID]; !exists {
				blockerOrder = append(blockerOrder, blocker.shortID)
			}
			candidates[blocker.shortID] = append(candidates[blocker.shortID], t)
		}
	}
	type pair struct{ from, to string }
	ingressBlocked := map[pair]bool{}
	for _, blockerSID := range blockerOrder {
		blocked := candidates[blockerSID]
		sort.SliceStable(blocked, func(i, j int) bool {
			return preorderPos[blocked[i].id] < preorderPos[blocked[j].id]
		})
		nearest := blocked[0]
		lineIdx, ok := lineByStop[nearest.shortID]
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
			switch item.Kind {
			case LineItemStop:
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
			case LineItemMore:
				edges = append(edges, SubwayEdge{
					FromShortID: prev,
					ToShortID:   MoreShortID(line.AnchorShortID),
					Kind:        SubwayEdgeFlow,
				})
			}
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

// Line is a single subway line / row: an anchor (the row's
// leftmost) followed by an ordered sequence of items (stops and
// elision markers). The anchor's semantics depend on the cluster's
// rendering mode:
//
//   - Legacy parent-rooted line: anchor is the parent task whose
//     children render as stops along this line.
//   - Single-focal preorder window mode (project/2026-04-27-graph-
//     row-merging.md): anchor is the project root; items are the
//     focal's ±N preorder neighbors within the project tree.
//   - Multi-focal tree-map mode (same doc): anchor is the row's
//     leftmost in the focal-path subgraph; items are the visible
//     stops on that row's branch with per-row ±N windowing.
//     ParentShortID identifies the row's parent in the subgraph
//     for the branch-curve geometry; it is empty for the topmost
//     row.
//
// Tree order, not claim order: stops follow their underlying
// sort_order so the layout stays stable as claims churn.
type Line struct {
	AnchorShortID string
	ParentShortID string
	Items         []LineItem
}

// LineItem is one element in a Line's sequence — a stop, an in-gap
// elision marker, or a terminal More pill.
//
// Stop: ShortID is set; renders as a node disc on the line.
//
// Elision: drawn as small dots in the gap between two surrounding
// items on the line (anchor↔stop, stop↔stop). It does not consume a
// column slot and only carries presentational meaning.
//
// More: a terminal `(+N)` pill that stands in for trailing siblings
// hidden after the last visible stop. MoreCount is the number of
// elided trailing siblings; it consumes a column slot like a stop
// and renders as a small pill at the end of the line.
type LineItem struct {
	Kind      LineItemKind
	ShortID   string
	MoreCount int
}

// MoreShortID returns the synthetic short ID assigned to the
// terminal More pill on the line whose anchor is anchorShortID.
// The `~` prefix never collides with real short IDs (which are
// alphanumeric). Edges originating from the last visible stop and
// terminating at the More pill are addressed via this synthetic ID;
// the layout step resolves it to the pill's pixel position.
func MoreShortID(anchorShortID string) string {
	return "~more~" + anchorShortID
}

// LineItemKind tags a LineItem as a stop, an in-gap elision, a
// broken-line elision (single-focal preorder mode), a terminating
// ellipsis (single-focal preorder mode), or a terminal More pill.
type LineItemKind int

const (
	// LineItemStop is a rendered stop on the line; ShortID is set.
	LineItemStop LineItemKind = iota
	// LineItemElision is an in-gap dots marker; ShortID is empty,
	// MoreCount is zero. Sits between two surrounding items, takes
	// no slot. Used by the legacy parent-rooted line model.
	LineItemElision
	// LineItemMore is a terminal `(+N)` pill summarizing the count
	// of hidden trailing siblings. MoreCount > 0; ShortID is empty.
	// Takes a column slot like a stop. Used by the legacy
	// parent-rooted line model; the single-focal preorder mode
	// renders trailing elision as LineItemElisionTerminating instead.
	LineItemMore
	// LineItemElisionBroken is a broken-line elision used by the
	// single-focal preorder window mode: a short stub from the
	// preceding item, three small SVG circles in negative space, a
	// short stub into the following item. Used between leftmost and
	// the row's first content stop when the -N walk doesn't reach
	// the project root, and between two non-adjacent visible windows
	// inside the same row. Takes no slot.
	LineItemElisionBroken
	// LineItemElisionTerminating is a trailing terminating ellipsis
	// at the right edge of a single-focal preorder row: disc →
	// segment → three dots, no trailing arrow stub. Replaces the
	// legacy LineItemMore pill in the new mode. Takes no slot.
	LineItemElisionTerminating
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
// focalAnchors are claimed children of parent (or, when nothing is
// claimed globally, the single globally-next leaf). They always
// anchor a ±N window in applyWindow.
//
// lookaheadAnchors are children of parent reached via the +L leaf
// walk from a focal. They anchor a window only when their index
// isn't already inside some focal's ±N window on the same parent —
// the dedup that keeps the focal's line capped at ±N around the
// focal in the common adjacent-lookahead case, while still
// preserving a disjoint window for far lookaheads.
//
// Both slices are sorted in tree order (sort_order). A child can
// appear in only one slice: if a task is both a focal and a
// lookahead target, it stays a focal.
type lineSeed struct {
	parent           *graphTask
	focalAnchors     []*graphTask
	lookaheadAnchors []*graphTask
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
		parent     *graphTask
		focals     map[int64]*graphTask
		lookaheads map[int64]*graphTask
	}
	drafts := map[int64]*lineDraft{}

	getDraft := func(parent *graphTask) *lineDraft {
		d, ok := drafts[parent.id]
		if !ok {
			d = &lineDraft{
				parent:     parent,
				focals:     map[int64]*graphTask{},
				lookaheads: map[int64]*graphTask{},
			}
			drafts[parent.id] = d
		}
		return d
	}
	// focalParents is the set of every parent that a focal sits
	// under. A lookahead anchor whose parent is one of these — or a
	// descendant of one — is inside an already-rendered line's
	// subtree and earns no new line. Only lookaheads that *exit*
	// upward across a parent boundary (cousin or further) seed a new
	// peek-line. The fork machinery extends the LCA chain in that
	// case.
	focalParents := map[int64]bool{}
	for _, focal := range focals {
		if focal.parent != nil {
			focalParents[focal.parent.id] = true
		}
	}
	inFocalSubtree := func(stop *graphTask) bool {
		if stop == nil || stop.parent == nil {
			return false
		}
		for cur := stop.parent; cur != nil; cur = cur.parent {
			if focalParents[cur.id] {
				return true
			}
		}
		return false
	}
	addFocal := func(stop *graphTask) {
		if stop == nil || stop.parent == nil {
			return
		}
		getDraft(stop.parent).focals[stop.id] = stop
	}
	addLookahead := func(stop *graphTask) {
		if stop == nil || stop.parent == nil {
			return
		}
		if inFocalSubtree(stop) {
			return
		}
		getDraft(stop.parent).lookaheads[stop.id] = stop
	}

	for _, focal := range focals {
		addFocal(focal)
		cur := focal
		for range L {
			next := nextLeaf(w, cur)
			if next == nil {
				break
			}
			addLookahead(next)
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
		// A task that is both a focal and a lookahead target stays a
		// focal — focals always anchor a window, lookaheads only when
		// not already covered.
		focals := make([]*graphTask, 0, len(d.focals))
		for _, a := range d.focals {
			focals = append(focals, a)
		}
		sort.SliceStable(focals, func(i, j int) bool {
			return focals[i].sortOrder < focals[j].sortOrder
		})
		lookaheads := make([]*graphTask, 0, len(d.lookaheads))
		for _, a := range d.lookaheads {
			if _, isFocal := d.focals[a.id]; isFocal {
				continue
			}
			lookaheads = append(lookaheads, a)
		}
		sort.SliceStable(lookaheads, func(i, j int) bool {
			return lookaheads[i].sortOrder < lookaheads[j].sortOrder
		})
		out = append(out, &lineSeed{
			parent:           d.parent,
			focalAnchors:     focals,
			lookaheadAnchors: lookaheads,
		})
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
	addWindow := func(idx int) {
		lo := max(idx-N, 0)
		hi := min(idx+N, len(children)-1)
		for i := lo; i <= hi; i++ {
			visibleSet[i] = true
		}
	}
	for _, anchor := range seed.focalAnchors {
		idx, ok := indexOf[anchor.id]
		if !ok {
			continue
		}
		addWindow(idx)
	}
	// Lookahead anchors anchor a window only when their index isn't
	// already covered by a focal's window — adjacent lookahead stays
	// inside the focal's frame (the common single-claim case caps at
	// ±N around the focal), far lookahead opens a disjoint window
	// that the elision pass joins to the focal's span with an in-line
	// `…`.
	for _, anchor := range seed.lookaheadAnchors {
		idx, ok := indexOf[anchor.id]
		if !ok {
			continue
		}
		if visibleSet[idx] {
			continue
		}
		addWindow(idx)
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
	if last := visible[len(visible)-1]; last < len(children)-1 {
		// Trailing siblings beyond the visible window collapse to a
		// terminating ellipsis; the legacy `(+N)` pill (LineItemMore)
		// is gone from the data path (project/2026-04-27-graph-row-
		// merging.md, S1). The template/CSS for the pill linger until
		// S4 cleanup.
		line.Items = append(line.Items, LineItem{Kind: LineItemElisionTerminating})
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

// buildSingleFocalLine returns the Line for a single-focal cluster
// under the new preorder window mode (project/2026-04-27-graph-row-
// merging.md). The row's leftmost is the project root containing
// focal; content stops are the focal's ±N preorder neighbors within
// that project tree.
//
// AnchorShortID is the project root's short ID. Items lists the
// visible content stops in preorder, excluding the project root
// itself (rendered as the anchor at col 0). Leading broken-line
// elision (LineItemElisionBroken) sits before the first content stop
// when the -N walk doesn't reach the project root; trailing
// terminating elision (LineItemElisionTerminating) sits at the right
// edge when the +N walk continues past the row's last visible stop.
//
// The function never emits LineItemMore — trailing siblings collapse
// to LineItemElisionTerminating. It also never emits LineItemElision
// (the old in-gap dots marker), which belongs to the legacy
// parent-rooted line model.
func buildSingleFocalLine(_ *graphWorld, focal *graphTask, N int) Line {
	if focal == nil {
		return Line{}
	}
	root := focal
	for root.parent != nil {
		root = root.parent
	}
	preorder := preorderSubtree(root)
	focalPos := -1
	for i, t := range preorder {
		if t.id == focal.id {
			focalPos = i
			break
		}
	}
	if focalPos < 0 {
		return Line{AnchorShortID: root.shortID}
	}

	startPos := max(focalPos-N, 0)
	endPos := min(focalPos+N, len(preorder)-1)

	line := Line{AnchorShortID: root.shortID}
	// Leading broken-line elision when the -N walk doesn't reach the
	// project root. The leftmost (anchor) is rendered separately at
	// col 0; the elision sits between it and the row's first content
	// stop.
	if startPos > 0 {
		line.Items = append(line.Items, LineItem{Kind: LineItemElisionBroken})
	}
	// Content stops in preorder. Skip the project root itself when it
	// falls inside the window (it's already rendered as the anchor;
	// invariant 2 forbids rendering a node twice).
	for i := startPos; i <= endPos; i++ {
		t := preorder[i]
		if t.id == root.id {
			continue
		}
		line.Items = append(line.Items, LineItem{
			Kind:    LineItemStop,
			ShortID: t.shortID,
		})
	}
	// Trailing terminating elision when the +N walk continues past
	// the row's last visible stop. If the focal sits at or past the
	// preorder's last position, the row terminates at the focal —
	// no trailing dots.
	if endPos < len(preorder)-1 {
		line.Items = append(line.Items, LineItem{Kind: LineItemElisionTerminating})
	}
	return line
}

// focalPathSubgraph returns the LCA of focals and the IDs of every
// node on the path from the LCA down to each focal. Focals must
// share a project root; otherwise the function returns (nil, nil).
//
// The subgraph is the structural skeleton used by the multi-focal
// tree-map mode (project/2026-04-27-graph-row-merging.md): every
// rendered node in that mode comes from this set, every fork point
// lives within it, and non-focal-bearing branches off the LCA's
// subtree are excluded.
func focalPathSubgraph(focals []*graphTask) (*graphTask, map[int64]bool) {
	if len(focals) == 0 {
		return nil, nil
	}
	lca := focals[0]
	for i := 1; i < len(focals); i++ {
		lca = lcaPair(lca, focals[i])
		if lca == nil {
			return nil, nil
		}
	}
	ids := map[int64]bool{}
	for _, f := range focals {
		for cur := f; cur != nil; cur = cur.parent {
			ids[cur.id] = true
			if cur.id == lca.id {
				break
			}
		}
	}
	return lca, ids
}

// subgraphForkPoints returns the subgraph nodes that have two or
// more in-subgraph children, in tree (preorder) order. These are
// the divergence points where rows split in the multi-focal tree-
// map mode.
func subgraphForkPoints(ids map[int64]bool, lca *graphTask) []*graphTask {
	if lca == nil || !ids[lca.id] {
		return nil
	}
	var forks []*graphTask
	var visit func(t *graphTask)
	visit = func(t *graphTask) {
		if !ids[t.id] {
			return
		}
		inCount := 0
		for _, c := range t.children {
			if ids[c.id] {
				inCount++
			}
		}
		if inCount >= 2 {
			forks = append(forks, t)
		}
		for _, c := range t.children {
			visit(c)
		}
	}
	visit(lca)
	return forks
}

// buildMultiFocalRows returns the rows for a multi-focal cluster
// under the new tree-map mode (project/2026-04-27-graph-row-
// merging.md). Each row is a maximal linear chain through the
// focal-path subgraph between fork points; every row's leftmost
// is depth-aligned and rendered as the curve target for any
// incoming branch curve. Rows beyond the topmost carry a
// ParentShortID identifying the parent's rendered position so the
// layout can draw the branch curve.
//
// All focals must share a project root (cluster invariant). The
// topmost row's leftmost is the LCA of all focals; sub-rows
// branch off fork points (subgraph nodes with ≥2 in-subgraph
// children). The same-parent-siblings carve-out keeps focal
// siblings sharing a parent on one row, with ≤2 non-focal
// siblings rendered inline and ≥3 collapsed to a mid-line broken-
// line elision.
func buildMultiFocalRows(w *graphWorld, focals []*graphTask, N int) []Line {
	if len(focals) == 0 {
		return nil
	}
	lca, subgraph := focalPathSubgraph(focals)
	if lca == nil {
		return nil
	}
	focalSet := map[int64]bool{}
	for _, f := range focals {
		focalSet[f.id] = true
	}

	var rows []Line
	queue := []buildOneRowResult{{leftmost: lca, parentShort: ""}}
	for len(queue) > 0 {
		rt := queue[0]
		queue = queue[1:]
		row, subRows := buildOneRow(w, rt.leftmost, rt.parentShort, subgraph, focalSet, N)
		rows = append(rows, row)
		queue = append(queue, subRows...)
	}
	return rows
}

// buildOneRowResult is the per-row sub-row task carrying the next
// row's leftmost and parent.
type buildOneRowResult struct {
	leftmost    *graphTask
	parentShort string
}

// buildOneRow constructs a single row starting at leftmost, walking
// the subgraph along the inline child at each fork point until it
// hits a leaf or a same-parent-siblings carve-out. Returns the
// constructed Line plus the sub-row tasks for non-inline subgraph
// children encountered along the chain. Sub-rows are returned in
// tree (preorder) order so the caller can append them to the row
// queue.
func buildOneRow(
	_ *graphWorld,
	leftmost *graphTask,
	parentShort string,
	subgraph map[int64]bool,
	focalSet map[int64]bool,
	N int,
) (Line, []buildOneRowResult) {
	excluded := map[int64]bool{}
	chainIDs := map[int64]bool{leftmost.id: true}
	var subRows []buildOneRowResult
	cur := leftmost
	for {
		var inSubKids, focalLeafKids, deeperKids []*graphTask
		for _, c := range cur.children {
			if !subgraph[c.id] {
				continue
			}
			inSubKids = append(inSubKids, c)
			if focalSet[c.id] {
				focalLeafKids = append(focalLeafKids, c)
			} else {
				deeperKids = append(deeperKids, c)
			}
		}
		if len(inSubKids) == 0 {
			break
		}
		// Carve-out: ≥2 focal-leaf-children share cur's row. Deeper
		// children (if any) become sub-rows branching off cur.
		if len(focalLeafKids) >= 2 {
			for _, c := range deeperKids {
				excluded[c.id] = true
				subRows = append(subRows, buildOneRowResult{
					leftmost: c, parentShort: cur.shortID,
				})
			}
			break
		}
		// No carve-out at this node. If only one in-subgraph child,
		// continue the chain. If ≥2, this is a real fork — every
		// in-subgraph child becomes its own sub-row branching off
		// cur. The data layer doesn't inline; the layout step
		// (project/2026-04-27-graph-row-merging.md, S3) chooses
		// which sub-row to render inline with the parent visually.
		if len(inSubKids) == 1 {
			next := inSubKids[0]
			chainIDs[next.id] = true
			cur = next
			continue
		}
		for _, c := range inSubKids {
			excluded[c.id] = true
			subRows = append(subRows, buildOneRowResult{
				leftmost: c, parentShort: cur.shortID,
			})
		}
		break
	}

	// Preorder walk of leftmost's subtree, skipping subtrees rooted
	// at excluded sub-row leftmosts. The leftmost itself sits at
	// position 0 (anchor); content stops are positions 1..len-1.
	var rowPreorder []*graphTask
	var visit func(t *graphTask)
	visit = func(t *graphTask) {
		if excluded[t.id] {
			return
		}
		rowPreorder = append(rowPreorder, t)
		for _, c := range t.children {
			visit(c)
		}
	}
	visit(leftmost)

	line := Line{AnchorShortID: leftmost.shortID, ParentShortID: parentShort}

	// Identify focal positions on this row.
	var focalPositions []int
	for i, t := range rowPreorder {
		if focalSet[t.id] {
			focalPositions = append(focalPositions, i)
		}
	}
	if len(focalPositions) == 0 {
		// Row's chain ends at a fork point with no focal of its
		// own (its focals live on sub-rows). Items list is empty —
		// only the row's leftmost (anchor) renders, serving as the
		// curve target for sub-rows.
		return line, subRows
	}

	// Compute visible window: union of ±N around each focal in
	// preorder. Then apply the same-parent-siblings carve-out
	// collapse: if there are ≥3 non-focal direct siblings between
	// two focal siblings on a carve-out parent, drop those
	// positions so they elide regardless of windowing.
	visible := map[int]bool{}
	for _, p := range focalPositions {
		lo, hi := p-N, p+N
		if lo < 0 {
			lo = 0
		}
		if hi > len(rowPreorder)-1 {
			hi = len(rowPreorder) - 1
		}
		for i := lo; i <= hi; i++ {
			visible[i] = true
		}
	}
	collapsed := carveOutCollapseRanges(rowPreorder, focalSet)
	for _, rng := range collapsed {
		for i := rng.lo; i <= rng.hi; i++ {
			delete(visible, i)
		}
	}
	var visibleIdx []int
	for i := range visible {
		visibleIdx = append(visibleIdx, i)
	}
	sort.Ints(visibleIdx)
	if len(visibleIdx) == 0 {
		return line, subRows
	}

	// Leading broken-line elision: when the first visible content
	// stop sits past position 1 (the leftmost's first child).
	if visibleIdx[0] > 1 {
		line.Items = append(line.Items, LineItem{Kind: LineItemElisionBroken})
	}
	// Walk visible indices, emitting stops and broken-line elisions
	// for internal gaps. Skip the leftmost (pos 0) — it's the
	// anchor.
	prev := -1
	for _, idx := range visibleIdx {
		if idx == 0 {
			prev = idx
			continue
		}
		if prev >= 1 && idx > prev+1 {
			line.Items = append(line.Items, LineItem{Kind: LineItemElisionBroken})
		}
		line.Items = append(line.Items, LineItem{
			Kind:    LineItemStop,
			ShortID: rowPreorder[idx].shortID,
		})
		prev = idx
	}
	// Trailing terminating elision: visible window doesn't reach
	// the row's last preorder position.
	if visibleIdx[len(visibleIdx)-1] < len(rowPreorder)-1 {
		line.Items = append(line.Items, LineItem{Kind: LineItemElisionTerminating})
	}
	return line, subRows
}

// carveOutCollapseRanges identifies non-focal sibling runs of ≥3
// between two focal-leaf siblings sharing the same parent within
// rowPreorder. The returned ranges are positions inside
// rowPreorder; the caller drops these from the visible window and
// inserts a single broken-line elision marker between the
// surrounding focal stops.
func carveOutCollapseRanges(rowPreorder []*graphTask, focalSet map[int64]bool) []struct{ lo, hi int } {
	posByID := make(map[int64]int, len(rowPreorder))
	for i, t := range rowPreorder {
		posByID[t.id] = i
	}
	// For each parent that has ≥2 focal-leaf children present in
	// rowPreorder, walk between adjacent focal siblings and look
	// for runs of ≥3 non-focal siblings.
	var out []struct{ lo, hi int }
	parentsSeen := map[int64]bool{}
	for _, t := range rowPreorder {
		p := t.parent
		if p == nil || parentsSeen[p.id] {
			continue
		}
		parentsSeen[p.id] = true
		var focalChildren []*graphTask
		for _, c := range p.children {
			if focalSet[c.id] {
				if _, ok := posByID[c.id]; ok {
					focalChildren = append(focalChildren, c)
				}
			}
		}
		if len(focalChildren) < 2 {
			continue
		}
		// Sort by sortOrder (already stable in p.children order).
		sort.SliceStable(focalChildren, func(i, j int) bool {
			return focalChildren[i].sortOrder < focalChildren[j].sortOrder
		})
		for i := 0; i+1 < len(focalChildren); i++ {
			a, b := focalChildren[i], focalChildren[i+1]
			// Count direct non-focal siblings between a and b in
			// p.children order.
			var run []*graphTask
			between := false
			for _, c := range p.children {
				if c.id == a.id {
					between = true
					continue
				}
				if c.id == b.id {
					break
				}
				if !between {
					continue
				}
				if !focalSet[c.id] {
					run = append(run, c)
				}
			}
			if len(run) >= 3 {
				lo := posByID[run[0].id]
				hi := posByID[run[len(run)-1].id]
				out = append(out, struct{ lo, hi int }{lo, hi})
			}
		}
	}
	return out
}

// preorderSubtree returns every task rooted at root in DFS preorder.
// Used by the single-focal preorder window mode to address tasks by
// their position relative to the focal within the focal's project
// tree.
func preorderSubtree(root *graphTask) []*graphTask {
	var out []*graphTask
	var visit func(t *graphTask)
	visit = func(t *graphTask) {
		out = append(out, t)
		for _, c := range t.children {
			visit(c)
		}
	}
	visit(root)
	return out
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
