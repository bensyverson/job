package handlers

import (
	"net/http"

	"github.com/bensyverson/jobs/internal/web/templates"
)

// Home renders the landing "Now" view with signal cards and the
// mini-graph. See vision §3.1. Currently a chrome-only placeholder;
// the signal cards and mini-graph arrive with the Home-view task.
func Home(deps Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		renderPage(deps, w, "home", templates.Chrome{ActiveTab: "home"})
	})
}
