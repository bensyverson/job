package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	job "github.com/bensyverson/jobs/internal/job"
	"github.com/bensyverson/jobs/internal/web/render"
	"github.com/bensyverson/jobs/internal/web/templates"
)

// PlanPageData is the payload rendered by the plan template.
type PlanPageData struct {
	templates.Chrome
	Roots    []*PlanNode
	HasTasks bool
}

// PlanNode is one node in the rendered plan tree. All fields are
// preformatted so the template stays decision-free.
type PlanNode struct {
	ShortID       string
	URL           string
	Title         string
	Description   string
	DisplayStatus string
	Actor         string
	Labels        []string
	RelTime       string
	ISOTime       string
	BlockedBy     []PlanBlockerRef
	Notes         []PlanNote
	Children      []*PlanNode
	// Depth is 0 for root tasks, 1 for their direct children, etc. The
	// template uses it to pick heading weight (root → lg, depth 1 → md).
	Depth int
	// HasChildren controls whether the following .c-plan-subtree wrapper
	// renders — a template convenience, not a collapsibility signal.
	HasChildren bool
	// Collapsible is true when the row has anything to hide: children,
	// a description, or (future) a rollup metric. Drives the disclosure
	// button's presence and the data-collapsed attribute. A bare leaf
	// row carries neither and stays chevron-free.
	Collapsible bool
	// Collapsed is true when the node's subtree is fully done/canceled;
	// CSS hides the description, blocked-by, notes, and subtree on
	// collapsed rows. Later phases attach a JS toggle.
	Collapsed bool
}

// PlanNote is one note entry rendered under a task as a c-plan-note row.
type PlanNote struct {
	Actor         string
	RelTime       string
	ISOTime       string
	Text          string
	DisplayStatus string
}

// PlanBlockerRef is one blocker link shown in the "Blocked by" row.
type PlanBlockerRef struct {
	ShortID string
	URL     string
}

// Plan renders the document-mode tree view. See vision §3.2.
// SSR-only for now; collapse-toggle JS, label/archive filters, and
// live-update wiring land in later Phase 4 tasks.
func Plan(deps Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		roots, err := job.RunListFiltered(deps.DB, job.ListFilter{ShowAll: true})
		if err != nil {
			InternalError(deps, w, "plan list", err)
			return
		}

		ids := collectTaskIDs(roots)

		labels, err := job.GetLabelsForTaskIDs(deps.DB, ids)
		if err != nil {
			InternalError(deps, w, "plan labels", err)
			return
		}
		blockers, err := job.GetBlockersForTaskIDs(deps.DB, ids)
		if err != nil {
			InternalError(deps, w, "plan blockers", err)
			return
		}
		notes, err := loadPlanNotes(deps.DB, ids)
		if err != nil {
			InternalError(deps, w, "plan notes", err)
			return
		}
		actors, err := loadPlanActors(deps.DB, ids)
		if err != nil {
			InternalError(deps, w, "plan actors", err)
			return
		}

		now := time.Now()
		planRoots := buildPlanNodes(roots, labels, blockers, notes, actors, now, 0)

		data := PlanPageData{
			Chrome:   templates.Chrome{ActiveTab: "plan"},
			Roots:    planRoots,
			HasTasks: len(planRoots) > 0,
		}
		renderPage(deps, w, "plan", data)
	})
}

// collectTaskIDs walks a task forest in pre-order and returns every
// task id. Single pass so we can batch the follow-up lookups.
func collectTaskIDs(nodes []*job.TaskNode) []int64 {
	var ids []int64
	var walk func([]*job.TaskNode)
	walk = func(ns []*job.TaskNode) {
		for _, n := range ns {
			ids = append(ids, n.Task.ID)
			walk(n.Children)
		}
	}
	walk(nodes)
	return ids
}

// buildPlanNodes maps the domain forest into template-ready PlanNodes.
// Post-order so children are built first; a node is collapsed only
// when every descendant (including itself) has a closed status, which
// matches "auto-collapse fully-done subtrees" from the task spec.
func buildPlanNodes(
	nodes []*job.TaskNode,
	labels map[int64][]string,
	blockers map[int64][]string,
	notes map[int64][]PlanNote,
	actors map[int64]string,
	now time.Time,
	depth int,
) []*PlanNode {
	out := make([]*PlanNode, 0, len(nodes))
	for _, n := range nodes {
		children := buildPlanNodes(n.Children, labels, blockers, notes, actors, now, depth+1)

		taskBlockers := blockers[n.Task.ID]
		displayStatus := DisplayStatus(n.Task.Status, len(taskBlockers) > 0)

		subtreeHasOpen := isOpenStatus(displayStatus)
		for _, c := range children {
			if !c.Collapsed || isOpenStatus(c.DisplayStatus) {
				subtreeHasOpen = true
				break
			}
		}

		// Rollup: a still-open branch whose subtree contains active
		// (claimed) work shows as active itself, so the tree glows
		// where something is actually in progress. Done and canceled
		// parents keep their own status — a closed branch stays closed
		// even if a reopened descendant has picked up life again.
		if isOpenStatus(displayStatus) {
			for _, c := range children {
				if c.DisplayStatus == "active" {
					displayStatus = "active"
					break
				}
			}
		}

		ts := time.Unix(n.Task.UpdatedAt, 0)
		hasChildren := len(children) > 0
		hasDesc := strings.TrimSpace(n.Task.Description) != ""
		out = append(out, &PlanNode{
			ShortID:       n.Task.ShortID,
			URL:           "/tasks/" + n.Task.ShortID,
			Title:         n.Task.Title,
			Description:   n.Task.Description,
			DisplayStatus: displayStatus,
			Actor:         actors[n.Task.ID],
			Labels:        labels[n.Task.ID],
			RelTime:       render.RelativeTime(now, ts),
			ISOTime:       ts.UTC().Format(time.RFC3339),
			BlockedBy:     buildBlockerRefs(taskBlockers),
			Notes:         markNotesStatus(notes[n.Task.ID], displayStatus),
			Children:      children,
			Depth:         depth,
			HasChildren:   hasChildren,
			Collapsible:   hasChildren || hasDesc,
			Collapsed:     !subtreeHasOpen,
		})
	}
	return out
}

// isOpenStatus is true for any status that still warrants attention.
// Done and canceled subtrees can collapse; everything else expands.
func isOpenStatus(displayStatus string) bool {
	return displayStatus != "done" && displayStatus != "canceled"
}

func buildBlockerRefs(shortIDs []string) []PlanBlockerRef {
	if len(shortIDs) == 0 {
		return nil
	}
	out := make([]PlanBlockerRef, len(shortIDs))
	for i, s := range shortIDs {
		out[i] = PlanBlockerRef{ShortID: s, URL: "/tasks/" + s}
	}
	return out
}

// markNotesStatus copies the task's display status onto each of its
// notes so the c-plan-note row can pick up the same tint as its
// parent task (muted when done, live when active, etc.).
func markNotesStatus(notes []PlanNote, displayStatus string) []PlanNote {
	if len(notes) == 0 {
		return nil
	}
	out := make([]PlanNote, len(notes))
	for i, n := range notes {
		n.DisplayStatus = displayStatus
		out[i] = n
	}
	return out
}

// loadPlanActors returns the display-actor for each task id: the most
// recent actor who claimed, completed, or canceled it. Tasks that
// have no such event (brand-new, never claimed) are absent from the
// map so the template renders an empty avatar slot. One query.
func loadPlanActors(db *sql.DB, ids []int64) (map[int64]string, error) {
	out := make(map[int64]string)
	if len(ids) == 0 {
		return out, nil
	}
	q, args := inClause(
		`SELECT task_id, actor FROM events
		 WHERE event_type IN ('claimed','done','canceled') AND task_id IN `,
		ids)
	q += ` ORDER BY created_at ASC, id ASC`
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var taskID int64
		var actor string
		if err := rows.Scan(&taskID, &actor); err != nil {
			return nil, err
		}
		out[taskID] = actor // last write wins → latest relevant event
	}
	return out, rows.Err()
}

// loadPlanNotes returns all 'noted' events grouped by task id, in
// chronological order. The note body is the `text` field of the
// JSON detail payload emitted by RunNote.
func loadPlanNotes(db *sql.DB, ids []int64) (map[int64][]PlanNote, error) {
	out := make(map[int64][]PlanNote)
	if len(ids) == 0 {
		return out, nil
	}
	q, args := inClause(
		`SELECT task_id, actor, detail, created_at FROM events
		 WHERE event_type = 'noted' AND task_id IN `,
		ids)
	q += ` ORDER BY created_at ASC, id ASC`
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	now := time.Now()
	for rows.Next() {
		var taskID int64
		var actor, detail string
		var createdAt int64
		if err := rows.Scan(&taskID, &actor, &detail, &createdAt); err != nil {
			return nil, err
		}
		text := noteTextFromDetail(detail)
		if text == "" {
			continue
		}
		ts := time.Unix(createdAt, 0)
		out[taskID] = append(out[taskID], PlanNote{
			Actor:   actor,
			RelTime: render.RelativeTime(now, ts),
			ISOTime: ts.UTC().Format(time.RFC3339),
			Text:    text,
		})
	}
	return out, rows.Err()
}

// noteTextFromDetail extracts the body text from a 'noted' event's
// detail blob. Returns empty string if the JSON is malformed or the
// text field is missing/empty — a silent skip is kinder than a panic
// on a hand-edited DB row.
func noteTextFromDetail(detail string) string {
	if detail == "" {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(detail), &payload); err != nil {
		return ""
	}
	s, _ := payload["text"].(string)
	return strings.TrimSpace(s)
}

// inClause builds `prefix (?,?,?,…)` for a fixed-length id slice and
// returns the bound args. Callers append their own ORDER BY / LIMIT.
// Kept local to plan.go because the log view's equivalent is trivial
// enough to live inline and the two diverge in shape.
func inClause(prefix string, ids []int64) (string, []any) {
	var b strings.Builder
	b.WriteString(prefix)
	b.WriteString("(")
	args := make([]any, len(ids))
	for i, id := range ids {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("?")
		args[i] = id
	}
	b.WriteString(")")
	return b.String(), args
}
