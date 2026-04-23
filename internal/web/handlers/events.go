package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	job "github.com/bensyverson/jobs/internal/job"
)

// EventsFilter is the subset of LogFilters the SSE endpoint supports.
// Uses the same semantics (empty string = no filter) so a client can
// reuse a /log query string on /events.
type EventsFilter struct {
	Actor string
	Task  string
	Label string
	Type  string
}

// Matches reports whether an event satisfies every non-empty filter.
// Used by both the SSE and JSON code paths so filter semantics are
// identical across them. Label matching requires the taskLabels map
// passed in; empty map means no label filter can match.
func (f EventsFilter) Matches(e job.EventEntry, taskLabels map[int64][]string) bool {
	if f.Actor != "" && e.Actor != f.Actor {
		return false
	}
	if f.Type != "" && e.EventType != f.Type {
		return false
	}
	if f.Task != "" && e.ShortID != f.Task {
		return false
	}
	if f.Label != "" {
		found := slices.Contains(taskLabels[e.TaskID], f.Label)
		if !found {
			return false
		}
	}
	return true
}

// Events serves /events: SSE live tail when the client sends
// Accept: text/event-stream, JSON replay otherwise. See vision §6.2
// and §6.3.
func Events(deps Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		filter := EventsFilter{
			Actor: q.Get("actor"),
			Task:  q.Get("task"),
			Label: q.Get("label"),
			Type:  q.Get("type"),
		}

		var sinceID int64
		if raw := q.Get("since"); raw != "" {
			if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
				sinceID = n
			}
		}

		// Limit caps how many backfill events we return; default 500
		// keeps a cold-load under the 200ms cold-load budget from
		// vision §6.5. Zero or negative falls back to default.
		limit := 500
		if raw := q.Get("limit"); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}

		accept := r.Header.Get("Accept")
		wantsSSE := strings.Contains(accept, "text/event-stream")

		if wantsSSE {
			if deps.Broadcaster == nil {
				http.Error(w, "live stream unavailable (broadcaster not configured)", http.StatusServiceUnavailable)
				return
			}
			serveSSE(deps, w, r, filter, sinceID)
			return
		}
		serveJSON(deps, w, r, filter, sinceID, limit)
	})
}

// serveJSON is the replay-mode response: one JSON array of events
// (filtered + limited) with no live tail.
func serveJSON(deps Deps, w http.ResponseWriter, r *http.Request, filter EventsFilter, sinceID int64, limit int) {
	events, err := job.GetEventsAfterID(deps.DB, "", sinceID)
	if err != nil {
		InternalError(deps, w, "events query", err)
		return
	}
	labels, err := labelMapFor(deps, events)
	if err != nil {
		InternalError(deps, w, "event labels", err)
		return
	}

	filtered := make([]job.EventEntry, 0, len(events))
	for _, e := range events {
		if !filter.Matches(e, labels) {
			continue
		}
		filtered = append(filtered, e)
		if len(filtered) >= limit {
			break
		}
	}

	titles, err := titleMapFor(deps, filtered)
	if err != nil {
		InternalError(deps, w, "event titles", err)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	payload := make([]eventJSON, len(filtered))
	for i, e := range filtered {
		payload[i] = toEventJSON(e, titles[e.TaskID])
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		// Can't rewrite headers at this point; server will close the
		// connection on return and the client will see truncation.
		return
	}
}

// serveSSE splices a backfill (events with id in (sinceID, cutoff])
// into a live tail from the broadcaster (events with id > cutoff),
// dropping any overlap on the live channel. Returns when the client
// disconnects or the broadcaster closes the channel.
func serveSSE(deps Deps, w http.ResponseWriter, r *http.Request, filter EventsFilter, sinceID int64) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		InternalError(deps, w, "SSE", fmt.Errorf("ResponseWriter does not support flushing"))
		return
	}

	// Subscribe first so any event arriving during backfill also lands
	// in the channel (we'll dedup against the subscription's LastID).
	sub := deps.Broadcaster.Subscribe()
	defer sub.Cancel()
	cutoff := sub.LastID

	// Backfill: events in (sinceID, cutoff], filtered.
	backfill, err := job.GetEventsAfterID(deps.DB, "", sinceID)
	if err != nil {
		InternalError(deps, w, "events backfill", err)
		return
	}
	// Trim to cutoff — events above cutoff come through the live
	// channel, so including them here would duplicate.
	i := len(backfill)
	for j, e := range backfill {
		if e.ID > cutoff {
			i = j
			break
		}
	}
	backfill = backfill[:i]
	labels, err := labelMapFor(deps, backfill)
	if err != nil {
		InternalError(deps, w, "backfill labels", err)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering
	w.WriteHeader(http.StatusOK)
	// Flush headers immediately so a client calling http.Client.Do
	// returns without waiting for the first event frame.
	flusher.Flush()

	titles, err := titleMapFor(deps, backfill)
	if err != nil {
		InternalError(deps, w, "backfill titles", err)
		return
	}
	for _, e := range backfill {
		if !filter.Matches(e, labels) {
			continue
		}
		if !writeSSEFrame(w, e, titles[e.TaskID]) {
			return
		}
		flusher.Flush()
	}

	// Live tail. Labels for arbitrary new events need per-arrival
	// lookup since the TaskID might not be in the backfill label map;
	// cheap because a single event's labels is one query.
	liveCtx := r.Context()
	for {
		select {
		case <-liveCtx.Done():
			return
		case e, ok := <-sub.Events:
			if !ok {
				// Broadcaster shutdown.
				return
			}
			if e.ID <= cutoff {
				// Already delivered via backfill.
				continue
			}
			if filter.Label != "" {
				// Single-event label lookup on demand. A subscriber
				// with no label filter skips this path entirely.
				m, lerr := labelMapFor(deps, []job.EventEntry{e})
				if lerr != nil || !filter.Matches(e, m) {
					continue
				}
			} else if !filter.Matches(e, nil) {
				continue
			}
			title := titleForTask(deps, e.TaskID)
			if !writeSSEFrame(w, e, title) {
				return
			}
			flusher.Flush()
		}
	}
}

// titleMapFor returns a map of task_id → title covering every task
// referenced by events. Batched so the SSE backfill and JSON replay
// don't pay per-event DB lookups.
func titleMapFor(deps Deps, events []job.EventEntry) (map[int64]string, error) {
	if len(events) == 0 {
		return map[int64]string{}, nil
	}
	seen := make(map[int64]bool, len(events))
	ids := make([]int64, 0, len(events))
	for _, e := range events {
		if seen[e.TaskID] {
			continue
		}
		seen[e.TaskID] = true
		ids = append(ids, e.TaskID)
	}
	return job.TaskTitlesByID(deps.DB, ids)
}

// titleForTask is the single-event lookup used by the SSE live loop.
// Cheap at 1 Hz poll cadence; swap for a per-subscription cache if the
// event rate climbs.
func titleForTask(deps Deps, taskID int64) string {
	titles, err := job.TaskTitlesByID(deps.DB, []int64{taskID})
	if err != nil {
		return ""
	}
	return titles[taskID]
}

// labelMapFor returns a map of task_id → []labels covering every
// task referenced by events. Empty input yields an empty map.
func labelMapFor(deps Deps, events []job.EventEntry) (map[int64][]string, error) {
	if len(events) == 0 {
		return map[int64][]string{}, nil
	}
	seen := make(map[int64]bool, len(events))
	ids := make([]int64, 0, len(events))
	for _, e := range events {
		if seen[e.TaskID] {
			continue
		}
		seen[e.TaskID] = true
		ids = append(ids, e.TaskID)
	}
	return job.GetLabelsForTaskIDs(deps.DB, ids)
}

// eventJSON is the wire shape of an event. Separate from job.EventEntry
// so the JSON field names are a public API we control. TaskTitle is
// included so the dashboard can render a log row without a second
// lookup — important for live SSE frames where the client has no
// batch title cache.
type eventJSON struct {
	ID        int64  `json:"id"`
	TaskID    string `json:"task_id"`
	TaskTitle string `json:"task_title,omitempty"`
	EventType string `json:"event_type"`
	Actor     string `json:"actor"`
	Detail    string `json:"detail"`
	CreatedAt string `json:"created_at"`
}

func toEventJSON(e job.EventEntry, title string) eventJSON {
	return eventJSON{
		ID:        e.ID,
		TaskID:    e.ShortID,
		TaskTitle: title,
		EventType: e.EventType,
		Actor:     e.Actor,
		Detail:    e.Detail,
		CreatedAt: time.Unix(e.CreatedAt, 0).UTC().Format(time.RFC3339),
	}
}

// writeSSEFrame writes one SSE frame for event e. Returns false on
// write error (client disconnected), true on success. Format:
//
//	id: <event_id>
//	event: <event_type>
//	data: <json>
//
//	(blank line terminates the frame)
func writeSSEFrame(w http.ResponseWriter, e job.EventEntry, title string) bool {
	payload, err := json.Marshal(toEventJSON(e, title))
	if err != nil {
		return false
	}
	_, err = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", e.ID, e.EventType, payload)
	return err == nil
}
