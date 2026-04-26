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

func TestTask_BlockedAndUnblockedHistoryReadsAsByActor(t *testing.T) {
	db := setupLogTestDB(t)
	subject := mustAdd(t, db, "alice", "Subject", nil, nil)
	blocker := mustAdd(t, db, "alice", "Blocker", nil, nil)
	if err := job.RunBlock(db, subject, blocker, "alice"); err != nil {
		t.Fatalf("RunBlock: %v", err)
	}
	if err := job.RunUnblock(db, subject, blocker, "alice"); err != nil {
		t.Fatalf("RunUnblock: %v", err)
	}

	deps := newLogDeps(t, db)
	w := fetchTask(t, deps, subject)
	body := w.Body.String()

	// Both verbs must include "by" so the rendered row reads
	// "blocked by alice" / "unblocked by alice", not the previous
	// "blocked alice" / "unblocked alice".
	mustContain(t, body, `blocked by <strong>alice</strong>`)
	mustContain(t, body, `unblocked by <strong>alice</strong>`)
}

func TestTask_HistoryDoesNotLeakReasonField(t *testing.T) {
	db := setupLogTestDB(t)
	subject := mustAdd(t, db, "alice", "Subject", nil, nil)
	blocker := mustAdd(t, db, "alice", "Blocker", nil, nil)
	if err := job.RunBlock(db, subject, blocker, "alice"); err != nil {
		t.Fatalf("RunBlock: %v", err)
	}
	if err := job.RunUnblock(db, subject, blocker, "alice"); err != nil {
		t.Fatalf("RunUnblock: %v", err)
	}

	deps := newLogDeps(t, db)
	w := fetchTask(t, deps, subject)
	body := w.Body.String()

	// The unblock event records reason="manual" internally; that
	// field is system categorization and should never reach the
	// rendered history row as user prose.
	if strings.Contains(body, "manual") {
		t.Errorf("history should not surface internal reason values; got body containing 'manual'")
	}
	if strings.Contains(body, "blocker_done") {
		t.Errorf("history should not surface internal reason values; got body containing 'blocker_done'")
	}
}

func TestTask_ClaimExpiredHistoryReadsCleanly(t *testing.T) {
	db := setupLogTestDB(t)
	id := mustAdd(t, db, "alice", "alice-task", nil, nil)
	taskID := taskIDForShortID(t, db, id)
	// Synthesize a claim_expired event (the sweep records the
	// expiring actor; the dashboard should still render it as a
	// system-attributed event).
	if _, err := db.Exec(`INSERT INTO events (task_id, event_type, actor, detail, created_at) VALUES (?, 'claim_expired', 'alice', '', ?)`, taskID, 1); err != nil {
		t.Fatalf("seed claim_expired: %v", err)
	}

	deps := newLogDeps(t, db)
	w := fetchTask(t, deps, id)
	body := w.Body.String()

	// Verb reads naturally — no trailing "by" with no name, no raw
	// "claim_expired" enum.
	if strings.Contains(body, `claim_expired by`) {
		t.Errorf("claim_expired history row should not read 'claim_expired by'")
	}
	if strings.Contains(body, `claim expired by </span>`) || strings.Contains(body, `expired by </span>`) {
		t.Errorf("claim_expired history row should not have a dangling 'by'")
	}
	mustContain(t, body, `claim expired`)
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
