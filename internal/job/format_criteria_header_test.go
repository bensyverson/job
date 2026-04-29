package job

import (
	"bytes"
	"strings"
	"testing"
)

// TestRenderInfoMarkdown_CriteriaHeader_PendingCount verifies the briefing
// surfaces a pending-count summary above the criteria list so an agent who
// just claimed the task knows what they'll need to mark before close.
func TestRenderInfoMarkdown_CriteriaHeader_PendingCount(t *testing.T) {
	db := SetupTestDB(t)
	id := MustAdd(t, db, "", "Task")
	if _, err := RunAddCriteria(db, id, []Criterion{
		{Label: "alpha"}, {Label: "beta"}, {Label: "gamma"},
	}, TestActor); err != nil {
		t.Fatalf("RunAddCriteria: %v", err)
	}

	info, err := RunInfo(db, id)
	if err != nil {
		t.Fatalf("RunInfo: %v", err)
	}
	var buf bytes.Buffer
	RenderInfoMarkdown(&buf, info)
	got := buf.String()
	if !strings.Contains(got, "3 pending") {
		t.Errorf("briefing should surface pending count (3 pending) above criteria list:\n%s", got)
	}
}

// TestRenderInfoMarkdown_CriteriaHeader_PartialPending shows only the
// remaining pending count when the operator has already marked some
// criteria — the count tracks "what's left to mark," not the total.
func TestRenderInfoMarkdown_CriteriaHeader_PartialPending(t *testing.T) {
	db := SetupTestDB(t)
	id := MustAdd(t, db, "", "Task")
	if _, err := RunAddCriteria(db, id, []Criterion{
		{Label: "a"}, {Label: "b"}, {Label: "c"}, {Label: "d"},
	}, TestActor); err != nil {
		t.Fatalf("RunAddCriteria: %v", err)
	}
	if _, err := RunSetCriterion(db, id, "a", CriterionPassed, TestActor); err != nil {
		t.Fatalf("set a: %v", err)
	}
	if _, err := RunSetCriterion(db, id, "b", CriterionSkipped, TestActor); err != nil {
		t.Fatalf("set b: %v", err)
	}

	info, err := RunInfo(db, id)
	if err != nil {
		t.Fatalf("RunInfo: %v", err)
	}
	var buf bytes.Buffer
	RenderInfoMarkdown(&buf, info)
	got := buf.String()
	if !strings.Contains(got, "2 pending") {
		t.Errorf("briefing should report 2 pending (4 total, 2 marked):\n%s", got)
	}
}

// TestRenderInfoMarkdown_CriteriaHeader_NamesOverrideFlag pins the
// matching-phrasing criterion: the header references
// --force-close-with-pending so the operator recognizes the same constraint
// they'll see if they hit the strict-close gate later.
func TestRenderInfoMarkdown_CriteriaHeader_NamesOverrideFlag(t *testing.T) {
	db := SetupTestDB(t)
	id := MustAdd(t, db, "", "Task")
	if _, err := RunAddCriteria(db, id, []Criterion{{Label: "x"}}, TestActor); err != nil {
		t.Fatalf("RunAddCriteria: %v", err)
	}

	info, err := RunInfo(db, id)
	if err != nil {
		t.Fatalf("RunInfo: %v", err)
	}
	var buf bytes.Buffer
	RenderInfoMarkdown(&buf, info)
	got := buf.String()
	if !strings.Contains(got, "--force-close-with-pending") {
		t.Errorf("briefing should reference --force-close-with-pending so phrasing matches strict-close error:\n%s", got)
	}
}

// TestRenderInfoMarkdown_CriteriaHeader_OmittedWhenAllResolved guards the
// "no friction when fully marked" case: once nothing is pending, the count
// drops out so the briefing isn't cluttered with "0 pending."
func TestRenderInfoMarkdown_CriteriaHeader_OmittedWhenAllResolved(t *testing.T) {
	db := SetupTestDB(t)
	id := MustAdd(t, db, "", "Task")
	if _, err := RunAddCriteria(db, id, []Criterion{
		{Label: "p"}, {Label: "q"},
	}, TestActor); err != nil {
		t.Fatalf("RunAddCriteria: %v", err)
	}
	if _, err := RunSetCriterion(db, id, "p", CriterionPassed, TestActor); err != nil {
		t.Fatalf("set p: %v", err)
	}
	if _, err := RunSetCriterion(db, id, "q", CriterionPassed, TestActor); err != nil {
		t.Fatalf("set q: %v", err)
	}

	info, err := RunInfo(db, id)
	if err != nil {
		t.Fatalf("RunInfo: %v", err)
	}
	var buf bytes.Buffer
	RenderInfoMarkdown(&buf, info)
	got := buf.String()
	if strings.Contains(got, "pending") {
		t.Errorf("briefing should not mention 'pending' when nothing is pending:\n%s", got)
	}
	if strings.Contains(got, "--force-close-with-pending") {
		t.Errorf("briefing should not name the override flag when no friction applies:\n%s", got)
	}
	// The Criteria section itself must still render so passed/skipped
	// states remain visible.
	if !strings.Contains(got, "Criteria:") {
		t.Errorf("Criteria section should still appear when criteria exist:\n%s", got)
	}
}

// TestRenderInfoMarkdown_CriteriaHeader_NoCriteriaNoSection regression-checks
// that tasks without criteria still skip the section entirely (no header,
// no list).
func TestRenderInfoMarkdown_CriteriaHeader_NoCriteriaNoSection(t *testing.T) {
	db := SetupTestDB(t)
	id := MustAdd(t, db, "", "Task")

	info, err := RunInfo(db, id)
	if err != nil {
		t.Fatalf("RunInfo: %v", err)
	}
	var buf bytes.Buffer
	RenderInfoMarkdown(&buf, info)
	got := buf.String()
	if strings.Contains(got, "Criteria") {
		t.Errorf("Criteria section should not appear for a task without criteria:\n%s", got)
	}
}
