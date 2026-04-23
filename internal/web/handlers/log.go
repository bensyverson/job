package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	job "github.com/bensyverson/jobs/internal/job"
	"github.com/bensyverson/jobs/internal/web/render"
	"github.com/bensyverson/jobs/internal/web/templates"
)

// LogFilters are the query-param-driven filters on /log. Zero-value
// means "no filter on that axis." Vision §6.4 defines the axes;
// multi-valued filters (repeated `&label=` etc.) are a later concern.
type LogFilters struct {
	Actor string
	Task  string
	Label string
	Type  string
	Since time.Time // zero means "no since floor"
}

// LogChip is one clickable chip in the log-view filter bar. HRef is a
// fully-formed query string so the template can emit <a href=…>.
type LogChip struct {
	Label  string
	HRef   string
	Active bool
	Actor  string // non-empty for actor chips — paints the avatar dot
	LabelK string // non-empty for label chips — paints the pill tint
}

// LogEventRow is one already-rendered row in the log view. Building
// this once on the server side keeps the template simple and pushes
// id-formatting / time-formatting into Go where it belongs.
type LogEventRow struct {
	// EventID is the event's numeric id. Rendered on the row as
	// data-event-id so the live-tail script can dedup incoming SSE
	// frames against the server-rendered set (prevents double-render
	// when the backfill window overlaps SSR output).
	EventID   int64
	ShortID   string
	Actor     string
	EventType string
	Title     string
	Note      string
	RelTime   string
	ISOTime   string
	TaskURL   string
	ActorURL  string
}

// LogPageData is the full payload the log template renders.
type LogPageData struct {
	templates.Chrome
	Filters     LogFilters
	Events      []LogEventRow
	EventTypes  []LogChip
	Actors      []LogChip
	Labels      []LogChip
	TotalShown  int
	TotalEvents int
	// EventsURL is the SSE subscription URL — /events plus the same
	// filter query params as the page itself, so the live tail only
	// delivers events that match the current filter state.
	EventsURL string
}

// knownEventTypes is the canonical ordered set of event types surfaced
// in the filter bar. Order matches the prototype so users see the
// same layout regardless of which events are present in the DB.
var knownEventTypes = []string{
	"created", "claimed", "done", "blocked", "unblocked",
	"noted", "released", "canceled",
}

// Log renders the filterable event stream view. See vision §3.4.
// SSR-only for now; the live-tail swap lands in a later phase.
func Log(deps Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		filters := ParseLogFilters(r.URL.Query())

		events, totalEvents, err := loadLogEvents(deps.DB, filters)
		if err != nil {
			InternalError(deps, w, "log query", err)
			return
		}

		actors, err := job.DistinctActors(deps.DB)
		if err != nil {
			InternalError(deps, w, "actors query", err)
			return
		}
		labels, err := job.DistinctLabels(deps.DB)
		if err != nil {
			InternalError(deps, w, "labels query", err)
			return
		}

		data := LogPageData{
			Chrome:      templates.Chrome{ActiveTab: "log"},
			Filters:     filters,
			Events:      events,
			EventTypes:  buildTypeChips(filters),
			Actors:      buildActorChips(filters, actors),
			Labels:      buildLabelChips(filters, labels),
			TotalShown:  len(events),
			TotalEvents: totalEvents,
			EventsURL:   eventsURL(filters),
		}
		renderPage(deps, w, "log", data)
	})
}

// ParseLogFilters reads a /log query string into a LogFilters value.
// Unknown keys are ignored; "since" accepts RFC3339 first, then a
// fallback of a unix-seconds integer. Malformed since values are
// silently dropped — we'd rather render with a zero since than return
// a 400 for a bookmarked URL that drifted.
func ParseLogFilters(q url.Values) LogFilters {
	f := LogFilters{
		Actor: q.Get("actor"),
		Task:  q.Get("task"),
		Label: q.Get("label"),
		Type:  q.Get("type"),
	}
	if raw := q.Get("since"); raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			f.Since = t
		} else if sec, err := strconv.ParseInt(raw, 10, 64); err == nil {
			f.Since = time.Unix(sec, 0)
		}
	}
	return f
}

// loadLogEvents fetches events scoped to the task filter (or globally
// if unset), then applies actor/type/label/since filters in memory.
// v1 accepts the simplicity tax; a real SQL push-down comes when the
// event table grows beyond "fits in RAM."
func loadLogEvents(db *sql.DB, f LogFilters) (rows []LogEventRow, total int, err error) {
	raw, err := job.GetEventsForTaskTree(db, f.Task)
	if err != nil {
		return nil, 0, err
	}
	total = len(raw)

	// Label filter: resolve to a task-ID set once.
	var labelTaskIDs map[int64]bool
	if f.Label != "" {
		ids, err := taskIDsWithLabel(db, f.Label)
		if err != nil {
			return nil, 0, err
		}
		labelTaskIDs = make(map[int64]bool, len(ids))
		for _, id := range ids {
			labelTaskIDs[id] = true
		}
	}

	filtered := make([]job.EventEntry, 0, len(raw))
	for _, e := range raw {
		if f.Actor != "" && e.Actor != f.Actor {
			continue
		}
		if f.Type != "" && e.EventType != f.Type {
			continue
		}
		if !f.Since.IsZero() && time.Unix(e.CreatedAt, 0).Before(f.Since) {
			continue
		}
		if labelTaskIDs != nil && !labelTaskIDs[e.TaskID] {
			continue
		}
		filtered = append(filtered, e)
	}

	// Titles: one batched lookup for the tasks still in view.
	ids := make([]int64, 0, len(filtered))
	seen := make(map[int64]bool, len(filtered))
	for _, e := range filtered {
		if seen[e.TaskID] {
			continue
		}
		seen[e.TaskID] = true
		ids = append(ids, e.TaskID)
	}
	titles, err := job.TaskTitlesByID(db, ids)
	if err != nil {
		return nil, 0, err
	}

	// Reverse-chrono (newest first) for display; DB returns ascending
	// by created_at.
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt > filtered[j].CreatedAt
	})

	now := time.Now()
	rows = make([]LogEventRow, len(filtered))
	for i, e := range filtered {
		ts := time.Unix(e.CreatedAt, 0)
		rows[i] = LogEventRow{
			EventID:   e.ID,
			ShortID:   e.ShortID,
			Actor:     e.Actor,
			EventType: e.EventType,
			Title:     titles[e.TaskID],
			Note:      notePreviewFromDetail(e.EventType, e.Detail),
			RelTime:   render.RelativeTime(now, ts),
			ISOTime:   ts.UTC().Format(time.RFC3339),
			TaskURL:   "/tasks/" + e.ShortID,
			ActorURL:  "/actors/" + url.PathEscape(e.Actor),
		}
	}
	return rows, total, nil
}

// notePreviewFromDetail extracts the free-text body from the JSON
// detail payload emitted by done/canceled/noted events. Other event
// types carry structural metadata (blocker IDs, move targets) that
// would add noise more than signal in a log row, so they're skipped.
func notePreviewFromDetail(eventType, detail string) string {
	if detail == "" {
		return ""
	}
	var field string
	switch eventType {
	case "done", "canceled":
		field = "note"
	case "noted":
		field = "text"
	default:
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(detail), &payload); err != nil {
		return ""
	}
	s, _ := payload[field].(string)
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if len(s) > 160 {
		return s[:160] + "…"
	}
	return s
}

func taskIDsWithLabel(db *sql.DB, label string) ([]int64, error) {
	rows, err := db.Query(`SELECT task_id FROM task_labels WHERE name = ?`, label)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func buildTypeChips(f LogFilters) []LogChip {
	chips := []LogChip{
		{Label: "all", HRef: logURL(f, "type", ""), Active: f.Type == ""},
	}
	for _, t := range knownEventTypes {
		chips = append(chips, LogChip{
			Label:  t,
			HRef:   logURL(f, "type", t),
			Active: f.Type == t,
		})
	}
	return chips
}

func buildActorChips(f LogFilters, actors []string) []LogChip {
	chips := []LogChip{
		{Label: "any", HRef: logURL(f, "actor", ""), Active: f.Actor == ""},
	}
	for _, a := range actors {
		chips = append(chips, LogChip{
			Label:  a,
			HRef:   logURL(f, "actor", a),
			Active: f.Actor == a,
			Actor:  a,
		})
	}
	return chips
}

func buildLabelChips(f LogFilters, labels []string) []LogChip {
	chips := []LogChip{
		{Label: "any", HRef: logURL(f, "label", ""), Active: f.Label == ""},
	}
	for _, l := range labels {
		chips = append(chips, LogChip{
			Label:  l,
			HRef:   logURL(f, "label", l),
			Active: f.Label == l,
			LabelK: l,
		})
	}
	return chips
}

// eventsURL builds /events?… reflecting the same filter state as the
// page, so the SSE live-tail only emits events that match.
func eventsURL(f LogFilters) string {
	q := url.Values{}
	if f.Actor != "" {
		q.Set("actor", f.Actor)
	}
	if f.Task != "" {
		q.Set("task", f.Task)
	}
	if f.Label != "" {
		q.Set("label", f.Label)
	}
	if f.Type != "" {
		q.Set("type", f.Type)
	}
	if !f.Since.IsZero() {
		q.Set("since", f.Since.UTC().Format(time.RFC3339))
	}
	if len(q) == 0 {
		return "/events"
	}
	return "/events?" + q.Encode()
}

// logURL rebuilds /log?… with one key set (or cleared if value=="")
// while preserving the rest of the filter state. Lets chips toggle
// their own axis without forgetting the others.
func logURL(f LogFilters, setKey, setValue string) string {
	q := url.Values{}
	set := func(k, v string) {
		if v != "" {
			q.Set(k, v)
		}
	}
	set("actor", f.Actor)
	set("task", f.Task)
	set("label", f.Label)
	set("type", f.Type)
	if !f.Since.IsZero() {
		q.Set("since", f.Since.UTC().Format(time.RFC3339))
	}
	if setValue == "" {
		q.Del(setKey)
	} else {
		q.Set(setKey, setValue)
	}
	if len(q) == 0 {
		return "/log"
	}
	return "/log?" + q.Encode()
}
