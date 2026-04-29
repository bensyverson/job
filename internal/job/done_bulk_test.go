package job

import (
	"testing"
)

// TestRunDone_BulkCriteriaStateRecorded pins the contract between the CLI's
// --all-passed shorthand and the event log: when the caller passes a non-
// empty bulkCriteriaState, RunDone preserves it on the done event so the
// close shape ("all marked passed via the shorthand") survives in History.
func TestRunDone_BulkCriteriaStateRecorded(t *testing.T) {
	db := SetupTestDB(t)
	id := MustAdd(t, db, "", "Task")

	if _, _, err := RunDone(db, []string{id}, false, "", nil, TestActor, false, "passed"); err != nil {
		t.Fatalf("RunDone: %v", err)
	}
	task := MustGet(t, db, id)
	detail, err := GetLatestEventDetail(db, task.ID, "done")
	if err != nil {
		t.Fatalf("GetLatestEventDetail: %v", err)
	}
	if detail["criteria_bulk_state"] != "passed" {
		t.Errorf("criteria_bulk_state: got %v, want \"passed\"", detail["criteria_bulk_state"])
	}
}

// TestRunDone_EmptyBulkCriteriaStateNotRecorded confirms the field is
// absent when the caller didn't use the shorthand — it must not appear as
// an empty string, since History rendering would then misreport every
// close as a bulk operation.
func TestRunDone_EmptyBulkCriteriaStateNotRecorded(t *testing.T) {
	db := SetupTestDB(t)
	id := MustAdd(t, db, "", "Task")

	if _, _, err := RunDone(db, []string{id}, false, "", nil, TestActor, false, ""); err != nil {
		t.Fatalf("RunDone: %v", err)
	}
	task := MustGet(t, db, id)
	detail, err := GetLatestEventDetail(db, task.ID, "done")
	if err != nil {
		t.Fatalf("GetLatestEventDetail: %v", err)
	}
	if _, ok := detail["criteria_bulk_state"]; ok {
		t.Errorf("criteria_bulk_state should be absent when the shorthand was not used; got %v", detail["criteria_bulk_state"])
	}
}
