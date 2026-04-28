package signals

import (
	"testing"
)

// pickFocals must filter out claimed parents — focals are leaves only
// (project/2026-04-27-graph-row-merging.md, invariant 1). The leaf-
// frontier rule normally prevents parent claims, but legacy data and
// non-CLI clients can still produce them; this guard is the safety
// net so the multi-focal mode never tries to render a parent as a
// focal stop.

// Claimed parent with at least one open child must NOT appear in the
// focals slice. The leaf claim under it (or globalNext) takes over.
func TestPickFocals_ClaimedParentWithOpenChildren_NotAFocal(t *testing.T) {
	w := newTestWorld([]tt{
		{short: "A", parent: "", status: "available"},
		{short: "B", parent: "A", status: "claimed"},
		{short: "C", parent: "B", status: "available"},
	})

	focals := pickFocals(w)
	for _, f := range focals {
		if f.shortID == "B" {
			t.Errorf("pickFocals returned claimed parent B; want B filtered out (focals are leaves only)")
		}
	}
}

// Claimed leaf (no children at all) IS a focal — the common case.
func TestPickFocals_ClaimedLeaf_IsAFocal(t *testing.T) {
	w := newTestWorld([]tt{
		{short: "A", parent: "", status: "available"},
		{short: "B", parent: "A", status: "claimed"},
	})

	focals := pickFocals(w)
	if len(focals) != 1 || focals[0].shortID != "B" {
		short := ""
		if len(focals) > 0 {
			short = focals[0].shortID
		}
		t.Errorf("pickFocals: got %d focals (first=%q), want exactly [B]", len(focals), short)
	}
}

// Claimed parent whose children are all done/canceled still has
// children structurally — pickFocals filters on len(t.children) > 0,
// not on open-frontier semantics. Keeps the guard simple and
// uncoupled from status churn.
func TestPickFocals_ClaimedParentWithAllChildrenClosed_NotAFocal(t *testing.T) {
	w := newTestWorld([]tt{
		{short: "A", parent: "", status: "available"},
		{short: "B", parent: "A", status: "claimed"},
		{short: "C", parent: "B", status: "done"},
		{short: "D", parent: "B", status: "canceled"},
	})

	focals := pickFocals(w)
	for _, f := range focals {
		if f.shortID == "B" {
			t.Errorf("pickFocals returned claimed parent B (children all closed); want B filtered out")
		}
	}
}
