package signals

import (
	"testing"
)

// ------------------------------------------------------------------
// Single-focal preorder window tests
//
// Reference: project/2026-04-27-graph-row-merging.md.
//
// Under the new model the row is a preorder slice of the focal's
// project tree, capped at ±N preorder steps around the focal. The
// row's leftmost is the project root; content stops are the visible
// neighbors of the focal in preorder, excluding the project root
// (rendered separately as the anchor). Backward broken-line elision
// sits between leftmost and the first content stop when the -N walk
// doesn't reach the project root; forward terminating elision sits
// at the right edge when the +N walk continues past the row's last
// visible stop.
// ------------------------------------------------------------------

// elisionBroken is the test-side constructor for the broken-line
// elision marker used by the single-focal preorder mode.
func elisionBroken() expectedItem {
	return expectedItem{kind: LineItemElisionBroken}
}

// elisionTerminating is the test-side constructor for the trailing
// terminating ellipsis used by the single-focal preorder mode.
func elisionTerminating() expectedItem {
	return expectedItem{kind: LineItemElisionTerminating}
}

// Walk reaches the project root — the focal sits two preorder steps
// below it, so -N=2 covers the entire history. No leading elision;
// no trailing elision because +N walks past the tree's last stop.
func TestSingleFocalLine_WalkReachesRoot_NoLeadingElision(t *testing.T) {
	w := newTestWorld([]tt{
		{short: "Root", parent: "", status: "available"},
		{short: "B", parent: "Root", status: "available"},
		{short: "C", parent: "B", status: "claimed"},
	})
	focal := mustTask(w, "C")

	line := buildSingleFocalLine(w, focal, 2)

	// Preorder of the project tree: [Root, B, C]. Focal C at pos 2.
	// Window: [0, 2] — covers the whole tree. AnchorShortID = Root;
	// items skip the root itself (rendered as anchor) and list
	// remaining preorder stops in order.
	assertLine(t, line, "Root", []expectedItem{stop("B"), stop("C")})
}

// -N doesn't reach the project root → broken-line leading elision
// sits between leftmost (the project root) and the first content
// stop in the window.
func TestSingleFocalLine_LeadingBrokenElision(t *testing.T) {
	w := newTestWorld([]tt{
		{short: "Root", parent: "", status: "available"},
		{short: "B", parent: "Root", status: "available"},
		{short: "C", parent: "B", status: "available"},
		{short: "D", parent: "C", status: "available"},
		{short: "E", parent: "D", status: "available"},
		{short: "F", parent: "E", status: "claimed"},
	})
	focal := mustTask(w, "F")

	line := buildSingleFocalLine(w, focal, 2)

	// Preorder: [Root, B, C, D, E, F]. Focal F at pos 5. N=2.
	// Window: [3, 5] = {D, E, F}. Pos 3 > 0 → leading broken-line
	// elision. +N=7 past last (5) → no trailing elision.
	assertLine(t, line, "Root", []expectedItem{
		elisionBroken(),
		stop("D"), stop("E"), stop("F"),
	})
}

// Focal sits at the tree's last preorder position; +N walks past the
// last leaf. Per the design doc: row terminates at the focal — no
// trailing elision.
func TestSingleFocalLine_FocalAtEnd_NoTrailingElision(t *testing.T) {
	w := newTestWorld([]tt{
		{short: "Root", parent: "", status: "available"},
		{short: "A", parent: "Root", status: "available"},
		{short: "B", parent: "Root", status: "claimed"},
	})
	focal := mustTask(w, "B")

	line := buildSingleFocalLine(w, focal, 2)

	// Preorder: [Root, A, B]. Focal B at pos 2 (last). N=2.
	// Window: [0, 2] — covers everything. No elision either side.
	assertLine(t, line, "Root", []expectedItem{stop("A"), stop("B")})
}

// Focal mid-tree, +N cuts the row mid-walk → terminating ellipsis
// at the right edge. Also exercises leading broken-line elision when
// -N starts past the project root. No (+N) count anywhere.
func TestSingleFocalLine_TrailingTerminatingElision(t *testing.T) {
	w := newTestWorld([]tt{
		{short: "Root", parent: "", status: "available"},
		{short: "A", parent: "Root", status: "available"},
		{short: "B", parent: "Root", status: "claimed"},
		{short: "C", parent: "Root", status: "available"},
		{short: "D", parent: "Root", status: "available"},
		{short: "E", parent: "Root", status: "available"},
	})
	focal := mustTask(w, "B")

	line := buildSingleFocalLine(w, focal, 1)

	// Preorder: [Root, A, B, C, D, E]. Focal B at pos 2. N=1.
	// Window: [1, 3] = {A, B, C}. Pos 1 > 0 → leading broken-line
	// elision. +N=3 < len-1=5 → trailing terminating elision.
	assertLine(t, line, "Root", []expectedItem{
		elisionBroken(),
		stop("A"), stop("B"), stop("C"),
		elisionTerminating(),
	})
}

// The window crosses a parent boundary: the new parent renders as
// its own preorder stop on the row (e.g., Phase 7 leaf → Phase 8 →
// Phase 8 leaf → [focal] in production data).
func TestSingleFocalLine_ParentBoundaryCrossing(t *testing.T) {
	w := newTestWorld([]tt{
		{short: "Root", parent: "", status: "available"},
		{short: "B", parent: "Root", status: "available"},
		{short: "C", parent: "B", status: "available"},
		{short: "D", parent: "B", status: "available"},
		{short: "E", parent: "Root", status: "available"},
		{short: "F", parent: "E", status: "claimed"},
		{short: "G", parent: "E", status: "available"},
	})
	focal := mustTask(w, "F")

	line := buildSingleFocalLine(w, focal, 2)

	// Preorder: [Root, B, C, D, E, F, G]. Focal F at pos 5. N=2.
	// Window: [3, 6] = {D, E, F, G}. E (a parent) renders as its
	// own preorder stop in the row — the window crosses from B's
	// subtree (D) up over the boundary to E and down into F. Pos 3
	// > 0 → leading elision. +N=7 past last (6) → no trailing.
	assertLine(t, line, "Root", []expectedItem{
		elisionBroken(),
		stop("D"), stop("E"), stop("F"), stop("G"),
	})
}

// Reproduces ?at=1288 (focal k9fFC, ±2): row reads
// … → 1SYqo ✓ → hNTiB ✓ → [k9fFC] → oDKYC ✓ → tpC4u ✓ → …
//
// Synthetic tree: Root with a chain of leaves placed so the focal
// k9fFC sits surrounded by 1SYqo/hNTiB on the left and oDKYC/tpC4u
// on the right, with extra leaves on either side that fall outside
// the ±2 window.
func TestSingleFocalLine_ReproducesAt1288Shape(t *testing.T) {
	w := newTestWorld([]tt{
		{short: "Root", parent: "", status: "available"},
		{short: "phasePre1", parent: "Root", status: "done"},
		{short: "phasePre2", parent: "Root", status: "done"},
		{short: "phasePre3", parent: "Root", status: "done"},
		{short: "1SYqo", parent: "Root", status: "done"},
		{short: "hNTiB", parent: "Root", status: "done"},
		{short: "k9fFC", parent: "Root", status: "claimed"},
		{short: "oDKYC", parent: "Root", status: "done"},
		{short: "tpC4u", parent: "Root", status: "done"},
		{short: "post1", parent: "Root", status: "available"},
		{short: "post2", parent: "Root", status: "available"},
	})
	focal := mustTask(w, "k9fFC")

	line := buildSingleFocalLine(w, focal, 2)

	// Preorder: Root, phasePre1..3, 1SYqo, hNTiB, k9fFC, oDKYC,
	// tpC4u, post1, post2. Focal k9fFC at pos 6. N=2.
	// Window: [4, 8] = {1SYqo, hNTiB, k9fFC, oDKYC, tpC4u}. Pos 4
	// > 0 → leading broken-line elision; +N=8 < len-1=10 →
	// trailing terminating elision.
	assertLine(t, line, "Root", []expectedItem{
		elisionBroken(),
		stop("1SYqo"), stop("hNTiB"), stop("k9fFC"), stop("oDKYC"), stop("tpC4u"),
		elisionTerminating(),
	})
}
