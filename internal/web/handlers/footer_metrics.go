package handlers

import (
	"context"
	"database/sql"
	"time"

	"github.com/bensyverson/jobs/internal/web/templates"
)

// LoadFooterMetrics returns the four always-now metric values for the
// footer strip: distinct holders of currently-live claims, the count
// of open (not-deleted) tasks, the rolling-60-minute events-per-minute
// average, and the count of done events in the last 60 minutes.
//
// The values are "now," not point-in-time at the time-travel cursor.
// The footer is a system status bar — the rest of the page scrubs,
// the footer doesn't.
func LoadFooterMetrics(ctx context.Context, db *sql.DB, now time.Time) (templates.FooterMetrics, error) {
	var m templates.FooterMetrics

	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT claimed_by)
		FROM tasks
		WHERE claimed_by IS NOT NULL
		  AND claim_expires_at > ?
		  AND deleted_at IS NULL
	`, now.Unix()).Scan(&m.Actors); err != nil {
		return m, err
	}

	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM tasks
		WHERE status IN ('available', 'claimed')
		  AND deleted_at IS NULL
	`).Scan(&m.WIP); err != nil {
		return m, err
	}

	windowStart := now.Add(-60 * time.Minute).Unix()

	var totalEvents int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM events WHERE created_at >= ?
	`, windowStart).Scan(&totalEvents); err != nil {
		return m, err
	}
	m.EventsPerMin = totalEvents / 60

	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM events
		WHERE event_type = 'done' AND created_at >= ?
	`, windowStart).Scan(&m.TasksPerHour); err != nil {
		return m, err
	}

	return m, nil
}
