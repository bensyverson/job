package handlers

import (
	"log"
	"net/http"
)

// Peek renders the peek sheet body as an HTML fragment at
// /tasks/<id>/peek. No layout chrome (no <html>, no header, no
// footer, no scripts) — the client (peek WebComponent) drops the
// fragment into a sheet element it owns. A non-existent id returns
// a fragment-shaped 404 so the client can drop the response into
// the sheet without doubled chrome.
func Peek(deps Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// loadTaskPageData would write a full styled error page on
		// 404/500 (which goes through the layout). Peek must respond
		// as a fragment regardless, so we do our own resolve and
		// fall back to a fragment error.
		shortID := r.PathValue("id")
		if shortID == "" {
			peekError(deps, w, http.StatusNotFound, "Task not found.")
			return
		}

		// Reuse the same data loader used by the full page so the
		// two views can never drift on the underlying shape. We
		// can't call the standard error helpers because they emit a
		// full styled page; instead we shadow the relevant errors
		// and produce fragment-shaped responses.
		dr := newFragmentResponseRecorder(deps)
		data, ok := loadTaskPageData(deps, dr, r)
		if !ok {
			peekError(deps, w, dr.status, dr.message)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := deps.Templates.RenderFragment(w, "peek", "peek", data); err != nil {
			log.Printf("peek render: %v", err)
			peekError(deps, w, http.StatusInternalServerError, "Could not render peek.")
		}
	})
}

// peekError writes a small fragment-shaped error response. Keeps the
// peek client's swap target consistent — a parsing failure on the
// client should never have to differentiate "fragment" vs "full
// page" depending on whether the lookup hit a 404.
func peekError(deps Deps, w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := deps.Templates.RenderFragment(w, "peek", "peek-error", peekErrorData{Message: message}); err != nil {
		// Last-resort plaintext if even the fragment template is
		// busted — preserves the status code and stays DOM-safe.
		log.Printf("peek error fragment render: %v", err)
		_, _ = w.Write([]byte("<aside class=\"c-peek-sheet\"><p>" + message + "</p></aside>"))
	}
}

type peekErrorData struct {
	Message string
}

// fragmentResponseRecorder is a minimal http.ResponseWriter that
// captures status + a derived message from loadTaskPageData's error
// paths without committing them to the real wire. It only records
// the first WriteHeader + the body; the peek handler then re-emits
// a proper fragment shape.
type fragmentResponseRecorder struct {
	deps    Deps
	status  int
	message string
	header  http.Header
}

func newFragmentResponseRecorder(deps Deps) *fragmentResponseRecorder {
	return &fragmentResponseRecorder{deps: deps, header: http.Header{}}
}

func (r *fragmentResponseRecorder) Header() http.Header { return r.header }

func (r *fragmentResponseRecorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
}

func (r *fragmentResponseRecorder) Write(b []byte) (int, error) {
	// We only need the status — the body emitted by the standard
	// error helpers is full-page HTML and not appropriate to forward
	// here. Default the message based on status; the handler can
	// override before calling peekError if it has better text.
	if r.message == "" {
		switch r.status {
		case http.StatusNotFound:
			r.message = "Task not found."
		case http.StatusInternalServerError:
			r.message = "Could not load this task."
		default:
			r.message = "Unexpected error."
		}
	}
	return len(b), nil
}
