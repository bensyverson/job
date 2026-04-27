package job

import (
	"database/sql"
	"fmt"
	"strings"
	"unicode/utf8"
)

// SearchHit is a single result from a global search query. Kind discriminates
// task vs label; the relevant subset of fields is populated for each kind.
//
// Note: tasks.description is the rolled-up state including any note text
// appended by RunNote, so a search hit on a note surfaces here as a
// MatchSource of "description".
type SearchHit struct {
	Kind string // "task" or "label"

	// Task fields (Kind == "task").
	ID          int64
	ShortID     string
	Title       string
	Status      string
	MatchSource string // "short_id" | "title" | "description"
	Excerpt     string // snippet centered on first match (description only)

	// Label fields (Kind == "label").
	Name string
}

const (
	maxLabelHits   = 5
	excerptWindow  = 40
	excerptElision = "…"
)

// RunSearch returns task and label results matching query. Tasks match across
// short_id (exact + substring), title, and description and are ordered by
// match-quality rank then status priority. Distinct labels whose name matches
// are returned as kind="label" hits (up to maxLabelHits).
//
// Composition: any exact-short_id task hits come first (they are the strongest
// possible match), then label hits, then remaining task hits in rank order.
// The combined slice is trimmed to limit. Empty / whitespace queries return
// nil.
func RunSearch(db *sql.DB, query string, limit int) ([]SearchHit, error) {
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}

	likePattern := "%" + escapeLike(query) + "%"

	taskHits, err := searchTasks(db, query, likePattern, limit)
	if err != nil {
		return nil, err
	}
	labelHits, err := searchLabels(db, likePattern, maxLabelHits)
	if err != nil {
		return nil, err
	}

	var exact, rest []SearchHit
	for _, h := range taskHits {
		if h.MatchSource == "short_id" && strings.EqualFold(h.ShortID, query) {
			exact = append(exact, h)
		} else {
			rest = append(rest, h)
		}
	}

	out := make([]SearchHit, 0, len(exact)+len(labelHits)+len(rest))
	out = append(out, exact...)
	out = append(out, labelHits...)
	out = append(out, rest...)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func searchTasks(db *sql.DB, query, likePattern string, limit int) ([]SearchHit, error) {
	const sqlText = `
		SELECT t.id, t.short_id, t.title, t.status, t.description,
		  CASE
		    WHEN t.short_id = ? THEN 1
		    WHEN t.title LIKE ? ESCAPE '\' THEN 2
		    WHEN t.description LIKE ? ESCAPE '\' THEN 3
		    WHEN t.short_id LIKE ? ESCAPE '\' THEN 4
		    ELSE 99
		  END AS match_rank
		FROM tasks t
		WHERE t.deleted_at IS NULL
		  AND (
		    t.short_id = ?
		    OR t.short_id LIKE ? ESCAPE '\'
		    OR t.title LIKE ? ESCAPE '\'
		    OR t.description LIKE ? ESCAPE '\'
		  )
		ORDER BY
		  match_rank,
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
	`
	rows, err := db.Query(sqlText,
		query, likePattern, likePattern, likePattern, // CASE branches
		query, likePattern, likePattern, likePattern, // WHERE
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("search tasks: %w", err)
	}
	defer rows.Close()

	var hits []SearchHit
	for rows.Next() {
		var (
			id      int64
			shortID string
			title   string
			status  string
			desc    sql.NullString
			rank    int
		)
		if err := rows.Scan(&id, &shortID, &title, &status, &desc, &rank); err != nil {
			return nil, err
		}
		h := SearchHit{
			Kind:        "task",
			ID:          id,
			ShortID:     shortID,
			Title:       title,
			Status:      status,
			MatchSource: matchSourceForRank(rank),
		}
		if rank == 3 {
			h.Excerpt = makeExcerpt(desc.String, query)
		}
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

func searchLabels(db *sql.DB, likePattern string, max int) ([]SearchHit, error) {
	rows, err := db.Query(`
		SELECT name FROM task_labels
		WHERE name LIKE ? ESCAPE '\'
		GROUP BY name
		ORDER BY name ASC
		LIMIT ?
	`, likePattern, max)
	if err != nil {
		return nil, fmt.Errorf("search labels: %w", err)
	}
	defer rows.Close()

	var hits []SearchHit
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		hits = append(hits, SearchHit{Kind: "label", Name: name})
	}
	return hits, rows.Err()
}

func matchSourceForRank(rank int) string {
	switch rank {
	case 1, 4:
		return "short_id"
	case 2:
		return "title"
	case 3:
		return "description"
	}
	return ""
}

// makeExcerpt returns a snippet of text centered on the first case-insensitive
// match of query, with "…" elision on either side when the snippet doesn't
// reach the original endpoints. Returns "" if either input is empty or the
// match isn't found.
func makeExcerpt(text, query string) string {
	if text == "" || query == "" {
		return ""
	}
	idx := strings.Index(strings.ToLower(text), strings.ToLower(query))
	if idx < 0 {
		return ""
	}
	start := idx - excerptWindow
	if start < 0 {
		start = 0
	} else {
		// Walk forward to the next rune boundary so we don't slice mid-rune.
		for start < len(text) && !utf8.RuneStart(text[start]) {
			start++
		}
	}
	end := idx + len(query) + excerptWindow
	if end > len(text) {
		end = len(text)
	} else {
		for end < len(text) && !utf8.RuneStart(text[end]) {
			end++
		}
	}
	out := text[start:end]
	if start > 0 {
		out = excerptElision + out
	}
	if end < len(text) {
		out = out + excerptElision
	}
	return out
}

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	return s
}
