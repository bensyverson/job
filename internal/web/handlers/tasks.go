package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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
	Criteria       []TaskCriterion
	CompletionNote string
	ClaimedBy      string
	ProgressNotes  []TaskProgressNote
	History        []TaskHistoryEntry
}

// TaskProgressNote is one row in the "Progress notes" section, ordered
// newest-first. The body is the raw note text; the avatar and actor
// link mirror how History rows render their actor.
type TaskProgressNote struct {
	Actor    string
	ActorURL string
	Text     string
	RelTime  string
	ISOTime  string
}

// TaskCriterion is one row in the Criteria checklist on the task page
// and peek sheet. State is one of "pending", "passed", "skipped",
// "failed" — same vocabulary as the CLI's CriterionState. StateBadge
// is the human-readable trailing label rendered for non-pending rows;
// blank when the row is pending and the section just shows the empty
// glyph.
type TaskCriterion struct {
	Label      string
	State      string
	StateBadge string
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
// view, pre-rendered to avoid template-side conditionals. The actor's
// verb (and timestamp) is the entire row — note bodies live in their
// own Progress notes / Completion note sections above, so History
// stays a terse audit trail.
type TaskHistoryEntry struct {
	EventType string
	Actor     string
	Verb      string
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
	criteria, err := job.GetCriteria(deps.DB, task.ID)
	if err != nil {
		InternalError(deps, w, "criteria", err)
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
		Criteria:       buildTaskCriteria(criteria),
		CompletionNote: derefString(task.CompletionNote),
		ClaimedBy:      derefString(task.ClaimedBy),
		ProgressNotes:  buildProgressNotes(events),
		History:        buildHistory(events),
	}, true
}

// buildTaskCriteria pre-renders the criterion rows for the template.
// Mirrors the CLI's criterionGlyph vocabulary so screen-readers and
// color-blind users get a coherent story: pending rows have no badge
// (the empty checkbox carries the meaning); the other three each get
// a one-word badge that reads as the state.
func buildTaskCriteria(items []job.Criterion) []TaskCriterion {
	out := make([]TaskCriterion, len(items))
	for i, c := range items {
		state := string(c.State)
		var badge string
		switch c.State {
		case job.CriterionPassed:
			badge = "passed"
		case job.CriterionSkipped:
			badge = "skipped"
		case job.CriterionFailed:
			badge = "failed"
		}
		out[i] = TaskCriterion{Label: c.Label, State: state, StateBadge: badge}
	}
	return out
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

// buildProgressNotes filters the event log down to noted events with
// non-empty bodies, returning rows newest-first. Caller passes the
// event slice already sorted newest-first (GetEventsForTask uses
// ORDER BY created_at DESC, id DESC), so a forward walk preserves
// that order. Empty-body or malformed-detail events are skipped
// silently; History will still surface a "noted by" row for them, so
// the dashboard does not lose the fact that the event happened.
func buildProgressNotes(events []job.EventEntry) []TaskProgressNote {
	now := time.Now()
	out := make([]TaskProgressNote, 0)
	for _, e := range events {
		if e.EventType != "noted" {
			continue
		}
		text := noteTextFromDetail(e.Detail)
		if text == "" {
			continue
		}
		ts := time.Unix(e.CreatedAt, 0)
		out = append(out, TaskProgressNote{
			Actor:    e.Actor,
			ActorURL: "/actors/" + url.PathEscape(e.Actor),
			Text:     text,
			RelTime:  render.RelativeTime(now, ts),
			ISOTime:  ts.UTC().Format(time.RFC3339),
		})
	}
	return out
}

func buildHistory(events []job.EventEntry) []TaskHistoryEntry {
	now := time.Now()
	out := make([]TaskHistoryEntry, len(events))
	for i, e := range events {
		ts := time.Unix(e.CreatedAt, 0)
		entry := TaskHistoryEntry{
			EventType: e.EventType,
			Actor:     e.Actor,
			Verb:      eventVerb(e.EventType, e.Detail),
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
// blanked out below. detail is consulted for events whose phrasing
// folds in payload data (criteria count, criterion label/state).
func eventVerb(t, detail string) string {
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
	case "criteria_added":
		return criteriaAddedVerb(detail) + " by"
	case "criterion_state":
		return criterionStateVerb(detail) + " by"
	default:
		return t + " by"
	}
}

// criteriaAddedVerb renders the count-aware phrase for a
// criteria_added event payload. Falls back to a count-less phrase if
// the detail is missing or malformed so a corrupt event still reads
// as prose rather than leaking the snake_case enum.
func criteriaAddedVerb(detailJSON string) string {
	count := 0
	if detailJSON != "" {
		var detail map[string]any
		if err := json.Unmarshal([]byte(detailJSON), &detail); err == nil {
			if list, ok := detail["criteria"].([]any); ok {
				count = len(list)
			}
		}
	}
	noun := "criteria"
	if count == 1 {
		noun = "criterion"
	}
	if count == 0 {
		return "added criteria"
	}
	return fmt.Sprintf("added %d %s", count, noun)
}

// criterionStateVerb renders the label+state phrase for a
// criterion_state event payload. Mirrors the CLI vocabulary so the
// two surfaces tell the same story.
func criterionStateVerb(detailJSON string) string {
	label, state := "", ""
	if detailJSON != "" {
		var detail map[string]any
		if err := json.Unmarshal([]byte(detailJSON), &detail); err == nil {
			if v, ok := detail["label"].(string); ok {
				label = v
			}
			if v, ok := detail["state"].(string); ok {
				state = v
			}
		}
	}
	if label == "" || state == "" {
		return "marked a criterion"
	}
	return fmt.Sprintf("marked %q %s", label, state)
}

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
