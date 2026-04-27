package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	job "github.com/bensyverson/jobs/internal/job"
)

// SearchResult is the JSON payload for a single search hit.
type SearchResult struct {
	ShortID       string `json:"short_id"`
	Title         string `json:"title"`
	Status        string `json:"status"`
	DisplayStatus string `json:"display_status"`
	URL           string `json:"url"`
}

// Search returns JSON search results for GET /search?q=...&limit=N.
func Search(deps Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		limitStr := r.URL.Query().Get("limit")
		limit := 20
		if limitStr != "" {
			if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
				limit = n
			}
		}

		hits, err := job.RunSearch(deps.DB, q, limit)
		if err != nil {
			InternalError(deps, w, "search", err)
			return
		}

		// Build a display-status map: we need blocker info for "available"
		// and "claimed" tasks to know if they render as "blocked".
		ids := make([]int64, len(hits))
		for i, h := range hits {
			ids[i] = h.ID
		}
		blockerMap := map[int64][]string{}
		if len(ids) > 0 {
			bm, err := job.GetBlockersForTaskIDs(deps.DB, ids)
			if err != nil {
				InternalError(deps, w, "search blockers", err)
				return
			}
			blockerMap = bm
		}

		results := make([]SearchResult, len(hits))
		for i, h := range hits {
			hasBlockers := len(blockerMap[h.ID]) > 0
			results[i] = SearchResult{
				ShortID:       h.ShortID,
				Title:         h.Title,
				Status:        h.Status,
				DisplayStatus: DisplayStatus(h.Status, hasBlockers),
				URL:           "/tasks/" + h.ShortID,
			}
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(results)
	})
}
