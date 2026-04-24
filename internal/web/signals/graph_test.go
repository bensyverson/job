package signals

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	job "github.com/bensyverson/jobs/internal/job"
)

// seedTreeTask extends the base seedTask with parent/ordering fields
// so graph tests can build explicit preorder trees. Returns the
// inserted task's rowid for use as a parent_id in later calls.
func seedTreeTask(t *testing.T, db *sql.DB, shortID, title, status string, parentID *int64, sortOrder int, createdAt time.Time) int64 {
	t.Helper()
	res, err := db.Exec(`
		INSERT INTO tasks (short_id, title, description, status, parent_id, sort_order, created_at, updated_at)
		VALUES (?, ?, '', ?, ?, ?, ?, ?)
	`, shortID, title, status, parentID, sortOrder, createdAt.Unix(), createdAt.Unix())
	if err != nil {
		t.Fatalf("seed tree task %q: %v", shortID, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}
	return id
}

// findNode looks up a node in a MiniGraph by ShortID.
func findNode(mg MiniGraph, shortID string) (GraphNode, bool) {
	for _, n := range mg.Nodes {
		if n.ShortID == shortID {
			return n, true
		}
	}
	return GraphNode{}, false
}

// hasEdge asserts the presence of an edge of the given kind between
// the two nodes; direction is significant (Flow and Blocker both
// point from predecessor/blocker to successor/blocked).
func hasEdge(mg MiniGraph, from, to string, kind GraphEdgeKind) bool {
	for _, e := range mg.Edges {
		if e.FromShortID == from && e.ToShortID == to && e.Kind == kind {
			return true
		}
	}
	return false
}

// assertSpine fails if lane.Spine doesn't match the expected
// 5-slot array. Empty string means an absent hop.
func assertSpine(t *testing.T, lane Lane, want [5]string) {
	t.Helper()
	if lane.Spine != want {
		t.Errorf("lane spine: got %v, want %v", lane.Spine, want)
	}
}

func TestMiniGraph_EmptyDatabase(t *testing.T) {
	db := job.SetupTestDB(t)
	now := time.Unix(1_700_000_000, 0)

	mg, err := ComputeMiniGraph(context.Background(), db, now)
	if err != nil {
		t.Fatalf("ComputeMiniGraph: %v", err)
	}
	if len(mg.Lanes) != 0 {
		t.Errorf("Lanes: got %d, want 0", len(mg.Lanes))
	}
	if len(mg.Nodes) != 0 {
		t.Errorf("Nodes: got %d, want 0", len(mg.Nodes))
	}
	if len(mg.Edges) != 0 {
		t.Errorf("Edges: got %d, want 0", len(mg.Edges))
	}
}

// Scenario A — focal is a mid-sibling active task. Lane walks two
// hops left (preceding sibling, then cousin-substituted parent) and
// two hops right (succeeding sibling, then cousin-substituted next
// top-level parent).
//
// Tree:
//
//	Phase 2 (done, root)
//	Phase 3 (available, root)
//	  Step 1 (done)
//	  Step 2 (claimed by alice)      <-- focal
//	  Step 3 (available, blocked by Step 2)
//	Phase 4 (available, root) with 6 todo children
//
// Expected spine: Phase3 -> Step1 -> [Step2] -> Step3 -> Phase4(+6)
func TestMiniGraph_ScenarioA_MidSiblingFocal(t *testing.T) {
	db := job.SetupTestDB(t)
	now := time.Unix(1_700_000_000, 0)
	created := now.Add(-1 * time.Hour)

	seedTreeTask(t, db, "Phase2", "Phase 2", "done", nil, 1, created)
	p3 := seedTreeTask(t, db, "Phase3", "Phase 3", "available", nil, 2, created)
	seedTreeTask(t, db, "Step1_", "Step 1", "done", &p3, 1, created)
	s2 := seedTreeTask(t, db, "Step2_", "Step 2", "available", &p3, 2, created)
	s3 := seedTreeTask(t, db, "Step3_", "Step 3", "available", &p3, 3, created)
	p4 := seedTreeTask(t, db, "Phase4", "Phase 4", "available", nil, 3, created)
	for i := range 6 {
		short := fmt.Sprintf("p4c%02d", i)
		seedTreeTask(t, db, short, fmt.Sprintf("Phase4 child %d", i), "available", &p4, i+1, created)
	}

	seedClaim(t, db, s2, "alice", now.Add(-10*time.Minute))
	seedBlock(t, db, s3, s2, now.Add(-20*time.Minute))

	mg, err := ComputeMiniGraph(context.Background(), db, now)
	if err != nil {
		t.Fatalf("ComputeMiniGraph: %v", err)
	}

	if len(mg.Lanes) != 1 {
		t.Fatalf("Lanes: got %d, want 1", len(mg.Lanes))
	}
	assertSpine(t, mg.Lanes[0], [5]string{"Phase3", "Step1_", "Step2_", "Step3_", "Phase4"})
	if mg.Lanes[0].FocalShortID != "Step2_" {
		t.Errorf("FocalShortID: got %q, want Step2_", mg.Lanes[0].FocalShortID)
	}
	if len(mg.Lanes[0].Stacked) != 0 {
		t.Errorf("Stacked: got %v, want empty", mg.Lanes[0].Stacked)
	}

	expectNode := map[string]struct {
		state            GraphNodeState
		actor            string
		isParent         bool
		childrenNotShown int
	}{
		"Phase3": {isParent: true, childrenNotShown: 0},
		"Step1_": {state: GraphNodeDone},
		"Step2_": {state: GraphNodeActive, actor: "alice"},
		"Step3_": {state: GraphNodeBlocked},
		"Phase4": {isParent: true, childrenNotShown: 6},
	}
	for id, w := range expectNode {
		n, ok := findNode(mg, id)
		if !ok {
			t.Errorf("missing node %q", id)
			continue
		}
		if n.State != w.state {
			t.Errorf("%s state: got %v, want %v", id, n.State, w.state)
		}
		if n.Actor != w.actor {
			t.Errorf("%s actor: got %q, want %q", id, n.Actor, w.actor)
		}
		if n.IsParent != w.isParent {
			t.Errorf("%s isParent: got %v, want %v", id, n.IsParent, w.isParent)
		}
		if n.ChildrenNotShown != w.childrenNotShown {
			t.Errorf("%s +N: got %d, want %d", id, n.ChildrenNotShown, w.childrenNotShown)
		}
	}

	for _, e := range []struct {
		from, to string
		kind     GraphEdgeKind
	}{
		{"Phase3", "Step1_", GraphEdgeFlow},
		{"Step1_", "Step2_", GraphEdgeFlow},
		{"Step2_", "Step3_", GraphEdgeFlow},
		{"Step3_", "Phase4", GraphEdgeCousin},
		{"Step2_", "Step3_", GraphEdgeBlocker},
	} {
		if !hasEdge(mg, e.from, e.to, e.kind) {
			t.Errorf("missing edge %s -> %s (kind %v)", e.from, e.to, e.kind)
		}
	}
}

// Scenario B — firstborn focal. L1 is the parent itself; L2 is the
// parent's preceding sibling.
func TestMiniGraph_ScenarioB_FirstbornFocal(t *testing.T) {
	db := job.SetupTestDB(t)
	now := time.Unix(1_700_000_000, 0)
	created := now.Add(-1 * time.Hour)

	seedTreeTask(t, db, "Phase2", "Phase 2", "done", nil, 1, created)
	p3 := seedTreeTask(t, db, "Phase3", "Phase 3", "available", nil, 2, created)
	s1 := seedTreeTask(t, db, "Step1_", "Step 1", "available", &p3, 1, created)
	seedTreeTask(t, db, "Step2_", "Step 2", "available", &p3, 2, created)
	seedTreeTask(t, db, "Step3_", "Step 3", "available", &p3, 3, created)
	seedClaim(t, db, s1, "alice", now.Add(-10*time.Minute))

	mg, err := ComputeMiniGraph(context.Background(), db, now)
	if err != nil {
		t.Fatalf("ComputeMiniGraph: %v", err)
	}
	if len(mg.Lanes) != 1 {
		t.Fatalf("Lanes: got %d, want 1", len(mg.Lanes))
	}
	assertSpine(t, mg.Lanes[0], [5]string{"Phase2", "Phase3", "Step1_", "Step2_", "Step3_"})

	for _, e := range []struct {
		from, to string
		kind     GraphEdgeKind
	}{
		{"Phase2", "Phase3", GraphEdgeFlow},
		{"Phase3", "Step1_", GraphEdgeFlow},
		{"Step1_", "Step2_", GraphEdgeFlow},
		{"Step2_", "Step3_", GraphEdgeFlow},
	} {
		if !hasEdge(mg, e.from, e.to, e.kind) {
			t.Errorf("missing edge %s -> %s (kind %v)", e.from, e.to, e.kind)
		}
	}
}

// Scenario C — lastborn focal. Right side exits the subtree via
// cousin substitution to the next phase; the phase after that is
// a normal sibling edge at root level.
func TestMiniGraph_ScenarioC_LastbornFocal(t *testing.T) {
	db := job.SetupTestDB(t)
	now := time.Unix(1_700_000_000, 0)
	created := now.Add(-1 * time.Hour)

	p3 := seedTreeTask(t, db, "Phase3", "Phase 3", "available", nil, 1, created)
	seedTreeTask(t, db, "Step1_", "Step 1", "done", &p3, 1, created)
	seedTreeTask(t, db, "Step2_", "Step 2", "done", &p3, 2, created)
	s3 := seedTreeTask(t, db, "Step3_", "Step 3", "available", &p3, 3, created)
	p4 := seedTreeTask(t, db, "Phase4", "Phase 4", "available", nil, 2, created)
	for i := range 6 {
		seedTreeTask(t, db, fmt.Sprintf("p4c%02d", i), "", "available", &p4, i+1, created)
	}
	p5 := seedTreeTask(t, db, "Phase5", "Phase 5", "available", nil, 3, created)
	for i := range 10 {
		seedTreeTask(t, db, fmt.Sprintf("p5c%02d", i), "", "available", &p5, i+1, created)
	}
	seedClaim(t, db, s3, "alice", now.Add(-10*time.Minute))

	mg, err := ComputeMiniGraph(context.Background(), db, now)
	if err != nil {
		t.Fatalf("ComputeMiniGraph: %v", err)
	}
	if len(mg.Lanes) != 1 {
		t.Fatalf("Lanes: got %d, want 1", len(mg.Lanes))
	}
	assertSpine(t, mg.Lanes[0], [5]string{"Step1_", "Step2_", "Step3_", "Phase4", "Phase5"})

	if n, _ := findNode(mg, "Phase4"); n.ChildrenNotShown != 6 {
		t.Errorf("Phase4 +N: got %d, want 6", n.ChildrenNotShown)
	}
	if n, _ := findNode(mg, "Phase5"); n.ChildrenNotShown != 10 {
		t.Errorf("Phase5 +N: got %d, want 10", n.ChildrenNotShown)
	}
	for _, e := range []struct {
		from, to string
		kind     GraphEdgeKind
	}{
		{"Step1_", "Step2_", GraphEdgeFlow},
		{"Step2_", "Step3_", GraphEdgeFlow},
		{"Step3_", "Phase4", GraphEdgeCousin},
		{"Phase4", "Phase5", GraphEdgeFlow},
	} {
		if !hasEdge(mg, e.from, e.to, e.kind) {
			t.Errorf("missing edge %s -> %s (kind %v)", e.from, e.to, e.kind)
		}
	}
}

// Scenario E — focal is a top-level phase. All hops are root-level
// siblings; every edge is Flow.
func TestMiniGraph_ScenarioE_TopLevelFocal(t *testing.T) {
	db := job.SetupTestDB(t)
	now := time.Unix(1_700_000_000, 0)
	created := now.Add(-1 * time.Hour)

	seedTreeTask(t, db, "Phase1", "Phase 1", "done", nil, 1, created)
	seedTreeTask(t, db, "Phase2", "Phase 2", "done", nil, 2, created)
	p3 := seedTreeTask(t, db, "Phase3", "Phase 3", "available", nil, 3, created)
	seedTreeTask(t, db, "Phase4", "Phase 4", "available", nil, 4, created)
	seedTreeTask(t, db, "Phase5", "Phase 5", "available", nil, 5, created)
	seedClaim(t, db, p3, "alice", now.Add(-5*time.Minute))

	mg, err := ComputeMiniGraph(context.Background(), db, now)
	if err != nil {
		t.Fatalf("ComputeMiniGraph: %v", err)
	}
	if len(mg.Lanes) != 1 {
		t.Fatalf("Lanes: got %d, want 1", len(mg.Lanes))
	}
	assertSpine(t, mg.Lanes[0], [5]string{"Phase1", "Phase2", "Phase3", "Phase4", "Phase5"})
	for _, pair := range []struct{ from, to string }{
		{"Phase1", "Phase2"}, {"Phase2", "Phase3"},
		{"Phase3", "Phase4"}, {"Phase4", "Phase5"},
	} {
		if !hasEdge(mg, pair.from, pair.to, GraphEdgeFlow) {
			t.Errorf("missing Flow edge %s -> %s", pair.from, pair.to)
		}
	}
}

// No-active fallback — with nothing claimed, focal is the globally-
// next available unblocked leaf.
func TestMiniGraph_NoActiveFallsBackToNext(t *testing.T) {
	db := job.SetupTestDB(t)
	now := time.Unix(1_700_000_000, 0)
	created := now.Add(-1 * time.Hour)

	seedTreeTask(t, db, "Phase2", "Phase 2", "done", nil, 1, created)
	p3 := seedTreeTask(t, db, "Phase3", "Phase 3", "available", nil, 2, created)
	seedTreeTask(t, db, "Step1_", "Step 1", "done", &p3, 1, created)
	seedTreeTask(t, db, "Step2_", "Step 2", "available", &p3, 2, created)
	s3 := seedTreeTask(t, db, "Step3_", "Step 3", "available", &p3, 3, created)
	seedTreeTask(t, db, "Phase4", "Phase 4", "available", nil, 3, created)
	// Step 3 blocked by Step 2; globally-next = Step 2.
	var s2ID int64
	if err := db.QueryRow(`SELECT id FROM tasks WHERE short_id='Step2_'`).Scan(&s2ID); err != nil {
		t.Fatalf("lookup Step2_: %v", err)
	}
	seedBlock(t, db, s3, s2ID, now.Add(-20*time.Minute))

	mg, err := ComputeMiniGraph(context.Background(), db, now)
	if err != nil {
		t.Fatalf("ComputeMiniGraph: %v", err)
	}
	if len(mg.Lanes) != 1 {
		t.Fatalf("Lanes: got %d, want 1", len(mg.Lanes))
	}
	if mg.Lanes[0].FocalShortID != "Step2_" {
		t.Errorf("FocalShortID: got %q, want Step2_", mg.Lanes[0].FocalShortID)
	}
	if n, _ := findNode(mg, "Step2_"); n.State != GraphNodeTodo {
		t.Errorf("focal state: got %v, want Todo", n.State)
	}
}

// Multi-lane with shared blocker — two claims in disjoint subtrees
// are both blocked by the same open task. That task appears exactly
// once in the node pool; both focals carry a dashed amber edge
// back to it. Lanes are distinct entries in mg.Lanes.
func TestMiniGraph_MultiLane_SharedBlockerDeduped(t *testing.T) {
	db := job.SetupTestDB(t)
	now := time.Unix(1_700_000_000, 0)
	created := now.Add(-1 * time.Hour)

	blk := seedTreeTask(t, db, "Blkr__", "Shared blocker", "available", nil, 1, created)
	pa := seedTreeTask(t, db, "PhaseA", "Phase A", "available", nil, 2, created)
	sa1 := seedTreeTask(t, db, "StepA1", "Step A1", "available", &pa, 1, created)
	pb := seedTreeTask(t, db, "PhaseB", "Phase B", "available", nil, 3, created)
	sb1 := seedTreeTask(t, db, "StepB1", "Step B1", "available", &pb, 1, created)

	seedClaim(t, db, blk, "carol", now.Add(-20*time.Minute))
	seedClaim(t, db, sa1, "alice", now.Add(-10*time.Minute))
	seedClaim(t, db, sb1, "bob", now.Add(-5*time.Minute))
	seedBlock(t, db, sa1, blk, now.Add(-15*time.Minute))
	seedBlock(t, db, sb1, blk, now.Add(-15*time.Minute))

	mg, err := ComputeMiniGraph(context.Background(), db, now)
	if err != nil {
		t.Fatalf("ComputeMiniGraph: %v", err)
	}

	// Three active claims → three lanes (including carol on the
	// shared blocker itself).
	if len(mg.Lanes) != 3 {
		t.Fatalf("Lanes: got %d, want 3", len(mg.Lanes))
	}
	focals := map[string]bool{}
	for _, l := range mg.Lanes {
		focals[l.FocalShortID] = true
	}
	for _, want := range []string{"StepA1", "StepB1", "Blkr__"} {
		if !focals[want] {
			t.Errorf("missing focal lane for %q", want)
		}
	}

	// Dedup: shared blocker appears exactly once in the node pool.
	blkrCount := 0
	for _, n := range mg.Nodes {
		if n.ShortID == "Blkr__" {
			blkrCount++
		}
	}
	if blkrCount != 1 {
		t.Errorf("shared blocker rendered %d times, want 1", blkrCount)
	}

	// Both focals carry a dashed amber edge back to the blocker.
	if !hasEdge(mg, "Blkr__", "StepA1", GraphEdgeBlocker) {
		t.Errorf("missing blocker edge Blkr__ -> StepA1")
	}
	if !hasEdge(mg, "Blkr__", "StepB1", GraphEdgeBlocker) {
		t.Errorf("missing blocker edge Blkr__ -> StepB1")
	}
}

// Vertical sibling stacking — open siblings of the focal that fall
// outside the 2-hop horizontal spine budget are reported in
// Lane.Stacked. Siblings already on the spine don't appear there.
func TestMiniGraph_VerticalSiblingStacking(t *testing.T) {
	db := job.SetupTestDB(t)
	now := time.Unix(1_700_000_000, 0)
	created := now.Add(-1 * time.Hour)

	p := seedTreeTask(t, db, "Phase_", "Phase", "available", nil, 1, created)
	seedTreeTask(t, db, "S1____", "S1", "done", &p, 1, created)
	seedTreeTask(t, db, "S2____", "S2", "done", &p, 2, created)
	s3 := seedTreeTask(t, db, "S3____", "S3", "available", &p, 3, created)
	seedTreeTask(t, db, "S4____", "S4", "available", &p, 4, created)
	seedTreeTask(t, db, "S5____", "S5", "available", &p, 5, created)
	seedTreeTask(t, db, "S6____", "S6", "available", &p, 6, created)
	seedClaim(t, db, s3, "alice", now.Add(-5*time.Minute))

	mg, err := ComputeMiniGraph(context.Background(), db, now)
	if err != nil {
		t.Fatalf("ComputeMiniGraph: %v", err)
	}
	if len(mg.Lanes) != 1 {
		t.Fatalf("Lanes: got %d, want 1", len(mg.Lanes))
	}
	assertSpine(t, mg.Lanes[0], [5]string{"S1____", "S2____", "S3____", "S4____", "S5____"})
	if got, want := mg.Lanes[0].Stacked, []string{"S6____"}; !equalStrings(got, want) {
		t.Errorf("Stacked: got %v, want %v", got, want)
	}
	// S6 exists in the node pool with its state metadata, so the
	// layout task can render it without another lookup.
	if n, ok := findNode(mg, "S6____"); !ok {
		t.Error("S6 missing from node pool")
	} else if n.State != GraphNodeTodo {
		t.Errorf("S6 state: got %v, want Todo", n.State)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Done blockers do not earn a dashed amber edge — historical, not
// a live constraint.
func TestMiniGraph_DoneBlockerSuppressesBlockerEdge(t *testing.T) {
	db := job.SetupTestDB(t)
	now := time.Unix(1_700_000_000, 0)
	created := now.Add(-1 * time.Hour)

	done := seedTreeTask(t, db, "PastBl", "Past blocker", "done", nil, 1, created)
	p := seedTreeTask(t, db, "Phase_", "Phase", "available", nil, 2, created)
	focal := seedTreeTask(t, db, "Focal_", "Focal", "available", &p, 1, created)
	seedClaim(t, db, focal, "alice", now.Add(-5*time.Minute))
	seedBlock(t, db, focal, done, now.Add(-30*time.Minute))

	mg, err := ComputeMiniGraph(context.Background(), db, now)
	if err != nil {
		t.Fatalf("ComputeMiniGraph: %v", err)
	}
	for _, e := range mg.Edges {
		if e.Kind == GraphEdgeBlocker && e.FromShortID == "PastBl" {
			t.Errorf("unexpected blocker edge from done task: %+v", e)
		}
	}
}
