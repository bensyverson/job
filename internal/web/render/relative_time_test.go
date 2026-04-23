package render_test

import (
	"testing"
	"time"

	"github.com/bensyverson/jobs/internal/web/render"
)

func TestRelativeTime_TableDriven(t *testing.T) {
	now := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		then time.Time
		want string
	}{
		{"sub-second", now.Add(-500 * time.Millisecond), "just now"},
		{"1 second", now.Add(-time.Second), "1s"},
		{"45 seconds", now.Add(-45 * time.Second), "45s"},
		{"1 minute", now.Add(-time.Minute), "1m"},
		{"42 minutes", now.Add(-42 * time.Minute), "42m"},
		{"1 hour flat", now.Add(-time.Hour), "1h"},
		{"1h 5m", now.Add(-(time.Hour + 5*time.Minute)), "1h 5m"},
		{"23h 59m", now.Add(-(23*time.Hour + 59*time.Minute)), "23h 59m"},
		{"1 day flat", now.Add(-24 * time.Hour), "1d"},
		{"1d 3h", now.Add(-(27 * time.Hour)), "1d 3h"},
		{"future time (mirror)", now.Add(30 * time.Minute), "30m"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := render.RelativeTime(now, tc.then)
			if got != tc.want {
				t.Errorf("RelativeTime(%v, %v) = %q, want %q", now, tc.then, got, tc.want)
			}
		})
	}
}
