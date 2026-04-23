package render

import (
	"fmt"
	"time"
)

// RelativeTime renders a short "Ns / Nm / Nh Mm" label for the delta
// between now and then. Mirrors the prototype's time column ("4s",
// "18s", "1m", "1h 5m"). Future times (then > now) also render
// non-negative; "just now" for sub-second deltas.
func RelativeTime(now, then time.Time) string {
	d := now.Sub(then)
	if d < 0 {
		d = -d
	}
	if d < time.Second {
		return "just now"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) - hours*60
	if d < 24*time.Hour {
		if mins == 0 {
			return fmt.Sprintf("%dh", hours)
		}
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	days := hours / 24
	remHours := hours - days*24
	if remHours == 0 {
		return fmt.Sprintf("%dd", days)
	}
	return fmt.Sprintf("%dd %dh", days, remHours)
}
