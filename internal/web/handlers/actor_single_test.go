package handlers_test

import (
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bensyverson/jobs/internal/web/handlers"
)

func fetchActorSingle(t *testing.T, deps handlers.Deps, name string) (int, string) {
	t.Helper()
	req := httptest.NewRequest("GET", "/actors/"+name, nil)
	req.SetPathValue("name", name)
	w := httptest.NewRecorder()
	handlers.ActorSingle(deps).ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

func mustFetchActorSingle(t *testing.T, deps handlers.Deps, name string) string {
	t.Helper()
	code, body := fetchActorSingle(t, deps, name)
	if code != 200 {
		t.Fatalf("GET /actors/%s: status %d, body=%s", name, code, body)
	}
	return body
}

func TestActorSingle_RendersHeroWithNameAndAvatar(t *testing.T) {
	db := setupLogTestDB(t)
	mustAdd(t, db, "alice", "alice-task", nil, nil)

	deps := newLogDeps(t, db)
	body := mustFetchActorSingle(t, deps, "alice")

	mustContain(t, body, `c-actor-hero`)
	mustContain(t, body, `c-actor-hero__avatar`)
	mustContain(t, body, `data-actor="alice"`)
	mustContain(t, body, `>alice<`)
	// Initial inside the hero avatar
	mustContain(t, body, `>A<`)
}

func TestActorSingle_BreadcrumbLinksBackToActors(t *testing.T) {
	db := setupLogTestDB(t)
	mustAdd(t, db, "alice", "alice-task", nil, nil)

	deps := newLogDeps(t, db)
	body := mustFetchActorSingle(t, deps, "alice")

	mustContain(t, body, `href="/actors"`)
	mustContain(t, body, `All actors`)
}

func TestActorSingle_HeaderTabActorsActive(t *testing.T) {
	db := setupLogTestDB(t)
	mustAdd(t, db, "alice", "t", nil, nil)

	deps := newLogDeps(t, db)
	body := mustFetchActorSingle(t, deps, "alice")

	mustContain(t, body, `href="/actors" class="c-tab c-tab--active"`)
}

func TestActorSingle_HeroStatsTilesRender(t *testing.T) {
	db := setupLogTestDB(t)
	mustAdd(t, db, "alice", "t", nil, nil)

	deps := newLogDeps(t, db)
	body := mustFetchActorSingle(t, deps, "alice")

	for _, label := range []string{"In flight", "Done 1h", "Done 24h", "Blocked"} {
		if !strings.Contains(body, label) {
			t.Errorf("missing stat tile label %q", label)
		}
	}
	// Four stat tiles expected.
	tiles := strings.Count(body, `class="c-actor-stat"`)
	if tiles != 4 {
		t.Errorf("c-actor-stat tile count: got %d, want 4", tiles)
	}
}

func TestActorSingle_StatsCountsAreAccurate(t *testing.T) {
	db := setupLogTestDB(t)
	now := time.Now()

	// alice: 2 currently claimed, 3 done in last 1h, 5 done in 24h total.
	for i := range 2 {
		shortID := "c" + strconv.Itoa(i)
		taskID := homeSeedTask(t, db, shortID, "claimed-"+strconv.Itoa(i), "claimed", now.Add(-10*time.Minute))
		homeSeedEventActor(t, db, taskID, "claimed", "alice", now.Add(-10*time.Minute))
		if _, err := db.Exec(`UPDATE tasks SET claimed_by='alice', claim_expires_at=? WHERE id=?`,
			now.Add(30*time.Minute).Unix(), taskID); err != nil {
			t.Fatalf("set claim: %v", err)
		}
	}
	// 3 done in last hour
	for i := range 3 {
		shortID := "d" + strconv.Itoa(i)
		taskID := homeSeedTask(t, db, shortID, "done-1h-"+strconv.Itoa(i), "done", now.Add(-30*time.Minute))
		homeSeedEventActor(t, db, taskID, "done", "alice", now.Add(-30*time.Minute))
	}
	// 2 done between 1h and 24h ago — count toward 24h, not 1h
	for i := range 2 {
		shortID := "o" + strconv.Itoa(i)
		taskID := homeSeedTask(t, db, shortID, "done-old-"+strconv.Itoa(i), "done", now.Add(-6*time.Hour))
		homeSeedEventActor(t, db, taskID, "done", "alice", now.Add(-6*time.Hour))
	}

	deps := newLogDeps(t, db)
	body := mustFetchActorSingle(t, deps, "alice")

	// Locate each stat tile by label and confirm its value.
	checks := map[string]string{
		"In flight": "2",
		"Done 1h":   "3",
		"Done 24h":  "5",
		"Blocked":   "0",
	}
	for label, want := range checks {
		assertStatTileValue(t, body, label, want)
	}
}

func TestActorSingle_TimelineHasFiveLanes(t *testing.T) {
	db := setupLogTestDB(t)
	mustAdd(t, db, "alice", "t", nil, nil)

	deps := newLogDeps(t, db)
	body := mustFetchActorSingle(t, deps, "alice")

	mustContain(t, body, `c-actor-timeline`)
	for _, verb := range []string{"created", "claimed", "done", "blocked", "noted"} {
		marker := `c-actor-timeline__lane-label">` + verb
		if !strings.Contains(body, marker) {
			t.Errorf("missing timeline lane for %q", verb)
		}
	}
}

func TestActorSingle_TimelineMarksCarryXPercent(t *testing.T) {
	db := setupLogTestDB(t)
	now := time.Now()
	taskID := homeSeedTask(t, db, "task1", "task1", "available", now.Add(-12*time.Hour))
	homeSeedEventActor(t, db, taskID, "created", "alice", now.Add(-12*time.Hour))

	deps := newLogDeps(t, db)
	body := mustFetchActorSingle(t, deps, "alice")

	// At 12h ago on a 24h axis the mark sits ~50% in.
	mustContain(t, body, `c-actor-timeline__mark--created`)
	if !strings.Contains(body, `--x:50.`) && !strings.Contains(body, `--x:49.`) {
		t.Errorf("expected timeline mark near 50%% for an event 12h ago; body did not contain it")
	}
}

func TestActorSingle_EventListShowsOnlyThisActor(t *testing.T) {
	db := setupLogTestDB(t)
	mustAdd(t, db, "alice", "alice-task", nil, nil)
	mustAdd(t, db, "bob", "bob-task", nil, nil)

	deps := newLogDeps(t, db)
	body := stripInitialFrame(mustFetchActorSingle(t, deps, "alice"))

	mustContain(t, body, `alice-task`)
	if strings.Contains(body, `bob-task`) {
		t.Errorf("bob's events should not appear on alice's actor page")
	}
}

func TestActorSingle_404ForUnknownActor(t *testing.T) {
	db := setupLogTestDB(t)
	deps := newLogDeps(t, db)

	code, _ := fetchActorSingle(t, deps, "nobody")
	if code != 404 {
		t.Errorf("unknown actor: got status %d, want 404", code)
	}
}

func TestActorSingle_StatusLineMentionsClaimsAndLastSeen(t *testing.T) {
	db := setupLogTestDB(t)
	id := mustAdd(t, db, "alice", "t", nil, nil)
	mustClaim(t, db, id, "alice")

	deps := newLogDeps(t, db)
	body := mustFetchActorSingle(t, deps, "alice")

	mustContain(t, body, `1 claim`)
	mustContain(t, body, `last seen`)
}

func TestActorSingle_LiveRegionScopedToActor(t *testing.T) {
	db := setupLogTestDB(t)
	mustAdd(t, db, "alice", "t", nil, nil)

	deps := newLogDeps(t, db)
	body := mustFetchActorSingle(t, deps, "alice")

	mustContain(t, body, `<live-region src="/events?actor=alice">`)
}

func TestActorSingle_EventListCarriesActorMarker(t *testing.T) {
	db := setupLogTestDB(t)
	mustAdd(t, db, "alice", "t", nil, nil)

	deps := newLogDeps(t, db)
	body := mustFetchActorSingle(t, deps, "alice")

	mustContain(t, body, `data-actor-events="alice"`)
}

func TestActorSingle_EventListCapsAtLimit(t *testing.T) {
	db := setupLogTestDB(t)
	now := time.Now()
	// Seed more events than the cap. Each task gives one created
	// event, so cap+20 tasks = cap+20 events on alice's column.
	total := handlers.ActorEventListLimit + 20
	for i := range total {
		shortID := "x" + strconv.Itoa(i)
		taskID := homeSeedTask(t, db, shortID, "task-"+strconv.Itoa(i), "available", now.Add(-time.Duration(total-i)*time.Minute))
		homeSeedEventActor(t, db, taskID, "created", "alice", now.Add(-time.Duration(total-i)*time.Minute))
	}

	deps := newLogDeps(t, db)
	body := mustFetchActorSingle(t, deps, "alice")

	rows := strings.Count(body, `class="c-log-row__time"`)
	if rows != handlers.ActorEventListLimit {
		t.Errorf("event row count: got %d, want %d", rows, handlers.ActorEventListLimit)
	}
}

func TestActorSingle_ViewAllLinkPointsToLogFilter(t *testing.T) {
	db := setupLogTestDB(t)
	mustAdd(t, db, "alice", "t", nil, nil)

	deps := newLogDeps(t, db)
	body := mustFetchActorSingle(t, deps, "alice")

	mustContain(t, body, `href="/log?actor=alice"`)
	mustContain(t, body, `View all in Log`)
}

func TestActorSingle_StatsScopedToThisActorOnly(t *testing.T) {
	db := setupLogTestDB(t)
	now := time.Now()
	// Bob has done 5 tasks recently — none of those should pollute
	// alice's tile counts.
	for i := range 5 {
		shortID := "bob" + strconv.Itoa(i)
		taskID := homeSeedTask(t, db, shortID, "bobs-"+strconv.Itoa(i), "done", now.Add(-15*time.Minute))
		homeSeedEventActor(t, db, taskID, "done", "bob", now.Add(-15*time.Minute))
	}
	// Alice has 1 done in last hour.
	taskID := homeSeedTask(t, db, "ali1", "alice-done", "done", now.Add(-15*time.Minute))
	homeSeedEventActor(t, db, taskID, "done", "alice", now.Add(-15*time.Minute))

	deps := newLogDeps(t, db)
	body := mustFetchActorSingle(t, deps, "alice")

	assertStatTileValue(t, body, "Done 1h", "1")
	assertStatTileValue(t, body, "Done 24h", "1")
}

func TestActorSingle_TimelineExcludesEventsOlderThan24h(t *testing.T) {
	db := setupLogTestDB(t)
	now := time.Now()
	old := homeSeedTask(t, db, "old1", "old-task", "available", now.Add(-30*time.Hour))
	homeSeedEventActor(t, db, old, "created", "alice", now.Add(-30*time.Hour))

	deps := newLogDeps(t, db)
	body := mustFetchActorSingle(t, deps, "alice")

	if strings.Contains(body, `c-actor-timeline__mark--created`) {
		t.Errorf("event 30h ago should not produce a timeline mark")
	}
	mustContain(t, body, `0 events`)
}

func TestActorSingle_TimelineCountsTotalEventsInWindow(t *testing.T) {
	db := setupLogTestDB(t)
	now := time.Now()
	for i := range 3 {
		shortID := "t" + strconv.Itoa(i)
		taskID := homeSeedTask(t, db, shortID, "t-"+strconv.Itoa(i), "available", now.Add(-30*time.Minute))
		homeSeedEventActor(t, db, taskID, "created", "alice", now.Add(-30*time.Minute))
	}

	deps := newLogDeps(t, db)
	body := mustFetchActorSingle(t, deps, "alice")

	mustContain(t, body, `3 events`)
}

func TestActorSingle_404RendersStyledErrorPage(t *testing.T) {
	db := setupLogTestDB(t)
	deps := newLogDeps(t, db)

	code, body := fetchActorSingle(t, deps, "nobody")
	if code != 404 {
		t.Fatalf("status: got %d, want 404", code)
	}
	// Styled page goes through the shared layout (page chrome).
	mustContain(t, body, `c-header`)
	mustContain(t, body, `Actor not found`)
}

func TestActorSingle_ClaimExpiredRendersAsSystem(t *testing.T) {
	db := setupLogTestDB(t)
	id := mustAdd(t, db, "alice", "expired-task", nil, nil)
	taskID := taskIDForShortID(t, db, id)
	if _, err := db.Exec(`INSERT INTO events (task_id, event_type, actor, detail, created_at) VALUES (?, 'claim_expired', 'alice', '', ?)`, taskID, time.Now().Unix()); err != nil {
		t.Fatalf("seed claim_expired: %v", err)
	}

	deps := newLogDeps(t, db)
	body := mustFetchActorSingle(t, deps, "alice")

	// Find the claim_expired row and confirm it renders the system
	// actor + "expired" verb, with no anchor on its actor cell.
	rows := splitLogRows(body)
	var found bool
	for _, row := range rows {
		if !strings.Contains(row, `c-log-row--claim_expired`) {
			continue
		}
		found = true
		if !strings.Contains(row, `c-log-row__actor--system`) {
			t.Errorf("claim_expired row missing system actor marker:\n%s", row)
		}
		if !strings.Contains(row, `>Jobs<`) {
			t.Errorf("claim_expired row should label the actor as Jobs:\n%s", row)
		}
		if !strings.Contains(row, `>expired</span>`) {
			t.Errorf("claim_expired row should render verb text 'expired':\n%s", row)
		}
		if strings.Contains(row, `href="/actors/alice"`) {
			t.Errorf("claim_expired row should not link to the prior claimer:\n%s", row)
		}
	}
	if !found {
		t.Fatalf("no claim_expired row in body:\n%s", body)
	}
}

func TestActorSingle_BlockedTilePositiveCount(t *testing.T) {
	db := setupLogTestDB(t)
	now := time.Now()
	// Two tasks alice claims; both are blocked by an open blocker.
	for i := range 2 {
		shortID := "k" + strconv.Itoa(i)
		blockerID := homeSeedTask(t, db, "B"+strconv.Itoa(i), "blocker-"+strconv.Itoa(i), "available", now.Add(-1*time.Hour))
		taskID := homeSeedTask(t, db, shortID, "stuck-"+strconv.Itoa(i), "claimed", now.Add(-30*time.Minute))
		if _, err := db.Exec(`UPDATE tasks SET claimed_by='alice', claim_expires_at=? WHERE id=?`,
			now.Add(30*time.Minute).Unix(), taskID); err != nil {
			t.Fatalf("set claim: %v", err)
		}
		if _, err := db.Exec(`INSERT INTO blocks (blocker_id, blocked_id, created_at) VALUES (?, ?, ?)`,
			blockerID, taskID, now.Unix()); err != nil {
			t.Fatalf("insert block: %v", err)
		}
		homeSeedEventActor(t, db, taskID, "claimed", "alice", now.Add(-30*time.Minute))
	}

	deps := newLogDeps(t, db)
	body := mustFetchActorSingle(t, deps, "alice")

	assertStatTileValue(t, body, "Blocked", "2")
}

func TestActorSingle_EventListExcludesSoftDeletedTasks(t *testing.T) {
	db := setupLogTestDB(t)
	now := time.Now()
	live := homeSeedTask(t, db, "live1", "live-task", "available", now.Add(-15*time.Minute))
	gone := homeSeedTask(t, db, "gone1", "ghost-task", "available", now.Add(-5*time.Minute))
	homeSeedEventActor(t, db, live, "created", "alice", now.Add(-15*time.Minute))
	homeSeedEventActor(t, db, gone, "created", "alice", now.Add(-5*time.Minute))
	if _, err := db.Exec(`UPDATE tasks SET deleted_at = ? WHERE id = ?`, now.Unix(), gone); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	deps := newLogDeps(t, db)
	body := mustFetchActorSingle(t, deps, "alice")

	mustContain(t, body, `live-task`)
	if strings.Contains(body, `ghost-task`) {
		t.Errorf("soft-deleted task should not appear in the event list")
	}
}

// assertStatTileValue locates a c-actor-stat tile whose label matches
// `label` and asserts its value matches `want`. The DOM order of the
// .c-actor-stat__value followed by .c-actor-stat__label inside the
// same tile lets us scope the value to the right tile.
func assertStatTileValue(t *testing.T, body, label, want string) {
	t.Helper()
	needle := `class="c-actor-stat__label">` + label
	before, _, ok := strings.Cut(body, needle)
	if !ok {
		t.Errorf("missing stat tile %q", label)
		return
	}
	// Walk back to find the preceding c-actor-stat__value.
	prefix := before
	valueOpen := strings.LastIndex(prefix, `class="c-actor-stat__value">`)
	if valueOpen < 0 {
		t.Errorf("no value tag preceding label %q", label)
		return
	}
	valueOpen += len(`class="c-actor-stat__value">`)
	valueEnd := strings.Index(prefix[valueOpen:], `<`)
	if valueEnd < 0 {
		t.Errorf("malformed value tag preceding label %q", label)
		return
	}
	got := strings.TrimSpace(prefix[valueOpen : valueOpen+valueEnd])
	if got != want {
		t.Errorf("%s tile value: got %q, want %q", label, got, want)
	}
}
