package handlers

import (
	"log"
	"net/http"

	"github.com/bensyverson/jobs/internal/web/templates"
)

// ErrorPageData drives the shared error template. Status is numeric
// (used in page title); Title and Message are the user-facing copy.
type ErrorPageData struct {
	templates.Chrome
	Status  int
	Title   string
	Message string
}

// RenderError writes a templated error response. Falls back to plain
// text if the template engine itself errors, so a broken template
// can't turn a 404 into a blank screen.
func RenderError(deps Deps, w http.ResponseWriter, status int, title, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	data := ErrorPageData{
		Chrome:  templates.Chrome{ActiveTab: ""},
		Status:  status,
		Title:   title,
		Message: message,
	}
	if err := deps.Templates.Render(w, "error", data); err != nil {
		log.Printf("error template failed: %v", err)
		http.Error(w, title+": "+message, status)
	}
}

// NotFound returns a handler that serves a templated 404 for any
// request. Intended as the router's catch-all for unmatched paths
// (mounted on "GET /"); the specific "GET /{$}" home pattern wins
// for the exact root.
func NotFound(deps Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		RenderError(deps, w,
			http.StatusNotFound,
			"Page not found",
			"The page you're looking for isn't here. Try one of the tabs above.",
		)
	})
}

// InternalError renders a 500 page using the shared error template.
// Logs the underlying error server-side; the user sees a calm message
// without implementation detail.
func InternalError(deps Deps, w http.ResponseWriter, context string, err error) {
	log.Printf("internal error (%s): %v", context, err)
	RenderError(deps, w,
		http.StatusInternalServerError,
		"Something went wrong",
		"We couldn't complete that request. Reload the page; if this persists, check the server logs.",
	)
}
