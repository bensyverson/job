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
	blockerID := mustAdd(t, db, "claude", "Blocker task", nil, nil)
	blockedID := mustAdd(t, db, "claude", "Blocked task", nil, nil)
	if err := job.RunBlock(db, blockedID, blockerID, "claude"); err != nil {
		t.Fatalf("RunBlock: %v", err)
	}

	deps := newPlanDeps(t, db)
	body := fetchPlan(t, deps, "")

	mustContain(t, body, `c-status-pill--blocked`)
	mustContain(t, body, `c-plan-row__blocked-by`)
	mustContain(t, body, `Blocked by`)
	mustContain(t, body, `/tasks/`+blockerID)
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

	// Parent and child rows roll up to active.
	mustContain(t, body, `data-plan-task="`+parent+`"`)
	if !strings.Contains(body, `c-plan-row--status-active" data-plan-task="`+parent+`"`) {
		t.Errorf("parent %q should render as active (rolled up from claimed grandchild)", parent)
	}
	if !strings.Contains(body, `c-plan-row--status-active" data-plan-task="`+child+`"`) {
		t.Errorf("child %q should render as active (rolled up from claimed grandchild)", child)
	}
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

	if !strings.Contains(body, `c-plan-row--status-done" data-plan-task="`+parent+`"`) {
		t.Errorf("done parent %q should stay done despite reopened claimed child", parent)
	}
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
