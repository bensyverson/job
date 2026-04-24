package render

import (
	"fmt"
	"time"
)

// ClaimDuration formats a claim's age at minute+second precision below
// the hour ("8m 47s"), mirroring the Home view's "Longest active claim"
// card. Above the hour it matches RelativeTime's "Hh Mm" / "Dd Hh" so
// the two helpers feel like a set.
func ClaimDuration(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		secs := int(d.Seconds()) - mins*60
		return fmt.Sprintf("%dm %ds", mins, secs)
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		mins := int(d.Minutes()) - hours*60
		if mins == 0 {
			return fmt.Sprintf("%dh", hours)
		}
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	hours := int(d.Hours())
	days := hours / 24
	remHours := hours - days*24
	if remHours == 0 {
		return fmt.Sprintf("%dd", days)
	}
	return fmt.Sprintf("%dd %dh", days, remHours)
}
