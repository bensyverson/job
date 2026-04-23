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

func TestLog_ChipsPreserveOtherFilters(t *testing.T) {
	db := setupLogTestDB(t)
	mustAdd(t, db, "alice", "A task", nil, nil)
	deps := newLogDeps(t, db)

	// With ?actor=alice, the type chips should encode &actor=alice.
	body := fetchLog(t, deps, "actor=alice")
	mustContain(t, body, `/log?actor=alice&amp;type=claimed`)
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

func mustContain(t *testing.T, body, needle string) {
	t.Helper()
	if !strings.Contains(body, needle) {
		t.Errorf("missing %q in body\n---\n%s", needle, body)
	}
}
