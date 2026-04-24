package render_test

import (
	"testing"
	"time"

	"github.com/bensyverson/jobs/internal/web/render"
)

func TestClaimDuration_TableDriven(t *testing.T) {
	cases := []struct {
		name string
		in   time.Duration
		want string
	}{
		{"zero", 0, "0s"},
		{"1 second", time.Second, "1s"},
		{"59 seconds", 59 * time.Second, "59s"},
		{"1m flat", time.Minute, "1m 0s"},
		{"8m 47s", 8*time.Minute + 47*time.Second, "8m 47s"},
		{"59m 59s", 59*time.Minute + 59*time.Second, "59m 59s"},
		{"1h flat", time.Hour, "1h"},
		{"1h 5m", time.Hour + 5*time.Minute, "1h 5m"},
		{"23h 59m", 23*time.Hour + 59*time.Minute, "23h 59m"},
		{"1d flat", 24 * time.Hour, "1d"},
		{"1d 3h", 24*time.Hour + 3*time.Hour, "1d 3h"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := render.ClaimDuration(tc.in)
			if got != tc.want {
				t.Errorf("ClaimDuration(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
