package job

import (
	"database/sql"
	"fmt"
	"io"
	"strings"
)

// Summary is a two-level rollup for `job summary <parent>`. The target's
// own rollup gives the headline numbers; one rollup per direct child
// adds enough granularity to tell which sub-phase needs attention,
// without redrawing the full tree.
type Summary struct {
	Target         *SubtreeRollup
	DirectChildren []*SubtreeRollup
}

type SubtreeRollup struct {
	ShortID   string
	Title     string
	Status    string
	HasKids   bool
	Done      int
	Open      int
	Blocked   int
	Available int
	InFlight  int
	Canceled  int
	NextID    string
}

func RunSummary(db *sql.DB, shortID string) (*Summary, error) {
	target, err := GetTaskByShortID(db, shortID)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, fmt.Errorf("task %q not found", shortID)
	}

	targetRollup, err := buildRollup(db, target)
	if err != nil {
		return nil, err
	}

	directChildren, err := getChildren(db, target.ID)
	if err != nil {
		return nil, err
	}

	var childRollups []*SubtreeRollup
	for _, c := range directChildren {
		cr, err := buildRollup(db, c)
		if err != nil {
			return nil, err
		}
		childRollups = append(childRollups, cr)
	}

	return &Summary{Target: targetRollup, DirectChildren: childRollups}, nil
}

// buildRollup walks the descendants of t (excluding t itself) and
// computes count slices. NextID names the first claimable leaf scoped
// to t — uses the same machinery as `next [parent]`.
func buildRollup(db *sql.DB, t *Task) (*SubtreeRollup, error) {
	r := &SubtreeRollup{
		ShortID: t.ShortID,
		Title:   t.Title,
		Status:  t.Status,
	}

	descendants, err := collectDescendants(db, t.ID)
	if err != nil {
		return nil, err
	}
	r.HasKids = len(descendants) > 0

	descIDs := make([]int64, 0, len(descendants))
	for _, d := range descendants {
		descIDs = append(descIDs, d.ID)
	}
	blockerCounts, err := openBlockerCounts(db, descIDs)
	if err != nil {
		return nil, err
	}

	for _, d := range descendants {
		switch d.Status {
		case "done":
			r.Done++
		case "canceled":
			r.Canceled++
		case "claimed":
			r.Open++
			r.InFlight++
		default:
			r.Open++
			isBlocked := blockerCounts[d.ID] > 0
			if isBlocked {
				r.Blocked++
			}
		}
	}

	// "Available" reports the truly claimable leaves — same definition as
	// `next` and the leaf-frontier docs. Reuse the canonical query so we
	// don't drift.
	avail, err := queryAvailableTasks(db, t.ShortID, 0, "", false)
	if err != nil {
		return nil, err
	}
	r.Available = len(avail)
	if len(avail) > 0 {
		r.NextID = avail[0].ShortID
	}

	return r, nil
}

func collectDescendants(db *sql.DB, taskID int64) ([]*Task, error) {
	var out []*Task
	queue := []int64{taskID}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		children, err := getChildren(db, id)
		if err != nil {
			return nil, err
		}
		for _, c := range children {
			out = append(out, c)
			queue = append(queue, c.ID)
		}
	}
	return out, nil
}

// openBlockerCounts returns the count of open (non-done) blockers per
// task id. A task with count > 0 is currently blocked.
func openBlockerCounts(db *sql.DB, taskIDs []int64) (map[int64]int, error) {
	out := make(map[int64]int, len(taskIDs))
	if len(taskIDs) == 0 {
		return out, nil
	}
	placeholders := strings.Repeat("?,", len(taskIDs))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, 0, len(taskIDs))
	for _, id := range taskIDs {
		args = append(args, id)
	}
	rows, err := db.Query(`
		SELECT b.blocked_id, COUNT(*)
		FROM blocks b
		JOIN tasks t ON t.id = b.blocker_id
		WHERE b.blocked_id IN (`+placeholders+`)
		  AND t.status != 'done'
		  AND t.deleted_at IS NULL
		GROUP BY b.blocked_id
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

func RenderSummary(w io.Writer, s *Summary) {
	t := s.Target
	fmt.Fprintf(w, "%s · %s\n", t.Title, t.ShortID)
	if t.HasKids {
		fmt.Fprintf(w, "  %d of %d done · %d blocked · %d available · %d in flight",
			t.Done, t.Done+t.Open, t.Blocked, t.Available, t.InFlight)
		if t.Canceled > 0 {
			fmt.Fprintf(w, " · %d canceled", t.Canceled)
		}
		fmt.Fprintln(w)
	} else {
		fmt.Fprintf(w, "  status: %s\n", t.Status)
	}

	for _, c := range s.DirectChildren {
		fmt.Fprintf(w, "  %s (%s): %s\n", c.Title, c.ShortID, summarizeChild(c))
	}
}

// summarizeChild renders the per-child rollup tail. Three cases:
//   - leaf child or child with no live work: report its own status.
//   - subtree fully closed: report "done" (or "canceled" mix).
//   - subtree in progress: "X of Y done · next <id>".
func summarizeChild(c *SubtreeRollup) string {
	if !c.HasKids {
		return c.Status
	}
	if c.Open == 0 {
		return c.Status
	}
	tail := fmt.Sprintf("%d of %d done", c.Done, c.Done+c.Open)
	if c.NextID != "" {
		tail += " · next " + c.NextID
	}
	return tail
}
