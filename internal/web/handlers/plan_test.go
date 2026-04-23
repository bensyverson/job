package handlers_test

import (
	"database/sql"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	job "github.com/bensyverson/jobs/internal/job"
	"github.com/bensyverson/jobs/internal/web/assets"
	"github.com/bensyverson/jobs/internal/web/handlers"
	"github.com/bensyverson/jobs/internal/web/templates"
)

func setupPlanTestDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "plan.db")
	db, err := job.CreateDB(path)
	if err != nil {
		t.Fatalf("CreateDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func newPlanDeps(t *testing.T, db *sql.DB) handlers.Deps {
	t.Helper()
	m, err := assets.BuildManifest()
	if err != nil {
		t.Fatalf("BuildManifest: %v", err)
	}
	e, err := templates.New(m)
	if err != nil {
		t.Fatalf("templates.New: %v", err)
	}
	return handlers.Deps{DB: db, Templates: e}
}

// mustAddWithDesc mirrors mustAdd but lets the test set a description.
// Separate helper so the existing log-test mustAdd signature stays
// narrow — most tests don't care about descriptions.
func mustAddWithDesc(t *testing.T, db *sql.DB, actor, title, desc string, parent *string, labels []string) string {
	t.Helper()
	parentID := ""
	if parent != nil {
		parentID = *parent
	}
	res, err := job.RunAdd(db, parentID, title, desc, "", labels, actor)
	if err != nil {
		t.Fatalf("RunAdd(%q, %q): %v", actor, title, err)
	}
	return res.ShortID
}

func fetchPlan(t *testing.T, deps handlers.Deps, query string) string {
	t.Helper()
	url := "/plan"
	if query != "" {
		url += "?" + query
	}
	req := httptest.NewRequest("GET", url, nil)
	w := httptest.NewRecorder()
	handlers.Plan(deps).ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("GET %s: status %d, body=%s", url, w.Code, w.Body.String())
	}
	return w.Body.String()
}

func TestPlan_RendersTreeWithRootAndChildren(t *testing.T) {
	db := setupPlanTestDB(t)
	root := mustAdd(t, db, "claude", "Root task", nil, []string{"web"})
	_ = mustAdd(t, db, "claude", "Child one", &root, nil)
	_ = mustAdd(t, db, "claude", "Child two", &root, []string{"tests"})

	deps := newPlanDeps(t, db)
	body := fetchPlan(t, deps, "")

	mustContain(t, body, `c-plan-row`)
	mustContain(t, body, `Root task`)
	mustContain(t, body, `Child one`)
	mustContain(t, body, `Child two`)
	// Children are nested inside a subtree container.
	mustContain(t, body, `c-plan-subtree`)
	// Labels render as label pills on the row.
	mustContain(t, body, `data-label="web"`)
	mustContain(t, body, `data-label="tests"`)
	// The short id renders as a link to the task detail page.
	mustContain(t, body, `href="/tasks/`+root+`"`)
}

func TestPlan_StatusPillsReflectTaskState(t *testing.T) {
	db := setupPlanTestDB(t)
	// available (todo) root
	todoID := mustAdd(t, db, "claude", "Todo task", nil, nil)
	_ = todoID
	// active: claimed
	activeID := mustAdd(t, db, "claude", "Active task", nil, nil)
	mustClaim(t, db, activeID, "claude")
	// done
	doneID := mustAdd(t, db, "claude", "Done task", nil, nil)
	if _, _, err := job.RunDone(db, []string{doneID}, false, "", nil, "claude"); err != nil {
		t.Fatalf("RunDone: %v", err)
	}

	deps := newPlanDeps(t, db)
	body := fetchPlan(t, deps, "")

	mustContain(t, body, `c-status-pill--todo`)
	mustContain(t, body, `c-status-pill--active`)
	mustContain(t, body, `c-status-pill--done`)
}

func TestPlan_BlockedRowShowsBlockedByAffordance(t *testing.T) {
	db := setupPlanTestDB(t)
	blockerID := mustAdd(t, db, "claude", "Blocker title to hover", nil, nil)
	blockedID := mustAdd(t, db, "claude", "Blocked task", nil, nil)
	if err := job.RunBlock(db, blockedID, blockerID, "claude"); err != nil {
		t.Fatalf("RunBlock: %v", err)
	}

	deps := newPlanDeps(t, db)
	body := fetchPlan(t, deps, "")

	mustContain(t, body, `c-status-pill--blocked`)
	mustContain(t, body, `c-plan-row__blocked-by`)
	mustContain(t, body, `Blocked by`)
	// Blocker link is an in-page anchor to the blocker's row, not a
	// navigation to /tasks/<id>. Keeps the reader on the plan document.
	mustContain(t, body, `href="#task-`+blockerID+`"`)
	// And the full blocker title is exposed via the hover tooltip so
	// users can parse "Blocked by <id>" without navigation — still useful
	// when the blocker lives inside a currently-collapsed subtree.
	mustContain(t, body, `title="Blocker title to hover"`)
}

func TestPlan_EveryRowCarriesAnchorID(t *testing.T) {
	db := setupPlanTestDB(t)
	root := mustAdd(t, db, "claude", "Root", nil, nil)
	child := mustAdd(t, db, "claude", "Child", &root, nil)

	deps := newPlanDeps(t, db)
	body := fetchPlan(t, deps, "")

	mustContain(t, body, `id="task-`+root+`"`)
	mustContain(t, body, `id="task-`+child+`"`)
}

func TestPlan_AutoCollapsesFullyDoneSubtree(t *testing.T) {
	db := setupPlanTestDB(t)
	activeRoot := mustAdd(t, db, "claude", "Active root", nil, nil)
	_ = mustAdd(t, db, "claude", "Open child", &activeRoot, nil)

	doneRoot := mustAdd(t, db, "claude", "Done root", nil, nil)
	doneChild := mustAdd(t, db, "claude", "Done child", &doneRoot, nil)
	if _, _, err := job.RunDone(db, []string{doneChild}, false, "", nil, "claude"); err != nil {
		t.Fatalf("RunDone child: %v", err)
	}
	if _, _, err := job.RunDone(db, []string{doneRoot}, false, "", nil, "claude"); err != nil {
		t.Fatalf("RunDone root: %v", err)
	}

	deps := newPlanDeps(t, db)
	body := fetchPlan(t, deps, "")

	// Active root's subtree is expanded by default.
	mustContain(t, body, `data-plan-task="`+activeRoot+`" data-collapsed="false"`)
	// Done root's subtree is collapsed by default (no open descendants).
	mustContain(t, body, `data-plan-task="`+doneRoot+`" data-collapsed="true"`)
}

func TestPlan_ParentRollsUpToActiveWhenDescendantIsClaimed(t *testing.T) {
	db := setupPlanTestDB(t)
	// Parent is available (todo in its own right); a grandchild is
	// claimed. The plan view should render the parent as active so the
	// tree shows at a glance where the work is happening.
	parent := mustAdd(t, db, "claude", "Parent", nil, nil)
	child := mustAdd(t, db, "claude", "Child", &parent, nil)
	grandchild := mustAdd(t, db, "claude", "Grandchild", &child, nil)
	mustClaim(t, db, grandchild, "claude")

	deps := newPlanDeps(t, db)
	body := fetchPlan(t, deps, "")

	// Parent and child rows roll up to active. Matched via a small
	// helper so the assertion tolerates attribute reshuffles on the row
	// (id, data-plan-task, data-collapsed all share the opening tag).
	assertRowHasClass(t, body, parent, "c-plan-row--status-active")
	assertRowHasClass(t, body, child, "c-plan-row--status-active")
}

func TestPlan_RollupDoesNotOverrideDoneParent(t *testing.T) {
	db := setupPlanTestDB(t)
	// Degenerate but possible: a done parent with a reopened descendant.
	// The done parent's own status should win — rollup doesn't resurrect
	// a closed branch visually.
	parent := mustAdd(t, db, "claude", "Done parent", nil, nil)
	child := mustAdd(t, db, "claude", "Reopened child", &parent, nil)
	if _, _, err := job.RunDone(db, []string{child}, false, "", nil, "claude"); err != nil {
		t.Fatalf("RunDone child: %v", err)
	}
	if _, _, err := job.RunDone(db, []string{parent}, false, "", nil, "claude"); err != nil {
		t.Fatalf("RunDone parent: %v", err)
	}
	if _, err := job.RunReopen(db, child, false, "claude"); err != nil {
		t.Fatalf("RunReopen child: %v", err)
	}
	mustClaim(t, db, child, "claude")

	deps := newPlanDeps(t, db)
	body := fetchPlan(t, deps, "")

	assertRowHasClass(t, body, parent, "c-plan-row--status-done")
}

func TestPlan_NotesRenderInCollapsibleDetails(t *testing.T) {
	db := setupPlanTestDB(t)
	id := mustAdd(t, db, "claude", "Task with a note", nil, nil)
	if err := job.RunNote(db, id, "progress check-in text", nil, "claude"); err != nil {
		t.Fatalf("RunNote: %v", err)
	}

	deps := newPlanDeps(t, db)
	body := fetchPlan(t, deps, "")

	// Notes live in a <details> block, collapsed by default (no open attr).
	mustContain(t, body, `<details class="c-plan-notes-group"`)
	if strings.Contains(body, `<details class="c-plan-notes-group" open`) {
		t.Errorf("notes <details> should be collapsed by default (no open attribute)")
	}
	mustContain(t, body, `progress check-in text`)
	mustContain(t, body, `c-plan-note`)
}

func TestPlan_LeafWithDescriptionIsCollapsible(t *testing.T) {
	db := setupPlanTestDB(t)
	// A leaf task with a description is "technically still collapsible"
	// because the description is part of what collapses. A done leaf
	// should start collapsed so the page stays skimmable.
	active := mustAddWithDesc(t, db, "claude", "Active leaf", "An open description.", nil, nil)
	done := mustAddWithDesc(t, db, "claude", "Done leaf", "A closed description.", nil, nil)
	if _, _, err := job.RunDone(db, []string{done}, false, "", nil, "claude"); err != nil {
		t.Fatalf("RunDone: %v", err)
	}

	deps := newPlanDeps(t, db)
	body := fetchPlan(t, deps, "")

	// Active leaf with a description: collapsible but expanded.
	mustContain(t, body, `data-plan-task="`+active+`" data-collapsed="false"`)
	// Done leaf with a description: collapsible and collapsed.
	mustContain(t, body, `data-plan-task="`+done+`" data-collapsed="true"`)
	// Description still rendered in the DOM — CSS hides it when collapsed.
	mustContain(t, body, `A closed description.`)
}

func TestPlan_RowWithoutDescriptionOrChildrenHasNoDisclosure(t *testing.T) {
	db := setupPlanTestDB(t)
	id := mustAdd(t, db, "claude", "Bare leaf", nil, nil)

	deps := newPlanDeps(t, db)
	body := fetchPlan(t, deps, "")

	// A row with nothing to hide has no data-collapsed attribute and
	// no disclosure button.
	if strings.Contains(body, `data-plan-task="`+id+`" data-collapsed=`) {
		t.Errorf("bare leaf %q should not carry data-collapsed (nothing to hide)", id)
	}
	// Isolate the single row's markup and confirm no disclosure button.
	idx := strings.Index(body, `data-plan-task="`+id+`"`)
	if idx == -1 {
		t.Fatalf("row %q not rendered", id)
	}
	rowEnd := strings.Index(body[idx:], "</div>")
	fragment := body[idx : idx+rowEnd]
	if strings.Contains(fragment, `c-plan-row__disclosure`) {
		t.Errorf("bare leaf %q should not render a disclosure button, got:\n%s", id, fragment)
	}
}

func TestPlan_EmptyDatabaseRendersQuietPlaceholder(t *testing.T) {
	db := setupPlanTestDB(t)
	deps := newPlanDeps(t, db)
	body := fetchPlan(t, deps, "")

	mustContain(t, body, `c-plan-empty`)
}

func TestPlan_FilterTabsRenderInChrome(t *testing.T) {
	db := setupPlanTestDB(t)
	_ = mustAdd(t, db, "claude", "A task", nil, nil)

	deps := newPlanDeps(t, db)
	body := fetchPlan(t, deps, "")

	// Active/Archived/All tabs (inert in p4-ssr; wired in p4-archive).
	mustContain(t, body, `Active`)
	mustContain(t, body, `Archived`)
	mustContain(t, body, `>All<`)
}

func TestPlan_LabelFilter_KeepsMatchingTasksAndAncestors(t *testing.T) {
	db := setupPlanTestDB(t)
	// Tree:
	//   rootA (no label) → childA1 (label=web) → leafA1a (no label)
	//   rootB (no label) → childB1 (label=other)
	// ?label=web keeps rootA (ancestor of a match), childA1 (match);
	// drops leafA1a (unlabeled descendant of a match is filtered too,
	// per the spec's "scopes the tree to tasks matching the label,
	// keeping ancestor chain visible for context"), rootB, childB1.
	rootA := mustAdd(t, db, "claude", "Root A", nil, nil)
	childA1 := mustAdd(t, db, "claude", "Child A1 web", &rootA, []string{"web"})
	leafA1a := mustAdd(t, db, "claude", "Leaf A1a", &childA1, nil)

	rootB := mustAdd(t, db, "claude", "Root B", nil, nil)
	childB1 := mustAdd(t, db, "claude", "Child B1 other", &rootB, []string{"other"})

	deps := newPlanDeps(t, db)
	body := fetchPlan(t, deps, "label=web")

	mustContain(t, body, `id="task-`+rootA+`"`)
	mustContain(t, body, `id="task-`+childA1+`"`)
	if strings.Contains(body, `id="task-`+leafA1a+`"`) {
		t.Errorf("leafA1a %q (unlabeled descendant of a match) should be filtered out by ?label=web", leafA1a)
	}
	if strings.Contains(body, `id="task-`+rootB+`"`) {
		t.Errorf("rootB %q should be filtered out by ?label=web", rootB)
	}
	if strings.Contains(body, `id="task-`+childB1+`"`) {
		t.Errorf("childB1 %q should be filtered out by ?label=web", childB1)
	}
}

func TestPlan_LabelFilter_RendersActivePillAndClearLink(t *testing.T) {
	db := setupPlanTestDB(t)
	_ = mustAdd(t, db, "claude", "A", nil, []string{"web"})
	_ = mustAdd(t, db, "claude", "B", nil, []string{"other"})

	deps := newPlanDeps(t, db)
	body := fetchPlan(t, deps, "label=web")

	// Each distinct label is a clickable pill; the active one carries
	// the --active modifier.
	mustContain(t, body, `c-label-pill--active" data-label="web"`)
	mustContain(t, body, `href="/plan?label=other"`)
	// A clear link appears only when a filter is active.
	mustContain(t, body, `href="/plan"`)
}

func TestPlan_LabelFilter_UnknownLabelShowsEmptyState(t *testing.T) {
	db := setupPlanTestDB(t)
	_ = mustAdd(t, db, "claude", "A", nil, []string{"web"})

	deps := newPlanDeps(t, db)
	body := fetchPlan(t, deps, "label=nonesuch")

	mustContain(t, body, `c-plan-empty`)
}

func TestPlan_NoLabelFilter_OmitsClearLinkAndNoActivePill(t *testing.T) {
	db := setupPlanTestDB(t)
	_ = mustAdd(t, db, "claude", "A", nil, []string{"web"})

	deps := newPlanDeps(t, db)
	body := fetchPlan(t, deps, "")

	// Pill exists, but without --active.
	mustContain(t, body, `data-label="web"`)
	if strings.Contains(body, `c-label-pill--active`) {
		t.Errorf("no ?label= → no active pill should render")
	}
}

// assertRowHasClass finds the row tagged with data-plan-task="<shortID>"
// and confirms its class attribute contains `wantClass`. Resilient to
// attribute reordering inside the opening <div> tag.
func assertRowHasClass(t *testing.T, body, shortID, wantClass string) {
	t.Helper()
	marker := `data-plan-task="` + shortID + `"`
	idx := strings.Index(body, marker)
	if idx == -1 {
		t.Fatalf("row %q not found in body", shortID)
	}
	tagStart := strings.LastIndex(body[:idx], "<div ")
	tagEnd := strings.Index(body[idx:], ">")
	if tagStart == -1 || tagEnd == -1 {
		t.Fatalf("could not bracket row %q opening tag", shortID)
	}
	opening := body[tagStart : idx+tagEnd+1]
	if !strings.Contains(opening, wantClass) {
		t.Errorf("row %q missing class %q in opening tag:\n%s", shortID, wantClass, opening)
	}
}
