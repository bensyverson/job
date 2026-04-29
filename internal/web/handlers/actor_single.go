package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"time"

	"github.com/bensyverson/jobs/internal/web/render"
	"github.com/bensyverson/jobs/internal/web/templates"
)

// ActorSinglePageData is the template payload for /actors/{name}.
type ActorSinglePageData struct {
	templates.Chrome
	Name       string
	Initial    string
	StatusText string
	Stats      ActorHeroStats
	Timeline   ActorTimeline
	Events     []LogEventRow
	BackURL    string
	// LogURL is the "View all in Log" target — /log scoped to this
	// actor. Rendered next to the Events heading regardless of cap;
	// the Log view is the place for deep history with full filters.
	LogURL string
}

// ActorHeroStats are the four tile values in the hero band.
type ActorHeroStats struct {
	InFlight int
	Done1h   int
	Done24h  int
	Blocked  int
}

// ActorTimeline is the 24-hour activity strip — five lanes (created /
// claimed / done / blocked / noted), each with marks positioned along
// the axis as a percent from "24h ago" (0%) to "now" (100%).
type ActorTimeline struct {
	TotalEvents int
	Lanes       []ActorTimelineLane
}

// ActorTimelineLane is one verb's row of marks. LaneClass is the
// per-verb modifier on the mark element (c-actor-timeline__mark--…).
type ActorTimelineLane struct {
	Verb      string
	LaneClass string
	Marks     []ActorTimelineMark
}

// ActorTimelineMark is one event positioned along the 24h axis.
type ActorTimelineMark struct {
	XPercent string // formatted "%.1f" — empty string is invalid
}

// ActorEventListLimit caps the per-actor event list to keep DOM
// bounded. The Log view paginates at 200 across all actors; the
// per-actor page is a hub, not a deep history scroll — past the cap
// the user follows the "View all in Log" link to /log?actor={name}.
const ActorEventListLimit = 100

// timelineVerbs is the canonical lane order on the 24h timeline.
// The Log filter bar uses a different order (full event vocabulary);
// this is the subset that fits on a 5-lane chart.
var timelineVerbs = []string{"created", "claimed", "done", "blocked", "noted"}

// ActorSingle renders the per-actor hero + timeline + event list at
// /actors/{name}. Unknown actors get a 404. Live updates and the
// timeline scrubber arrive in later phase tasks.
func ActorSingle(deps Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			RenderError(deps, w, http.StatusNotFound, "Actor not found",
				"That actor isn't on the board.")
			return
		}

		now := time.Now()
		exists, err := actorExists(r.Context(), deps.DB, name)
		if err != nil {
			InternalError(deps, w, "actor exists", err)
			return
		}
		if !exists {
			RenderError(deps, w, http.StatusNotFound, "Actor not found",
				fmt.Sprintf("No events recorded for %q yet.", name))
			return
		}

		stats, lastSeen, err := loadActorStats(r.Context(), deps.DB, name, now)
		if err != nil {
			InternalError(deps, w, "actor stats", err)
			return
		}
		timeline, err := loadActorTimeline(r.Context(), deps.DB, name, now)
		if err != nil {
			InternalError(deps, w, "actor timeline", err)
			return
		}
		events, err := loadActorEvents(r.Context(), deps.DB, name, now)
		if err != nil {
			InternalError(deps, w, "actor events", err)
			return
		}

		chrome, err := newChrome(r.Context(), deps, "actors", now)
		if err != nil {
			InternalError(deps, w, "actor single initial frame", err)
			return
		}
		q := url.Values{}
		q.Set("actor", name)
		data := ActorSinglePageData{
			Chrome:     chrome,
			Name:       name,
			Initial:    render.InitialOf(name),
			Stats:      stats,
			Timeline:   timeline,
			Events:     events,
			BackURL:    "/actors",
			LogURL:     "/log?" + q.Encode(),
			StatusText: actorSingleStatusText(stats.InFlight, lastSeen, now),
		}
		renderPage(deps, w, "actor_single", data)
	})
}

func actorExists(ctx context.Context, db *sql.DB, name string) (bool, error) {
	var present int
	err := db.QueryRowContext(ctx,
		`SELECT 1 FROM events WHERE actor = ? LIMIT 1`, name,
	).Scan(&present)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// loadActorStats computes the four hero tiles plus the actor's
// most-recent event time (used for the status line).
func loadActorStats(ctx context.Context, db *sql.DB, name string, now time.Time) (ActorHeroStats, int64, error) {
	var stats ActorHeroStats
	var lastSeen int64

	if err := db.QueryRowContext(ctx, `
		SELECT MAX(created_at) FROM events WHERE actor = ?
	`, name).Scan(&lastSeen); err != nil {
		return stats, 0, err
	}

	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM tasks
		WHERE claimed_by = ? AND status = 'claimed' AND deleted_at IS NULL
	`, name).Scan(&stats.InFlight); err != nil {
		return stats, 0, err
	}

	cutoff1h := now.Add(-1 * time.Hour).Unix()
	cutoff24h := now.Add(-24 * time.Hour).Unix()
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM events
		WHERE actor = ? AND event_type = 'done' AND created_at >= ?
	`, name, cutoff1h).Scan(&stats.Done1h); err != nil {
		return stats, 0, err
	}
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM events
		WHERE actor = ? AND event_type = 'done' AND created_at >= ?
	`, name, cutoff24h).Scan(&stats.Done24h); err != nil {
		return stats, 0, err
	}

	// "Blocked" tile: tasks claimed by this actor that have at least
	// one still-active blocker. Surfaces "what is this actor stuck
	// on" rather than how many `blocked` events they emitted.
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT t.id) FROM tasks t
		JOIN blocks b ON b.blocked_id = t.id
		JOIN tasks bt ON bt.id = b.blocker_id
		WHERE t.claimed_by = ?
		  AND t.status = 'claimed'
		  AND t.deleted_at IS NULL
		  AND bt.status != 'done'
		  AND bt.deleted_at IS NULL
	`, name).Scan(&stats.Blocked); err != nil {
		return stats, 0, err
	}
	return stats, lastSeen, nil
}

// loadActorTimeline buckets every event by this actor over the last
// 24 hours into the five canonical lanes. Each event becomes a single
// mark whose --x is its position from "24h ago" (0%) to "now" (100%).
func loadActorTimeline(ctx context.Context, db *sql.DB, name string, now time.Time) (ActorTimeline, error) {
	cutoff := now.Add(-24 * time.Hour).Unix()
	windowSecs := float64(24 * 60 * 60)

	rows, err := db.QueryContext(ctx, `
		SELECT event_type, created_at FROM events
		WHERE actor = ? AND created_at >= ?
		ORDER BY created_at ASC, id ASC
	`, name, cutoff)
	if err != nil {
		return ActorTimeline{}, err
	}
	defer rows.Close()

	byVerb := make(map[string][]ActorTimelineMark, len(timelineVerbs))
	total := 0
	for rows.Next() {
		var verb string
		var at int64
		if err := rows.Scan(&verb, &at); err != nil {
			return ActorTimeline{}, err
		}
		total++
		// Only the lanes the timeline actually renders pick up marks.
		// Other event types (released, canceled, claim_expired) still
		// count toward TotalEvents.
		if !isTimelineVerb(verb) {
			continue
		}
		offset := float64(at-cutoff) / windowSecs * 100.0
		if offset < 0 {
			offset = 0
		} else if offset > 100 {
			offset = 100
		}
		byVerb[verb] = append(byVerb[verb], ActorTimelineMark{
			XPercent: fmt.Sprintf("%.1f", offset),
		})
	}
	if err := rows.Err(); err != nil {
		return ActorTimeline{}, err
	}

	lanes := make([]ActorTimelineLane, 0, len(timelineVerbs))
	for _, v := range timelineVerbs {
		lanes = append(lanes, ActorTimelineLane{
			Verb:      v,
			LaneClass: "c-actor-timeline__mark--" + v,
			Marks:     byVerb[v],
		})
	}
	return ActorTimeline{TotalEvents: total, Lanes: lanes}, nil
}

func isTimelineVerb(v string) bool {
	return slices.Contains(timelineVerbs, v)
}

// loadActorEvents renders this actor's most recent events through the
// LogEventRow row component so the actor page reuses the Log view's
// row layout. Capped at ActorEventListLimit — pagination follows in
// a later phase task if needed.
func loadActorEvents(ctx context.Context, db *sql.DB, name string, now time.Time) ([]LogEventRow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT e.id, e.event_type, e.created_at, e.detail, t.short_id, t.title
		FROM events e
		JOIN tasks t ON t.id = e.task_id
		WHERE e.actor = ? AND t.deleted_at IS NULL
		ORDER BY e.created_at DESC, e.id DESC
		LIMIT ?
	`, name, ActorEventListLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LogEventRow
	for rows.Next() {
		var id, createdAt int64
		var verb, detail, shortID, title string
		if err := rows.Scan(&id, &verb, &createdAt, &detail, &shortID, &title); err != nil {
			return nil, err
		}
		ts := time.Unix(createdAt, 0)
		row := LogEventRow{
			EventID:   id,
			ShortID:   shortID,
			Actor:     name,
			EventType: verb,
			VerbText:  verb,
			Title:     title,
			RelTime:   render.RelativeTime(now, ts),
			ISOTime:   ts.UTC().Format(time.RFC3339),
			TaskURL:   "/tasks/" + shortID,
			ActorURL:  "/actors/" + name,
		}
		if verb == "claim_expired" {
			row.VerbText = "expired"
			row.Actor = "Jobs"
			row.ActorURL = ""
			row.IsSystem = true
		}
		// Mirror log.go's folded-detail verb mapping so criteria
		// activity reads as prose on the actor page too.
		switch verb {
		case "criteria_added":
			row.VerbText = criteriaAddedVerb(detail)
		case "criterion_state":
			row.VerbText = criterionStateVerb(detail)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Defensive sort: query is already ordered, but keep deterministic
	// behavior in case the DB driver re-orders.
	sort.SliceStable(out, func(i, j int) bool { return out[i].EventID > out[j].EventID })
	return out, nil
}

func actorSingleStatusText(inFlight int, lastSeen int64, now time.Time) string {
	last := render.RelativeTime(now, time.Unix(lastSeen, 0))
	switch {
	case inFlight == 0:
		return "idle · last seen " + last
	case inFlight == 1:
		return "1 claim · last seen " + last
	default:
		return fmt.Sprintf("%d claims · last seen %s", inFlight, last)
	}
}
