package handlers_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	job "github.com/bensyverson/jobs/internal/job"
	"github.com/bensyverson/jobs/internal/web/handlers"
)

func fetchTask(t *testing.T, deps handlers.Deps, shortID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", "/tasks/"+shortID, nil)
	req.SetPathValue("id", shortID)
	w := httptest.NewRecorder()
	handlers.Task(deps).ServeHTTP(w, req)
	return w
}

func TestTask_UnknownID_Returns404(t *testing.T) {
	db := setupLogTestDB(t)
	deps := newLogDeps(t, db)
	w := fetchTask(t, deps, "zzzzz")
	if w.Code != 404 {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestTask_ExistingTask_RendersSections(t *testing.T) {
	db := setupLogTestDB(t)
	id := mustAdd(t, db, "alice", "Primary task", nil, []string{"web"})

	deps := newLogDeps(t, db)
	w := fetchTask(t, deps, id)
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	mustContain(t, body, `<h1 id="task-title">Primary task</h1>`)
	mustContain(t, body, `c-status-pill`)
	mustContain(t, body, `>Labels<`)
	mustContain(t, body, `>History<`)
}

func TestTask_ClaimedTask_RendersActiveStatus(t *testing.T) {
	db := setupLogTestDB(t)
	id := mustAdd(t, db, "alice", "A task", nil, nil)
	mustClaim(t, db, id, "alice")

	deps := newLogDeps(t, db)
	body := fetchTask(t, deps, id).Body.String()
	if !strings.Contains(body, `c-status-pill--active`) {
		t.Errorf("claimed task should render Active status pill\n---\n%s", body)
	}
}

func TestTask_BlockedTask_RendersBlockedStatus(t *testing.T) {
	db := setupLogTestDB(t)
	targetID := mustAdd(t, db, "alice", "Blocked task", nil, nil)
	blockerID := mustAdd(t, db, "alice", "The blocker", nil, nil)
	if err := job.RunBlock(db, targetID, blockerID, "alice"); err != nil {
		t.Fatalf("RunBlock: %v", err)
	}

	deps := newLogDeps(t, db)
	body := fetchTask(t, deps, targetID).Body.String()
	if !strings.Contains(body, `c-status-pill--blocked`) {
		t.Errorf("task with open blockers should render Blocked status pill")
	}
	// And the blocker should appear in the "Blocked by" section.
	mustContain(t, body, ">Blocked by<")
	mustContain(t, body, "The blocker")
}

func TestTask_HistoryShowsEvents(t *testing.T) {
	db := setupLogTestDB(t)
	id := mustAdd(t, db, "alice", "Historied", nil, nil)
	mustClaim(t, db, id, "alice")

	deps := newLogDeps(t, db)
	body := fetchTask(t, deps, id).Body.String()
	// The history section should mention both events.
	if !strings.Contains(body, "added by") {
		t.Errorf("history missing 'added by' entry")
	}
	if !strings.Contains(body, "claimed by") {
		t.Errorf("history missing 'claimed by' entry")
	}
}
