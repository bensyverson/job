package handlers_test

import (
	"testing"

	"github.com/bensyverson/jobs/internal/web/handlers"
)

func TestDisplayStatus_TableDriven(t *testing.T) {
	cases := []struct {
		raw      string
		blocked  bool
		want     string
		wantNote string
	}{
		{"available", false, "todo", "default open task"},
		{"available", true, "blocked", "available but with open blockers"},
		{"claimed", false, "active", "someone's working on it"},
		{"claimed", true, "blocked", "claimed but blocked — blockers override"},
		{"done", false, "done", "done never shows as blocked"},
		{"done", true, "done", "done beats blocker override"},
		{"canceled", false, "canceled", "canceled passes through"},
		{"canceled", true, "canceled", "canceled beats blocker override"},
		{"weird", false, "weird", "unknown statuses pass through unchanged"},
	}
	for _, c := range cases {
		t.Run(c.raw+"_blockers="+boolStr(c.blocked), func(t *testing.T) {
			got := handlers.DisplayStatus(c.raw, c.blocked)
			if got != c.want {
				t.Errorf("DisplayStatus(%q, %v) = %q, want %q (%s)",
					c.raw, c.blocked, got, c.want, c.wantNote)
			}
		})
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
