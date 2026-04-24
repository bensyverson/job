// Package signals computes the four Home-view signal cards from the
// live event/task state in SQLite. Activity histogram + three
// alarm-style signals (newly blocked, longest active claim, oldest
// todo), each carrying a 0..1 progress value the template uses to
// render the ambient underline.
package signals

import (
	"context"
	"database/sql"
	"time"
)

// Thresholds beyond which a signal saturates its progress underline.
// Exposed as package-level constants so templates can label them
// ("at threshold", "over threshold") without duplicating magic numbers.
const (
	// ActivityWindow is the span the histogram covers.
	ActivityWindow = 60 * time.Minute
	// ActivityBuckets is the number of per-minute bars in the histogram.
	ActivityBuckets = 60

	// NewlyBlockedWindow is the recency window for the "newly blocked"
	// card. Block edges created within this window are counted.
	NewlyBlockedWindow = 10 * time.Minute
	// NewlyBlockedThreshold is the count at which the progress bar
	// saturates.
	NewlyBlockedThreshold = 5
	// NewlyBlockedItemLimit caps how many recent edges are returned in
	// the context line.
	NewlyBlockedItemLimit = 5

	// LongestClaimThreshold is the duration at which the claim card's
	// progress saturates. Tied to the default claim TTL — a claim that
	// has outlived a full TTL window is conspicuous.
	LongestClaimThreshold = 30 * time.Minute

	// OldestTodoThreshold is the age at which the oldest-todo card's
	// progress saturates. A week-old unclaimed task is stale enough to
	// earn the full warning.
	OldestTodoThreshold = 7 * 24 * time.Hour
)

// Signals is the snapshot computed for a single Home-view render.
type Signals struct {
	Activity     Activity
	NewlyBlocked NewlyBlocked
	LongestClaim LongestClaim
	OldestTodo   OldestTodo
}

// Activity is the 60-minute event histogram. Buckets[0] is the oldest
// minute (59m..60m ago), Buckets[59] is the most recent (0m..1m ago).
// Per-type totals cover the whole window.
type Activity struct {
	Buckets     [ActivityBuckets]ActivityBucket
	TotalDone   int
	TotalClaim  int
	TotalCreate int
	TotalBlock  int
}

// TotalEvents is the sum across all event types in the window.
func (a Activity) TotalEvents() int {
	return a.TotalDone + a.TotalClaim + a.TotalCreate + a.TotalBlock
}

// ActivityBucket holds the per-type event counts for a single minute.
type ActivityBucket struct {
	Done   int
	Claim  int
	Create int
	Block  int
}

// Total is the bucket's stacked-bar height.
func (b ActivityBucket) Total() int {
	return b.Done + b.Claim + b.Create + b.Block
}

// NewlyBlocked summarizes block edges created in the recency window.
type NewlyBlocked struct {
	Count     int
	Items     []BlockRef // newest first; capped at NewlyBlockedItemLimit
	Threshold int
	Progress  float64
}

// BlockRef is the (blocked → waiting-on) pair for one block edge.
type BlockRef struct {
	BlockedShortID   string
	WaitingOnShortID string
}

// LongestClaim is the currently-held claim with the earliest start.
// Absent when no tasks are claimed.
type LongestClaim struct {
	Present          bool
	Actor            string
	TaskShortID      string
	TaskTitle        string
	DurationSeconds  int64
	ThresholdSeconds int64
	Progress         float64
}

// OldestTodo is the oldest available, unblocked, non-deleted task.
// Absent when no such task exists.
type OldestTodo struct {
	Present          bool
	TaskShortID      string
	Title            string
	AgeSeconds       int64
	ThresholdSeconds int64
	Progress         float64
}

// Compute returns a snapshot of all four signals at the given instant.
// Each sub-query is independent; failures short-circuit with the error
// so the caller can render an error state rather than a partial card set.
func Compute(ctx context.Context, db *sql.DB, now time.Time) (Signals, error) {
	var out Signals

	act, err := computeActivity(ctx, db, now)
	if err != nil {
		return out, err
	}
	out.Activity = act

	nb, err := computeNewlyBlocked(ctx, db, now)
	if err != nil {
		return out, err
	}
	out.NewlyBlocked = nb

	lc, err := computeLongestClaim(ctx, db, now)
	if err != nil {
		return out, err
	}
	out.LongestClaim = lc

	ot, err := computeOldestTodo(ctx, db, now)
	if err != nil {
		return out, err
	}
	out.OldestTodo = ot

	return out, nil
}

// saturate clamps progress into [0, 1].
func saturate(x float64) float64 {
	switch {
	case x < 0:
		return 0
	case x > 1:
		return 1
	default:
		return x
	}
}

// computeActivity builds the per-minute stacked histogram for the
// last ActivityWindow. Bucket[59] is the most recent minute; bucket[0]
// is the oldest. Events older than the window are ignored; unknown
// event types (noted, labeled, heartbeat, canceled, …) are excluded.
func computeActivity(ctx context.Context, db *sql.DB, now time.Time) (Activity, error) {
	var a Activity
	nowUnix := now.Unix()
	cutoff := nowUnix - int64(ActivityWindow/time.Second)

	rows, err := db.QueryContext(ctx, `
		SELECT event_type, created_at
		FROM events
		WHERE created_at > ?
		  AND created_at <= ?
		  AND event_type IN ('done', 'claimed', 'created', 'blocked')
	`, cutoff, nowUnix)
	if err != nil {
		return a, err
	}
	defer rows.Close()

	for rows.Next() {
		var etype string
		var createdAt int64
		if err := rows.Scan(&etype, &createdAt); err != nil {
			return a, err
		}
		minutesAgo := int((nowUnix - createdAt) / 60)
		if minutesAgo < 0 || minutesAgo >= ActivityBuckets {
			continue
		}
		idx := ActivityBuckets - 1 - minutesAgo
		switch etype {
		case "done":
			a.Buckets[idx].Done++
			a.TotalDone++
		case "claimed":
			a.Buckets[idx].Claim++
			a.TotalClaim++
		case "created":
			a.Buckets[idx].Create++
			a.TotalCreate++
		case "blocked":
			a.Buckets[idx].Block++
			a.TotalBlock++
		}
	}
	return a, rows.Err()
}

// computeNewlyBlocked counts block edges created in the last
// NewlyBlockedWindow. The context line shows the most recent few.
func computeNewlyBlocked(ctx context.Context, db *sql.DB, now time.Time) (NewlyBlocked, error) {
	nb := NewlyBlocked{Threshold: NewlyBlockedThreshold}
	cutoff := now.Unix() - int64(NewlyBlockedWindow/time.Second)

	rows, err := db.QueryContext(ctx, `
		SELECT tb.short_id, tk.short_id
		FROM blocks b
		JOIN tasks tb ON tb.id = b.blocked_id
		JOIN tasks tk ON tk.id = b.blocker_id
		WHERE b.created_at > ?
		  AND b.created_at <= ?
		ORDER BY b.created_at DESC, b.blocked_id DESC, b.blocker_id DESC
	`, cutoff, now.Unix())
	if err != nil {
		return nb, err
	}
	defer rows.Close()

	for rows.Next() {
		var ref BlockRef
		if err := rows.Scan(&ref.BlockedShortID, &ref.WaitingOnShortID); err != nil {
			return nb, err
		}
		nb.Count++
		if len(nb.Items) < NewlyBlockedItemLimit {
			nb.Items = append(nb.Items, ref)
		}
	}
	if err := rows.Err(); err != nil {
		return nb, err
	}
	nb.Progress = saturate(float64(nb.Count) / float64(NewlyBlockedThreshold))
	return nb, nil
}

// computeLongestClaim returns the active claim whose most recent
// 'claimed' event is furthest in the past. Heartbeat-driven TTL
// extensions don't move the claim start, so this tracks the true
// "how long has this actor been on this task" duration.
func computeLongestClaim(ctx context.Context, db *sql.DB, now time.Time) (LongestClaim, error) {
	lc := LongestClaim{ThresholdSeconds: int64(LongestClaimThreshold / time.Second)}

	// For every active claim, find the most recent 'claimed' event for
	// the current holder. The row with the smallest such timestamp is
	// the longest-running claim.
	row := db.QueryRowContext(ctx, `
		SELECT t.short_id, t.title, t.claimed_by, MAX(e.created_at) AS claimed_at
		FROM tasks t
		JOIN events e ON e.task_id = t.id
		WHERE t.status = 'claimed'
		  AND t.deleted_at IS NULL
		  AND t.claimed_by IS NOT NULL
		  AND e.event_type = 'claimed'
		  AND e.actor = t.claimed_by
		GROUP BY t.id
		ORDER BY claimed_at ASC, t.id ASC
		LIMIT 1
	`)
	var shortID, title, actor string
	var claimedAt int64
	if err := row.Scan(&shortID, &title, &actor, &claimedAt); err != nil {
		if err == sql.ErrNoRows {
			return lc, nil
		}
		return lc, err
	}
	lc.Present = true
	lc.TaskShortID = shortID
	lc.TaskTitle = title
	lc.Actor = actor
	lc.DurationSeconds = max(now.Unix()-claimedAt, 0)
	lc.Progress = saturate(float64(lc.DurationSeconds) / float64(lc.ThresholdSeconds))
	return lc, nil
}

// computeOldestTodo returns the oldest available task that is not
// blocked and not deleted. Claimed and done/canceled tasks are
// excluded by the status filter; blocked tasks are excluded via
// NOT EXISTS against the blocks edge.
func computeOldestTodo(ctx context.Context, db *sql.DB, now time.Time) (OldestTodo, error) {
	ot := OldestTodo{ThresholdSeconds: int64(OldestTodoThreshold / time.Second)}

	row := db.QueryRowContext(ctx, `
		SELECT t.short_id, t.title, t.created_at
		FROM tasks t
		WHERE t.status = 'available'
		  AND t.deleted_at IS NULL
		  AND NOT EXISTS (
		    SELECT 1 FROM blocks b WHERE b.blocked_id = t.id
		  )
		ORDER BY t.created_at ASC, t.id ASC
		LIMIT 1
	`)
	var shortID, title string
	var createdAt int64
	if err := row.Scan(&shortID, &title, &createdAt); err != nil {
		if err == sql.ErrNoRows {
			return ot, nil
		}
		return ot, err
	}
	ot.Present = true
	ot.TaskShortID = shortID
	ot.Title = title
	ot.AgeSeconds = max(now.Unix()-createdAt, 0)
	ot.Progress = saturate(float64(ot.AgeSeconds) / float64(ot.ThresholdSeconds))
	return ot, nil
}
