package handlers_test

import (
	"context"
	"testing"
	"time"

	"github.com/bensyverson/jobs/internal/web/handlers"
)

func TestLoadFooterMetrics_ComputesAllFour(t *testing.T) {
	db := setupLogTestDB(t)
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)

	// Two open tasks (one available, one claimed by alice), one done.
	t1 := homeSeedTask(t, db, "AAAAA", "T1", "available", now.Add(-2*time.Hour))
	t2 := homeSeedTask(t, db, "BBBBB", "T2", "available", now.Add(-1*time.Hour))
	t3 := homeSeedTask(t, db, "CCCCC", "T3", "done", now.Add(-30*time.Minute))
	homeSeedClaim(t, db, t2, "alice", now.Add(-10*time.Minute))

	// A second claim by a different actor (bob) on a third available
	// task — gives 2 distinct claim holders.
	t4 := homeSeedTask(t, db, "DDDDD", "T4", "available", now.Add(-15*time.Minute))
	homeSeedClaim(t, db, t4, "bob", now.Add(-5*time.Minute))

	// Events: a few created, one done within the last 60 minutes.
	homeSeedEvent(t, db, t1, "created", now.Add(-50*time.Minute))
	homeSeedEvent(t, db, t2, "created", now.Add(-40*time.Minute))
	homeSeedEvent(t, db, t3, "done", now.Add(-30*time.Minute))
	homeSeedEvent(t, db, t1, "noted", now.Add(-20*time.Minute))

	got, err := handlers.LoadFooterMetrics(context.Background(), db, now)
	if err != nil {
		t.Fatalf("LoadFooterMetrics: %v", err)
	}

	if got.Actors != 2 {
		t.Errorf("Actors: got %d, want 2 (alice + bob)", got.Actors)
	}
	// WIP: t1 (available), t2 (claimed), t4 (claimed) = 3. t3 is done.
	if got.WIP != 3 {
		t.Errorf("WIP: got %d, want 3 (t1+t2+t4 open)", got.WIP)
	}
	// EventsPerMin: 4 created/noted/done + 2 claim events (homeSeedClaim
	// inserts a 'claimed' event each call) = 6 total in the last 60 min;
	// 6/60 = 0 with integer division.
	if got.EventsPerMin != 0 {
		t.Errorf("EventsPerMin: got %d, want 0 (6 events in 60 min ÷ 60)",
			got.EventsPerMin)
	}
	// TasksPerHour: 1 done event in the last 60 minutes.
	if got.TasksPerHour != 1 {
		t.Errorf("TasksPerHour: got %d, want 1", got.TasksPerHour)
	}
}

func TestLoadFooterMetrics_ExpiredClaimsExcluded(t *testing.T) {
	db := setupLogTestDB(t)
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)

	t1 := homeSeedTask(t, db, "AAAAA", "T1", "available", now.Add(-2*time.Hour))
	// homeSeedClaim sets claim_expires_at = claimedAt + 1800. With a
	// claim 60 minutes ago, expires 30 minutes ago — past `now`.
	homeSeedClaim(t, db, t1, "alice", now.Add(-60*time.Minute))

	got, err := handlers.LoadFooterMetrics(context.Background(), db, now)
	if err != nil {
		t.Fatalf("LoadFooterMetrics: %v", err)
	}

	if got.Actors != 0 {
		t.Errorf("Actors: got %d, want 0 (alice's claim expired before now)",
			got.Actors)
	}
}
