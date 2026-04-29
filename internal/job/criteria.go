package job

import (
	"database/sql"
	"fmt"
	"strings"
)

// CriterionState is the lifecycle state of an acceptance criterion.
type CriterionState string

const (
	CriterionPending CriterionState = "pending"
	CriterionPassed  CriterionState = "passed"
	CriterionSkipped CriterionState = "skipped"
	CriterionFailed  CriterionState = "failed"
)

// Criterion is a single acceptance-criterion row.
type Criterion struct {
	ID        int64
	TaskID    int64
	Label     string
	State     CriterionState
	SortOrder int
}

func ValidateCriterionState(raw string) (CriterionState, error) {
	switch raw {
	case "pending", "passed", "skipped", "failed":
		return CriterionState(raw), nil
	default:
		return "", fmt.Errorf("invalid criterion state %q (want pending|passed|skipped|failed)", raw)
	}
}

func validateCriterionLabel(raw string) (string, error) {
	label := strings.TrimSpace(raw)
	if label == "" {
		return "", fmt.Errorf("criterion label is empty")
	}
	return label, nil
}

// insertCriteria appends each label as a new criterion at the end of taskID's
// list, with state defaulting to pending unless overridden in the input.
// Returns the inserted Criterion records in input order.
func insertCriteria(tx dbtx, taskID int64, items []Criterion) ([]Criterion, error) {
	if len(items) == 0 {
		return nil, nil
	}

	var maxSort sql.NullInt64
	if err := tx.QueryRow(
		"SELECT MAX(sort_order) FROM task_criteria WHERE task_id = ?", taskID,
	).Scan(&maxSort); err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	next := 0
	if maxSort.Valid {
		next = int(maxSort.Int64) + 1
	}

	now := CurrentNowFunc().Unix()
	out := make([]Criterion, 0, len(items))
	for _, c := range items {
		label, err := validateCriterionLabel(c.Label)
		if err != nil {
			return nil, err
		}
		state := c.State
		if state == "" {
			state = CriterionPending
		}
		if _, err := ValidateCriterionState(string(state)); err != nil {
			return nil, err
		}
		var id int64
		err = tx.QueryRow(
			`INSERT INTO task_criteria (task_id, label, state, sort_order, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?) RETURNING id`,
			taskID, label, string(state), next, now, now,
		).Scan(&id)
		if err != nil {
			return nil, err
		}
		out = append(out, Criterion{
			ID:        id,
			TaskID:    taskID,
			Label:     label,
			State:     state,
			SortOrder: next,
		})
		next++
	}
	return out, nil
}

// GetCriteria returns the criteria for taskID in sort order.
func GetCriteria(db dbtx, taskID int64) ([]Criterion, error) {
	rows, err := db.Query(
		`SELECT id, task_id, label, state, sort_order
		 FROM task_criteria WHERE task_id = ? ORDER BY sort_order, id`,
		taskID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Criterion
	for rows.Next() {
		var c Criterion
		var state string
		if err := rows.Scan(&c.ID, &c.TaskID, &c.Label, &state, &c.SortOrder); err != nil {
			return nil, err
		}
		c.State = CriterionState(state)
		out = append(out, c)
	}
	return out, rows.Err()
}

// SetCriterionState updates the state of one criterion (matched by label,
// case-sensitive) on taskID. Returns the new state and the prior state.
func SetCriterionState(tx dbtx, taskID int64, label string, state CriterionState) (prior CriterionState, err error) {
	if _, err := ValidateCriterionState(string(state)); err != nil {
		return "", err
	}
	row := tx.QueryRow(
		"SELECT state FROM task_criteria WHERE task_id = ? AND label = ?",
		taskID, label,
	)
	var existing string
	if err := row.Scan(&existing); err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("no criterion %q on task", label)
		}
		return "", err
	}
	now := CurrentNowFunc().Unix()
	if _, err := tx.Exec(
		"UPDATE task_criteria SET state = ?, updated_at = ? WHERE task_id = ? AND label = ?",
		string(state), now, taskID, label,
	); err != nil {
		return "", err
	}
	return CriterionState(existing), nil
}

// criteriaEventDetail shapes a list of Criterion records for inclusion in an
// event detail JSON payload.
func criteriaEventDetail(items []Criterion) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, c := range items {
		out = append(out, map[string]any{
			"label": c.Label,
			"state": string(c.State),
		})
	}
	return out
}

// RunAddCriteria appends new criteria to an existing task, records a
// criteria_added event, and extends the actor's claim if held.
func RunAddCriteria(db *sql.DB, shortID string, items []Criterion, actor string) ([]Criterion, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("no criteria supplied")
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if err := expireStaleClaimsInTx(tx, actor); err != nil {
		return nil, err
	}
	task, err := GetTaskByShortID(tx, shortID)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, fmt.Errorf("task %q not found", shortID)
	}

	inserted, err := insertCriteria(tx, task.ID, items)
	if err != nil {
		return nil, err
	}
	if err := recordEvent(tx, task.ID, "criteria_added", actor, map[string]any{
		"criteria": criteriaEventDetail(inserted),
	}); err != nil {
		return nil, err
	}
	if err := maybeExtendClaim(tx, task.ID, actor); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return inserted, nil
}

// RunSetCriterion updates one criterion's state on a task and records a
// criterion_state event with the prior state for invertibility.
func RunSetCriterion(db *sql.DB, shortID, label string, state CriterionState, actor string) (prior CriterionState, err error) {
	if _, err := ValidateCriterionState(string(state)); err != nil {
		return "", err
	}
	tx, err := db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	if err := expireStaleClaimsInTx(tx, actor); err != nil {
		return "", err
	}
	task, err := GetTaskByShortID(tx, shortID)
	if err != nil {
		return "", err
	}
	if task == nil {
		return "", fmt.Errorf("task %q not found", shortID)
	}
	prior, err = SetCriterionState(tx, task.ID, label, state)
	if err != nil {
		return "", err
	}
	if err := recordEvent(tx, task.ID, "criterion_state", actor, map[string]any{
		"label": label,
		"state": string(state),
		"prior": string(prior),
	}); err != nil {
		return "", err
	}
	if err := maybeExtendClaim(tx, task.ID, actor); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return prior, nil
}

// CountPendingCriteria returns the number of criteria currently in pending state.
func CountPendingCriteria(db dbtx, taskID int64) (int, error) {
	var n int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM task_criteria WHERE task_id = ? AND state = 'pending'",
		taskID,
	).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
