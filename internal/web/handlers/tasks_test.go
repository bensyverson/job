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

func TestTask_CriteriaSection_RendersFourStates(t *testing.T) {
	db := setupLogTestDB(t)
	id := mustAdd(t, db, "alice", "with criteria", nil, nil)
	if _, err := job.RunAddCriteria(db, id, []job.Criterion{
		{Label: "still pending"},
		{Label: "now passing"},
		{Label: "skip me"},
		{Label: "i broke"},
	}, "alice"); err != nil {
		t.Fatalf("RunAddCriteria: %v", err)
	}
	if _, err := job.RunSetCriterion(db, id, "now passing", job.CriterionPassed, "alice"); err != nil {
		t.Fatalf("RunSetCriterion passed: %v", err)
	}
	if _, err := job.RunSetCriterion(db, id, "skip me", job.CriterionSkipped, "alice"); err != nil {
		t.Fatalf("RunSetCriterion skipped: %v", err)
	}
	if _, err := job.RunSetCriterion(db, id, "i broke", job.CriterionFailed, "alice"); err != nil {
		t.Fatalf("RunSetCriterion failed: %v", err)
	}

	deps := newLogDeps(t, db)
	body := stripInitialFrame(fetchTask(t, deps, id).Body.String())

	// Section header present.
	mustContain(t, body, ">Criteria<")
	// All four labels render.
	mustContainAll(t, body,
		"still pending", "now passing", "skip me", "i broke",
	)
	// Glyphs must match the CLI's criterionGlyph vocabulary so the two
	// surfaces tell the same story.
	mustContainAll(t, body,
		`data-criterion-state="pending"`,
		`data-criterion-state="passed"`,
		`data-criterion-state="skipped"`,
		`data-criterion-state="failed"`,
	)
	// Non-pending rows carry an accessible state badge so screen-readers
	// and color-blind users can read the state without the glyph alone.
	mustContainAll(t, body,
		`c-criterion__badge--passed`,
		`c-criterion__badge--skipped`,
		`c-criterion__badge--failed`,
	)
}

func TestTask_CriteriaSection_OmittedWhenZero(t *testing.T) {
	db := setupLogTestDB(t)
	id := mustAdd(t, db, "alice", "no criteria", nil, nil)

	deps := newLogDeps(t, db)
	body := stripInitialFrame(fetchTask(t, deps, id).Body.String())

	if strings.Contains(body, ">Criteria<") {
		t.Errorf("Criteria section should be omitted when task has zero criteria")
	}
}

func TestTask_HistoryRendersCriteriaAddedAndCriterionStateAsHumanVerbs(t *testing.T) {
	db := setupLogTestDB(t)
	id := mustAdd(t, db, "alice", "subject", nil, nil)
	if _, err := job.RunAddCriteria(db, id, []job.Criterion{
		{Label: "alpha"},
		{Label: "beta"},
	}, "alice"); err != nil {
		t.Fatalf("RunAddCriteria: %v", err)
	}
	if _, err := job.RunSetCriterion(db, id, "beta", job.CriterionPassed, "alice"); err != nil {
		t.Fatalf("RunSetCriterion: %v", err)
	}

	deps := newLogDeps(t, db)
	body := stripInitialFrame(fetchTask(t, deps, id).Body.String())

	// Snake-case event types must not leak into History rows.
	if strings.Contains(body, "criteria_added by") {
		t.Errorf("History should not surface raw 'criteria_added by'\n%s", body)
	}
	if strings.Contains(body, "criterion_state by") {
		t.Errorf("History should not surface raw 'criterion_state by'\n%s", body)
	}
	// Count-aware phrasing for criteria_added; label+state for criterion_state.
	mustContain(t, body, `added 2 criteria by`)
	mustContain(t, body, `marked &#34;beta&#34; passed by`)
}

func TestTask_ProgressNotesSection_RendersAndOmitsWhenZero(t *testing.T) {
	db := setupLogTestDB(t)
	idEmpty := mustAdd(t, db, "alice", "no notes here", nil, nil)
	idWithNotes := mustAdd(t, db, "alice", "noted task", nil, nil)
	mustClaim(t, db, idWithNotes, "alice")
	if err := job.RunNote(db, idWithNotes, "first observation", nil, "alice"); err != nil {
		t.Fatalf("RunNote first: %v", err)
	}
	if err := job.RunNote(db, idWithNotes, "later observation", nil, "alice"); err != nil {
		t.Fatalf("RunNote later: %v", err)
	}

	deps := newLogDeps(t, db)

	bodyEmpty := stripInitialFrame(fetchTask(t, deps, idEmpty).Body.String())
	if strings.Contains(bodyEmpty, ">Progress notes<") {
		t.Errorf("task without progress notes should omit Progress notes section")
	}

	body := stripInitialFrame(fetchTask(t, deps, idWithNotes).Body.String())

	mustContain(t, body, ">Progress notes<")
	mustContainAll(t, body, "first observation", "later observation")

	// Newest-first: "later observation" must appear before "first observation"
	// inside the rendered Progress notes section.
	section := body
	start := strings.Index(section, ">Progress notes<")
	if start < 0 {
		t.Fatalf("Progress notes header not found")
	}
	end := strings.Index(section[start:], ">History<")
	if end < 0 {
		end = len(section) - start
	}
	region := section[start : start+end]
	if strings.Index(region, "later observation") > strings.Index(region, "first observation") {
		t.Errorf("Progress notes should be ordered newest-first")
	}

	// Each row carries an actor avatar/link and a relative time.
	if !strings.Contains(region, `c-avatar`) {
		t.Errorf("Progress notes row should include the actor avatar")
	}
	if !strings.Contains(region, `<time`) {
		t.Errorf("Progress notes row should include a <time> element")
	}
}

func TestPeek_ProgressNotesSectionRendersAboveHistory(t *testing.T) {
	db := setupLogTestDB(t)
	id := mustAdd(t, db, "alice", "noted task", nil, nil)
	mustClaim(t, db, id, "alice")
	if err := job.RunNote(db, id, "an observation", nil, "alice"); err != nil {
		t.Fatalf("RunNote: %v", err)
	}

	deps := newLogDeps(t, db)
	body := mustFetchPeek(t, deps, id)

	mustContain(t, body, ">Progress notes<")
	mustContain(t, body, "an observation")

	// Order: Progress notes section appears before History section.
	pn := strings.Index(body, ">Progress notes<")
	hi := strings.Index(body, ">History<")
	if pn < 0 || hi < 0 || pn > hi {
		t.Errorf("Progress notes section should appear above History (pn=%d, hi=%d)", pn, hi)
	}
}

func TestTask_HistoryRowsCarryNoInlineNoteText(t *testing.T) {
	db := setupLogTestDB(t)
	id := mustAdd(t, db, "alice", "noted task", nil, nil)
	mustClaim(t, db, id, "alice")
	if err := job.RunNote(db, id, "this is a progress note", nil, "alice"); err != nil {
		t.Fatalf("RunNote: %v", err)
	}
	if _, _, err := job.RunDone(db, []string{id}, false, "the closing note", nil, "alice"); err != nil {
		t.Fatalf("RunDone: %v", err)
	}

	deps := newLogDeps(t, db)
	body := stripInitialFrame(fetchTask(t, deps, id).Body.String())

	// Isolate the History section so the assertion isn't fooled by
	// note bodies that legitimately appear in Progress notes / Completion
	// note sections above.
	hi := strings.Index(body, ">History<")
	if hi < 0 {
		t.Fatalf("History section not found")
	}
	historyRegion := body[hi:]
	for _, banned := range []string{
		"this is a progress note",
		"the closing note",
	} {
		if strings.Contains(historyRegion, banned) {
			t.Errorf("History row should not inline the note body %q\n--- region ---\n%s", banned, historyRegion)
		}
	}
	// Verbs themselves still render — we're only stripping the trailer.
	mustContain(t, historyRegion, `noted by`)
	mustContain(t, historyRegion, `done by`)
}

func TestPeek_HistoryRowsCarryNoInlineNoteText(t *testing.T) {
	db := setupLogTestDB(t)
	id := mustAdd(t, db, "alice", "noted task", nil, nil)
	mustClaim(t, db, id, "alice")
	if err := job.RunNote(db, id, "PEEK_NOTE_BODY", nil, "alice"); err != nil {
		t.Fatalf("RunNote: %v", err)
	}

	deps := newLogDeps(t, db)
	body := mustFetchPeek(t, deps, id)

	hi := strings.Index(body, ">History<")
	if hi < 0 {
		t.Fatalf("History section not found in peek")
	}
	historyRegion := body[hi:]
	if strings.Contains(historyRegion, "PEEK_NOTE_BODY") {
		t.Errorf("Peek History row should not inline the note body\n--- region ---\n%s", historyRegion)
	}
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
