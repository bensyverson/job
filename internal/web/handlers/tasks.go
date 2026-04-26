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
		data, ok := loadTaskPageData(deps, w, r)
		if !ok {
			return
		}
		renderPage(deps, w, "task", data)
	})
}

// loadTaskPageData centralizes the data loading shared between the
// full-page Task view and the Peek fragment. Returns ok=false after
// writing an error response (404 or 500); callers should return
// immediately when ok is false.
func loadTaskPageData(deps Deps, w http.ResponseWriter, r *http.Request) (TaskPageData, bool) {
	shortID := r.PathValue("id")
	if shortID == "" {
		NotFound(deps).ServeHTTP(w, r)
		return TaskPageData{}, false
	}

	task, err := job.GetTaskByShortID(deps.DB, shortID)
	if err != nil {
		InternalError(deps, w, "task lookup", err)
		return TaskPageData{}, false
	}
	if task == nil {
		RenderError(deps, w, http.StatusNotFound,
			"Task not found",
			"No task exists with id "+shortID+". It may have been deleted, or the link may be mistyped.")
		return TaskPageData{}, false
	}

	labelNames, err := job.GetLabels(deps.DB, task.ID)
	if err != nil {
		InternalError(deps, w, "labels", err)
		return TaskPageData{}, false
	}
	parent, err := job.GetTaskParent(deps.DB, shortID)
	if err != nil {
		InternalError(deps, w, "parent", err)
		return TaskPageData{}, false
	}
	blockers, err := job.GetBlockers(deps.DB, shortID)
	if err != nil {
		InternalError(deps, w, "blockers", err)
		return TaskPageData{}, false
	}
	blocking, err := job.GetBlocking(deps.DB, shortID)
	if err != nil {
		InternalError(deps, w, "blocking", err)
		return TaskPageData{}, false
	}
	events, err := job.GetEventsForTask(deps.DB, shortID)
	if err != nil {
		InternalError(deps, w, "events", err)
		return TaskPageData{}, false
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
		return TaskPageData{}, false
	}

	chrome, err := newChrome(r.Context(), deps, "")
	if err != nil {
		InternalError(deps, w, "task initial frame", err)
		return TaskPageData{}, false
	}

	return TaskPageData{
		Chrome:         chrome,
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
	}, true
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
		entry := TaskHistoryEntry{
			EventType: e.EventType,
			Actor:     e.Actor,
			Verb:      eventVerb(e.EventType),
			Note:      extractNoteFromDetail(e.EventType, e.Detail),
			RelTime:   render.RelativeTime(now, ts),
			ISOTime:   ts.UTC().Format(time.RFC3339),
			ActorURL:  "/actors/" + url.PathEscape(e.Actor),
		}
		// System events have no human/agent doer. Blank the actor
		// fields so the template can skip the "<verb> by <actor>"
		// trailer and render just the verb (e.g. "claim expired").
		if e.EventType == "claim_expired" {
			entry.Actor = ""
			entry.ActorURL = ""
		}
		out[i] = entry
	}
	return out
}

// eventVerb returns the display verb for an event type. The phrase
// is followed in templates by the actor name when the event has a
// human/agent actor, so verbs typically end in "by". For system
// events (e.g. claim expiration) the verb stands alone — the
// template suppresses the actor trailer when EventEntry.Actor is
// blanked out below.
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
		return "blocked by"
	case "unblocked":
		return "unblocked by"
	case "claim_expired":
		return "claim expired"
	case "labeled":
		return "labeled by"
	default:
		return t + " by"
	}
}

// extractNoteFromDetail pulls the free-text body out of the JSON
// detail payload for event types that carry one. Only user-supplied
// fields are surfaced — the internal `reason` field (`manual`,
// `blocker_done`, `blocker_canceled`) is system categorization, not
// prose, and would leak as garbled copy in the history column.
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
	for _, key := range []string{"note", "text"} {
		if s, ok := payload[key].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}
