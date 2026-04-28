package signals

import (
	"fmt"
	"slices"
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
// focals are tasks that always anchor a window; lookahead are tasks
// reached via the +L leaf walk that anchor a window only when not
// already covered. Tests that pre-date the focal/lookahead split set
// only focals.
type expectedLine struct {
	parent    string
	focals    []string
	lookahead []string
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
		gotFocals := taskShortIDs(got[i].focalAnchors)
		if !equalSubwayStrings(gotFocals, want[i].focals) {
			t.Errorf("line %d (%s) focals: got %v, want %v",
				i, want[i].parent, gotFocals, want[i].focals)
		}
		gotLookahead := taskShortIDs(got[i].lookaheadAnchors)
		if !equalSubwayStrings(gotLookahead, want[i].lookahead) {
			t.Errorf("line %d (%s) lookahead: got %v, want %v",
				i, want[i].parent, gotLookahead, want[i].lookahead)
		}
	}
}

func summarizeSeeds(ls []*lineSeed) string {
	parts := make([]string, len(ls))
	for i, l := range ls {
		focals := strings.Join(taskShortIDs(l.focalAnchors), ",")
		look := strings.Join(taskShortIDs(l.lookaheadAnchors), ",")
		parts[i] = l.parent.shortID + "{" + focals + "|" + look + "}"
	}
	return strings.Join(parts, " ")
}

func summarizeExpected(ls []expectedLine) string {
	parts := make([]string, len(ls))
	for i, l := range ls {
		parts[i] = l.parent + "{" + strings.Join(l.focals, ",") + "|" + strings.Join(l.lookahead, ",") + "}"
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
		{parent: "B", focals: []string{"D"}},
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
		{parent: "B", focals: []string{"D", "E"}},
		{parent: "G", lookahead: []string{"H"}},
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
		{parent: "B", focals: []string{"D", "F"}},
		{parent: "G", lookahead: []string{"H", "I"}},
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
		{parent: "B", focals: []string{"D"}},
		{parent: "G", focals: []string{"H"}},
		{parent: "J", lookahead: []string{"K"}},
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
		{parent: "B", focals: []string{"D"}},
		{parent: "J", focals: []string{"K"}},
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
		{parent: "G", focals: []string{"H"}},
		{parent: "J", focals: []string{"K"}},
	})
}

// Image #9 regression (cursor ?at=1288). The +L walk descends from
// the focal into a sibling's subtree (a leaf under another child of
// the focal's parent). Under the parent-boundary rule that lookahead
// is absorbed — a descent into the focal's parent's subtree does not
// "exit" the focal's parent and so does not earn a new line. Only
// focal F's parent contributes a line; the sibling A does not.
func TestCollectLines_LookaheadDescendsIntoSibling_NoNewLine(t *testing.T) {
	w := newTestWorld([]tt{
		{short: "P", parent: "", status: "available"},
		{short: "F", parent: "P", status: "claimed"},
		{short: "A", parent: "P", status: "available"},
		{short: "A1", parent: "A", status: "available"},
		{short: "A2", parent: "A", status: "available"},
	})
	focals := []*graphTask{mustTask(w, "F")}

	got := collectLines(w, focals, 1)

	assertLines(t, got, []expectedLine{
		{parent: "P", focals: []string{"F"}, lookahead: nil},
	})
}

// Cousin fence — when the +L walk does exit the focal's parent's
// subtree by climbing to the LCA and descending into a different
// branch, the cousin's parent earns its own line and the existing
// fork machinery extends the LCA chain back to the divergence point.
func TestCollectLines_LookaheadCrossesToCousin_NewLine(t *testing.T) {
	// A is the LCA; B and G are siblings under A. focal D is B's
	// only leaf, so nextLeaf(D) climbs to A and descends into G,
	// landing on H. H exits B's subtree → G earns a line.
	w := newTestWorld([]tt{
		{short: "A", parent: "", status: "available"},
		{short: "B", parent: "A", status: "available"},
		{short: "D", parent: "B", status: "claimed"},
		{short: "G", parent: "A", status: "available"},
		{short: "H", parent: "G", status: "available"},
	})
	focals := []*graphTask{mustTask(w, "D")}

	got := collectLines(w, focals, 1)

	assertLines(t, got, []expectedLine{
		{parent: "B", focals: []string{"D"}, lookahead: nil},
		{parent: "G", focals: nil, lookahead: []string{"H"}},
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

// ------------------------------------------------------------------
// Windowing tests
// ------------------------------------------------------------------

// newWideLine builds a graphWorld with a single parent "P" and
// len(statuses) children named c00, c01, ... in slice order. Returns
// the world, the parent, and the child slice for index addressing.
func newWideLine(statuses []string) (*graphWorld, *graphTask, []*graphTask) {
	tasks := []tt{{short: "P", parent: "", status: "available"}}
	for i, s := range statuses {
		tasks = append(tasks, tt{
			short:  fmt.Sprintf("c%02d", i),
			parent: "P",
			status: s,
		})
	}
	w := newTestWorld(tasks)
	parent := mustTask(w, "P")
	children := make([]*graphTask, len(statuses))
	for i := range statuses {
		children[i] = mustTask(w, fmt.Sprintf("c%02d", i))
	}
	return w, parent, children
}

// buildSeed builds a lineSeed with all supplied anchors marked as
// focals. Use buildSeedWithLookahead when a test needs to distinguish
// focal anchors (which always anchor a window) from lookahead-only
// anchors (which anchor a window only when not already covered).
func buildSeed(parent *graphTask, children []*graphTask, focalIndices []int) *lineSeed {
	return buildSeedWithLookahead(parent, children, focalIndices, nil)
}

func buildSeedWithLookahead(parent *graphTask, children []*graphTask, focalIndices, lookaheadIndices []int) *lineSeed {
	focals := make([]*graphTask, len(focalIndices))
	for i, idx := range focalIndices {
		focals[i] = children[idx]
	}
	lookaheads := make([]*graphTask, 0, len(lookaheadIndices))
	for _, idx := range lookaheadIndices {
		// A child can't be both focal and lookahead — focal wins.
		isFocal := slices.Contains(focalIndices, idx)
		if !isFocal {
			lookaheads = append(lookaheads, children[idx])
		}
	}
	return &lineSeed{
		parent:           parent,
		focalAnchors:     focals,
		lookaheadAnchors: lookaheads,
	}
}

type expectedItem struct {
	kind      LineItemKind
	short     string
	moreCount int
}

func stop(s string) expectedItem { return expectedItem{kind: LineItemStop, short: s} }
func elision() expectedItem      { return expectedItem{kind: LineItemElision} }
func more(n int) expectedItem    { return expectedItem{kind: LineItemMore, moreCount: n} }
func stops(short ...string) []expectedItem {
	out := make([]expectedItem, len(short))
	for i, s := range short {
		out[i] = stop(s)
	}
	return out
}

func assertLine(t *testing.T, got Line, wantAnchor string, wantItems []expectedItem) {
	t.Helper()
	if got.AnchorShortID != wantAnchor {
		t.Errorf("anchor: got %q, want %q", got.AnchorShortID, wantAnchor)
	}
	if !equalItems(got.Items, wantItems) {
		t.Errorf("items: got %s, want %s",
			summarizeItems(got.Items), summarizeWantItems(wantItems))
	}
}

func equalItems(got []LineItem, want []expectedItem) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i].Kind != want[i].kind {
			return false
		}
		switch want[i].kind {
		case LineItemStop:
			if got[i].ShortID != want[i].short {
				return false
			}
		case LineItemMore:
			if got[i].MoreCount != want[i].moreCount {
				return false
			}
		}
	}
	return true
}

func summarizeItems(items []LineItem) string {
	parts := make([]string, len(items))
	for i, it := range items {
		switch it.Kind {
		case LineItemElision:
			parts[i] = "…"
		case LineItemMore:
			parts[i] = fmt.Sprintf("(+%d)", it.MoreCount)
		default:
			parts[i] = it.ShortID
		}
	}
	return "[" + strings.Join(parts, " ") + "]"
}

func summarizeWantItems(items []expectedItem) string {
	parts := make([]string, len(items))
	for i, it := range items {
		switch it.kind {
		case LineItemElision:
			parts[i] = "…"
		case LineItemMore:
			parts[i] = fmt.Sprintf("(+%d)", it.moreCount)
		default:
			parts[i] = it.short
		}
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// Single anchor with all siblings inside ±N — no elision either side.
// Mirrors Scenario 1 of the design doc (B's line: C done, D claimed,
// E available, F available; anchors per collectLines = D + lookahead
// E, F).
func TestApplyWindow_AllVisible_NoElision(t *testing.T) {
	_, parent, children := newWideLine([]string{
		"done", "claimed", "available", "available",
	})
	seed := buildSeedWithLookahead(parent, children, []int{1}, []int{2, 3})

	line := applyWindow(seed, 2)

	assertLine(t, line, "P", stops("c00", "c01", "c02", "c03"))
}

// Long line, single focal mid-way — leading elision (in-gap dots),
// trailing collapsed to a terminating ellipsis. Per project/2026-04-
// 27-graph-row-merging.md the legacy `(+N)` pill is replaced with
// LineItemElisionTerminating in both code paths.
func TestApplyWindow_LongLine_ElisionBothSides(t *testing.T) {
	statuses := make([]string, 30)
	for i := range statuses {
		statuses[i] = "available"
	}
	statuses[17] = "claimed"
	_, parent, children := newWideLine(statuses)
	seed := buildSeed(parent, children, []int{17})

	line := applyWindow(seed, 2)

	want := []expectedItem{elision()}
	want = append(want, stops("c15", "c16", "c17", "c18", "c19")...)
	want = append(want, elisionTerminating())
	assertLine(t, line, "P", want)
}

// Two close focals (within 2N+1) — windows merge into one visible
// span; trailing siblings collapse to a terminating ellipsis.
func TestApplyWindow_MultiFocal_Contiguous(t *testing.T) {
	statuses := []string{"available", "claimed", "available", "claimed", "available", "available"}
	_, parent, children := newWideLine(statuses)
	seed := buildSeed(parent, children, []int{1, 3})

	line := applyWindow(seed, 1)

	want := []expectedItem{}
	want = append(want, stops("c00", "c01", "c02", "c03", "c04")...)
	want = append(want, elisionTerminating())
	assertLine(t, line, "P", want)
}

// Two distant focals — two visible windows separated by an in-gap
// `…`; trailing collapses to a terminating ellipsis.
func TestApplyWindow_MultiFocal_GapElided(t *testing.T) {
	statuses := make([]string, 12)
	for i := range statuses {
		statuses[i] = "available"
	}
	statuses[1] = "claimed"
	statuses[8] = "claimed"
	_, parent, children := newWideLine(statuses)
	seed := buildSeed(parent, children, []int{1, 8})

	line := applyWindow(seed, 1)

	want := []expectedItem{}
	want = append(want, stops("c00", "c01", "c02")...)
	want = append(want, elision())
	want = append(want, stops("c07", "c08", "c09")...)
	want = append(want, elisionTerminating())
	assertLine(t, line, "P", want)
}

// Anchor at start — no leading elision; trailing siblings collapse
// to a terminating ellipsis.
func TestApplyWindow_AnchorAtStart(t *testing.T) {
	statuses := []string{"claimed", "available", "available", "available", "available"}
	_, parent, children := newWideLine(statuses)
	seed := buildSeed(parent, children, []int{0})

	line := applyWindow(seed, 2)

	want := []expectedItem{}
	want = append(want, stops("c00", "c01", "c02")...)
	want = append(want, elisionTerminating())
	assertLine(t, line, "P", want)
}

// Anchor at end — no trailing elision (window's right edge clamps).
func TestApplyWindow_AnchorAtEnd(t *testing.T) {
	statuses := []string{"available", "available", "available", "available", "claimed"}
	_, parent, children := newWideLine(statuses)
	seed := buildSeed(parent, children, []int{4})

	line := applyWindow(seed, 2)

	want := []expectedItem{elision()}
	want = append(want, stops("c02", "c03", "c04")...)
	assertLine(t, line, "P", want)
}

// Done sibling between two focals (within union of ±N windows) —
// renders inline, line stays visually continuous.
func TestApplyWindow_DoneBetweenFocals(t *testing.T) {
	statuses := []string{"available", "claimed", "done", "claimed"}
	_, parent, children := newWideLine(statuses)
	seed := buildSeed(parent, children, []int{1, 3})

	line := applyWindow(seed, 2)

	assertLine(t, line, "P", stops("c00", "c01", "c02", "c03"))
}

// Lookahead anchor inside the focal's ±N window — the lookahead
// drops, the line stays capped at ±N around the focal. Mirrors the
// ?at=1216 case: a single claimed focal with the next leaf one step
// ahead and L=1 should produce 5 stops, not 6.
func TestApplyWindow_LookaheadInsideFocalWindow_Drops(t *testing.T) {
	statuses := make([]string, 10)
	for i := range statuses {
		statuses[i] = "available"
	}
	statuses[5] = "claimed"
	_, parent, children := newWideLine(statuses)
	seed := buildSeedWithLookahead(parent, children, []int{5}, []int{6})

	line := applyWindow(seed, 2)

	want := []expectedItem{elision()}
	want = append(want, stops("c03", "c04", "c05", "c06", "c07")...)
	want = append(want, elisionTerminating())
	assertLine(t, line, "P", want)
}

// Two real focals close together plus a lookahead inside their union
// window — the lookahead drops, the windows merge into one span;
// trailing siblings collapse to a terminating ellipsis.
func TestApplyWindow_TwoFocalsPlusInsideLookahead(t *testing.T) {
	statuses := []string{
		"available", "claimed", "available", "claimed",
		"available", "available", "available", "available",
	}
	_, parent, children := newWideLine(statuses)
	seed := buildSeedWithLookahead(parent, children, []int{1, 3}, []int{4})

	line := applyWindow(seed, 2)

	want := []expectedItem{}
	want = append(want, stops("c00", "c01", "c02", "c03", "c04", "c05")...)
	want = append(want, elisionTerminating())
	assertLine(t, line, "P", want)
}

// Empty parent — empty Line with anchor set, no items.
func TestApplyWindow_NoChildren(t *testing.T) {
	w := newTestWorld([]tt{{short: "P", parent: "", status: "available"}})
	parent := mustTask(w, "P")
	seed := &lineSeed{parent: parent}

	line := applyWindow(seed, 2)

	if line.AnchorShortID != "P" {
		t.Errorf("anchor: got %q, want P", line.AnchorShortID)
	}
	if len(line.Items) != 0 {
		t.Errorf("items: got %d, want 0", len(line.Items))
	}
}

// ------------------------------------------------------------------
// BuildSubway tests
//
// End-to-end composition of pickFocals + collectLines + computeFork +
// applyWindow with Nodes/Edges assembly. Reference scenarios from
// project/2026-04-25-graph-clarification.md, exercised through the
// in-memory graphWorld (no DB).
// ------------------------------------------------------------------

func hasSubwayEdge(edges []SubwayEdge, from, to string, kind SubwayEdgeKind) bool {
	for _, e := range edges {
		if e.FromShortID == from && e.ToShortID == to && e.Kind == kind {
			return true
		}
	}
	return false
}

func findSubwayNode(nodes []SubwayNode, short string) (SubwayNode, bool) {
	for _, n := range nodes {
		if n.ShortID == short {
			return n, true
		}
	}
	return SubwayNode{}, false
}

func subwayNodeShortIDs(nodes []SubwayNode) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = n.ShortID
	}
	return out
}

func edgeSummary(edges []SubwayEdge) string {
	parts := make([]string, len(edges))
	kindName := map[SubwayEdgeKind]string{
		SubwayEdgeFlow:         "flow",
		SubwayEdgeBranch:       "branch",
		SubwayEdgeBranchClosed: "branch⊘",
		SubwayEdgeBlocker:      "blocker",
	}
	for i, e := range edges {
		parts[i] = fmt.Sprintf("%s→%s(%s)", e.FromShortID, e.ToShortID, kindName[e.Kind])
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// Empty world → empty Subway. No focals, nothing to render.
func TestBuildSubway_NoFocals_EmptySubway(t *testing.T) {
	w := newTestWorld(nil)

	got := buildSubway(w)

	if len(got.Lines) != 0 {
		t.Errorf("Lines: got %d, want 0", len(got.Lines))
	}
	if len(got.Forks) != 0 {
		t.Errorf("Forks: got %d, want 0", len(got.Forks))
	}
	if len(got.Nodes) != 0 {
		t.Errorf("Nodes: got %d, want 0", len(got.Nodes))
	}
	if len(got.Edges) != 0 {
		t.Errorf("Edges: got %d, want 0", len(got.Edges))
	}
}

// All work done → no claims, no available leaf, empty Subway.
func TestBuildSubway_NothingActionable_EmptySubway(t *testing.T) {
	statuses := map[string]string{}
	for _, s := range []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L"} {
		statuses[s] = "done"
	}
	w := newTestWorld(referenceTree(statuses))

	got := buildSubway(w)

	if len(got.Lines) != 0 {
		t.Errorf("expected empty Subway when nothing actionable, got %d lines", len(got.Lines))
	}
}

// No claims but available leaf exists → falls back to globalNext.
// One focal → single-focal preorder window mode (project/2026-04-27-
// graph-row-merging.md): the line anchors on the project root and
// items walk preorder around the focal, not on the focal's parent.
func TestBuildSubway_FallsBackToGlobalNext_WhenNoClaims(t *testing.T) {
	w := newTestWorld(referenceTree(nil))

	got := buildSubway(w)

	// globalNext picks the first preorder available leaf with no
	// blockers — that's C in the reference tree. Single-focal
	// preorder mode anchors the row on the project root A and walks
	// ±N preorder steps around C.
	if len(got.Lines) != 1 {
		t.Fatalf("Lines: got %d, want 1 (single fallback line)", len(got.Lines))
	}
	if got.Lines[0].AnchorShortID != "A" {
		t.Errorf("Line anchor: got %q, want %q (project root)",
			got.Lines[0].AnchorShortID, "A")
	}
	if len(got.Forks) != 0 {
		t.Errorf("Forks: got %d, want 0 for single line", len(got.Forks))
	}
}

// Scenario 1 — D claimed (C done). One line on B, no fork.
func TestBuildSubway_Scenario1_OneLine_NoFork(t *testing.T) {
	w := newTestWorld(referenceTree(map[string]string{
		"C": "done",
		"D": "claimed",
	}))

	got := buildSubway(w)

	if len(got.Lines) != 1 {
		t.Fatalf("Lines: got %d, want 1", len(got.Lines))
	}
	if len(got.Forks) != 0 {
		t.Errorf("Forks: got %d, want 0 for single line", len(got.Forks))
	}

	// Nodes: anchor B + stops C, D, E, F.
	wantShorts := []string{"B", "C", "D", "E", "F"}
	for _, s := range wantShorts {
		if _, ok := findSubwayNode(got.Nodes, s); !ok {
			t.Errorf("node %q missing from Nodes %v", s, subwayNodeShortIDs(got.Nodes))
		}
	}

	// Node states.
	if n, _ := findSubwayNode(got.Nodes, "C"); n.State != SubwayNodeDone {
		t.Errorf("C state: got %d, want Done", n.State)
	}
	if n, _ := findSubwayNode(got.Nodes, "D"); n.State != SubwayNodeActive {
		t.Errorf("D state: got %d, want Active", n.State)
	}
	if n, _ := findSubwayNode(got.Nodes, "E"); n.State != SubwayNodeTodo {
		t.Errorf("E state: got %d, want Todo", n.State)
	}

	// Flow edges: anchor → first stop, then stop → stop.
	wantFlow := [][2]string{{"B", "C"}, {"C", "D"}, {"D", "E"}, {"E", "F"}}
	for _, p := range wantFlow {
		if !hasSubwayEdge(got.Edges, p[0], p[1], SubwayEdgeFlow) {
			t.Errorf("missing Flow edge %s→%s in %s", p[0], p[1], edgeSummary(got.Edges))
		}
	}

	// No Branch / BranchClosed without a fork.
	for _, e := range got.Edges {
		if e.Kind == SubwayEdgeBranch || e.Kind == SubwayEdgeBranchClosed {
			t.Errorf("unexpected branch edge %s→%s without fork", e.FromShortID, e.ToShortID)
		}
	}
}

// Scenario 2 — D and E claimed; G blocked by B → BranchClosed ingress
// to G's line. Two lines, fork at A. Requires L=2 for E's lookahead to
// reach into G's subtree; the production default is L=1 (which would
// leave only B's line). buildSubwayWith pins the design-doc scenario.
// Under the multi-focal tree-map mode (project/2026-04-27-graph-row-
// merging.md, S2), G no longer renders. The legacy parent-rooted
// model used +L lookahead from a focal to reach unrelated subtrees;
// the new mode only renders the focal-path subgraph (LCA→focals).
// Focals D and E share parent B, so the LCA is B, the subgraph is
// {B, D, E}, and the row is a single B-line. Closure on G's branch
// is no longer expressible because G isn't on the focal path.
func TestBuildSubway_Scenario2_BranchClosedToBlockedLine(t *testing.T) {
	w := newTestWorld(
		referenceTree(map[string]string{
			"C": "done",
			"D": "claimed",
			"E": "claimed",
		}),
		[2]string{"B", "G"},
	)

	got := buildSubwayWith(w, 2, 2)

	if len(got.Lines) != 1 {
		t.Fatalf("Lines: got %d, want 1", len(got.Lines))
	}
	if got.Lines[0].AnchorShortID != "B" {
		t.Errorf("anchor: got %q, want B", got.Lines[0].AnchorShortID)
	}
	// Single-row cluster: no Fork (the LCA is the row's leftmost).
	if len(got.Forks) != 0 {
		t.Errorf("Forks: got %d, want 0 (single row, LCA is anchor)", len(got.Forks))
	}
	// G is excluded from the subgraph.
	if _, ok := findSubwayNode(got.Nodes, "G"); ok {
		t.Errorf("G should not appear (off the focal path); got %v", subwayNodeShortIDs(got.Nodes))
	}
	// E is active.
	if n, _ := findSubwayNode(got.Nodes, "E"); n.State != SubwayNodeActive {
		t.Errorf("E state: got %d, want Active", n.State)
	}
	// D is active.
	if n, _ := findSubwayNode(got.Nodes, "D"); n.State != SubwayNodeActive {
		t.Errorf("D state: got %d, want Active", n.State)
	}
}

// Scenario 3 — D and F claimed, E done between. Under the multi-
// focal tree-map mode (project/2026-04-27-graph-row-merging.md, S2),
// focals D and F share parent B → LCA is B, subgraph is {B, D, E, F}
// (E is in the row's preorder window between focals D and F). G is
// off the focal path and no longer renders; the legacy "lookahead
// reaches G via L=2" path is gone.
func TestBuildSubway_Scenario3_DoneSiblingBetweenFocals(t *testing.T) {
	w := newTestWorld(
		referenceTree(map[string]string{
			"C": "done",
			"D": "claimed",
			"E": "done",
			"F": "claimed",
		}),
		[2]string{"B", "G"},
	)

	got := buildSubwayWith(w, 2, 2)

	if len(got.Lines) != 1 {
		t.Fatalf("Lines: got %d, want 1", len(got.Lines))
	}
	if got.Lines[0].AnchorShortID != "B" {
		t.Errorf("anchor: got %q, want B", got.Lines[0].AnchorShortID)
	}
	if n, ok := findSubwayNode(got.Nodes, "E"); !ok {
		t.Errorf("E missing from Nodes")
	} else if n.State != SubwayNodeDone {
		t.Errorf("E state: got %d, want Done", n.State)
	}
	// F is active.
	if n, _ := findSubwayNode(got.Nodes, "F"); n.State != SubwayNodeActive {
		t.Errorf("F state: got %d, want Active", n.State)
	}
	// G no longer renders — off the focal path subgraph.
	if _, ok := findSubwayNode(got.Nodes, "G"); ok {
		t.Errorf("G should not appear (off the focal path); got %v", subwayNodeShortIDs(got.Nodes))
	}
}

// Scenario 4 — D, H claimed (G unblocked). Under the multi-focal
// tree-map mode (project/2026-04-27-graph-row-merging.md, S2), the
// focals D and H have LCA=A; the focal-path subgraph is
// {A, B, D, G, H}. Rows: A topmost (leftmost-only), B sub-row, G
// sub-row. The legacy "L=2 lookahead to K → J peek line" path is
// gone — J/K/L are off the focal path.
func TestBuildSubway_Scenario4_ThreeLines_AllOpen(t *testing.T) {
	w := newTestWorld(referenceTree(map[string]string{
		"C": "done",
		"D": "claimed",
		"H": "claimed",
	}))

	got := buildSubwayWith(w, 2, 2)

	if len(got.Lines) != 3 {
		t.Fatalf("Lines: got %d, want 3", len(got.Lines))
	}
	// Topmost row anchored on A (leftmost-only); B and G sub-rows
	// branch off A under the Fork shim.
	if got.Lines[0].AnchorShortID != "A" {
		t.Errorf("Lines[0] anchor: got %q, want A (LCA topmost)",
			got.Lines[0].AnchorShortID)
	}
	if len(got.Forks) == 0 || got.Forks[0].AncestorChain[0] != "A" {
		t.Fatalf("Forks: want chain=[A], got %+v", got.Forks)
	}
	for _, anchor := range []string{"B", "G"} {
		if !hasSubwayEdge(got.Edges, "A", anchor, SubwayEdgeBranch) {
			t.Errorf("missing open Branch edge A→%s in %s", anchor, edgeSummary(got.Edges))
		}
	}
	// J/K/L are off the focal path under tree-map mode.
	for _, sid := range []string{"J", "K", "L"} {
		if _, ok := findSubwayNode(got.Nodes, sid); ok {
			t.Errorf("%s should not appear (off focal path); got %v",
				sid, subwayNodeShortIDs(got.Nodes))
		}
	}
}

// Scenario 5 — D and K claimed; G has no claim. Under the multi-
// focal tree-map mode (project/2026-04-27-graph-row-merging.md, S2),
// the focal-path subgraph is {A, B, D, J, K}; rows are A topmost
// (leftmost-only), B sub-row, J sub-row. G is off the path and
// stays absent — same outcome as before, but expressed via the
// new three-row shape.
func TestBuildSubway_Scenario5_GAbsent(t *testing.T) {
	w := newTestWorld(referenceTree(map[string]string{
		"C": "done",
		"D": "claimed",
		"K": "claimed",
	}))

	got := buildSubway(w)

	if len(got.Lines) != 3 {
		t.Fatalf("Lines: got %d, want 3", len(got.Lines))
	}
	if got.Lines[0].AnchorShortID != "A" {
		t.Errorf("Lines[0] anchor: got %q, want A (LCA topmost)",
			got.Lines[0].AnchorShortID)
	}
	for _, s := range []string{"B", "J"} {
		if _, ok := findSubwayNode(got.Nodes, s); !ok {
			t.Errorf("expected anchor %q in Nodes, got %v", s, subwayNodeShortIDs(got.Nodes))
		}
	}
	if _, ok := findSubwayNode(got.Nodes, "G"); ok {
		t.Errorf("did not expect G in Nodes (no line on G), got %v", subwayNodeShortIDs(got.Nodes))
	}
	if hasSubwayEdge(got.Edges, "A", "G", SubwayEdgeBranch) || hasSubwayEdge(got.Edges, "A", "G", SubwayEdgeBranchClosed) {
		t.Errorf("did not expect any A→G branch edge: %s", edgeSummary(got.Edges))
	}
}

// Scenario 6 — H, K claimed, B's subtree fully done. Under the
// multi-focal tree-map mode (project/2026-04-27-graph-row-
// merging.md, S2), the focal-path subgraph is {A, G, H, J, K}; rows
// are A topmost (leftmost-only), G sub-row, J sub-row. B/C/D/E/F
// are off the path and stay absent.
func TestBuildSubway_Scenario6_BSubtreeDropsOut(t *testing.T) {
	w := newTestWorld(referenceTree(map[string]string{
		"B": "done", "C": "done", "D": "done",
		"E": "done", "F": "done",
		"H": "claimed",
		"K": "claimed",
	}))

	got := buildSubway(w)

	if len(got.Lines) != 3 {
		t.Fatalf("Lines: got %d, want 3", len(got.Lines))
	}
	if got.Lines[0].AnchorShortID != "A" {
		t.Errorf("Lines[0] anchor: got %q, want A (LCA topmost)",
			got.Lines[0].AnchorShortID)
	}
	if _, ok := findSubwayNode(got.Nodes, "B"); ok {
		t.Errorf("did not expect B in Nodes (subtree done), got %v", subwayNodeShortIDs(got.Nodes))
	}
	for _, anchor := range []string{"G", "J"} {
		if !hasSubwayEdge(got.Edges, "A", anchor, SubwayEdgeBranch) {
			t.Errorf("missing open Branch A→%s in %s", anchor, edgeSummary(got.Edges))
		}
	}
}

// Regression for ?at=1321: Phase 10 had five open children with one
// preceding sibling already done (Dashboard polish pass) and no
// claims anywhere on the line, so the focal falls through to
// globalNext (Empty-state polish at idx 1). Pre-fix the lookahead
// union was widening the visible window past the focal's ±N.
//
// Under the single-focal preorder window mode (project/2026-04-27-
// graph-row-merging.md) the row anchors on the project root P, and
// trailing siblings beyond the +N walk render as LineItemElision-
// Terminating dots — there is no (+N) count anywhere. The window
// includes the project root preorder position, so no leading
// elision is needed.
func TestBuildSubway_GlobalNextWithDoneSibling_HonorsCenteringAndCount(t *testing.T) {
	tasks := []tt{{short: "P", parent: "", status: "available"}}
	statuses := []string{"done", "available", "available", "available", "available"}
	for i, s := range statuses {
		tasks = append(tasks, tt{
			short:  fmt.Sprintf("c%02d", i),
			parent: "P",
			status: s,
		})
	}
	w := newTestWorld(tasks)

	got := buildSubwayWith(w, 1, 2)

	if len(got.Lines) != 1 {
		t.Fatalf("Lines: got %d, want 1", len(got.Lines))
	}
	line := got.Lines[0]
	if line.AnchorShortID != "P" {
		t.Errorf("anchor: got %q, want P", line.AnchorShortID)
	}
	// Preorder: P, c00, c01, c02, c03, c04. globalNext = c01 at
	// pos 2; N=2 → window [0, 4]. P (anchor) drops out of items;
	// c04 is hidden → LineItemElisionTerminating.
	wantItems := []expectedItem{
		stop("c00"), stop("c01"), stop("c02"), stop("c03"),
		elisionTerminating(),
	}
	if !equalItems(line.Items, wantItems) {
		t.Errorf("items: got %s, want %s",
			summarizeItems(line.Items), summarizeWantItems(wantItems))
	}
	// Sanity: focal is c01 (globalNext skips c00 as done).
	if n, ok := findSubwayNode(got.Nodes, "c01"); !ok {
		t.Errorf("c01 missing from Nodes")
	} else if n.State != SubwayNodeTodo {
		t.Errorf("c01 State: got %d, want Todo (no claims, only globalNext)", n.State)
	}
}

// A row with hidden trailing siblings renders a terminating
// ellipsis on the topological model (project/2026-04-27-graph-
// row-merging.md): LineItemElisionTerminating sits at the row's
// right edge. The legacy LineItemMore (+N) pill is gone in
// single-focal preorder mode, so there is no synthetic edge target
// for trailing dots — the renderer paints the dots from the
// elision marker alone.
func TestBuildSubway_TrailingTerminatingElision_NoMorePill(t *testing.T) {
	tasks := []tt{{short: "P", parent: "", status: "available"}}
	for i := range 10 {
		s := "available"
		if i == 1 {
			s = "claimed"
		}
		tasks = append(tasks, tt{
			short:  fmt.Sprintf("c%02d", i),
			parent: "P",
			status: s,
		})
	}
	w := newTestWorld(tasks)

	got := buildSubwayWith(w, 0, 2)

	if len(got.Lines) != 1 {
		t.Fatalf("Lines: got %d, want 1", len(got.Lines))
	}
	line := got.Lines[0]
	hasTerminating := false
	for _, item := range line.Items {
		if item.Kind == LineItemMore {
			t.Errorf("unexpected LineItemMore in single-focal mode: %s",
				summarizeItems(line.Items))
		}
		if item.Kind == LineItemElisionTerminating {
			hasTerminating = true
		}
	}
	if !hasTerminating {
		t.Errorf("expected trailing LineItemElisionTerminating, got %s",
			summarizeItems(line.Items))
	}
	// No edge should target the legacy synthetic More short ID.
	moreID := MoreShortID("P")
	for _, e := range got.Edges {
		if e.ToShortID == moreID {
			t.Errorf("unexpected edge to legacy MoreShortID %q: %s",
				moreID, edgeSummary(got.Edges))
		}
	}
}

// Node metadata: Title, Actor, URL come straight off the underlying
// graphTask (URL is "/tasks/" + ShortID).
func TestBuildSubway_NodeMetadata_TitleActorURL(t *testing.T) {
	w := newTestWorld(referenceTree(map[string]string{
		"C": "done",
		"D": "claimed",
	}))
	d := mustTask(w, "D")
	d.title = "Wire JS to front-end"
	d.actor = "alice"

	got := buildSubway(w)

	n, ok := findSubwayNode(got.Nodes, "D")
	if !ok {
		t.Fatalf("D missing from Nodes")
	}
	if n.Title != "Wire JS to front-end" {
		t.Errorf("Title: got %q, want %q", n.Title, "Wire JS to front-end")
	}
	if n.Actor != "alice" {
		t.Errorf("Actor: got %q, want %q", n.Actor, "alice")
	}
	if n.URL != "/tasks/D" {
		t.Errorf("URL: got %q, want %q", n.URL, "/tasks/D")
	}
}

// Same-line stop blockage renders on the *immediate ingress* edge,
// not as a long span from the original blocker. D blocks F with E
// between them on B's line: the dashed marker sits on E→F (F's
// ingress), and there is no D→F edge.
func TestBuildSubway_BlockerEdge_OnIngressNotSpan(t *testing.T) {
	w := newTestWorld(
		referenceTree(map[string]string{
			"C": "done",
			"D": "claimed",
		}),
		[2]string{"D", "F"}, // D blocks F with E between them
	)

	got := buildSubway(w)

	if !hasSubwayEdge(got.Edges, "E", "F", SubwayEdgeBlocker) {
		t.Errorf("missing ingress Blocker E→F in %s", edgeSummary(got.Edges))
	}
	// No long span from the original blocker.
	if hasSubwayEdge(got.Edges, "D", "F", SubwayEdgeBlocker) {
		t.Errorf("did not expect long Blocker D→F (block sits on ingress); got: %s",
			edgeSummary(got.Edges))
	}
	// Flow E→F is replaced by Blocker E→F — they're mutually exclusive.
	if hasSubwayEdge(got.Edges, "E", "F", SubwayEdgeFlow) {
		t.Errorf("Flow E→F should be replaced by the Blocker; got: %s",
			edgeSummary(got.Edges))
	}
}

// When a Blocker edge covers a (from, to) pair, the Flow edge for the
// same pair is suppressed. Without this, the Flow's arrowhead reads
// as if the dashed amber blocker line itself has an arrow.
func TestBuildSubway_BlockerEdge_SuppressesAdjacentFlow(t *testing.T) {
	w := newTestWorld(
		referenceTree(map[string]string{
			"C": "done",
			"D": "claimed",
		}),
		[2]string{"D", "E"}, // D blocks E (consecutive stops on B's line)
	)

	got := buildSubway(w)

	if !hasSubwayEdge(got.Edges, "D", "E", SubwayEdgeBlocker) {
		t.Errorf("missing Blocker D→E in %s", edgeSummary(got.Edges))
	}
	if hasSubwayEdge(got.Edges, "D", "E", SubwayEdgeFlow) {
		t.Errorf("Flow D→E should be suppressed when Blocker D→E covers the pair: %s",
			edgeSummary(got.Edges))
	}
	// Other Flow edges remain — only the covered pair is dropped.
	for _, p := range [][2]string{{"B", "C"}, {"C", "D"}, {"E", "F"}} {
		if !hasSubwayEdge(got.Edges, p[0], p[1], SubwayEdgeFlow) {
			t.Errorf("missing Flow %s→%s in %s", p[0], p[1], edgeSummary(got.Edges))
		}
	}
}

// One blocker, multiple rendered blocked stops → only the nearest
// blocked stop (smallest preorder position) gets a Blocker edge.
// Subsequent blocks are transitive and would visually imply
// "intermediate stop blocks the next one" if drawn separately.
func TestBuildSubway_BlockerEdge_OnlyNearestBlocked(t *testing.T) {
	w := newTestWorld(
		referenceTree(map[string]string{
			"C": "done",
			"D": "claimed",
		}),
		[2]string{"D", "E"},
		[2]string{"D", "F"},
	)

	got := buildSubway(w)

	if !hasSubwayEdge(got.Edges, "D", "E", SubwayEdgeBlocker) {
		t.Errorf("missing Blocker D→E (nearest blocked) in %s", edgeSummary(got.Edges))
	}
	if hasSubwayEdge(got.Edges, "D", "F", SubwayEdgeBlocker) {
		t.Errorf("Blocker D→F should be suppressed (transitive); got: %s",
			edgeSummary(got.Edges))
	}
}

// Done blockers don't earn a Blocker edge — historical, not a live
// constraint.
func TestBuildSubway_DoneBlocker_NoBlockerEdge(t *testing.T) {
	w := newTestWorld(
		referenceTree(map[string]string{
			"C": "done",
			"D": "claimed",
		}),
		[2]string{"C", "F"}, // C blocks F, but C is done
	)

	got := buildSubway(w)

	if hasSubwayEdge(got.Edges, "C", "F", SubwayEdgeBlocker) {
		t.Errorf("did not expect Blocker C→F (C done): %s", edgeSummary(got.Edges))
	}
}

// ------------------------------------------------------------------
// Edge cases (per design doc — 3+ active phases, deep LCA path,
// mid-row deep focal, same-agent multiple claims)
// ------------------------------------------------------------------

// fourPhaseTree extends the reference shape with a fourth phase. Used
// to exercise the 3+-active-phases case.
//
//	A
//	├── B (phase 1) — children C, D
//	├── E (phase 2) — children F, G
//	├── H (phase 3) — children I, J
//	└── K (phase 4) — children L, M
func fourPhaseTree(statuses map[string]string) []tt {
	base := []tt{
		{short: "A", parent: ""},
		{short: "B", parent: "A"},
		{short: "C", parent: "B"},
		{short: "D", parent: "B"},
		{short: "E", parent: "A"},
		{short: "F", parent: "E"},
		{short: "G", parent: "E"},
		{short: "H", parent: "A"},
		{short: "I", parent: "H"},
		{short: "J", parent: "H"},
		{short: "K", parent: "A"},
		{short: "L", parent: "K"},
		{short: "M", parent: "K"},
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

// 3+ active phases — four claims, one per phase. Under the multi-
// focal tree-map mode (project/2026-04-27-graph-row-merging.md, S2),
// the LCA of {C, F, I, L} is A; rows are A topmost (leftmost-only)
// + four sub-rows (B, E, H, K), each branching off A.
func TestBuildSubway_FourActivePhases(t *testing.T) {
	w := newTestWorld(fourPhaseTree(map[string]string{
		"C": "claimed",
		"F": "claimed",
		"I": "claimed",
		"L": "claimed",
	}))

	got := buildSubway(w)

	if len(got.Lines) != 5 {
		t.Fatalf("Lines: got %d, want 5 (A topmost + 4 sub-rows)",
			len(got.Lines))
	}
	if got.Lines[0].AnchorShortID != "A" {
		t.Errorf("Lines[0] anchor: got %q, want A (LCA topmost)",
			got.Lines[0].AnchorShortID)
	}
	if len(got.Forks) == 0 || got.Forks[0].AncestorChain[0] != "A" {
		t.Fatalf("Forks: want chain=[A], got %+v", got.Forks)
	}
	for _, anchor := range []string{"B", "E", "H", "K"} {
		if !hasSubwayEdge(got.Edges, "A", anchor, SubwayEdgeBranch) {
			t.Errorf("missing open Branch A→%s in %s", anchor, edgeSummary(got.Edges))
		}
	}
}

// Deep LCA — two claims share an only-child ancestor (Solo) below
// the project root. The fork's AncestorChain should anchor at Solo,
// not Root.
//
//	Root
//	└── Solo
//	    ├── B  (children: C [claimed], D)
//	    └── G  (children: H [claimed], I)
func TestBuildSubway_DeepLCAPath(t *testing.T) {
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

	got := buildSubway(w)

	if len(got.Forks) == 0 {
		t.Fatalf("Fork: got nil, want non-nil")
	}
	if got.Forks[0].AncestorChain[0] != "Solo" {
		t.Errorf("Fork ancestor: got %q, want %q", got.Forks[0].AncestorChain[0], "Solo")
	}
	if _, ok := findSubwayNode(got.Nodes, "Solo"); !ok {
		t.Errorf("Solo missing from Nodes %v", subwayNodeShortIDs(got.Nodes))
	}
	// Root is above the divergence and should not appear.
	if _, ok := findSubwayNode(got.Nodes, "Root"); ok {
		t.Errorf("Root should not appear (above LCA); got %v", subwayNodeShortIDs(got.Nodes))
	}
	// Branch edges originate at Solo.
	for _, anchor := range []string{"B", "G"} {
		if !hasSubwayEdge(got.Edges, "Solo", anchor, SubwayEdgeBranch) {
			t.Errorf("missing Branch Solo→%s in %s", anchor, edgeSummary(got.Edges))
		}
	}
}

// Mid-row deep focal — claim is on a grandchild. Under the single-
// focal preorder window mode (project/2026-04-27-graph-row-
// merging.md) the row's leftmost is the project root; ±N preorder
// steps around the focal pull in the relevant ancestors and
// siblings. Top is now the row anchor (col 0); Mid sits between Top
// and B in the preorder walk.
//
//	Top   (project root, becomes the row anchor)
//	└── Mid
//	    └── B
//	        ├── X
//	        └── Y [claimed]
func TestBuildSubway_MidRowDeepFocal(t *testing.T) {
	w := newTestWorld([]tt{
		{short: "Top", parent: "", status: "available"},
		{short: "Mid", parent: "Top", status: "available"},
		{short: "B", parent: "Mid", status: "available"},
		{short: "X", parent: "B", status: "available"},
		{short: "Y", parent: "B", status: "claimed"},
	})

	got := buildSubway(w)

	if len(got.Lines) != 1 {
		t.Fatalf("Lines: got %d, want 1", len(got.Lines))
	}
	if got.Lines[0].AnchorShortID != "Top" {
		t.Errorf("anchor: got %q, want Top (project root)",
			got.Lines[0].AnchorShortID)
	}
	// Y renders as the active stop.
	if n, _ := findSubwayNode(got.Nodes, "Y"); n.State != SubwayNodeActive {
		t.Errorf("Y state: got %d, want Active", n.State)
	}
	// Preorder: Top, Mid, B, X, Y. Focal Y at pos 4; N=2 →
	// window [2, 4] = {B, X, Y}. Mid sits at pos 1, outside the
	// window, and should not appear.
	if _, ok := findSubwayNode(got.Nodes, "Mid"); ok {
		t.Errorf("Mid should not appear (outside preorder window); got %v",
			subwayNodeShortIDs(got.Nodes))
	}
	// Top is the anchor and is rendered.
	if _, ok := findSubwayNode(got.Nodes, "Top"); !ok {
		t.Errorf("Top should appear as the row anchor; got %v",
			subwayNodeShortIDs(got.Nodes))
	}
}

// Cross-project claims — focals under two different project roots
// produce one independent rendering per project root, stacked
// vertically (project/2026-04-27-graph-row-merging.md). Each
// cluster is single-focal here, so each renders via the new
// preorder window mode: the project root is the row's leftmost
// (col 0), no Fork is emitted (the project root IS the anchor; no
// transfer-station chrome is needed when only one row branches off
// it).
//
//	RootA            RootB
//	└── B            └── G
//	    ├── C [c]        ├── H [c]
//	    └── D            └── I
func TestBuildSubway_CrossProjectClaims_PerClusterRows(t *testing.T) {
	w := newTestWorld([]tt{
		{short: "RootA", parent: "", status: "available"},
		{short: "B", parent: "RootA", status: "available"},
		{short: "C", parent: "B", status: "claimed"},
		{short: "D", parent: "B", status: "available"},
		{short: "RootB", parent: "", status: "available"},
		{short: "G", parent: "RootB", status: "available"},
		{short: "H", parent: "G", status: "claimed"},
		{short: "I", parent: "G", status: "available"},
	})

	got := buildSubway(w)

	// Single-focal clusters do not emit Forks under the new mode —
	// the project root is the leftmost of its row, no transfer-
	// station chrome is needed.
	if len(got.Forks) != 0 {
		t.Errorf("Forks: got %d, want 0 (single-focal clusters need no fork)",
			len(got.Forks))
	}
	// Two rows, one per cluster, each anchored on its project root.
	if len(got.Lines) != 2 {
		t.Fatalf("Lines: got %d, want 2 (one per cluster)", len(got.Lines))
	}
	anchors := []string{got.Lines[0].AnchorShortID, got.Lines[1].AnchorShortID}
	for _, want := range []string{"RootA", "RootB"} {
		found := slices.Contains(anchors, want)
		if !found {
			t.Errorf("expected line anchored on %q; anchors=%v", want, anchors)
		}
	}
	// Both project roots render as nodes.
	for _, sid := range []string{"RootA", "RootB"} {
		if _, ok := findSubwayNode(got.Nodes, sid); !ok {
			t.Errorf("expected root %q in Nodes; got %v",
				sid, subwayNodeShortIDs(got.Nodes))
		}
	}
	// Each cluster's anchor flows to its first content stop (Flow,
	// not Branch — there is no fork in single-focal mode).
	for _, p := range [][2]string{{"RootA", "B"}, {"RootB", "G"}} {
		if !hasSubwayEdge(got.Edges, p[0], p[1], SubwayEdgeFlow) {
			t.Errorf("missing Flow %s→%s in %s", p[0], p[1], edgeSummary(got.Edges))
		}
	}
}

// Same-agent multiple claims — two focals owned by the same actor on
// different lines. The graph is about work, not workers; output
// should be identical to the multi-agent case. Under the multi-focal
// tree-map mode (project/2026-04-27-graph-row-merging.md, S2), the
// LCA of D and K is A; rows are A topmost (leftmost-only) + B sub-
// row + J sub-row.
func TestBuildSubway_SameAgentMultipleClaims(t *testing.T) {
	w := newTestWorld(referenceTree(map[string]string{
		"D": "claimed",
		"K": "claimed",
	}))
	mustTask(w, "D").actor = "alice"
	mustTask(w, "K").actor = "alice"

	got := buildSubway(w)

	if len(got.Lines) != 3 {
		t.Fatalf("Lines: got %d, want 3 (A topmost + B + J)", len(got.Lines))
	}
	if got.Lines[0].AnchorShortID != "A" {
		t.Errorf("Lines[0] anchor: got %q, want A (LCA topmost)",
			got.Lines[0].AnchorShortID)
	}
	// Both active stops carry the actor.
	for _, s := range []string{"D", "K"} {
		n, ok := findSubwayNode(got.Nodes, s)
		if !ok {
			t.Fatalf("%s missing from Nodes", s)
		}
		if n.State != SubwayNodeActive {
			t.Errorf("%s state: got %d, want Active", s, n.State)
		}
		if n.Actor != "alice" {
			t.Errorf("%s actor: got %q, want %q", s, n.Actor, "alice")
		}
	}
	// Fork still emerges at A regardless of actor identity.
	if len(got.Forks) == 0 || got.Forks[0].AncestorChain[0] != "A" {
		t.Errorf("Forks: want chain=[A], got %+v", got.Forks)
	}
}
