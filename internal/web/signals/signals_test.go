package signals

import (
	"context"
	"database/sql"
	"testing"
	"time"

	job "github.com/bensyverson/jobs/internal/job"
)

// Tests seed directly via SQL rather than going through the job
// package's Run* helpers because those helpers record created_at via
// time.Now(), not a hookable clock. Writing rows directly lets each
// test control timestamps precisely.

func seedTask(t *testing.T, db *sql.DB, shortID, title, status string, createdAt time.Time) int64 {
	t.Helper()
	res, err := db.Exec(`
		INSERT INTO tasks (short_id, title, description, status, sort_order, created_at, updated_at)
		VALUES (?, ?, '', ?, 0, ?, ?)
	`, shortID, title, status, createdAt.Unix(), createdAt.Unix())
	if err != nil {
		t.Fatalf("seed task %q: %v", shortID, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}
	return id
}

func seedClaim(t *testing.T, db *sql.DB, taskID int64, actor string, claimedAt time.Time) {
	t.Helper()
	expiresAt := claimedAt.Unix() + 1800
	_, err := db.Exec(`
		UPDATE tasks SET status = 'claimed', claimed_by = ?, claim_expires_at = ?
		WHERE id = ?
	`, actor, expiresAt, taskID)
	if err != nil {
		t.Fatalf("seed claim: %v", err)
	}
	seedEvent(t, db, taskID, "claimed", actor, claimedAt)
}

func seedEvent(t *testing.T, db *sql.DB, taskID int64, eventType, actor string, at time.Time) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO events (task_id, event_type, actor, detail, created_at)
		VALUES (?, ?, ?, '', ?)
	`, taskID, eventType, actor, at.Unix())
	if err != nil {
		t.Fatalf("seed event: %v", err)
	}
}

func seedBlock(t *testing.T, db *sql.DB, blockedID, blockerID int64, at time.Time) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO blocks (blocker_id, blocked_id, created_at)
		VALUES (?, ?, ?)
	`, blockerID, blockedID, at.Unix())
	if err != nil {
		t.Fatalf("seed block: %v", err)
	}
}

// ------------------------------------------------------------------
// Activity histogram
// ------------------------------------------------------------------

func TestActivity_EmptyDatabase(t *testing.T) {
	db := job.SetupTestDB(t)
	now := time.Unix(1_700_000_000, 0)

	sig, err := Compute(context.Background(), db, now)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if got := sig.Activity.TotalEvents(); got != 0 {
		t.Errorf("TotalEvents: got %d, want 0", got)
	}
	for i, b := range sig.Activity.Buckets {
		if b.Total() != 0 {
			t.Errorf("bucket[%d] total: got %d, want 0", i, b.Total())
		}
	}
}

func TestActivity_CountsEventsInLast60m(t *testing.T) {
	db := job.SetupTestDB(t)
	base := time.Unix(1_700_000_000, 0)
	tID := seedTask(t, db, "t1", "t", "available", base.Add(-2*time.Hour))

	// Three creates inside the 60m window.
	seedEvent(t, db, tID, "created", "u", base)
	seedEvent(t, db, tID, "created", "u", base.Add(-30*time.Minute))
	seedEvent(t, db, tID, "created", "u", base.Add(-59*time.Minute))
	// One create 90m ago — outside window.
	seedEvent(t, db, tID, "created", "u", base.Add(-90*time.Minute))

	sig, err := Compute(context.Background(), db, base)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if sig.Activity.TotalCreate != 3 {
		t.Errorf("TotalCreate: got %d, want 3", sig.Activity.TotalCreate)
	}
	if sig.Activity.TotalEvents() != 3 {
		t.Errorf("TotalEvents: got %d, want 3", sig.Activity.TotalEvents())
	}
}

func TestActivity_BucketPositionByMinutesAgo(t *testing.T) {
	db := job.SetupTestDB(t)
	base := time.Unix(1_700_000_000, 0)
	tID := seedTask(t, db, "t1", "t", "available", base.Add(-2*time.Hour))

	// 30s ago → bucket[59] (floor(30s/60s) = 0 minutes ago).
	seedEvent(t, db, tID, "created", "u", base.Add(-30*time.Second))
	// 5m30s ago → floor(5.5) = 5 → bucket[54].
	seedEvent(t, db, tID, "created", "u", base.Add(-5*time.Minute-30*time.Second))

	sig, err := Compute(context.Background(), db, base)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if sig.Activity.Buckets[59].Create != 1 {
		t.Errorf("bucket[59].Create: got %d, want 1", sig.Activity.Buckets[59].Create)
	}
	if sig.Activity.Buckets[54].Create != 1 {
		t.Errorf("bucket[54].Create: got %d, want 1", sig.Activity.Buckets[54].Create)
	}
}

func TestActivity_SplitsByEventType(t *testing.T) {
	db := job.SetupTestDB(t)
	base := time.Unix(1_700_000_000, 0)
	tID := seedTask(t, db, "t1", "t", "available", base.Add(-2*time.Hour))

	seedEvent(t, db, tID, "created", "u", base.Add(-5*time.Minute))
	seedEvent(t, db, tID, "created", "u", base.Add(-5*time.Minute))
	seedEvent(t, db, tID, "claimed", "u", base.Add(-4*time.Minute))
	seedEvent(t, db, tID, "done", "u", base.Add(-3*time.Minute))
	seedEvent(t, db, tID, "blocked", "u", base.Add(-2*time.Minute))
	// Event type outside the histogram's set — ignored.
	seedEvent(t, db, tID, "noted", "u", base.Add(-1*time.Minute))

	sig, err := Compute(context.Background(), db, base)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if sig.Activity.TotalCreate != 2 {
		t.Errorf("TotalCreate: got %d, want 2", sig.Activity.TotalCreate)
	}
	if sig.Activity.TotalClaim != 1 {
		t.Errorf("TotalClaim: got %d, want 1", sig.Activity.TotalClaim)
	}
	if sig.Activity.TotalDone != 1 {
		t.Errorf("TotalDone: got %d, want 1", sig.Activity.TotalDone)
	}
	if sig.Activity.TotalBlock != 1 {
		t.Errorf("TotalBlock: got %d, want 1", sig.Activity.TotalBlock)
	}
	if sig.Activity.TotalEvents() != 5 {
		t.Errorf("TotalEvents: got %d, want 5", sig.Activity.TotalEvents())
	}
}

// ------------------------------------------------------------------
// Newly blocked
// ------------------------------------------------------------------

func TestNewlyBlocked_EmptyWhenNoRecentBlocks(t *testing.T) {
	db := job.SetupTestDB(t)
	now := time.Unix(1_700_000_000, 0)

	sig, err := Compute(context.Background(), db, now)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if sig.NewlyBlocked.Count != 0 {
		t.Errorf("Count: got %d, want 0", sig.NewlyBlocked.Count)
	}
	if sig.NewlyBlocked.Progress != 0 {
		t.Errorf("Progress: got %f, want 0", sig.NewlyBlocked.Progress)
	}
}

func TestNewlyBlocked_CountsEdgesInLast10m(t *testing.T) {
	db := job.SetupTestDB(t)
	base := time.Unix(1_700_000_000, 0)

	aID := seedTask(t, db, "a", "a", "available", base.Add(-1*time.Hour))
	bID := seedTask(t, db, "b", "b", "available", base.Add(-1*time.Hour))
	cID := seedTask(t, db, "c", "c", "available", base.Add(-1*time.Hour))
	dID := seedTask(t, db, "d", "d", "available", base.Add(-1*time.Hour))

	// Edge 12m ago — outside window.
	seedBlock(t, db, bID, aID, base.Add(-12*time.Minute))
	// Two edges inside 10m window.
	seedBlock(t, db, cID, aID, base.Add(-5*time.Minute))
	seedBlock(t, db, dID, aID, base.Add(-2*time.Minute))

	sig, err := Compute(context.Background(), db, base)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if sig.NewlyBlocked.Count != 2 {
		t.Errorf("Count: got %d, want 2", sig.NewlyBlocked.Count)
	}
	if len(sig.NewlyBlocked.Items) != 2 {
		t.Fatalf("Items len: got %d, want 2", len(sig.NewlyBlocked.Items))
	}
	if sig.NewlyBlocked.Items[0].BlockedShortID != "d" {
		t.Errorf("Items[0].BlockedShortID: got %q, want %q",
			sig.NewlyBlocked.Items[0].BlockedShortID, "d")
	}
	if sig.NewlyBlocked.Items[0].WaitingOnShortID != "a" {
		t.Errorf("Items[0].WaitingOnShortID: got %q, want %q",
			sig.NewlyBlocked.Items[0].WaitingOnShortID, "a")
	}
}

func TestNewlyBlocked_ProgressSaturatesAtThreshold(t *testing.T) {
	db := job.SetupTestDB(t)
	base := time.Unix(1_700_000_000, 0)

	rootID := seedTask(t, db, "r", "r", "available", base.Add(-1*time.Hour))
	count := NewlyBlockedThreshold + 1
	for i := 0; i < count; i++ {
		sid := string(rune('A' + i))
		id := seedTask(t, db, sid, sid, "available", base.Add(-1*time.Hour))
		seedBlock(t, db, id, rootID, base.Add(-2*time.Minute))
	}

	sig, err := Compute(context.Background(), db, base)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if sig.NewlyBlocked.Count != count {
		t.Errorf("Count: got %d, want %d", sig.NewlyBlocked.Count, count)
	}
	if sig.NewlyBlocked.Progress != 1.0 {
		t.Errorf("Progress: got %f, want 1.0 (saturated)", sig.NewlyBlocked.Progress)
	}
	if len(sig.NewlyBlocked.Items) > NewlyBlockedItemLimit {
		t.Errorf("Items cap: got %d items, want <= %d",
			len(sig.NewlyBlocked.Items), NewlyBlockedItemLimit)
	}
}

// ------------------------------------------------------------------
// Longest active claim
// ------------------------------------------------------------------

func TestLongestClaim_AbsentWhenNoActiveClaims(t *testing.T) {
	db := job.SetupTestDB(t)
	now := time.Unix(1_700_000_000, 0)

	sig, err := Compute(context.Background(), db, now)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if sig.LongestClaim.Present {
		t.Errorf("Present: got true, want false (no active claims)")
	}
}

func TestLongestClaim_ReturnsOldestActiveClaim(t *testing.T) {
	db := job.SetupTestDB(t)
	base := time.Unix(1_700_000_000, 0)

	aID := seedTask(t, db, "a", "first", "available", base.Add(-1*time.Hour))
	bID := seedTask(t, db, "b", "second", "available", base.Add(-1*time.Hour))

	seedClaim(t, db, aID, "alice", base.Add(-10*time.Minute))
	seedClaim(t, db, bID, "bob", base.Add(-3*time.Minute))

	sig, err := Compute(context.Background(), db, base)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if !sig.LongestClaim.Present {
		t.Fatal("Present: got false, want true")
	}
	if sig.LongestClaim.TaskShortID != "a" {
		t.Errorf("TaskShortID: got %q, want %q", sig.LongestClaim.TaskShortID, "a")
	}
	if sig.LongestClaim.Actor != "alice" {
		t.Errorf("Actor: got %q, want %q", sig.LongestClaim.Actor, "alice")
	}
	if sig.LongestClaim.DurationSeconds != 600 {
		t.Errorf("DurationSeconds: got %d, want 600", sig.LongestClaim.DurationSeconds)
	}
	if sig.LongestClaim.Progress < 0.32 || sig.LongestClaim.Progress > 0.34 {
		t.Errorf("Progress: got %f, want ~0.33", sig.LongestClaim.Progress)
	}
}

func TestLongestClaim_UsesMostRecentClaimEventWhenTaskWasReclaimed(t *testing.T) {
	db := job.SetupTestDB(t)
	base := time.Unix(1_700_000_000, 0)

	aID := seedTask(t, db, "a", "reclaimed", "available", base.Add(-2*time.Hour))
	// Old 'claimed' event from a prior, since-released claim — must be ignored.
	seedEvent(t, db, aID, "claimed", "alice", base.Add(-90*time.Minute))
	// Current claim by bob started 7m ago.
	seedClaim(t, db, aID, "bob", base.Add(-7*time.Minute))

	sig, err := Compute(context.Background(), db, base)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if !sig.LongestClaim.Present {
		t.Fatal("Present: got false, want true")
	}
	if sig.LongestClaim.Actor != "bob" {
		t.Errorf("Actor: got %q, want %q", sig.LongestClaim.Actor, "bob")
	}
	if sig.LongestClaim.DurationSeconds != 7*60 {
		t.Errorf("DurationSeconds: got %d, want %d", sig.LongestClaim.DurationSeconds, 7*60)
	}
}

func TestLongestClaim_ProgressSaturatesAtThreshold(t *testing.T) {
	db := job.SetupTestDB(t)
	base := time.Unix(1_700_000_000, 0)

	aID := seedTask(t, db, "a", "stuck", "available", base.Add(-2*time.Hour))
	seedClaim(t, db, aID, "alice", base.Add(-80*time.Minute))

	sig, err := Compute(context.Background(), db, base)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if !sig.LongestClaim.Present {
		t.Fatal("Present: got false, want true")
	}
	if sig.LongestClaim.Progress != 1.0 {
		t.Errorf("Progress: got %f, want 1.0 (saturated)", sig.LongestClaim.Progress)
	}
}

// ------------------------------------------------------------------
// Oldest todo
// ------------------------------------------------------------------

func TestOldestTodo_AbsentWhenNoAvailableTasks(t *testing.T) {
	db := job.SetupTestDB(t)
	now := time.Unix(1_700_000_000, 0)

	sig, err := Compute(context.Background(), db, now)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if sig.OldestTodo.Present {
		t.Errorf("Present: got true, want false")
	}
}

func TestOldestTodo_PicksOldestUnclaimedUnblocked(t *testing.T) {
	db := job.SetupTestDB(t)
	base := time.Unix(1_700_000_000, 0)

	// Oldest available — should be picked.
	oldestID := seedTask(t, db, "old", "oldest available", "available", base.Add(-3*24*time.Hour))
	_ = oldestID

	// Newer available — should not be picked.
	seedTask(t, db, "new", "newer available", "available", base.Add(-1*time.Hour))

	// Very old claimed — excluded by status.
	claimedID := seedTask(t, db, "clm", "claimed ancient", "available", base.Add(-10*24*time.Hour))
	seedClaim(t, db, claimedID, "alice", base.Add(-1*time.Hour))

	// Very old but blocked — excluded by NOT EXISTS. The blocker itself
	// is marked done so it doesn't leak in as a candidate.
	blockerID := seedTask(t, db, "blk", "blocker", "done", base.Add(-9*24*time.Hour))
	blockedID := seedTask(t, db, "bld", "blocked ancient", "available", base.Add(-9*24*time.Hour))
	seedBlock(t, db, blockedID, blockerID, base.Add(-1*time.Hour))

	// Done task — excluded by status.
	doneID := seedTask(t, db, "dn", "done long ago", "done", base.Add(-20*24*time.Hour))
	_ = doneID

	sig, err := Compute(context.Background(), db, base)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if !sig.OldestTodo.Present {
		t.Fatal("Present: got false, want true")
	}
	if sig.OldestTodo.TaskShortID != "old" {
		t.Errorf("TaskShortID: got %q, want %q", sig.OldestTodo.TaskShortID, "old")
	}
	if sig.OldestTodo.Title != "oldest available" {
		t.Errorf("Title: got %q, want %q", sig.OldestTodo.Title, "oldest available")
	}
	wantAge := int64(3 * 24 * 60 * 60)
	if sig.OldestTodo.AgeSeconds != wantAge {
		t.Errorf("AgeSeconds: got %d, want %d", sig.OldestTodo.AgeSeconds, wantAge)
	}
}

func TestOldestTodo_ProgressSaturatesAtThreshold(t *testing.T) {
	db := job.SetupTestDB(t)
	base := time.Unix(1_700_000_000, 0)

	seedTask(t, db, "anc", "ancient", "available", base.Add(-30*24*time.Hour))

	sig, err := Compute(context.Background(), db, base)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if !sig.OldestTodo.Present {
		t.Fatal("Present: got false, want true")
	}
	if sig.OldestTodo.Progress != 1.0 {
		t.Errorf("Progress: got %f, want 1.0", sig.OldestTodo.Progress)
	}
}
