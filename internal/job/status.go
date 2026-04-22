package job

import (
	"database/sql"
	"fmt"
	"io"
	"strings"
)

type StatusSummary struct {
	Open         int
	Claimed      int
	Done         int
	Canceled     int
	ClaimedByYou int
	HasActor     bool
	LastActivity int64
	Total        int
}

func RunStatus(db *sql.DB, actor string) (*StatusSummary, error) {
	if err := expireStaleClaims(db, actor); err != nil {
		return nil, err
	}

	s := &StatusSummary{HasActor: actor != ""}

	rows, err := db.Query("SELECT status, COUNT(*) FROM tasks WHERE deleted_at IS NULL GROUP BY status")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		s.Total += count
		switch status {
		case "done":
			s.Done = count
		case "claimed":
			s.Claimed = count
		case "canceled":
			s.Canceled = count
		default:
			s.Open += count
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if s.HasActor {
		if err := db.QueryRow(
			"SELECT COUNT(*) FROM tasks WHERE claimed_by = ? AND status = 'claimed' AND deleted_at IS NULL",
			actor,
		).Scan(&s.ClaimedByYou); err != nil {
			return nil, err
		}
	}

	var lastActivity sql.NullInt64
	if err := db.QueryRow("SELECT MAX(created_at) FROM events").Scan(&lastActivity); err != nil {
		return nil, err
	}
	if lastActivity.Valid {
		s.LastActivity = lastActivity.Int64
	}

	return s, nil
}

func RenderStatus(w io.Writer, s *StatusSummary) {
	var parts []string
	// Claimed term is scoped to the caller when HasActor, else the global
	// live-claim count. Suppressed entirely when zero to avoid noise for
	// non-claiming callers.
	claimed := s.Claimed
	if s.HasActor {
		claimed = s.ClaimedByYou
	}
	if claimed > 0 {
		parts = append(parts, fmt.Sprintf("%d claimed", claimed))
	}
	parts = append(parts, fmt.Sprintf("%d open", s.Open))
	parts = append(parts, fmt.Sprintf("%d done", s.Done))
	if s.Canceled > 0 {
		parts = append(parts, fmt.Sprintf("%d canceled", s.Canceled))
	}

	line := strings.Join(parts, ", ")
	if s.LastActivity > 0 {
		ago := max(nowUnix()-s.LastActivity, 0)
		line += fmt.Sprintf(" (last activity: %s ago)", FormatDuration(ago))
	}
	fmt.Fprintln(w, line)
}
