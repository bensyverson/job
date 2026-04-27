package job

import (
	"database/sql"
	"fmt"
	"strings"
)

// SearchHit is a single matching task from a global search query.
type SearchHit struct {
	ID      int64
	ShortID string
	Title   string
	Status  string
}

// RunSearch returns tasks matching query across short_id, title, description,
// note text (from noted events), and labels. Results are deduplicated by task
// and sorted by status priority (claimed, available, done, canceled). An empty
// or whitespace-only query returns an empty slice.
func RunSearch(db *sql.DB, query string, limit int) ([]SearchHit, error) {
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}

	pattern := "%" + escapeLike(query) + "%"

	rows, err := db.Query(`
		SELECT t.id, t.short_id, t.title, t.status
		FROM tasks t
		WHERE t.deleted_at IS NULL
		  AND (
		    t.short_id LIKE ? ESCAPE '\'
		    OR t.title LIKE ? ESCAPE '\'
		    OR t.description LIKE ? ESCAPE '\'
		    OR EXISTS (
		      SELECT 1 FROM events e
		      WHERE e.task_id = t.id
		        AND e.event_type = 'noted'
		        AND json_extract(e.detail, '$.text') LIKE ? ESCAPE '\'
		    )
		    OR EXISTS (
		      SELECT 1 FROM task_labels tl
		      WHERE tl.task_id = t.id
		        AND tl.name LIKE ? ESCAPE '\'
		    )
		  )
		ORDER BY
		  CASE t.status
		    WHEN 'claimed' THEN 1
		    WHEN 'available' THEN 2
		    WHEN 'done' THEN 3
		    WHEN 'canceled' THEN 4
		    ELSE 5
		  END,
		  t.sort_order,
		  t.created_at DESC
		LIMIT ?
	`, pattern, pattern, pattern, pattern, pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("search query: %w", err)
	}
	defer rows.Close()

	var hits []SearchHit
	for rows.Next() {
		var h SearchHit
		if err := rows.Scan(&h.ID, &h.ShortID, &h.Title, &h.Status); err != nil {
			return nil, err
		}
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	return s
}
