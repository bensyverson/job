package handlers_test

import (
	"database/sql"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bensyverson/jobs/internal/web/handlers"
)

func fetchHome(t *testing.T, deps handlers.Deps) string {
	t.Helper()
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handlers.Home(deps).ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("GET /: status %d, body=%s", w.Code, w.Body.String())
	}
	return w.Body.String()
}

func homeSeedTask(t *testing.T, db *sql.DB, shortID, title, status string, createdAt time.Time) int64 {
	t.Helper()
	res, err := db.Exec(`
		INSERT INTO tasks (short_id, title, description, status, sort_order, created_at, updated_at)
		VALUES (?, ?, '', ?, 0, ?, ?)
	`, shortID, title, status, createdAt.Unix(), createdAt.Unix())
	if err != nil {
		t.Fatalf("seed task: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}
	return id
}

func homeSeedEvent(t *testing.T, db *sql.DB, taskID int64, eventType string, at time.Time) {
	t.Helper()
	homeSeedEventActor(t, db, taskID, eventType, "u", at)
}

func homeSeedEventActor(t *testing.T, db *sql.DB, taskID int64, eventType, actor string, at time.Time) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO events (task_id, event_type, actor, detail, created_at)
		VALUES (?, ?, ?, '', ?)
	`, taskID, eventType, actor, at.Unix())
	if err != nil {
		t.Fatalf("seed event: %v", err)
	}
}

func homeSeedClaim(t *testing.T, db *sql.DB, taskID int64, actor string, claimedAt time.Time) {
	t.Helper()
	expiresAt := claimedAt.Unix() + 1800
	_, err := db.Exec(`
		UPDATE tasks SET status = 'claimed', claimed_by = ?, claim_expires_at = ?
		WHERE id = ?
	`, actor, expiresAt, taskID)
	if err != nil {
		t.Fatalf("seed claim update: %v", err)
	}
	_, err = db.Exec(`
		INSERT INTO events (task_id, event_type, actor, detail, created_at)
		VALUES (?, 'claimed', ?, '', ?)
	`, taskID, actor, claimedAt.Unix())
	if err != nil {
		t.Fatalf("seed claim event: %v", err)
	}
}

func homeSeedBlock(t *testing.T, db *sql.DB, blockedID, blockerID int64, at time.Time) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO blocks (blocker_id, blocked_id, created_at)
		VALUES (?, ?, ?)
	`, blockerID, blockedID, at.Unix())
	if err != nil {
		t.Fatalf("seed block: %v", err)
	}
}

func TestHome_RendersFourSignalCards(t *testing.T) {
	db := setupLogTestDB(t)
	deps := newLogDeps(t, db)

	body := fetchHome(t, deps)

	mustContain(t, body, `class="c-grid-signals"`)

	// One activity card + three alarm cards = four total.
	cardRe := regexp.MustCompile(`class="c-signal-card c-signal-card--`)
	matches := cardRe.FindAllStringIndex(body, -1)
	if len(matches) != 4 {
		t.Errorf("c-signal-card article count: got %d, want 4", len(matches))
	}

	mustContain(t, body, `Activity`)
	mustContain(t, body, `Newly blocked`)
	mustContain(t, body, `Longest active claim`)
	mustContain(t, body, `Oldest todo`)
}

func TestHome_ActivityHistogram_RendersBarsAndLegend(t *testing.T) {
	db := setupLogTestDB(t)
	deps := newLogDeps(t, db)

	now := time.Now()
	tID := homeSeedTask(t, db, "a", "a", "available", now.Add(-2*time.Hour))
	// Seed a mix inside the 60m window so the legend has real numbers.
	homeSeedEvent(t, db, tID, "created", now.Add(-50*time.Minute))
	homeSeedEvent(t, db, tID, "claimed", now.Add(-30*time.Minute))
	homeSeedEvent(t, db, tID, "done", now.Add(-10*time.Minute))
	homeSeedEvent(t, db, tID, "blocked", now.Add(-2*time.Minute))

	body := fetchHome(t, deps)

	// All 60 bar slots rendered for layout stability — count the bar class.
	barRe := regexp.MustCompile(`class="c-histogram__bar`)
	matches := barRe.FindAllStringIndex(body, -1)
	if len(matches) != 60 {
		t.Errorf("c-histogram__bar count: got %d, want 60", len(matches))
	}

	// At least one non-empty bar has an inline --h style.
	if !strings.Contains(body, `style="--h:`) {
		t.Errorf("expected at least one bar with inline --h; body snippet:\n%s",
			bodyExcerpt(body, "c-histogram", 600))
	}

	// Legend totals: 1 each of done/claim/create/block.
	mustContain(t, body, `c-hist-swatch--done`)
	mustContain(t, body, `c-hist-swatch--claim`)
	mustContain(t, body, `c-hist-swatch--create`)
	mustContain(t, body, `c-hist-swatch--block`)
	mustContain(t, body, `>1 done<`)
	mustContain(t, body, `>1 claimed<`)
	mustContain(t, body, `>1 new<`)
	mustContain(t, body, `>1 blocked<`)
}

func TestHome_NewlyBlocked_RendersCountAndProgress(t *testing.T) {
	db := setupLogTestDB(t)
	deps := newLogDeps(t, db)

	now := time.Now()
	aID := homeSeedTask(t, db, "a", "alpha", "available", now.Add(-1*time.Hour))
	bID := homeSeedTask(t, db, "b", "beta", "available", now.Add(-1*time.Hour))
	cID := homeSeedTask(t, db, "c", "charlie", "available", now.Add(-1*time.Hour))
	// Two edges inside the 10m window.
	homeSeedBlock(t, db, bID, aID, now.Add(-5*time.Minute))
	homeSeedBlock(t, db, cID, aID, now.Add(-2*time.Minute))

	body := fetchHome(t, deps)

	// The card's value cell carries the count.
	mustContain(t, body, `class="c-signal-card__value">2<`)

	// --progress: 40% (2/5 threshold).
	if !strings.Contains(body, `--progress: 40%`) {
		t.Errorf("expected --progress: 40%% for newly-blocked 2/5\n%s",
			bodyExcerpt(body, "Newly blocked", 500))
	}

	// Context line mentions the blocked id + the waiting-on id.
	mustContain(t, body, `href="/tasks/c"`)
	mustContain(t, body, `href="/tasks/a"`)
}

func TestHome_NewlyBlocked_EmptyState(t *testing.T) {
	db := setupLogTestDB(t)
	deps := newLogDeps(t, db)

	body := fetchHome(t, deps)

	// Zero count with zero progress.
	mustContain(t, body, `class="c-signal-card__value">0<`)
	mustContain(t, body, `--progress: 0%`)
}

func TestHome_LongestClaim_RendersDurationAndActor(t *testing.T) {
	db := setupLogTestDB(t)
	deps := newLogDeps(t, db)

	now := time.Now()
	tID := homeSeedTask(t, db, "g7h8i", "SSR fragment endpoint", "available", now.Add(-1*time.Hour))
	homeSeedClaim(t, db, tID, "alice", now.Add(-8*time.Minute-47*time.Second))

	body := fetchHome(t, deps)

	// Duration with minute+second precision, actor name, link to task.
	mustContain(t, body, `>8m 47s<`)
	mustContain(t, body, `data-actor="alice"`)
	mustContain(t, body, `href="/tasks/g7h8i"`)

	// Progress = 8m47s / 30m ≈ 29% → rounds to 29.
	re := regexp.MustCompile(`--progress: (\d+)%`)
	found := false
	for _, m := range re.FindAllStringSubmatch(body, -1) {
		if m[1] == "29" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected --progress: 29%% for 8m47s over 30m threshold\n%s",
			bodyExcerpt(body, "Longest active claim", 500))
	}
}

func TestHome_LongestClaim_AbsentRendersEmDash(t *testing.T) {
	db := setupLogTestDB(t)
	deps := newLogDeps(t, db)

	body := fetchHome(t, deps)

	// When no claims exist, the value cell shows an em dash.
	idx := strings.Index(body, "Longest active claim")
	if idx < 0 {
		t.Fatal("card label not found")
	}
	snippet := body[idx:min(idx+400, len(body))]
	if !strings.Contains(snippet, `—`) {
		t.Errorf("expected em dash in absent-claim value\n%s", snippet)
	}
}

func TestHome_OldestTodo_RendersAgeAndLink(t *testing.T) {
	db := setupLogTestDB(t)
	deps := newLogDeps(t, db)

	now := time.Now()
	homeSeedTask(t, db, "mVc43", "Migrate config to sqlite", "available", now.Add(-3*24*time.Hour))

	body := fetchHome(t, deps)

	mustContain(t, body, `href="/tasks/mVc43"`)
	mustContain(t, body, `Migrate config to sqlite`)
	// "3d" from RelativeTime, not "72h".
	mustContain(t, body, `>3d<`)
}

func TestHome_OldestTodo_AbsentRendersEmDash(t *testing.T) {
	db := setupLogTestDB(t)
	deps := newLogDeps(t, db)

	body := fetchHome(t, deps)

	idx := strings.Index(body, "Oldest todo")
	if idx < 0 {
		t.Fatal("card label not found")
	}
	snippet := body[idx:min(idx+400, len(body))]
	if !strings.Contains(snippet, `—`) {
		t.Errorf("expected em dash in absent-todo value\n%s", snippet)
	}
}

// ------------------------------------------------------------------
// Active claims panel
// ------------------------------------------------------------------

func TestHome_ActiveClaims_EmptyStateRendersMessage(t *testing.T) {
	db := setupLogTestDB(t)
	deps := newLogDeps(t, db)

	body := fetchHome(t, deps)

	mustContain(t, body, `id="p-claims"`)
	mustContain(t, body, `Active claims`)
	// Empty-state copy and "0 in flight" meta.
	mustContain(t, body, `0 in flight`)
	mustContain(t, body, `No active claims`)
}

func TestHome_ActiveClaims_RendersOneRowPerActiveClaim(t *testing.T) {
	db := setupLogTestDB(t)
	deps := newLogDeps(t, db)

	now := time.Now()
	aID := homeSeedTask(t, db, "g7h8i", "SSR fragment endpoint", "available", now.Add(-1*time.Hour))
	bID := homeSeedTask(t, db, "kQn2r", "Broadcaster fan-out", "available", now.Add(-1*time.Hour))
	cID := homeSeedTask(t, db, "Wq7uV", "Actor column layout", "available", now.Add(-1*time.Hour))
	homeSeedClaim(t, db, aID, "alice", now.Add(-8*time.Minute-47*time.Second))
	homeSeedClaim(t, db, bID, "bob", now.Add(-3*time.Minute-12*time.Second))
	homeSeedClaim(t, db, cID, "dmitri", now.Add(-42*time.Second))

	// A done task with an old claim event — must not appear.
	doneID := homeSeedTask(t, db, "old", "old done", "done", now.Add(-2*time.Hour))
	homeSeedEvent(t, db, doneID, "claimed", now.Add(-1*time.Hour))

	body := fetchHome(t, deps)

	mustContain(t, body, `3 in flight`)

	// One c-panel-row per active claim. Match the row div's class
	// exactly (the "c-panel-row__title" / "c-panel-row__meta"
	// children are excluded by the closing-quote anchor).
	re := regexp.MustCompile(`class="c-panel-row"`)
	rows := re.FindAllStringIndex(body, -1)
	// Count only rows inside the claims panel by slicing from the
	// panel anchor forward.
	claimsStart := strings.Index(body, `id="p-claims"`)
	if claimsStart < 0 {
		t.Fatal("claims panel not found")
	}
	// Next panel or end of main — everything between is the claims panel.
	panelEnd := strings.Index(body[claimsStart:], `</section>`)
	panelSection := body[claimsStart : claimsStart+panelEnd]
	rowsInPanel := re.FindAllStringIndex(panelSection, -1)
	if len(rowsInPanel) != 3 {
		t.Errorf("rows inside claims panel: got %d, want 3 (total in body: %d)\n%s",
			len(rowsInPanel), len(rows), panelSection)
	}

	// Task details present.
	mustContain(t, body, `>SSR fragment endpoint<`)
	mustContain(t, body, `>Broadcaster fan-out<`)
	mustContain(t, body, `>Actor column layout<`)
	mustContain(t, body, `data-actor="alice"`)
	mustContain(t, body, `data-actor="bob"`)
	mustContain(t, body, `data-actor="dmitri"`)
	mustContain(t, body, `href="/actors/alice"`)
}

func TestHome_ActiveClaims_OrdersOldestFirst(t *testing.T) {
	db := setupLogTestDB(t)
	deps := newLogDeps(t, db)

	now := time.Now()
	youngID := homeSeedTask(t, db, "young", "young", "available", now.Add(-1*time.Hour))
	midID := homeSeedTask(t, db, "mid", "mid", "available", now.Add(-1*time.Hour))
	oldID := homeSeedTask(t, db, "old", "old", "available", now.Add(-1*time.Hour))
	homeSeedClaim(t, db, youngID, "a", now.Add(-10*time.Second))
	homeSeedClaim(t, db, midID, "a", now.Add(-5*time.Minute))
	homeSeedClaim(t, db, oldID, "a", now.Add(-15*time.Minute))

	body := fetchHome(t, deps)

	// Within the claims panel, the row order is old → mid → young.
	claimsStart := strings.Index(body, `id="p-claims"`)
	panelEnd := strings.Index(body[claimsStart:], `</section>`)
	section := body[claimsStart : claimsStart+panelEnd]

	oldIdx := strings.Index(section, "/tasks/old")
	midIdx := strings.Index(section, "/tasks/mid")
	youngIdx := strings.Index(section, "/tasks/young")
	if oldIdx < 0 || midIdx < 0 || youngIdx < 0 {
		t.Fatalf("missing row: old=%d mid=%d young=%d\n%s", oldIdx, midIdx, youngIdx, section)
	}
	if !(oldIdx < midIdx && midIdx < youngIdx) {
		t.Errorf("row order wrong: old=%d mid=%d young=%d, want old < mid < young",
			oldIdx, midIdx, youngIdx)
	}
}

func TestHome_ActiveClaims_IncludesClaimedAtForLiveTicker(t *testing.T) {
	db := setupLogTestDB(t)
	deps := newLogDeps(t, db)

	now := time.Now()
	tID := homeSeedTask(t, db, "t", "t", "available", now.Add(-1*time.Hour))
	claimedAt := now.Add(-5 * time.Minute)
	homeSeedClaim(t, db, tID, "alice", claimedAt)

	body := fetchHome(t, deps)

	// Row carries data-claimed-at so home-live.js can tick the idle
	// timer between server refreshes.
	needle := `data-claimed-at="` + strconv.FormatInt(claimedAt.Unix(), 10) + `"`
	if !strings.Contains(body, needle) {
		t.Errorf("expected %s on claim row\n%s", needle, bodyExcerpt(body, "id=\"p-claims\"", 900))
	}
}

// ------------------------------------------------------------------
// Recent completions panel
// ------------------------------------------------------------------

func TestHome_RecentCompletions_EmptyState(t *testing.T) {
	db := setupLogTestDB(t)
	deps := newLogDeps(t, db)

	body := fetchHome(t, deps)

	mustContain(t, body, `id="p-recent"`)
	mustContain(t, body, `Recent completions`)
	mustContain(t, body, `No recent completions`)
}

func TestHome_RecentCompletions_RendersDoneAndCanceled(t *testing.T) {
	db := setupLogTestDB(t)
	deps := newLogDeps(t, db)

	now := time.Now()
	doneID := homeSeedTask(t, db, "dn", "Shipped thing", "done", now.Add(-1*time.Hour))
	cancelID := homeSeedTask(t, db, "cn", "Abandoned thing", "canceled", now.Add(-1*time.Hour))
	// done event 5m ago, canceled event 2m ago. Expect both.
	homeSeedEventActor(t, db, doneID, "done", "alice", now.Add(-5*time.Minute))
	homeSeedEventActor(t, db, cancelID, "canceled", "bob", now.Add(-2*time.Minute))

	// A 'noted' event — must NOT appear in completions.
	otherID := homeSeedTask(t, db, "nt", "mid-work", "available", now.Add(-1*time.Hour))
	homeSeedEventActor(t, db, otherID, "noted", "alice", now.Add(-1*time.Minute))

	body := fetchHome(t, deps)

	recentStart := strings.Index(body, `id="p-recent"`)
	if recentStart < 0 {
		t.Fatal("recent panel not found")
	}
	panelEnd := strings.Index(body[recentStart:], `</section>`)
	section := body[recentStart : recentStart+panelEnd]

	mustContain(t, section, `>Shipped thing<`)
	mustContain(t, section, `>Abandoned thing<`)
	mustContain(t, section, `data-actor="alice"`)
	mustContain(t, section, `data-actor="bob"`)
	mustContain(t, section, `href="/tasks/dn"`)
	mustContain(t, section, `href="/tasks/cn"`)

	if strings.Contains(section, ">mid-work<") {
		t.Errorf("recent completions should not list non-terminal events")
	}
}

func TestHome_RecentCompletions_OrdersOldestFirst(t *testing.T) {
	db := setupLogTestDB(t)
	deps := newLogDeps(t, db)

	now := time.Now()
	oldID := homeSeedTask(t, db, "old", "oldest", "done", now.Add(-1*time.Hour))
	midID := homeSeedTask(t, db, "mid", "middle", "done", now.Add(-1*time.Hour))
	newID := homeSeedTask(t, db, "new", "newest", "done", now.Add(-1*time.Hour))
	homeSeedEventActor(t, db, oldID, "done", "a", now.Add(-12*time.Minute))
	homeSeedEventActor(t, db, midID, "done", "a", now.Add(-5*time.Minute))
	homeSeedEventActor(t, db, newID, "done", "a", now.Add(-1*time.Minute))

	body := fetchHome(t, deps)

	recentStart := strings.Index(body, `id="p-recent"`)
	panelEnd := strings.Index(body[recentStart:], `</section>`)
	section := body[recentStart : recentStart+panelEnd]

	oldIdx := strings.Index(section, "/tasks/old")
	midIdx := strings.Index(section, "/tasks/mid")
	newIdx := strings.Index(section, "/tasks/new")
	if oldIdx < 0 || midIdx < 0 || newIdx < 0 {
		t.Fatalf("missing row: old=%d mid=%d new=%d\n%s", oldIdx, midIdx, newIdx, section)
	}
	if !(oldIdx < midIdx && midIdx < newIdx) {
		t.Errorf("row order: old=%d mid=%d new=%d; want old < mid < new",
			oldIdx, midIdx, newIdx)
	}
}

func TestHome_RecentCompletions_CapsToLimitKeepingNewest(t *testing.T) {
	db := setupLogTestDB(t)
	deps := newLogDeps(t, db)

	now := time.Now()
	// Seed 12 completions; only the 10 newest should appear.
	for i := 0; i < 12; i++ {
		sid := "t" + strconv.Itoa(i)
		tID := homeSeedTask(t, db, sid, sid, "done", now.Add(-2*time.Hour))
		// i=0 is oldest (completed 12m ago); i=11 is newest (1m ago).
		ago := time.Duration(12-i) * time.Minute
		homeSeedEventActor(t, db, tID, "done", "a", now.Add(-ago))
	}

	body := fetchHome(t, deps)

	recentStart := strings.Index(body, `id="p-recent"`)
	panelEnd := strings.Index(body[recentStart:], `</section>`)
	section := body[recentStart : recentStart+panelEnd]

	re := regexp.MustCompile(`class="c-panel-row"`)
	rows := re.FindAllStringIndex(section, -1)
	if len(rows) != 10 {
		t.Errorf("row count: got %d, want 10 (limit)", len(rows))
	}
	// Oldest two (t0, t1) are dropped; t2..t11 remain.
	if strings.Contains(section, "/tasks/t0") {
		t.Errorf("t0 (oldest) should be trimmed")
	}
	if !strings.Contains(section, "/tasks/t2") {
		t.Errorf("t2 should remain as oldest of the retained set")
	}
	if !strings.Contains(section, "/tasks/t11") {
		t.Errorf("t11 (newest) should remain")
	}
}

// ------------------------------------------------------------------
// Blocked strip panel
// ------------------------------------------------------------------

func TestHome_Blocked_EmptyState(t *testing.T) {
	db := setupLogTestDB(t)
	deps := newLogDeps(t, db)

	body := fetchHome(t, deps)

	mustContain(t, body, `id="p-blocked"`)
	mustContain(t, body, `Blocked`)
	mustContain(t, body, `No blocked tasks`)
	mustContain(t, body, `0 waiting`)
}

func TestHome_Blocked_ListsBlockedWithBlockers(t *testing.T) {
	db := setupLogTestDB(t)
	deps := newLogDeps(t, db)

	now := time.Now()
	blockerA := homeSeedTask(t, db, "blkA", "Blocker A", "available", now.Add(-1*time.Hour))
	blockerB := homeSeedTask(t, db, "blkB", "Blocker B", "claimed", now.Add(-1*time.Hour))
	blocked1 := homeSeedTask(t, db, "b1", "Timeline strip", "available", now.Add(-1*time.Hour))
	blocked2 := homeSeedTask(t, db, "b2", "Error page styling", "available", now.Add(-1*time.Hour))

	homeSeedBlock(t, db, blocked1, blockerA, now.Add(-10*time.Minute))
	homeSeedBlock(t, db, blocked2, blockerA, now.Add(-5*time.Minute))
	homeSeedBlock(t, db, blocked2, blockerB, now.Add(-4*time.Minute))

	body := fetchHome(t, deps)

	blockedStart := strings.Index(body, `id="p-blocked"`)
	if blockedStart < 0 {
		t.Fatal("blocked panel not found")
	}
	panelEnd := strings.Index(body[blockedStart:], `</section>`)
	section := body[blockedStart : blockedStart+panelEnd]

	mustContain(t, section, `2 waiting`)
	mustContain(t, section, `>Timeline strip<`)
	mustContain(t, section, `>Error page styling<`)

	// Each blocked task row links to the blocker(s) via pill(s).
	mustContain(t, section, `href="/tasks/blkA"`)
	mustContain(t, section, `href="/tasks/blkB"`)
	// "waiting on" phrasing appears in each row's meta line.
	if n := strings.Count(section, "waiting on"); n != 2 {
		t.Errorf(`"waiting on" count: got %d, want 2`, n)
	}
}

func TestHome_Blocked_ExcludesDoneBlockers(t *testing.T) {
	db := setupLogTestDB(t)
	deps := newLogDeps(t, db)

	now := time.Now()
	// Blocker has already been done — its edge should be treated as
	// resolved, so the blocked task is no longer blocked.
	doneBlocker := homeSeedTask(t, db, "db", "done blocker", "done", now.Add(-1*time.Hour))
	freeTask := homeSeedTask(t, db, "ft", "should-be-free", "available", now.Add(-1*time.Hour))
	homeSeedBlock(t, db, freeTask, doneBlocker, now.Add(-5*time.Minute))

	// A separate, real block edge so the panel has at least one row.
	liveBlocker := homeSeedTask(t, db, "lb", "still blocking", "available", now.Add(-1*time.Hour))
	stuckTask := homeSeedTask(t, db, "st", "stuck task", "available", now.Add(-1*time.Hour))
	homeSeedBlock(t, db, stuckTask, liveBlocker, now.Add(-5*time.Minute))

	body := fetchHome(t, deps)

	blockedStart := strings.Index(body, `id="p-blocked"`)
	panelEnd := strings.Index(body[blockedStart:], `</section>`)
	section := body[blockedStart : blockedStart+panelEnd]

	mustContain(t, section, `1 waiting`)
	if strings.Contains(section, "should-be-free") {
		t.Errorf("should-be-free is no longer blocked (its blocker is done)")
	}
	mustContain(t, section, `>stuck task<`)
}

func TestHome_Blocked_ExcludesDoneOrCanceledBlockedTasks(t *testing.T) {
	db := setupLogTestDB(t)
	deps := newLogDeps(t, db)

	now := time.Now()
	blocker := homeSeedTask(t, db, "blk", "blocker", "available", now.Add(-1*time.Hour))
	// The "blocked" task has already been done — it's off the board.
	alreadyDone := homeSeedTask(t, db, "ad", "already done", "done", now.Add(-1*time.Hour))
	homeSeedBlock(t, db, alreadyDone, blocker, now.Add(-5*time.Minute))

	body := fetchHome(t, deps)

	blockedStart := strings.Index(body, `id="p-blocked"`)
	panelEnd := strings.Index(body[blockedStart:], `</section>`)
	section := body[blockedStart : blockedStart+panelEnd]

	mustContain(t, section, `0 waiting`)
	if strings.Contains(section, "already done") {
		t.Errorf("done tasks should not appear in the blocked strip")
	}
}

func TestHome_Blocked_MultipleBlockersRenderAsMultipleLinks(t *testing.T) {
	db := setupLogTestDB(t)
	deps := newLogDeps(t, db)

	now := time.Now()
	b1 := homeSeedTask(t, db, "one", "one", "available", now.Add(-1*time.Hour))
	b2 := homeSeedTask(t, db, "two", "two", "available", now.Add(-1*time.Hour))
	b3 := homeSeedTask(t, db, "three", "three", "available", now.Add(-1*time.Hour))
	stuck := homeSeedTask(t, db, "stk", "review gate", "available", now.Add(-1*time.Hour))
	homeSeedBlock(t, db, stuck, b1, now.Add(-5*time.Minute))
	homeSeedBlock(t, db, stuck, b2, now.Add(-4*time.Minute))
	homeSeedBlock(t, db, stuck, b3, now.Add(-3*time.Minute))

	body := fetchHome(t, deps)

	mustContain(t, body, `href="/tasks/one"`)
	mustContain(t, body, `href="/tasks/two"`)
	mustContain(t, body, `href="/tasks/three"`)
}

// bodyExcerpt returns `n` chars around the first occurrence of `anchor`
// for a more helpful test diff.
func bodyExcerpt(body, anchor string, n int) string {
	idx := strings.Index(body, anchor)
	if idx < 0 {
		if len(body) < n {
			return body
		}
		return body[:n]
	}
	start := idx - n/2
	if start < 0 {
		start = 0
	}
	end := start + n
	if end > len(body) {
		end = len(body)
	}
	return body[start:end]
}
