package handlers

import "net/http"

// Actors renders the per-actor column view and timeline strip. Also
// serves /actors/{name}. See vision §3.3.
func Actors(deps Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "actors view not yet implemented", http.StatusNotImplemented)
	})
}
