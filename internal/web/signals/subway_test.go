package signals

import (
	"strings"
	"testing"
)

// ------------------------------------------------------------------
// In-memory test fixtures
// ------------------------------------------------------------------

// tt is a compact task descriptor for in-memory graphWorld
// construction. Slice order determines sort_order; parent references
// another tt by short ID (empty = root).
type tt struct {
	short  string
	parent string
	status string
}

// newTestWorld builds an in-memory graphWorld from descriptors.
// blocks is an optional list of (blocker_short, blocked_short) pairs.
// openBlockers is incremented when the blocker's status is not
// done/canceled, mirroring loadGraphWorld's bookkeeping.
func newTestWorld(tasks []tt, blocks ...[2]string) *graphWorld {
	w := &graphWorld{byID: map[int64]*graphTask{}}
	byShort := map[string]*graphTask{}
	var nextID int64 = 1

	for i, td := range tasks {
		t := &graphTask{
			id:        nextID,
			shortID:   td.short,
			status:    td.status,
			sortOrder: i + 1,
		}
		nextID++
		w.byID[t.id] = t
		byShort[td.short] = t
	}
	for _, td := range tasks {
		t := byShort[td.short]
		if td.parent == "" {
			w.roots = append(w.roots, t)
			continue
		}
		p := byShort[td.parent]
		t.parent = p
		pid := p.id
		t.parentID = &pid
		p.children = append(p.children, t)
	}
	for _, b := range blocks {
		blocker := byShort[b[0]]
		blocked := byShort[b[1]]
		blocked.blockerIDs = append(blocked.blockerIDs, blocker.id)
		if blocker.status != "done" && blocker.status != "canceled" {
			blocked.openBlockers++
		}
	}
	return w
}

// referenceTree returns the standard tree from the design doc, with
// statuses overlaid from the supplied map. Any task not in the map
// defaults to "available".
//
// Tree:
//
//	A
//	├── B
//	│   ├── C
//	│   ├── D
//	│   ├── E
//	│   └── F
//	├── G       (blocked by B in scenarios that need it)
//	│   ├── H
//	│   └── I
//	└── J
//	    ├── K
//	    └── L
func referenceTree(statuses map[string]string) []tt {
	base := []tt{
		{short: "A", parent: ""},
		{short: "B", parent: "A"},
		{short: "C", parent: "B"},
		{short: "D", parent: "B"},
		{short: "E", parent: "B"},
		{short: "F", parent: "B"},
		{short: "G", parent: "A"},
		{short: "H", parent: "G"},
		{short: "I", parent: "G"},
		{short: "J", parent: "A"},
		{short: "K", parent: "J"},
		{short: "L", parent: "J"},
	}
	for i := range base {
		if s, ok := statuses[base[i].short]; ok {
			base[i].status = s
		} else {
			base[i].status = "available"
		}
	}
	return base
}

func mustTask(w *graphWorld, short string) *graphTask {
	for _, t := range w.byID {
		if t.shortID == short {
			return t
		}
	}
	panic("no task: " + short)
}

func taskShortIDs(ts []*graphTask) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.shortID
	}
	return out
}

// expectedLine is a test-side description of a single lineSeed.
type expectedLine struct {
	parent  string
	anchors []string
}

func assertLines(t *testing.T, got []*lineSeed, want []expectedLine) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("lines: got %d, want %d\n  got:  %s\n  want: %s",
			len(got), len(want), summarizeSeeds(got), summarizeExpected(want))
	}
	for i := range got {
		if got[i].parent.shortID != want[i].parent {
			t.Errorf("line %d parent: got %q, want %q",
				i, got[i].parent.shortID, want[i].parent)
		}
		gotAnchors := taskShortIDs(got[i].anchors)
		if !equalSubwayStrings(gotAnchors, want[i].anchors) {
			t.Errorf("line %d (%s) anchors: got %v, want %v",
				i, want[i].parent, gotAnchors, want[i].anchors)
		}
	}
}

func summarizeSeeds(ls []*lineSeed) string {
	parts := make([]string, len(ls))
	for i, l := range ls {
		parts[i] = l.parent.shortID + "{" + strings.Join(taskShortIDs(l.anchors), ",") + "}"
	}
	return strings.Join(parts, " ")
}

func summarizeExpected(ls []expectedLine) string {
	parts := make([]string, len(ls))
	for i, l := range ls {
		parts[i] = l.parent + "{" + strings.Join(l.anchors, ",") + "}"
	}
	return strings.Join(parts, " ")
}

func equalSubwayStrings(a, b []string) bool {
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

// ------------------------------------------------------------------
// Scenario tests for collectLines
//
// Reference: project/2026-04-25-graph-clarification.md
// Lookahead L = 2 throughout (the spec's default).
// ------------------------------------------------------------------

// Scenario 1 — D claimed (C done). Lookahead from D reaches E and F,
// both children of B. One line, no fork.
func TestCollectLines_Scenario1_DClaimed(t *testing.T) {
	w := newTestWorld(referenceTree(map[string]string{
		"C": "done",
		"D": "claimed",
	}))
	focals := []*graphTask{mustTask(w, "D")}

	got := collectLines(w, focals, 2)

	assertLines(t, got, []expectedLine{
		{parent: "B", anchors: []string{"D", "E", "F"}},
	})
}

// Scenario 2 — D and E claimed (siblings on B's line). Lookahead
// from E reaches F (in B) then H (first leaf of G's subtree, after
// traversal moves up from B → next sibling G → H), so G's line
// appears as a peek-ahead.
func TestCollectLines_Scenario2_DAndEClaimed(t *testing.T) {
	w := newTestWorld(
		referenceTree(map[string]string{
			"C": "done",
			"D": "claimed",
			"E": "claimed",
		}),
		[2]string{"B", "G"},
	)
	focals := []*graphTask{mustTask(w, "D"), mustTask(w, "E")}

	got := collectLines(w, focals, 2)

	assertLines(t, got, []expectedLine{
		{parent: "B", anchors: []string{"D", "E", "F"}},
		{parent: "G", anchors: []string{"H"}},
	})
}

// Scenario 3 — D and F claimed (E done between). Lookahead from F
// reaches H and I (in G), so G's line appears.
func TestCollectLines_Scenario3_DAndFClaimed(t *testing.T) {
	w := newTestWorld(
		referenceTree(map[string]string{
			"C": "done",
			"D": "claimed",
			"E": "done",
			"F": "claimed",
		}),
		[2]string{"B", "G"},
	)
	focals := []*graphTask{mustTask(w, "D"), mustTask(w, "F")}

	got := collectLines(w, focals, 2)

	assertLines(t, got, []expectedLine{
		{parent: "B", anchors: []string{"D", "E", "F"}},
		{parent: "G", anchors: []string{"H", "I"}},
	})
}

// Scenario 4 — D claimed, H claimed (G unblocked). Lookahead from H
// reaches I (next sibling) then K (first leaf of J's subtree), so J
// also appears as a peek line. Three lines, fork at A.
func TestCollectLines_Scenario4_DAndHClaimed(t *testing.T) {
	w := newTestWorld(referenceTree(map[string]string{
		"C": "done",
		"D": "claimed",
		"H": "claimed",
	}))
	focals := []*graphTask{mustTask(w, "D"), mustTask(w, "H")}

	got := collectLines(w, focals, 2)

	assertLines(t, got, []expectedLine{
		{parent: "B", anchors: []string{"D", "E", "F"}},
		{parent: "G", anchors: []string{"H", "I"}},
		{parent: "J", anchors: []string{"K"}},
	})
}

// Scenario 5 — D claimed, K claimed. G has no claims and lookahead
// doesn't reach it (D's lookahead stops at F; K's stops at L). Two
// lines, no G.
func TestCollectLines_Scenario5_DAndKClaimed(t *testing.T) {
	w := newTestWorld(referenceTree(map[string]string{
		"C": "done",
		"D": "claimed",
		"K": "claimed",
	}))
	focals := []*graphTask{mustTask(w, "D"), mustTask(w, "K")}

	got := collectLines(w, focals, 2)

	assertLines(t, got, []expectedLine{
		{parent: "B", anchors: []string{"D", "E", "F"}},
		{parent: "J", anchors: []string{"K", "L"}},
	})
}

// Scenario 6 — H claimed, K claimed, with B's subtree fully done.
// B's line drops out (no claims, no lookahead reaches it). Two lines.
func TestCollectLines_Scenario6_HAndKClaimed_BDone(t *testing.T) {
	w := newTestWorld(referenceTree(map[string]string{
		"B": "done",
		"C": "done",
		"D": "done",
		"E": "done",
		"F": "done",
		"H": "claimed",
		"K": "claimed",
	}))
	focals := []*graphTask{mustTask(w, "H"), mustTask(w, "K")}

	got := collectLines(w, focals, 2)

	assertLines(t, got, []expectedLine{
		{parent: "G", anchors: []string{"H", "I"}},
		{parent: "J", anchors: []string{"K", "L"}},
	})
}

// Sanity: no focals → no lines.
func TestCollectLines_NoFocals(t *testing.T) {
	w := newTestWorld(referenceTree(nil))

	got := collectLines(w, nil, 2)

	if len(got) != 0 {
		t.Errorf("expected no lines, got %d: %s", len(got), summarizeSeeds(got))
	}
}

// ------------------------------------------------------------------
// LCA fork tests
// ------------------------------------------------------------------

type expectedFork struct {
	chain   []string
	indices []int
}

func assertFork(t *testing.T, got *Fork, want *expectedFork) {
	t.Helper()
	if want == nil {
		if got != nil {
			t.Errorf("fork: got %+v, want nil", got)
		}
		return
	}
	if got == nil {
		t.Fatalf("fork: got nil, want chain=%v indices=%v",
			want.chain, want.indices)
	}
	if !equalSubwayStrings(got.AncestorChain, want.chain) {
		t.Errorf("fork chain: got %v, want %v", got.AncestorChain, want.chain)
	}
	if !equalIntSlice(got.LineIndices, want.indices) {
		t.Errorf("fork indices: got %v, want %v", got.LineIndices, want.indices)
	}
}

func equalIntSlice(a, b []int) bool {
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

// One line → no fork (chrome).
func TestComputeFork_SingleLine(t *testing.T) {
	w := newTestWorld(referenceTree(map[string]string{"D": "claimed"}))
	seeds := collectLines(w, []*graphTask{mustTask(w, "D")}, 2)

	assertFork(t, computeFork(seeds), nil)
}

// Two lines under the same root → fork at A.
func TestComputeFork_TwoLines_SharedRoot(t *testing.T) {
	w := newTestWorld(referenceTree(map[string]string{
		"D": "claimed",
		"K": "claimed",
	}))
	seeds := collectLines(w, []*graphTask{mustTask(w, "D"), mustTask(w, "K")}, 2)

	assertFork(t, computeFork(seeds), &expectedFork{
		chain:   []string{"A"},
		indices: []int{0, 1},
	})
}

// Three lines under the same root → fork at A, indices preserve
// preorder.
func TestComputeFork_ThreeLines_SharedRoot(t *testing.T) {
	w := newTestWorld(referenceTree(map[string]string{
		"D": "claimed",
		"H": "claimed",
	}))
	seeds := collectLines(w, []*graphTask{mustTask(w, "D"), mustTask(w, "H")}, 2)

	assertFork(t, computeFork(seeds), &expectedFork{
		chain:   []string{"A"},
		indices: []int{0, 1, 2},
	})
}

// Scenario 6 — B's subtree gone, G and J under A → fork at A.
func TestComputeFork_Scenario6(t *testing.T) {
	w := newTestWorld(referenceTree(map[string]string{
		"B": "done", "C": "done", "D": "done",
		"E": "done", "F": "done",
		"H": "claimed",
		"K": "claimed",
	}))
	seeds := collectLines(w, []*graphTask{mustTask(w, "H"), mustTask(w, "K")}, 2)

	assertFork(t, computeFork(seeds), &expectedFork{
		chain:   []string{"A"},
		indices: []int{0, 1},
	})
}

// Deep LCA: tree is Root → Solo → {B, G}. LCA of B and G is Solo,
// not Root. Chain has length 1 — extending the chain upward when the
// divergence sits below an only-child cascade is a documented
// refinement (see the spec's edge cases).
func TestComputeFork_DeepLCA_DivergenceBelowSolo(t *testing.T) {
	w := newTestWorld([]tt{
		{short: "Root", parent: "", status: "available"},
		{short: "Solo", parent: "Root", status: "available"},
		{short: "B", parent: "Solo", status: "available"},
		{short: "C", parent: "B", status: "claimed"},
		{short: "D", parent: "B", status: "available"},
		{short: "G", parent: "Solo", status: "available"},
		{short: "H", parent: "G", status: "claimed"},
		{short: "I", parent: "G", status: "available"},
	})
	seeds := collectLines(w, []*graphTask{mustTask(w, "C"), mustTask(w, "H")}, 2)

	assertFork(t, computeFork(seeds), &expectedFork{
		chain:   []string{"Solo"},
		indices: []int{0, 1},
	})
}

// Empty input → no fork.
func TestComputeFork_NoSeeds(t *testing.T) {
	assertFork(t, computeFork(nil), nil)
}
