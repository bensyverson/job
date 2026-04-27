package handlers

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	job "github.com/bensyverson/jobs/internal/job"
)

// SearchResult is the JSON payload for a single search hit. Kind discriminates
// task vs label; the relevant subset of fields is populated per kind.
type SearchResult struct {
	Kind string `json:"kind"`
	URL  string `json:"url"`

	// Task fields.
	ShortID       string `json:"short_id,omitempty"`
	Title         string `json:"title,omitempty"`
	Status        string `json:"status,omitempty"`
	DisplayStatus string `json:"display_status,omitempty"`
	MatchSource   string `json:"match_source,omitempty"`
	Excerpt       string `json:"excerpt,omitempty"`

	// Label fields.
	Name string `json:"name,omitempty"`
}

// Search returns JSON search results for GET /search?q=...&limit=N.
func Search(deps Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		limit := 20
		if s := r.URL.Query().Get("limit"); s != "" {
			if n, err := strconv.Atoi(s); err == nil && n > 0 {
				limit = n
			}
		}

		hits, err := job.RunSearch(deps.DB, q, limit)
		if err != nil {
			InternalError(deps, w, "search", err)
			return
		}

		var taskIDs []int64
		for _, h := range hits {
			if h.Kind == "task" {
				taskIDs = append(taskIDs, h.ID)
			}
		}
		blockerMap := map[int64][]string{}
		if len(taskIDs) > 0 {
			bm, err := job.GetBlockersForTaskIDs(deps.DB, taskIDs)
			if err != nil {
				InternalError(deps, w, "search blockers", err)
				return
			}
			blockerMap = bm
		}

		results := make([]SearchResult, 0, len(hits))
		for _, h := range hits {
			switch h.Kind {
			case "task":
				results = append(results, SearchResult{
					Kind:          "task",
					ShortID:       h.ShortID,
					Title:         h.Title,
					Status:        h.Status,
					DisplayStatus: DisplayStatus(h.Status, len(blockerMap[h.ID]) > 0),
					URL:           "/tasks/" + h.ShortID,
					MatchSource:   h.MatchSource,
					Excerpt:       h.Excerpt,
				})
			case "label":
				results = append(results, SearchResult{
					Kind: "label",
					Name: h.Name,
					URL:  "/labels/" + url.PathEscape(h.Name),
				})
			}
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(results)
	})
}
