package job

import (
	"strings"
	"testing"
)

// TestInsertCriteria_AssignsShortID confirms every newly inserted criterion
// row carries a non-empty server-minted short_id. This is the load-bearing
// invariant: every other piece of the short-id story (CLI lookup, event
// recording, JS replay) assumes the short_id is set at creation time.
func TestInsertCriteria_AssignsShortID(t *testing.T) {
	db := SetupTestDB(t)
	id := MustAdd(t, db, "", "Task")

	inserted, err := RunAddCriteria(db, id, []Criterion{
		{Label: "alpha"}, {Label: "beta"}, {Label: "gamma"},
	}, TestActor)
	if err != nil {
		t.Fatalf("RunAddCriteria: %v", err)
	}
	for _, c := range inserted {
		if c.ShortID == "" {
			t.Errorf("criterion %q: short_id is empty", c.Label)
		}
		if len(c.ShortID) != 3 {
			t.Errorf("criterion %q: short_id %q should be 3 chars", c.Label, c.ShortID)
		}
	}
}

// TestInsertCriteria_ShortIDsUniqueAcrossTasks pins the global-uniqueness
// claim. Cross-task references like "aez2c x7e" and the JS replay-fold's
// indexing both depend on short_id being a primary handle, not a per-task
// suffix.
func TestInsertCriteria_ShortIDsUniqueAcrossTasks(t *testing.T) {
	db := SetupTestDB(t)
	a := MustAdd(t, db, "", "Task A")
	b := MustAdd(t, db, "", "Task B")
	insA, err := RunAddCriteria(db, a, []Criterion{{Label: "p"}, {Label: "q"}}, TestActor)
	if err != nil {
		t.Fatalf("RunAddCriteria a: %v", err)
	}
	insB, err := RunAddCriteria(db, b, []Criterion{{Label: "r"}, {Label: "s"}}, TestActor)
	if err != nil {
		t.Fatalf("RunAddCriteria b: %v", err)
	}
	seen := map[string]bool{}
	for _, c := range insA {
		seen[c.ShortID] = true
	}
	for _, c := range insB {
		if seen[c.ShortID] {
			t.Errorf("collision: %q reused across tasks", c.ShortID)
		}
		seen[c.ShortID] = true
	}
}

// TestRunSetCriterion_AcceptsShortID confirms the resolver tries short_id
// first, so callers can pass a 3-char handle instead of a long quoted
// label and the criterion is still found.
func TestRunSetCriterion_AcceptsShortID(t *testing.T) {
	db := SetupTestDB(t)
	id := MustAdd(t, db, "", "Task")
	inserted, err := RunAddCriteria(db, id, []Criterion{{Label: "long quoted (label)"}}, TestActor)
	if err != nil {
		t.Fatalf("RunAddCriteria: %v", err)
	}
	if len(inserted) != 1 || inserted[0].ShortID == "" {
		t.Fatalf("expected one inserted criterion with a short_id; got %+v", inserted)
	}

	prior, err := RunSetCriterion(db, id, inserted[0].ShortID, CriterionPassed, TestActor)
	if err != nil {
		t.Fatalf("RunSetCriterion via short_id: %v", err)
	}
	if prior != CriterionPending {
		t.Errorf("prior: got %q, want pending", prior)
	}
}

// TestRunSetCriterion_LabelFallbackPreserved guards the backwards-compat
// criterion: existing label-based callers keep working unchanged. The
// fallback path also covers legacy `--criterion "Label=state"` forms.
func TestRunSetCriterion_LabelFallbackPreserved(t *testing.T) {
	db := SetupTestDB(t)
	id := MustAdd(t, db, "", "Task")
	if _, err := RunAddCriteria(db, id, []Criterion{{Label: "by-label"}}, TestActor); err != nil {
		t.Fatalf("RunAddCriteria: %v", err)
	}

	if _, err := RunSetCriterion(db, id, "by-label", CriterionPassed, TestActor); err != nil {
		t.Fatalf("RunSetCriterion via label: %v", err)
	}
}

// TestRunSetCriterion_RecordsShortIDOnEvent ensures the event log carries
// the stable identity, so a later `label` rewrite does not orphan the
// timeline. The forward replay-fold uses this to match by short_id.
func TestRunSetCriterion_RecordsShortIDOnEvent(t *testing.T) {
	db := SetupTestDB(t)
	id := MustAdd(t, db, "", "Task")
	inserted, _ := RunAddCriteria(db, id, []Criterion{{Label: "x"}}, TestActor)
	wantShort := inserted[0].ShortID

	if _, err := RunSetCriterion(db, id, "x", CriterionPassed, TestActor); err != nil {
		t.Fatalf("RunSetCriterion: %v", err)
	}
	task := MustGet(t, db, id)
	detail, err := GetLatestEventDetail(db, task.ID, "criterion_state")
	if err != nil {
		t.Fatalf("GetLatestEventDetail: %v", err)
	}
	if detail["short_id"] != wantShort {
		t.Errorf("event short_id: got %v, want %q", detail["short_id"], wantShort)
	}
	if detail["label"] != "x" {
		t.Errorf("event should still carry label for fallback rendering; got %v", detail["label"])
	}
}

// TestCriteriaAddedEvent_CarriesShortID covers the criteria_added event so
// the JS replay-fold can stamp the short_id onto the in-memory criterion
// record at append time, ahead of any later criterion_state event.
func TestCriteriaAddedEvent_CarriesShortID(t *testing.T) {
	db := SetupTestDB(t)
	id := MustAdd(t, db, "", "Task")
	inserted, err := RunAddCriteria(db, id, []Criterion{{Label: "alpha"}, {Label: "beta"}}, TestActor)
	if err != nil {
		t.Fatalf("RunAddCriteria: %v", err)
	}

	task := MustGet(t, db, id)
	detail, err := GetLatestEventDetail(db, task.ID, "criteria_added")
	if err != nil {
		t.Fatalf("GetLatestEventDetail: %v", err)
	}
	criteria, ok := detail["criteria"].([]any)
	if !ok {
		t.Fatalf("criteria payload: got %T", detail["criteria"])
	}
	if len(criteria) != len(inserted) {
		t.Fatalf("criteria count: got %d, want %d", len(criteria), len(inserted))
	}
	for i, c := range criteria {
		m := c.(map[string]any)
		if m["short_id"] != inserted[i].ShortID {
			t.Errorf("entry %d short_id: got %v, want %q", i, m["short_id"], inserted[i].ShortID)
		}
	}
}

// TestRenderInfoMarkdown_ShowsShortIDsInCriteriaList confirms the briefing
// surfaces the short_id so an operator can copy it into the next
// `--criterion <short_id>=passed` invocation without scrolling back to
// `job log`.
func TestRenderInfoMarkdown_ShowsShortIDsInCriteriaList(t *testing.T) {
	db := SetupTestDB(t)
	id := MustAdd(t, db, "", "Task")
	inserted, _ := RunAddCriteria(db, id, []Criterion{{Label: "alpha"}, {Label: "beta"}}, TestActor)

	info, err := RunInfo(db, id)
	if err != nil {
		t.Fatalf("RunInfo: %v", err)
	}
	var sb strings.Builder
	RenderInfoMarkdown(&sb, info)
	got := sb.String()
	for _, c := range inserted {
		if !strings.Contains(got, c.ShortID) {
			t.Errorf("briefing should contain short_id %q for %q:\n%s", c.ShortID, c.Label, got)
		}
	}
}

// TestBackfillCriteriaShortIDs_FillsLegacyRows simulates the pre-migration
// state by NULLing a row's short_id and confirms backfill mints a fresh
// one without disturbing the row's other columns.
func TestBackfillCriteriaShortIDs_FillsLegacyRows(t *testing.T) {
	db := SetupTestDB(t)
	id := MustAdd(t, db, "", "Task")
	if _, err := RunAddCriteria(db, id, []Criterion{{Label: "legacy"}}, TestActor); err != nil {
		t.Fatalf("RunAddCriteria: %v", err)
	}
	// Roll back the row to the pre-migration state by clearing short_id.
	if _, err := db.Exec("UPDATE task_criteria SET short_id = NULL WHERE label = ?", "legacy"); err != nil {
		t.Fatalf("clear short_id: %v", err)
	}

	if err := backfillCriteriaShortIDs(db); err != nil {
		t.Fatalf("backfill: %v", err)
	}
	task := MustGet(t, db, id)
	cs, _ := GetCriteria(db, task.ID)
	if len(cs) != 1 || cs[0].ShortID == "" {
		t.Errorf("backfill should mint short_id; got %+v", cs)
	}
}
