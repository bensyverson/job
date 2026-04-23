package handlers

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	job "github.com/bensyverson/jobs/internal/job"
	"github.com/bensyverson/jobs/internal/web/render"
	"github.com/bensyverson/jobs/internal/web/templates"
)

// TaskPageData is the payload rendered by the task-detail template.
// Fields stay flat so templates don't have to dig through nested
// "Task.Foo.Bar" chains.
type TaskPageData struct {
	templates.Chrome
	ShortID        string
	Title          string
	Description    string
	Status         string
	Labels         []TaskLabel
	Parent         *TaskRef
	BlockedBy      []TaskRef
	Blocking       []TaskRef
	CompletionNote string
	ClaimedBy      string
	History        []TaskHistoryEntry
}

// TaskRef is a minimal reference used for parents, blockers, and the
// "blocks" list — just enough to render a pill + title.
type TaskRef struct {
	ShortID string
	Title   string
	Status  string
	URL     string
}

// TaskLabel carries the label name and the URL that scopes a plan or
// log view to just that label.
type TaskLabel struct {
	Name string
	URL  string
}

// TaskHistoryEntry is one row in the history section of the detail
// view, pre-rendered to avoid template-side conditionals.
type TaskHistoryEntry struct {
	EventType string
	Actor     string
	Verb      string
	Note      string
	RelTime   string
	ISOTime   string
	ActorURL  string
}

// Task renders /tasks/<id>. See vision §3.5. The full-page view
// mirrors the peek sheet's sections (Labels, Parent, Blocks, Notes,
// History); a non-existent id 404s rather than rendering an empty
// shell.
func Task(deps Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		shortID := r.PathValue("id")
		if shortID == "" {
			NotFound(deps).ServeHTTP(w, r)
			return
		}

		task, err := job.GetTaskByShortID(deps.DB, shortID)
		if err != nil {
			InternalError(deps, w, "task lookup", err)
			return
		}
		if task == nil {
			RenderError(deps, w, http.StatusNotFound,
				"Task not found",
				"No task exists with id "+shortID+". It may have been deleted, or the link may be mistyped.")
			return
		}

		labelNames, err := job.GetLabels(deps.DB, task.ID)
		if err != nil {
			InternalError(deps, w, "labels", err)
			return
		}
		parent, err := job.GetTaskParent(deps.DB, shortID)
		if err != nil {
			InternalError(deps, w, "parent", err)
			return
		}
		blockers, err := job.GetBlockers(deps.DB, shortID)
		if err != nil {
			InternalError(deps, w, "blockers", err)
			return
		}
		blocking, err := job.GetBlocking(deps.DB, shortID)
		if err != nil {
			InternalError(deps, w, "blocking", err)
			return
		}
		events, err := job.GetEventsForTask(deps.DB, shortID)
		if err != nil {
			InternalError(deps, w, "events", err)
			return
		}

		// One batched lookup so the Blocked-by / Blocks / Parent rows
		// all get a blocker-aware status without an N+1 per related
		// task.
		relIDs := make([]int64, 0, len(blockers)+len(blocking)+1)
		for _, t := range blockers {
			relIDs = append(relIDs, t.ID)
		}
		for _, t := range blocking {
			relIDs = append(relIDs, t.ID)
		}
		if parent != nil {
			relIDs = append(relIDs, parent.ID)
		}
		relBlockers, err := job.GetBlockersForTaskIDs(deps.DB, relIDs)
		if err != nil {
			InternalError(deps, w, "related blockers", err)
			return
		}

		data := TaskPageData{
			Chrome:         templates.Chrome{ActiveTab: ""},
			ShortID:        task.ShortID,
			Title:          task.Title,
			Description:    task.Description,
			Status:         DisplayStatus(task.Status, len(blockers) > 0),
			Labels:         buildTaskLabels(labelNames),
			Parent:         taskRefOrNil(parent, relBlockers),
			BlockedBy:      taskRefs(blockers, relBlockers),
			Blocking:       taskRefs(blocking, relBlockers),
			CompletionNote: derefString(task.CompletionNote),
			ClaimedBy:      derefString(task.ClaimedBy),
			History:        buildHistory(events),
		}
		renderPage(deps, w, "task", data)
	})
}

func buildTaskLabels(names []string) []TaskLabel {
	out := make([]TaskLabel, len(names))
	for i, n := range names {
		out[i] = TaskLabel{Name: n, URL: "/log?label=" + url.QueryEscape(n)}
	}
	return out
}

func taskRefOrNil(t *job.Task, blockerMap map[int64][]string) *TaskRef {
	if t == nil {
		return nil
	}
	r := toTaskRef(t, blockerMap)
	return &r
}

func taskRefs(ts []*job.Task, blockerMap map[int64][]string) []TaskRef {
	out := make([]TaskRef, len(ts))
	for i, t := range ts {
		out[i] = toTaskRef(t, blockerMap)
	}
	return out
}

func toTaskRef(t *job.Task, blockerMap map[int64][]string) TaskRef {
	hasBlockers := len(blockerMap[t.ID]) > 0
	return TaskRef{
		ShortID: t.ShortID,
		Title:   t.Title,
		Status:  DisplayStatus(t.Status, hasBlockers),
		URL:     "/tasks/" + t.ShortID,
	}
}

func buildHistory(events []job.EventEntry) []TaskHistoryEntry {
	now := time.Now()
	out := make([]TaskHistoryEntry, len(events))
	for i, e := range events {
		ts := time.Unix(e.CreatedAt, 0)
		out[i] = TaskHistoryEntry{
			EventType: e.EventType,
			Actor:     e.Actor,
			Verb:      eventVerb(e.EventType),
			Note:      extractNoteFromDetail(e.EventType, e.Detail),
			RelTime:   render.RelativeTime(now, ts),
			ISOTime:   ts.UTC().Format(time.RFC3339),
			ActorURL:  "/actors/" + url.PathEscape(e.Actor),
		}
	}
	return out
}

// eventVerb returns the display verb for an event type. Matches the
// prototype's copy ("claimed by", "done by", "unblocked").
func eventVerb(t string) string {
	switch t {
	case "created":
		return "added by"
	case "claimed":
		return "claimed by"
	case "done":
		return "done by"
	case "canceled":
		return "canceled by"
	case "released":
		return "released by"
	case "noted":
		return "noted by"
	case "blocked":
		return "blocked"
	case "unblocked":
		return "unblocked"
	default:
		return t + " by"
	}
}

// extractNoteFromDetail pulls the free-text body out of the JSON
// detail payload for event types that carry one. Mirrors the log
// view's extractor; lives here so the detail page can show notes
// that the log row collapses away (e.g. "blocked" edge payloads).
// DisplayStatus maps the DB's raw status column into the four visual
// status categories used by the dashboard's c-status-pill ("todo",
// "active", "blocked", "done"). An open task that has any unresolved
// blocker renders as "blocked" even though the column says "available"
// or "claimed" — the blocker relation is derived-state, not stored on
// the task row. Kept exported so handlers and future render helpers
// can share one normalization rule.
func DisplayStatus(raw string, hasOpenBlockers bool) string {
	switch raw {
	case "done":
		return "done"
	case "canceled":
		return "canceled"
	case "claimed":
		if hasOpenBlockers {
			return "blocked"
		}
		return "active"
	case "available":
		if hasOpenBlockers {
			return "blocked"
		}
		return "todo"
	default:
		return raw
	}
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func extractNoteFromDetail(eventType, detail string) string {
	if detail == "" {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(detail), &payload); err != nil {
		return ""
	}
	for _, key := range []string{"note", "text", "reason"} {
		if s, ok := payload[key].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}
