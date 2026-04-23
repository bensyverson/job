package handlers

import "net/http"

// Plan renders the tree/outline view. Also serves /plan/{id} and
// /labels/{name}. See vision §3.2.
func Plan(deps Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "plan view not yet implemented", http.StatusNotImplemented)
	})
}
