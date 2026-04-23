package handlers

import "net/http"

// Events serves the /events endpoint: SSE live tail when the client
// sends Accept: text/event-stream, JSON replay otherwise. See vision
// §6.2 and §6.3.
func Events(deps Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "events endpoint not yet implemented", http.StatusNotImplemented)
	})
}
