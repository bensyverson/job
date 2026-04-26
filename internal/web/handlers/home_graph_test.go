package handlers_test

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bensyverson/jobs/internal/web/handlers"
)

// POST /home/graph: scrubber's debounced graph refetch lands here.
// JS sends {tasks, blocks} from the cursor's frame, server runs the
// same Subway core /home runs, returns the c-mini-graph fragment.

func postHomeGraph(t *testing.T, deps handlers.Deps, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/home/graph", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handlers.HomeGraph(deps).ServeHTTP(w, req)
	return w
}

func TestHomeGraph_EmptyInputRendersEmptyState(t *testing.T) {
	deps := newLogDeps(t, setupLogTestDB(t))
	w := postHomeGraph(t, deps, `{"tasks":[],"blocks":[]}`)
	if w.Code != 200 {
		t.Fatalf("status: got %d, want 200; body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "c-mini-graph") {
		t.Errorf("missing c-mini-graph wrapper; body=%s", body)
	}
	if !strings.Contains(body, "No active claims or upcoming work") {
		t.Errorf("missing empty state; body=%s", body)
	}
	if strings.Contains(body, "c-graph-canvas") {
		t.Errorf("empty input should not render canvas; body=%s", body)
	}
}

func TestHomeGraph_ClaimedTaskRendersCanvas(t *testing.T) {
	deps := newLogDeps(t, setupLogTestDB(t))
	// Minimal graph: parent ph3 with three children, st2 claimed by alice.
	body := `{
		"tasks":[
			{"shortId":"ph3","title":"Phase 3","status":"available","sortOrder":2},
			{"shortId":"st1","title":"Step 1","status":"done","parentShortId":"ph3","sortOrder":1},
			{"shortId":"st2","title":"Step 2","status":"claimed","parentShortId":"ph3","sortOrder":2,"claimedBy":"alice"},
			{"shortId":"st3","title":"Step 3","status":"available","parentShortId":"ph3","sortOrder":3}
		],
		"blocks":[]
	}`
	w := postHomeGraph(t, deps, body)
	if w.Code != 200 {
		t.Fatalf("status: got %d, want 200; body=%s", w.Code, w.Body.String())
	}
	resp := w.Body.String()
	for _, want := range []string{
		`c-graph-canvas`,
		`c-graph-node--active`,
		`data-task-id="st2"`,
		`data-actor="alice"`,
		`c-graph-edge`,
	} {
		if !strings.Contains(resp, want) {
			t.Errorf("missing %q in fragment; body=%s", want, resp)
		}
	}
}

func TestHomeGraph_BadJSONReturns400(t *testing.T) {
	deps := newLogDeps(t, setupLogTestDB(t))
	w := postHomeGraph(t, deps, `{not json`)
	if w.Code != 400 {
		t.Fatalf("status: got %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestHomeGraph_RejectsNonPOST(t *testing.T) {
	deps := newLogDeps(t, setupLogTestDB(t))
	req := httptest.NewRequest("GET", "/home/graph", nil)
	w := httptest.NewRecorder()
	handlers.HomeGraph(deps).ServeHTTP(w, req)
	if w.Code != 405 {
		t.Fatalf("status: got %d, want 405; body=%s", w.Code, w.Body.String())
	}
}
