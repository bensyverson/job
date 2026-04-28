package handlers_test

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bensyverson/jobs/internal/web/handlers"
)

func TestNotFound_RendersHTMLWith404Status(t *testing.T) {
	db := setupLogTestDB(t)
	deps := newLogDeps(t, db)

	req := httptest.NewRequest("GET", "/some-random-path", nil)
	w := httptest.NewRecorder()
	handlers.NotFound(deps).ServeHTTP(w, req)

	if w.Code != 404 {
		t.Errorf("status = %d, want 404", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	body := w.Body.String()
	mustContain(t, body, "Page not found")
	mustContain(t, body, "Error 404")
	mustContain(t, body, `href="/"`) // "Back to home" CTA is present.
}

func TestRenderError_CustomMessage(t *testing.T) {
	db := setupLogTestDB(t)
	deps := newLogDeps(t, db)
	w := httptest.NewRecorder()
	handlers.RenderError(deps, w, 418, "Teapot", "short and stout")
	if w.Code != 418 {
		t.Errorf("status = %d, want 418", w.Code)
	}
	body := w.Body.String()
	mustContain(t, body, "Teapot")
	mustContain(t, body, "short and stout")
	mustContain(t, body, "Error 418")
}

func TestIsDBLocked_DetectsBusyAndLockedMessages(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"unrelated", errors.New("disk full"), false},
		{"locked literal", errors.New("database is locked"), true},
		{"locked wrapped", errors.New("get task: database is locked"), true},
		{"sqlite busy text", errors.New("The database file is locked (SQLITE_BUSY)"), true},
		{"table locked", errors.New("database table is locked"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := handlers.IsDBLocked(tc.err); got != tc.want {
				t.Errorf("IsDBLocked(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestInternalError_DBLockRendersBusy503(t *testing.T) {
	db := setupLogTestDB(t)
	deps := newLogDeps(t, db)
	w := httptest.NewRecorder()
	handlers.InternalError(deps, w, "task lookup", errors.New("database is locked"))

	if w.Code != 503 {
		t.Errorf("status = %d, want 503 for db-locked", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "store is busy") && !strings.Contains(body, "Store is busy") {
		t.Errorf("missing busy-store copy; body=%s", body)
	}
	mustContain(t, body, "Error 503")
}

func TestInternalError_NonLockKeeps500(t *testing.T) {
	db := setupLogTestDB(t)
	deps := newLogDeps(t, db)
	w := httptest.NewRecorder()
	handlers.InternalError(deps, w, "task lookup", errors.New("disk full"))

	if w.Code != 500 {
		t.Errorf("status = %d, want 500 for generic error", w.Code)
	}
	mustContain(t, w.Body.String(), "Error 500")
}
