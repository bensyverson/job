package job

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// SearchHit is a single result from a global search query. Kind discriminates
// task vs label; the relevant subset of fields is populated for each kind.
//
// Notes live exclusively as `noted` events on the events table — they are not
// merged into tasks.description — so a note-body match surfaces here as
// MatchSource="note", with the excerpt drawn from the matching note body.
type SearchHit struct {
	Kind string // "task" or "label"

	// Task fields (Kind == "task").
	ID          int64
	ShortID     string
	Title       string
	Status      string
	MatchSource string // "short_id" | "title" | "description" | "note"
	Excerpt     string // snippet centered on first match (description / note)

	// Label fields (Kind == "label").
	Name string
}

const (
	maxLabelHits   = 5
	excerptWindow  = 40
	excerptElision = "…"
)

// RunSearch returns task and label results matching query. Tasks match across
// short_id (exact + substring), title, description, and `noted` event bodies
// and are ordered by match-quality rank then status priority. Distinct labels
// whose name matches are returned as kind="label" hits (up to maxLabelHits).
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
	// Two passes: tasks matched by their own columns, then tasks matched by
	// `noted` event bodies. The two streams are merged in Go and deduped by
	// task id with the better (lower) rank winning.
	const taskSQL = `
		SELECT t.id, t.short_id, t.title, t.status, t.description,
		  CASE
		    WHEN t.short_id = ? THEN 1
		    WHEN t.title LIKE ? ESCAPE '\' THEN 2
		    WHEN t.description LIKE ? ESCAPE '\' THEN 3
		    WHEN t.short_id LIKE ? ESCAPE '\' THEN 5
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
	`
	taskRows, err := db.Query(taskSQL,
		query, likePattern, likePattern, likePattern, // CASE branches
		query, likePattern, likePattern, likePattern, // WHERE
	)
	if err != nil {
		return nil, fmt.Errorf("search tasks: %w", err)
	}
	defer taskRows.Close()

	type taskRow struct {
		id      int64
		shortID string
		title   string
		status  string
		desc    string
		rank    int
		excerpt string
	}
	byID := map[int64]*taskRow{}
	for taskRows.Next() {
		var (
			id      int64
			shortID string
			title   string
			status  string
			desc    sql.NullString
			rank    int
		)
		if err := taskRows.Scan(&id, &shortID, &title, &status, &desc, &rank); err != nil {
			return nil, err
		}
		row := &taskRow{id: id, shortID: shortID, title: title, status: status, desc: desc.String, rank: rank}
		if rank == 3 {
			row.excerpt = makeExcerpt(desc.String, query)
		}
		byID[id] = row
	}
	if err := taskRows.Err(); err != nil {
		return nil, err
	}

	// Note pass: any task whose latest matching `noted` event body matches
	// the query. Rank 4 — sits below description matches (rank 3) and above
	// short_id substring matches (rank 5).
	const noteSQL = `
		SELECT t.id, t.short_id, t.title, t.status, t.description,
		       json_extract(e.detail, '$.text') AS note_text
		FROM events e
		JOIN tasks t ON t.id = e.task_id
		WHERE e.event_type = 'noted'
		  AND t.deleted_at IS NULL
		  AND json_extract(e.detail, '$.text') LIKE ? ESCAPE '\'
		ORDER BY e.id DESC
	`
	noteRows, err := db.Query(noteSQL, likePattern)
	if err != nil {
		return nil, fmt.Errorf("search notes: %w", err)
	}
	defer noteRows.Close()
	for noteRows.Next() {
		var (
			id      int64
			shortID string
			title   string
			status  string
			desc    sql.NullString
			note    sql.NullString
		)
		if err := noteRows.Scan(&id, &shortID, &title, &status, &desc, &note); err != nil {
			return nil, err
		}
		if existing, ok := byID[id]; ok {
			// Already matched via task columns; keep the better (lower) rank.
			// If the existing rank is worse than 4, upgrade to a note hit so
			// the excerpt comes from the actual note body.
			if existing.rank > 4 {
				existing.rank = 4
				existing.excerpt = makeExcerpt(note.String, query)
			}
			continue
		}
		byID[id] = &taskRow{
			id:      id,
			shortID: shortID,
			title:   title,
			status:  status,
			desc:    desc.String,
			rank:    4,
			excerpt: makeExcerpt(note.String, query),
		}
	}
	if err := noteRows.Err(); err != nil {
		return nil, err
	}

	rows := make([]*taskRow, 0, len(byID))
	for _, r := range byID {
		rows = append(rows, r)
	}
	statusPriority := func(s string) int {
		switch s {
		case "claimed":
			return 1
		case "available":
			return 2
		case "done":
			return 3
		case "canceled":
			return 4
		default:
			return 5
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].rank != rows[j].rank {
			return rows[i].rank < rows[j].rank
		}
		pi, pj := statusPriority(rows[i].status), statusPriority(rows[j].status)
		if pi != pj {
			return pi < pj
		}
		return rows[i].id < rows[j].id
	})

	if len(rows) > limit {
		rows = rows[:limit]
	}
	hits := make([]SearchHit, 0, len(rows))
	for _, r := range rows {
		hits = append(hits, SearchHit{
			Kind:        "task",
			ID:          r.id,
			ShortID:     r.shortID,
			Title:       r.title,
			Status:      r.status,
			MatchSource: matchSourceForRank(r.rank),
			Excerpt:     r.excerpt,
		})
	}
	return hits, nil
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
	case 1, 5:
		return "short_id"
	case 2:
		return "title"
	case 3:
		return "description"
	case 4:
		return "note"
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
