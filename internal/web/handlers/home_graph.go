package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/bensyverson/jobs/internal/web/render"
	"github.com/bensyverson/jobs/internal/web/signals"
)

// HomeGraph powers the scrubber's debounced graph refetch. The
// dashboard's JS replay buffer produces a Frame at any cursor; the
// home-scrub driver projects that frame into a SubwayInput and POSTs
// it here. The server runs the same Subway core /home runs and
// returns just the c-mini-graph fragment, so the driver can swap a
// historical graph in without bloating the JS bundle with a port of
// the layout pipeline.
//
// The reducer therefore stays in JS only — the frame is the single
// source of truth for "world at event N." This handler just rebuilds
// a graphWorld from the supplied bundle and runs through
// BuildSubwayFromInput → render.LayoutSubway → home_graph template.
func HomeGraph(deps Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var in signals.SubwayInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		view := render.LayoutSubway(signals.BuildSubwayFromInput(in))
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := deps.Templates.RenderFragment(w, "home", "home_graph", view); err != nil {
			InternalError(deps, w, "render home_graph", err)
			return
		}
	})
}
