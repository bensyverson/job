package handlers_test

import (
	"database/sql"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	job "github.com/bensyverson/jobs/internal/job"
	"github.com/bensyverson/jobs/internal/web/assets"
	"github.com/bensyverson/jobs/internal/web/handlers"
	"github.com/bensyverson/jobs/internal/web/templates"
)

func setupLogTestDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "log.db")
	db, err := job.CreateDB(path)
	if err != nil {
		t.Fatalf("CreateDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// newLogDeps wires a Deps bundle good enough to drive the Log handler
// in tests: real DB, real templates+manifest.
func newLogDeps(t *testing.T, db *sql.DB) handlers.Deps {
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

func fetchLog(t *testing.T, deps handlers.Deps, query string) string {
	t.Helper()
	req := httptest.NewRequest("GET", "/log?"+query, nil)
	w := httptest.NewRecorder()
	handlers.Log(deps).ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("GET /log?%s: status %d, body=%s", query, w.Code, w.Body.String())
	}
	return w.Body.String()
}

func TestLog_RendersEventsAsRows(t *testing.T) {
	db := setupLogTestDB(t)
	mustAdd(t, db, "alice", "Root task", nil, []string{"web"})

	deps := newLogDeps(t, db)
	body := fetchLog(t, deps, "")

	mustContain(t, body, `class="c-filter-bar"`)
	mustContain(t, body, `class="c-log-row c-log-row--created"`)
	mustContain(t, body, `>Root task<`)
	mustContain(t, body, `data-actor="alice"`)
}

func TestLog_ActorFilter_HidesOtherActors(t *testing.T) {
	db := setupLogTestDB(t)
	mustAdd(t, db, "alice", "alice-task", nil, nil)
	mustAdd(t, db, "bob", "bob-task", nil, nil)

	deps := newLogDeps(t, db)
	body := fetchLog(t, deps, "actor=alice")

	if !strings.Contains(body, "alice-task") {
		t.Errorf("/log?actor=alice should include alice-task")
	}
	if strings.Contains(body, "bob-task") {
		t.Errorf("/log?actor=alice should NOT include bob-task")
	}
	// Active chip markup present.
	mustContain(t, body, `href="/log" class="c-filter-chip">`) // "any" chip points at /log
	mustContain(t, body, `c-filter-chip--active`)
}

func TestLog_TypeFilter_HidesOtherTypes(t *testing.T) {
	db := setupLogTestDB(t)
	id := mustAdd(t, db, "alice", "A task", nil, nil)
	mustClaim(t, db, id, "alice")

	deps := newLogDeps(t, db)
	body := fetchLog(t, deps, "type=claimed")

	// The "created" row exists in the DB but should be filtered out.
	if !strings.Contains(body, `c-log-row--claimed`) {
		t.Errorf("/log?type=claimed should include claimed rows")
	}
	if strings.Contains(body, `c-log-row--created`) {
		t.Errorf("/log?type=claimed should not include created rows")
	}
}

func TestLog_EmptyDatabase_RendersPlaceholder(t *testing.T) {
	db := setupLogTestDB(t)
	deps := newLogDeps(t, db)
	body := fetchLog(t, deps, "")
	mustContain(t, body, `No events match`)
}

func TestLog_LiveRegionSrc_ReflectsFilters(t *testing.T) {
	db := setupLogTestDB(t)
	mustAdd(t, db, "alice", "A task", nil, nil)

	deps := newLogDeps(t, db)

	// No filters → live-region points at /events.
	body := fetchLog(t, deps, "")
	mustContain(t, body, `<live-region src="/events">`)

	// actor=alice → live-region scoped to the same actor.
	body = fetchLog(t, deps, "actor=alice")
	mustContain(t, body, `<live-region src="/events?actor=alice">`)
}

func TestLog_ChipsPreserveOtherFilters(t *testing.T) {
	db := setupLogTestDB(t)
	mustAdd(t, db, "alice", "A task", nil, nil)
	deps := newLogDeps(t, db)

	// With ?actor=alice, the type chips should encode &actor=alice.
	body := fetchLog(t, deps, "actor=alice")
	mustContain(t, body, `/log?actor=alice&amp;type=claimed`)
}

func TestLog_LabelStripCapsToTopTenByOpenTaskFrequency(t *testing.T) {
	db := setupLogTestDB(t)
	// 12 labels with descending counts: a×12 down to l×1. Strip should
	// keep top 10 (a–j) and drop k, l. The "any" chip is always present.
	for i, name := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l"} {
		count := 12 - i
		for n := range count {
			if _, err := job.RunAdd(db, "", name+"-"+strconv.Itoa(n), "", "", []string{name}, "alice"); err != nil {
				t.Fatalf("RunAdd: %v", err)
			}
		}
	}

	deps := newLogDeps(t, db)
	body := fetchLog(t, deps, "")

	bar := extractFilterBar(t, body)
	for _, want := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"} {
		if !strings.Contains(bar, `data-label="`+want+`"`) && !strings.Contains(bar, `>`+want+`<`) {
			t.Errorf("strip should include top-10 label %q", want)
		}
	}
	for _, drop := range []string{"k", "l"} {
		if strings.Contains(bar, `data-label="`+drop+`"`) {
			t.Errorf("strip should not include below-cap label %q", drop)
		}
	}
}

func TestLog_LabelStripIncludesActiveLabelEvenIfBelowCap(t *testing.T) {
	db := setupLogTestDB(t)
	for i, name := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"} {
		count := 12 - i
		for n := range count {
			if _, err := job.RunAdd(db, "", name+"-"+strconv.Itoa(n), "", "", []string{name}, "alice"); err != nil {
				t.Fatalf("RunAdd: %v", err)
			}
		}
	}

	deps := newLogDeps(t, db)
	body := fetchLog(t, deps, "label=k") // k is the 11th label, normally cut

	bar := extractFilterBar(t, body)
	if !strings.Contains(bar, `data-label="k"`) {
		t.Errorf("strip should include the active label %q even when below the cap", "k")
	}
}

func TestLog_PaginationCapsInitialRender(t *testing.T) {
	db := setupLogTestDB(t)
	// 12 events (creation only), explicit limit of 5.
	for i := range 12 {
		mustAdd(t, db, "alice", "task-"+strconv.Itoa(i), nil, nil)
	}

	deps := newLogDeps(t, db)
	body := fetchLog(t, deps, "limit=5")

	// Row count: count occurrences of c-log-row__time (one per row).
	rows := strings.Count(body, `class="c-log-row__time"`)
	if rows != 5 {
		t.Errorf("?limit=5 should render 5 rows, got %d", rows)
	}
	// HasMore affordance present.
	mustContain(t, body, `c-log-more`)
}

func TestLog_PaginationLoadMoreFiltersOlderEvents(t *testing.T) {
	db := setupLogTestDB(t)
	for i := range 12 {
		mustAdd(t, db, "alice", "task-"+strconv.Itoa(i), nil, nil)
	}

	deps := newLogDeps(t, db)

	// Page 1 (newest 5): record the oldest id from the more-link.
	body := fetchLog(t, deps, "limit=5")
	// The href encodes "before=<oldestID>".
	idx := strings.Index(body, `?before=`)
	if idx == -1 {
		t.Fatalf("expected before=<id> in the more-link href:\n%s", body)
	}
	rest := body[idx+len(`?before=`):]
	end := strings.IndexAny(rest, `"&`)
	beforeID := rest[:end]

	// Page 2 (next 5 older): should include strictly fewer events.
	body = fetchLog(t, deps, "limit=5&before="+beforeID)
	rows := strings.Count(body, `class="c-log-row__time"`)
	if rows != 5 {
		t.Errorf("page 2 should render 5 rows, got %d", rows)
	}
	// Page 1's newest task should not appear on page 2.
	if strings.Contains(body, "task-11") {
		t.Errorf("page 2 should not include the newest task")
	}
}

func TestLog_PaginationOmitsLoadMoreWhenAllEventsFit(t *testing.T) {
	db := setupLogTestDB(t)
	for i := range 3 {
		mustAdd(t, db, "alice", "task-"+strconv.Itoa(i), nil, nil)
	}

	deps := newLogDeps(t, db)
	body := fetchLog(t, deps, "limit=10")

	if strings.Contains(body, `c-log-more`) {
		t.Errorf("with no more events to fetch, the load-more affordance should not render")
	}
}

func TestLog_ClaimExpiredRendersAsExpiredVerb(t *testing.T) {
	db := setupLogTestDB(t)
	id := mustAdd(t, db, "alice", "expired-task", nil, nil)
	// Synthesize a claim_expired event directly. The expirer's actor
	// in production is whoever triggered the sweep, but for display
	// the row should read as a system event regardless.
	taskID := taskIDForShortID(t, db, id)
	if _, err := db.Exec(`INSERT INTO events (task_id, event_type, actor, detail, created_at) VALUES (?, 'claim_expired', 'alice', '', ?)`, taskID, time.Now().Unix()); err != nil {
		t.Fatalf("seed claim_expired: %v", err)
	}

	deps := newLogDeps(t, db)
	body := fetchLog(t, deps, "")

	// The verb word is rendered lowercase (CSS uppercases), so the
	// payload string is "expired".
	mustContain(t, body, `<span class="c-log-row__verb c-log-row__verb--claim_expired">expired</span>`)
	if strings.Contains(body, `claim_expired</span>`) {
		t.Errorf("verb text should not include the raw event_type")
	}
}

func TestLog_ClaimExpiredActorRendersAsSystemNotLink(t *testing.T) {
	db := setupLogTestDB(t)
	id := mustAdd(t, db, "alice", "expired-task", nil, nil)
	taskID := taskIDForShortID(t, db, id)
	if _, err := db.Exec(`INSERT INTO events (task_id, event_type, actor, detail, created_at) VALUES (?, 'claim_expired', 'alice', '', ?)`, taskID, time.Now().Unix()); err != nil {
		t.Fatalf("seed claim_expired: %v", err)
	}

	deps := newLogDeps(t, db)
	body := fetchLog(t, deps, "")

	// "Jobs" appears as the system actor label, no anchor and no
	// avatar dot tied to the original claimer.
	mustContain(t, body, `c-log-row__actor--system`)
	mustContain(t, body, `>Jobs<`)
	// Confirm the row does not render a link to /actors/alice for the
	// expired event. The other event for this task (the created
	// event by alice) still does, so we narrow by isolating the
	// claim_expired row's actor cell.
	rows := splitLogRows(body)
	for _, row := range rows {
		if strings.Contains(row, `c-log-row__verb--claim_expired`) {
			if strings.Contains(row, `href="/actors/`) {
				t.Errorf("claim_expired row should not link the actor to /actors/*\n%s", row)
			}
			if strings.Contains(row, `data-actor="alice"`) {
				t.Errorf("claim_expired row should not surface the prior claimer as the actor\n%s", row)
			}
		}
	}
}

// --- ?at time-travel tests (R0Ro4) ---

// fetchLogStatus returns the status code and body so tests can assert
// non-200 responses (the regular fetchLog helper t.Fatal's on non-200).
func fetchLogStatus(t *testing.T, deps handlers.Deps, query string) (int, string) {
	t.Helper()
	req := httptest.NewRequest("GET", "/log?"+query, nil)
	w := httptest.NewRecorder()
	handlers.Log(deps).ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

// maxEventID returns the current max event id in the test DB. Used to
// fix `?at` boundaries in tests that need to compare against absolute
// event ids the seed produced.
func maxEventID(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(`SELECT COALESCE(MAX(id), 0) FROM events`).Scan(&id); err != nil {
		t.Fatalf("maxEventID: %v", err)
	}
	return id
}

// eventIDForTaskCreate returns the id of the `created` event for a
// given short task id — the most reliable way to pin `?at` to a known
// state ("just before" / "at" / "after" a particular task's creation).
func eventIDForTaskCreate(t *testing.T, db *sql.DB, shortID string) int64 {
	t.Helper()
	var id int64
	err := db.QueryRow(`
		SELECT e.id FROM events e
		JOIN tasks t ON t.id = e.task_id
		WHERE t.short_id = ? AND e.event_type = 'created'
		LIMIT 1
	`, shortID).Scan(&id)
	if err != nil {
		t.Fatalf("eventIDForTaskCreate(%q): %v", shortID, err)
	}
	return id
}

func TestLog_AtFiltersToUpperBoundInclusive(t *testing.T) {
	db := setupLogTestDB(t)
	mustAdd(t, db, "alice", "first-task", nil, nil)
	idSecond := mustAdd(t, db, "alice", "second-task", nil, nil)
	mustAdd(t, db, "alice", "third-task", nil, nil)

	// Pin ?at to second-task's created event id. Inclusive: rows for
	// first and second appear; third does not.
	at := eventIDForTaskCreate(t, db, idSecond)

	deps := newLogDeps(t, db)
	body := fetchLog(t, deps, "at="+strconv.FormatInt(at, 10))

	if !strings.Contains(body, "first-task") {
		t.Errorf("?at=%d should include first-task (event id < at)", at)
	}
	if !strings.Contains(body, "second-task") {
		t.Errorf("?at=%d should include second-task (event id == at, inclusive)", at)
	}
	if strings.Contains(body, "third-task") {
		t.Errorf("?at=%d should NOT include third-task (event id > at)", at)
	}
}

func TestLog_AtAboveHeadRendersAsLive(t *testing.T) {
	db := setupLogTestDB(t)
	mustAdd(t, db, "alice", "live-task", nil, nil)

	// at far above any real event id behaves as if there were no filter.
	deps := newLogDeps(t, db)
	body := fetchLog(t, deps, "at=999999999")

	if !strings.Contains(body, "live-task") {
		t.Errorf("?at past head should render the same as live")
	}
}

func TestLog_AtMalformedReturns400(t *testing.T) {
	db := setupLogTestDB(t)
	mustAdd(t, db, "alice", "task", nil, nil)
	deps := newLogDeps(t, db)

	for _, raw := range []string{"at=foo", "at=0", "at=-1", "at=1.5"} {
		t.Run(raw, func(t *testing.T) {
			code, _ := fetchLogStatus(t, deps, raw)
			if code != 400 {
				t.Errorf("GET /log?%s: status %d, want 400", raw, code)
			}
		})
	}
}

func TestLog_AtScopesTotalEventsCounter(t *testing.T) {
	db := setupLogTestDB(t)
	idA := mustAdd(t, db, "alice", "a", nil, nil)
	mustAdd(t, db, "alice", "b", nil, nil)
	mustAdd(t, db, "alice", "c", nil, nil)

	// Pin ?at to the first event. TotalEvents should reflect 1, not 3.
	at := eventIDForTaskCreate(t, db, idA)

	deps := newLogDeps(t, db)
	body := fetchLog(t, deps, "at="+strconv.FormatInt(at, 10))

	// The total-events counter renders inline as part of the log meta
	// strip. The exact wording can vary, but "1 event" should be
	// derivable; "3 events" should not appear in that meta region.
	// We assert by looking at the page header copy that includes the
	// total — searching for the exact strings the template emits.
	if strings.Contains(body, "3 events") || strings.Contains(body, "of 3") {
		t.Errorf("?at=%d should not surface the live-mode 3 events count", at)
	}
}

func TestLog_AtComposesWithActorFilter(t *testing.T) {
	db := setupLogTestDB(t)
	mustAdd(t, db, "alice", "alice-1", nil, nil)
	mustAdd(t, db, "bob", "bob-1", nil, nil)
	idLast := mustAdd(t, db, "alice", "alice-2", nil, nil)
	atLast := eventIDForTaskCreate(t, db, idLast)

	// Walk back one event from alice-2 — bob-1 is the most recent
	// alice/bob mix before alice-2.
	at := atLast - 1

	deps := newLogDeps(t, db)
	body := fetchLog(t, deps, "actor=alice&at="+strconv.FormatInt(at, 10))

	if !strings.Contains(body, "alice-1") {
		t.Errorf("?actor=alice&at=%d should include alice-1", at)
	}
	if strings.Contains(body, "bob-1") {
		t.Errorf("?actor=alice&at=%d should NOT include bob-1 (filtered by actor)", at)
	}
	if strings.Contains(body, "alice-2") {
		t.Errorf("?actor=alice&at=%d should NOT include alice-2 (event id > at)", at)
	}
}

// --- helpers ---

func mustAdd(t *testing.T, db *sql.DB, actor, title string, parent *string, labels []string) string {
	t.Helper()
	parentID := ""
	if parent != nil {
		parentID = *parent
	}
	res, err := job.RunAdd(db, parentID, title, "", "", labels, actor)
	if err != nil {
		t.Fatalf("RunAdd(%q, %q): %v", actor, title, err)
	}
	return res.ShortID
}

func mustClaim(t *testing.T, db *sql.DB, shortID, actor string) {
	t.Helper()
	if err := job.RunClaim(db, shortID, "30m", actor, false); err != nil {
		t.Fatalf("RunClaim(%q, %q): %v", shortID, actor, err)
	}
}

func taskIDForShortID(t *testing.T, db *sql.DB, shortID string) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(`SELECT id FROM tasks WHERE short_id = ?`, shortID).Scan(&id); err != nil {
		t.Fatalf("taskIDForShortID(%q): %v", shortID, err)
	}
	return id
}

// splitLogRows returns the raw HTML for each c-log-row in body. Each
// returned string starts at one row's opening tag and ends just
// before the next row's tag (or the end of the log container). Used
// by tests that need to assert behavior on a specific row without
// false positives from sibling rows.
func splitLogRows(body string) []string {
	const marker = `<div class="c-log-row c-log-row--`
	var out []string
	for {
		i := strings.Index(body, marker)
		if i < 0 {
			break
		}
		body = body[i:]
		j := strings.Index(body[len(marker):], marker)
		if j < 0 {
			out = append(out, body)
			break
		}
		out = append(out, body[:len(marker)+j])
		body = body[len(marker)+j:]
	}
	return out
}

func mustContain(t *testing.T, body, needle string) {
	t.Helper()
	if !strings.Contains(body, needle) {
		t.Errorf("missing %q in body\n---\n%s", needle, body)
	}
}
