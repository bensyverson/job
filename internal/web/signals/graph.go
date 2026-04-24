package signals

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"
)

// MiniGraph is a purely topological snapshot of the Home-view
// dependency graph. It describes the lanes each active claim drives
// — their 5-slot spine (L-2, L-1, focal, R+1, R+2) and any extra
// open siblings stacked at the focal — plus a deduplicated pool of
// node metadata and a typed edge list. Grid/pixel positioning and
// SVG routing are the layout task's job; nothing here knows about
// columns, rows, or stroke styles.
type MiniGraph struct {
	Lanes []Lane
	Nodes []GraphNode
	Edges []GraphEdge
}

// Lane is a single focal's spine plus its stacked open siblings.
// Spine is a fixed-width array indexed by SpineL2/SpineL1/
// SpineFocal/SpineR1/SpineR2; an empty string means that slot has
// no hop (e.g. a top-level focal with no preceding phase fills
// SpineFocal only).
type Lane struct {
	FocalShortID string
	Spine        [5]string
	Stacked      []string
}

// Spine slot indices. SpineFocal is always populated for any Lane;
// the others may be empty.
const (
	SpineL2    = 0
	SpineL1    = 1
	SpineFocal = 2
	SpineR1    = 3
	SpineR2    = 4
)

// GraphNode is a single task's presentation metadata, unique per
// graph by ShortID. ChildrenNotShown drives the "+N" pill on parent
// nodes that stand in at phase boundaries — zero means no pill.
type GraphNode struct {
	ShortID          string
	Title            string
	State            GraphNodeState
	Actor            string
	IsParent         bool
	ChildrenNotShown int
	URL              string
}

// GraphNodeState drives the node glyph.
type GraphNodeState int

const (
	GraphNodeTodo GraphNodeState = iota
	GraphNodeActive
	GraphNodeBlocked
	GraphNodeDone
	GraphNodeCluster
)

// GraphEdge connects two nodes by ShortID. Flow is the default solid
// stroke for sibling or parent/child hops; Cousin is a lighter stroke
// for hops that cross a parent boundary (when an ancestor's following
// sibling stands in for a subtree exit); Blocker is a dashed amber
// arc from a blocker task to the task it is blocking.
type GraphEdge struct {
	FromShortID string
	ToShortID   string
	Kind        GraphEdgeKind
}

type GraphEdgeKind int

const (
	GraphEdgeFlow GraphEdgeKind = iota
	GraphEdgeCousin
	GraphEdgeBlocker
)

// Hop budget per lane: 2 left of focal, focal, 2 right of focal.
const (
	graphHopsLeft  = 2
	graphHopsRight = 2
)

// ComputeMiniGraph produces a MiniGraph from the current database
// state. Snapshot-per-request; a later task may wire SSE updates on
// top of the data-home-graph container in the template.
func ComputeMiniGraph(ctx context.Context, db *sql.DB, now time.Time) (MiniGraph, error) {
	w, err := loadGraphWorld(ctx, db)
	if err != nil {
		return MiniGraph{}, err
	}
	focals := pickFocals(w)
	if len(focals) == 0 {
		return MiniGraph{}, nil
	}

	var g MiniGraph
	rendered := map[int64]bool{}
	for _, focal := range focals {
		buildLane(&g, w, focal, rendered)
	}
	addBlockerEdges(&g, w, rendered)
	return g, nil
}

// ------------------------------------------------------------------
// In-memory world
// ------------------------------------------------------------------

type graphTask struct {
	id        int64
	shortID   string
	title     string
	status    string
	actor     string
	parentID  *int64
	sortOrder int
	parent    *graphTask
	children  []*graphTask
	// openBlockers counts upstream blocker tasks that are not yet
	// done or canceled. When > 0 the task renders as blocked even if
	// its own status is "available".
	openBlockers int
	// blockerIDs is the full list of upstream blocker task IDs
	// (regardless of resolution status) — the graph only emits an
	// edge for pairs where both endpoints are rendered.
	blockerIDs []int64
}

type graphWorld struct {
	byID  map[int64]*graphTask
	roots []*graphTask
}

func loadGraphWorld(ctx context.Context, db *sql.DB) (*graphWorld, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, short_id, title, status,
		       COALESCE(claimed_by, ''),
		       parent_id, sort_order
		FROM tasks
		WHERE deleted_at IS NULL
	`)
	if err != nil {
		return nil, fmt.Errorf("query tasks: %w", err)
	}
	defer rows.Close()

	w := &graphWorld{byID: make(map[int64]*graphTask)}
	for rows.Next() {
		t := &graphTask{}
		if err := rows.Scan(&t.id, &t.shortID, &t.title, &t.status,
			&t.actor, &t.parentID, &t.sortOrder); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		w.byID[t.id] = t
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, t := range w.byID {
		if t.parentID == nil {
			w.roots = append(w.roots, t)
			continue
		}
		if p, ok := w.byID[*t.parentID]; ok {
			t.parent = p
			p.children = append(p.children, t)
		}
	}
	sortBySortOrder(w.roots)
	for _, t := range w.byID {
		sortBySortOrder(t.children)
	}

	// Blocker edges. A blocker is "open" when its own status is not
	// done/canceled; that flag drives the blocked visual state.
	blockRows, err := db.QueryContext(ctx, `
		SELECT b.blocker_id, b.blocked_id, t.status
		FROM blocks b
		JOIN tasks t ON t.id = b.blocker_id
		WHERE t.deleted_at IS NULL
	`)
	if err != nil {
		return nil, fmt.Errorf("query blocks: %w", err)
	}
	defer blockRows.Close()
	for blockRows.Next() {
		var blockerID, blockedID int64
		var blockerStatus string
		if err := blockRows.Scan(&blockerID, &blockedID, &blockerStatus); err != nil {
			return nil, fmt.Errorf("scan block: %w", err)
		}
		blocked := w.byID[blockedID]
		if blocked == nil {
			continue
		}
		blocked.blockerIDs = append(blocked.blockerIDs, blockerID)
		if blockerStatus != "done" && blockerStatus != "canceled" {
			blocked.openBlockers++
		}
	}
	if err := blockRows.Err(); err != nil {
		return nil, err
	}
	return w, nil
}

func sortBySortOrder(ts []*graphTask) {
	sort.SliceStable(ts, func(i, j int) bool {
		if ts[i].sortOrder != ts[j].sortOrder {
			return ts[i].sortOrder < ts[j].sortOrder
		}
		return ts[i].id < ts[j].id
	})
}

// ------------------------------------------------------------------
// Focal selection
// ------------------------------------------------------------------

// pickFocals returns the tasks each lane should be centered on. When
// any tasks are claimed, every claim becomes its own lane; otherwise
// the lane is anchored on the globally-next available leaf.
func pickFocals(w *graphWorld) []*graphTask {
	var active []*graphTask
	for _, t := range w.byID {
		if t.status == "claimed" {
			active = append(active, t)
		}
	}
	if len(active) > 0 {
		// Stable order: preorder position in the project tree so
		// visually-close lanes sit next to each other.
		preorder := preorderAll(w)
		pos := make(map[int64]int, len(preorder))
		for i, t := range preorder {
			pos[t.id] = i
		}
		sort.SliceStable(active, func(i, j int) bool {
			return pos[active[i].id] < pos[active[j].id]
		})
		return active
	}
	if next := globalNext(w); next != nil {
		return []*graphTask{next}
	}
	return nil
}

// preorderAll returns every task in DFS-preorder, rooted at the
// project roots sorted by declaration order. Used for stable lane
// ordering.
func preorderAll(w *graphWorld) []*graphTask {
	var out []*graphTask
	var visit func(t *graphTask)
	visit = func(t *graphTask) {
		out = append(out, t)
		for _, c := range t.children {
			visit(c)
		}
	}
	for _, r := range w.roots {
		visit(r)
	}
	return out
}

// globalNext mirrors the `Next:` computation in job status: the
// first preorder available leaf with no open blockers.
func globalNext(w *graphWorld) *graphTask {
	for _, t := range preorderAll(w) {
		if t.status != "available" {
			continue
		}
		if len(t.children) > 0 {
			continue
		}
		if t.openBlockers > 0 {
			continue
		}
		return t
	}
	return nil
}

// ------------------------------------------------------------------
// Lane construction
// ------------------------------------------------------------------

// buildLane walks the spine for a single focal, populates a new
// Lane entry on g, and emits the lane's nodes + edges. rendered
// tracks task IDs already represented in g.Nodes so subsequent
// lanes converge on a single node record for shared tasks.
func buildLane(g *MiniGraph, w *graphWorld, focal *graphTask, rendered map[int64]bool) {
	lane := Lane{FocalShortID: focal.shortID}
	lane.Spine[SpineFocal] = focal.shortID
	addNode(g, focal, rendered)

	// Walk left, filling slots SpineL1, SpineL2.
	cur := focal
	for i := 1; i <= graphHopsLeft; i++ {
		prev := leftOf(w, cur)
		if prev == nil {
			break
		}
		lane.Spine[SpineFocal-i] = prev.shortID
		addNode(g, prev, rendered)
		addFlowEdge(g, prev, cur)
		cur = prev
	}

	// Walk right, filling slots SpineR1, SpineR2.
	cur = focal
	for i := 1; i <= graphHopsRight; i++ {
		next := rightOf(w, cur)
		if next == nil {
			break
		}
		lane.Spine[SpineFocal+i] = next.shortID
		addNode(g, next, rendered)
		addFlowEdge(g, cur, next)
		cur = next
	}

	// Vertical sibling stacking: parallel work that falls outside
	// the 2-hop horizontal spine budget. Open siblings of the focal
	// not already on any lane get listed in Stacked for the layout
	// task to render below (or near) the focal.
	lane.Stacked = collectStackedSiblings(g, w, focal, rendered)

	g.Lanes = append(g.Lanes, lane)
}

// collectStackedSiblings returns the short IDs of the focal's open
// siblings that are neither the focal itself nor already rendered
// elsewhere. Order follows sort_order so the vertical arrangement
// mirrors the underlying task list. Nodes are also added to
// g.Nodes as a side effect so the layout task sees their metadata.
func collectStackedSiblings(g *MiniGraph, w *graphWorld, focal *graphTask, rendered map[int64]bool) []string {
	var out []string
	for _, s := range siblingList(w, focal) {
		if s.id == focal.id {
			continue
		}
		if s.status == "done" || s.status == "canceled" {
			continue
		}
		if rendered[s.id] {
			continue
		}
		addNode(g, s, rendered)
		out = append(out, s.shortID)
	}
	return out
}

// leftOf returns the node that immediately precedes t in the LTR
// subway model: the preceding sibling when one exists, otherwise
// t's parent (the parent standing in for any cousin predecessor).
// Returns nil at the very beginning of the tree.
func leftOf(w *graphWorld, t *graphTask) *graphTask {
	siblings := siblingList(w, t)
	for i, s := range siblings {
		if s.id == t.id {
			if i > 0 {
				return siblings[i-1]
			}
			break
		}
	}
	return t.parent // nil when t is a root with no preceding sibling
}

// rightOf returns the node that immediately follows t in the LTR
// subway model: the following sibling when one exists, otherwise
// the following sibling of the nearest ancestor that has one
// (parent nodes themselves are never drilled into on the right).
func rightOf(w *graphWorld, t *graphTask) *graphTask {
	for cur := t; cur != nil; cur = cur.parent {
		siblings := siblingList(w, cur)
		for i, s := range siblings {
			if s.id == cur.id && i+1 < len(siblings) {
				return siblings[i+1]
			}
		}
	}
	return nil
}

// siblingList returns the ordered sibling list t belongs to (either
// its parent's children or the root list).
func siblingList(w *graphWorld, t *graphTask) []*graphTask {
	if t.parent != nil {
		return t.parent.children
	}
	return w.roots
}

// ------------------------------------------------------------------
// Node / edge emission
// ------------------------------------------------------------------

func addNode(g *MiniGraph, t *graphTask, rendered map[int64]bool) {
	if rendered[t.id] {
		return
	}
	rendered[t.id] = true
	g.Nodes = append(g.Nodes, GraphNode{
		ShortID:          t.shortID,
		Title:            t.title,
		State:            nodeState(t),
		Actor:            t.actor,
		IsParent:         len(t.children) > 0,
		ChildrenNotShown: countChildrenNotShown(t, rendered),
		URL:              "/tasks/" + t.shortID,
	})
	// A node slotted after a parent may have flipped the parent's
	// "not shown" count; refresh any already-rendered parent.
	if t.parent != nil {
		for i := range g.Nodes {
			if g.Nodes[i].ShortID == t.parent.shortID {
				g.Nodes[i].ChildrenNotShown = countChildrenNotShown(t.parent, rendered)
			}
		}
	}
}

// countChildrenNotShown returns the number of direct children of t
// whose node is not currently present in the rendered set.
func countChildrenNotShown(t *graphTask, rendered map[int64]bool) int {
	if len(t.children) == 0 {
		return 0
	}
	missing := 0
	for _, c := range t.children {
		if !rendered[c.id] {
			missing++
		}
	}
	return missing
}

func nodeState(t *graphTask) GraphNodeState {
	switch t.status {
	case "done", "canceled":
		return GraphNodeDone
	case "claimed":
		return GraphNodeActive
	}
	if t.openBlockers > 0 {
		return GraphNodeBlocked
	}
	return GraphNodeTodo
}

func addFlowEdge(g *MiniGraph, from, to *graphTask) {
	g.Edges = append(g.Edges, GraphEdge{
		FromShortID: from.shortID,
		ToShortID:   to.shortID,
		Kind:        flowKindBetween(from, to),
	})
}

// flowKindBetween classifies a spine edge as Flow (same parent, or
// parent/child) or Cousin (crosses a parent boundary).
func flowKindBetween(from, to *graphTask) GraphEdgeKind {
	// Parent/child in either direction.
	if from.parent != nil && from.parent.id == to.id {
		return GraphEdgeFlow
	}
	if to.parent != nil && to.parent.id == from.id {
		return GraphEdgeFlow
	}
	// Siblings (including root-root).
	if samParent(from, to) {
		return GraphEdgeFlow
	}
	return GraphEdgeCousin
}

func samParent(a, b *graphTask) bool {
	if a.parent == nil && b.parent == nil {
		return true
	}
	if a.parent == nil || b.parent == nil {
		return false
	}
	return a.parent.id == b.parent.id
}

// addBlockerEdges scans the rendered set and emits a dashed amber
// edge for every (blocker, blocked) pair where both endpoints are
// on the graph. Off-graph blockers are intentionally suppressed —
// the Home view's Blocked strip already covers them, and a dangling
// arrow would be noise.
func addBlockerEdges(g *MiniGraph, w *graphWorld, rendered map[int64]bool) {
	if len(rendered) == 0 {
		return
	}
	for blockedID := range rendered {
		blocked := w.byID[blockedID]
		if blocked == nil {
			continue
		}
		for _, blockerID := range blocked.blockerIDs {
			if !rendered[blockerID] {
				continue
			}
			blocker := w.byID[blockerID]
			if blocker == nil {
				continue
			}
			// Only open blockers earn a dashed amber arc — a done
			// blocker is historical and drawing it would misread
			// as a live constraint.
			if blocker.status == "done" || blocker.status == "canceled" {
				continue
			}
			g.Edges = append(g.Edges, GraphEdge{
				FromShortID: blocker.shortID,
				ToShortID:   blocked.shortID,
				Kind:        GraphEdgeBlocker,
			})
		}
	}
}
