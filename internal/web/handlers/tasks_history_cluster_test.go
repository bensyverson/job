package handlers

import (
	"testing"

	job "github.com/bensyverson/jobs/internal/job"
)

// TestBuildHistory_ClustersTrailingCriterionStateUnderDone pins the
// dashboard's clustering rule: a contiguous trailing run of
// criterion_state events on the same task that ends in a done event
// collapses onto that done row as a parenthetical roll-up
// (ClusterSummary), not as N+1 separate timeline rows. The structural
// pattern (rather than a time-window) is what the spike on 2026-04-29
// settled on, since every criteria-heavy close in the live data shows
// the criterion_state events recorded in the same second as the done.
func TestBuildHistory_ClustersTrailingCriterionStateUnderDone(t *testing.T) {
	events := []job.EventEntry{
		{ID: 10, EventType: "created", Actor: "alice", CreatedAt: 1000, Detail: ""},
		{ID: 11, EventType: "criteria_added", Actor: "alice", CreatedAt: 1010, Detail: `{"criteria":[{"label":"a"},{"label":"b"},{"label":"c"}]}`},
		{ID: 12, EventType: "criterion_state", Actor: "alice", CreatedAt: 1020, Detail: `{"label":"a","state":"passed","prior":"pending"}`},
		{ID: 13, EventType: "criterion_state", Actor: "alice", CreatedAt: 1020, Detail: `{"label":"b","state":"passed","prior":"pending"}`},
		{ID: 14, EventType: "criterion_state", Actor: "alice", CreatedAt: 1020, Detail: `{"label":"c","state":"passed","prior":"pending"}`},
		{ID: 15, EventType: "done", Actor: "alice", CreatedAt: 1020, Detail: `{}`},
	}

	hist := buildHistory(events)
	if len(hist) != 3 {
		t.Fatalf("expected 3 top-level rows (created, criteria_added, done); got %d:\n%+v", len(hist), hist)
	}
	if hist[2].EventType != "done" {
		t.Errorf("third row should be done; got %q", hist[2].EventType)
	}
	if hist[2].ClusterSummary != "3 criteria marked passed" {
		t.Errorf("done row should carry roll-up %q; got %q", "3 criteria marked passed", hist[2].ClusterSummary)
	}
}

// TestBuildHistory_StandaloneCriterionStateNotClustered covers the
// criterion 2Gs: a criterion_state event with no following done renders
// as its own row, since the operator may want to mark a criterion long
// before deciding to close.
func TestBuildHistory_StandaloneCriterionStateNotClustered(t *testing.T) {
	events := []job.EventEntry{
		{ID: 1, EventType: "created", Actor: "alice", CreatedAt: 1000, Detail: ""},
		{ID: 2, EventType: "criterion_state", Actor: "alice", CreatedAt: 1100, Detail: `{"label":"a","state":"passed","prior":"pending"}`},
	}

	hist := buildHistory(events)
	if len(hist) != 2 {
		t.Fatalf("expected 2 rows (created + standalone criterion_state); got %d", len(hist))
	}
	if hist[1].EventType != "criterion_state" {
		t.Errorf("trailing criterion_state with no following close should stand alone; got %q", hist[1].EventType)
	}
	if hist[1].ClusterSummary != "" {
		t.Errorf("standalone criterion_state should not carry a roll-up; got %q", hist[1].ClusterSummary)
	}
}

// TestBuildHistory_NonContiguousCriterionStateNotClustered guards the
// "trailing run" half of the rule: a criterion_state event that is not
// immediately followed (after only other criterion_state events) by a
// done must not cluster, because some other event broke the run.
func TestBuildHistory_NonContiguousCriterionStateNotClustered(t *testing.T) {
	events := []job.EventEntry{
		{ID: 1, EventType: "created", Actor: "alice", CreatedAt: 1000, Detail: ""},
		{ID: 2, EventType: "criterion_state", Actor: "alice", CreatedAt: 1010, Detail: `{"label":"early","state":"passed","prior":"pending"}`},
		{ID: 3, EventType: "noted", Actor: "alice", CreatedAt: 1020, Detail: ""},
		{ID: 4, EventType: "criterion_state", Actor: "alice", CreatedAt: 1030, Detail: `{"label":"late","state":"passed","prior":"pending"}`},
		{ID: 5, EventType: "done", Actor: "alice", CreatedAt: 1030, Detail: `{}`},
	}

	hist := buildHistory(events)
	if len(hist) != 4 {
		t.Fatalf("expected 4 rows (created, early-cs standalone, noted, done with 1 in roll-up); got %d", len(hist))
	}
	// hist[1] is the early criterion_state — must stand alone.
	if hist[1].EventType != "criterion_state" {
		t.Errorf("early criterion_state should remain a standalone row; got %q", hist[1].EventType)
	}
	if hist[1].ClusterSummary != "" {
		t.Errorf("early criterion_state should have no roll-up; got %q", hist[1].ClusterSummary)
	}
	// hist[3] is the done; its roll-up should reflect only the late one.
	if hist[3].EventType != "done" {
		t.Errorf("hist[3] should be done; got %q", hist[3].EventType)
	}
	if hist[3].ClusterSummary != "1 criterion marked passed" {
		t.Errorf("done should roll up only the trailing run; got %q", hist[3].ClusterSummary)
	}
}

// TestBuildHistory_DoneWithoutPrecedingCriterionState_NoCluster
// regression-checks the common case (most closes have no per-row criteria
// marking): the done row renders normally with no roll-up, so the
// template's empty-summary branch keeps the legacy single-row layout.
func TestBuildHistory_DoneWithoutPrecedingCriterionState_NoCluster(t *testing.T) {
	events := []job.EventEntry{
		{ID: 1, EventType: "created", Actor: "alice", CreatedAt: 1000, Detail: ""},
		{ID: 2, EventType: "done", Actor: "alice", CreatedAt: 1010, Detail: `{}`},
	}
	hist := buildHistory(events)
	if len(hist) != 2 {
		t.Fatalf("expected 2 rows; got %d", len(hist))
	}
	if hist[1].EventType != "done" {
		t.Fatalf("hist[1] should be done; got %q", hist[1].EventType)
	}
	if hist[1].ClusterSummary != "" {
		t.Errorf("plain close should not carry a roll-up; got %q", hist[1].ClusterSummary)
	}
}
