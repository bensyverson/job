package signals

import (
	"sort"
	"testing"
)

// ------------------------------------------------------------------
// Multi-focal tree-map: focal-path subgraph + fork detection
//
// Reference: project/2026-04-27-graph-row-merging.md.
//
// The focal-path subgraph is the set of nodes on the path from the
// LCA of all focals down to each focal. Fork points are subgraph
// nodes with ≥2 in-subgraph children — these are the divergence
// points where rows split in the multi-focal tree-map render mode.
// ------------------------------------------------------------------

func subgraphSorted(ids map[int64]bool, w *graphWorld) []string {
	out := make([]string, 0, len(ids))
	for id := range ids {
		if t, ok := w.byID[id]; ok {
			out = append(out, t.shortID)
		}
	}
	sort.Strings(out)
	return out
}

func equalShorts(a, b []string) bool {
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

// LCA at the project root: focals at different depths under the
// same root. The LCA is the lowest node that is an ancestor of all
// focals, not necessarily the root.
func TestFocalPathSubgraph_LCA_AtDifferentDepths(t *testing.T) {
	// Tree:
	//   Root
	//   ├── Solo
	//   │   ├── B
	//   │   │   └── C [focal]   (depth 3)
	//   │   └── D [focal]       (depth 2)
	//   └── E (no focal in this branch)
	w := newTestWorld([]tt{
		{short: "Root", parent: "", status: "available"},
		{short: "Solo", parent: "Root", status: "available"},
		{short: "B", parent: "Solo", status: "available"},
		{short: "C", parent: "B", status: "claimed"},
		{short: "D", parent: "Solo", status: "claimed"},
		{short: "E", parent: "Root", status: "available"},
	})
	focals := []*graphTask{mustTask(w, "C"), mustTask(w, "D")}

	lca, ids := focalPathSubgraph(focals)

	if lca == nil {
		t.Fatalf("LCA: got nil, want Solo")
	}
	if lca.shortID != "Solo" {
		t.Errorf("LCA: got %q, want %q", lca.shortID, "Solo")
	}
	wantIDs := []string{"B", "C", "D", "Solo"}
	if got := subgraphSorted(ids, w); !equalShorts(got, wantIDs) {
		t.Errorf("subgraph: got %v, want %v", got, wantIDs)
	}
}

// Fork-point identification: a subgraph node with two or more
// in-subgraph children is a fork point.
//
//	A
//	├── B
//	│   ├── C [focal]
//	│   └── D
//	│       └── E [focal]
//	└── F
//	    └── G [focal]
//
// Subgraph: {A, B, C, D, E, F, G}. Forks: A (children B, F both
// in subgraph) and B (children C, D both in subgraph). D has only
// one in-subgraph child (E) and is not a fork.
func TestSubgraphForkPoints_TwoOrMoreInSubgraphChildren(t *testing.T) {
	w := newTestWorld([]tt{
		{short: "A", parent: "", status: "available"},
		{short: "B", parent: "A", status: "available"},
		{short: "C", parent: "B", status: "claimed"},
		{short: "D", parent: "B", status: "available"},
		{short: "E", parent: "D", status: "claimed"},
		{short: "F", parent: "A", status: "available"},
		{short: "G", parent: "F", status: "claimed"},
	})
	focals := []*graphTask{
		mustTask(w, "C"),
		mustTask(w, "E"),
		mustTask(w, "G"),
	}

	lca, ids := focalPathSubgraph(focals)
	if lca == nil || lca.shortID != "A" {
		t.Fatalf("LCA: got %v, want A", lca)
	}

	forks := subgraphForkPoints(ids, lca)

	gotForks := make([]string, len(forks))
	for i, f := range forks {
		gotForks[i] = f.shortID
	}
	wantForks := []string{"A", "B"}
	if !equalShorts(gotForks, wantForks) {
		t.Errorf("forks: got %v, want %v", gotForks, wantForks)
	}
}

// Non-focal-bearing branches are excluded from the subgraph: a
// sibling subtree without any focal does not contribute nodes.
//
//	A
//	├── B
//	│   └── C [focal]
//	└── D
//	    └── E (no focal under D)
//
// Subgraph: {A, B, C}. D and E are out.
func TestFocalPathSubgraph_NonFocalBearingBranchesExcluded(t *testing.T) {
	w := newTestWorld([]tt{
		{short: "A", parent: "", status: "available"},
		{short: "B", parent: "A", status: "available"},
		{short: "C", parent: "B", status: "claimed"},
		{short: "D", parent: "A", status: "available"},
		{short: "E", parent: "D", status: "available"},
	})
	focals := []*graphTask{mustTask(w, "C")}

	lca, ids := focalPathSubgraph(focals)

	if lca == nil || lca.shortID != "C" {
		t.Errorf("LCA for single focal C: got %v, want C", lca)
	}
	got := subgraphSorted(ids, w)
	want := []string{"C"}
	if !equalShorts(got, want) {
		t.Errorf("subgraph for single focal: got %v, want %v", got, want)
	}

	// Two focals on the same branch — the subgraph still excludes
	// the non-focal sibling subtree (D, E).
	w2 := newTestWorld([]tt{
		{short: "A", parent: "", status: "available"},
		{short: "B", parent: "A", status: "available"},
		{short: "C", parent: "B", status: "claimed"},
		{short: "F", parent: "A", status: "claimed"},
		{short: "D", parent: "A", status: "available"},
		{short: "E", parent: "D", status: "available"},
	})
	focals2 := []*graphTask{mustTask(w2, "C"), mustTask(w2, "F")}

	lca2, ids2 := focalPathSubgraph(focals2)
	if lca2 == nil || lca2.shortID != "A" {
		t.Errorf("LCA for {C, F}: got %v, want A", lca2)
	}
	got2 := subgraphSorted(ids2, w2)
	want2 := []string{"A", "B", "C", "F"}
	if !equalShorts(got2, want2) {
		t.Errorf("subgraph for {C, F}: got %v, want %v (D/E excluded)", got2, want2)
	}
}

// A linear chain through the subgraph splits into independent rows
// at every fork point. Verified through the high-level rows
// builder: the number of rows reflects the number of branches at
// each fork.
//
//	Root (LCA)
//	├── B
//	│   └── C [focal]
//	└── D
//	    └── E [focal]
//
// Subgraph: {Root, B, C, D, E}. Forks: Root. Two branches → at
// least one parent row plus one row per non-inline branch (the
// inline child rides on the parent's row).
func TestMultiFocalRows_LinearChainSplitsAtEveryFork(t *testing.T) {
	w := newTestWorld([]tt{
		{short: "Root", parent: "", status: "available"},
		{short: "B", parent: "Root", status: "available"},
		{short: "C", parent: "B", status: "claimed"},
		{short: "D", parent: "Root", status: "available"},
		{short: "E", parent: "D", status: "claimed"},
	})
	focals := []*graphTask{mustTask(w, "C"), mustTask(w, "E")}

	rows := buildMultiFocalRows(w, focals, 2)

	if len(rows) < 2 {
		t.Fatalf("rows: got %d, want ≥2 (one per branch off the Root fork)", len(rows))
	}

	// Every focal must appear in exactly one row's items as a stop.
	stopRow := map[string]int{}
	for i, r := range rows {
		for _, it := range r.Items {
			if it.Kind == LineItemStop {
				if prev, dup := stopRow[it.ShortID]; dup {
					t.Errorf("stop %q appears in rows %d and %d (invariant 2: each tree node renders at most once)",
						it.ShortID, prev, i)
				}
				stopRow[it.ShortID] = i
			}
		}
	}
	for _, focalSID := range []string{"C", "E"} {
		if _, ok := stopRow[focalSID]; !ok {
			t.Errorf("focal %q missing from any row", focalSID)
		}
	}
	// At least one row must carry a ParentShortID (the non-inline
	// branch's curve target). Multi-focal rows beyond the topmost
	// must identify their parent for branch-curve geometry.
	hasParent := false
	for _, r := range rows {
		if r.ParentShortID != "" {
			hasParent = true
			break
		}
	}
	if !hasParent {
		t.Errorf("expected at least one row with non-empty ParentShortID for branch-curve geometry")
	}
}
