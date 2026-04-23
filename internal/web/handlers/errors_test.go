package handlers_test

import (
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
