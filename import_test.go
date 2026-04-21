package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempPlan(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "plan.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}
	return path
}

func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func TestImport_HappyPath(t *testing.T) {
	db := setupTestDB(t)

	body := "# Plan\n\nSome narrative.\n\n" +
		"```yaml\n" +
		"tasks:\n" +
		"  - title: Ship v1\n" +
		"    ref: ship\n" +
		"    children:\n" +
		"      - title: Write tests\n" +
		"        desc: cover happy path\n" +
		"        labels: [testing, phase-2]\n" +
		"      - title: Fix CI\n" +
		"        blockedBy: [Write tests]\n" +
		"  - title: Ship v2\n" +
		"    blockedBy: [ship]\n" +
		"    children:\n" +
		"      - title: Plan v2\n" +
		"```\n"

	path := writeTempPlan(t, body)

	res, err := runImport(db, path, "", false, "alice")
	if err != nil {
		t.Fatalf("runImport: %v", err)
	}
	if res == nil {
		t.Fatal("expected result")
	}
	if res.DryRun {
		t.Error("DryRun should be false")
	}
	if len(res.Tasks) != 5 {
		t.Fatalf("tasks: got %d, want 5", len(res.Tasks))
	}

	wantTitles := []string{"Ship v1", "Write tests", "Fix CI", "Ship v2", "Plan v2"}
	for i, want := range wantTitles {
		if res.Tasks[i].Title != want {
			t.Errorf("tasks[%d].Title = %q, want %q", i, res.Tasks[i].Title, want)
		}
		if res.Tasks[i].ID == "" {
			t.Errorf("tasks[%d].ID must be non-empty on real run", i)
		}
	}
	// Ship v1 and Ship v2 are roots (Parent empty); others have parent short IDs.
	if res.Tasks[0].Parent != "" {
		t.Errorf("Ship v1 Parent = %q, want empty", res.Tasks[0].Parent)
	}
	if res.Tasks[3].Parent != "" {
		t.Errorf("Ship v2 Parent = %q, want empty", res.Tasks[3].Parent)
	}
	if res.Tasks[1].Parent != res.Tasks[0].ID {
		t.Errorf("Write tests parent = %q, want %q", res.Tasks[1].Parent, res.Tasks[0].ID)
	}

	// Verify DB state
	shipV1, _ := getTaskByShortID(db, res.Tasks[0].ID)
	writeTests, _ := getTaskByShortID(db, res.Tasks[1].ID)
	fixCI, _ := getTaskByShortID(db, res.Tasks[2].ID)
	shipV2, _ := getTaskByShortID(db, res.Tasks[3].ID)

	if shipV1 == nil || writeTests == nil || fixCI == nil || shipV2 == nil {
		t.Fatal("tasks not found after import")
	}
	if writeTests.ParentID == nil || *writeTests.ParentID != shipV1.ID {
		t.Error("Write tests parent is not Ship v1")
	}
	if writeTests.Description != "cover happy path" {
		t.Errorf("Write tests description = %q", writeTests.Description)
	}

	// blocks: Fix CI blocked by Write tests; Ship v2 blocked by Ship v1
	var n int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM blocks WHERE blocker_id = ? AND blocked_id = ?",
		writeTests.ID, fixCI.ID,
	).Scan(&n); err != nil {
		t.Fatalf("query blocks: %v", err)
	}
	if n != 1 {
		t.Errorf("block Fix CI <- Write tests: got %d rows, want 1", n)
	}
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM blocks WHERE blocker_id = ? AND blocked_id = ?",
		shipV1.ID, shipV2.ID,
	).Scan(&n); err != nil {
		t.Fatalf("query blocks: %v", err)
	}
	if n != 1 {
		t.Errorf("block Ship v2 <- Ship v1: got %d rows, want 1", n)
	}

	// Every task has a created event with actor=alice.
	for _, rt := range res.Tasks {
		task, _ := getTaskByShortID(db, rt.ID)
		var actor string
		if err := db.QueryRow(
			"SELECT actor FROM events WHERE task_id = ? AND event_type = 'created' LIMIT 1",
			task.ID,
		).Scan(&actor); err != nil {
			t.Fatalf("select actor: %v", err)
		}
		if actor != "alice" {
			t.Errorf("%s actor = %q, want alice", rt.Title, actor)
		}
	}
}

func TestImport_Atomic_ValidationFailsNoWrites(t *testing.T) {
	db := setupTestDB(t)
	before := countRows(t, db, "tasks")
	beforeEvents := countRows(t, db, "events")

	body := "```yaml\n" +
		"tasks:\n" +
		"  - title: Root\n" +
		"    children:\n" +
		"      - title: Child\n" +
		"        blockedBy: [nonexistent-ref-or-title]\n" +
		"```\n"
	path := writeTempPlan(t, body)

	_, err := runImport(db, path, "", false, "alice")
	if err == nil {
		t.Fatal("expected error")
	}
	want := `tasks[0].children[0]: blockedBy[0] "nonexistent-ref-or-title" does not match any ref, imported task title, or existing task ID`
	if err.Error() != want {
		t.Errorf("error:\n  got:  %q\n  want: %q", err.Error(), want)
	}

	if got := countRows(t, db, "tasks"); got != before {
		t.Errorf("tasks rows: got %d, want %d (no writes)", got, before)
	}
	if got := countRows(t, db, "events"); got != beforeEvents {
		t.Errorf("events rows: got %d, want %d (no writes)", got, beforeEvents)
	}
}

func TestImport_MissingTitle(t *testing.T) {
	db := setupTestDB(t)
	body := "```yaml\n" +
		"tasks:\n" +
		"  - title: Root\n" +
		"    children:\n" +
		"      - desc: no title here\n" +
		"```\n"
	path := writeTempPlan(t, body)

	_, err := runImport(db, path, "", false, "alice")
	if err == nil {
		t.Fatal("expected error")
	}
	want := `tasks[0].children[0]: title is required`
	if err.Error() != want {
		t.Errorf("error:\n  got:  %q\n  want: %q", err.Error(), want)
	}
}

func TestImport_DuplicateRef(t *testing.T) {
	db := setupTestDB(t)
	body := "```yaml\n" +
		"tasks:\n" +
		"  - title: A\n" +
		"    ref: foo\n" +
		"  - title: B\n" +
		"    ref: foo\n" +
		"```\n"
	path := writeTempPlan(t, body)

	_, err := runImport(db, path, "", false, "alice")
	if err == nil {
		t.Fatal("expected error")
	}
	want := `tasks[1]: ref "foo" is already used at tasks[0]`
	if err.Error() != want {
		t.Errorf("error:\n  got:  %q\n  want: %q", err.Error(), want)
	}
}

func TestImport_AmbiguousBlockedByTitle(t *testing.T) {
	db := setupTestDB(t)
	body := "```yaml\n" +
		"tasks:\n" +
		"  - title: Write tests\n" +
		"  - title: Write tests\n" +
		"  - title: Ship\n" +
		"    blockedBy: [Write tests]\n" +
		"```\n"
	path := writeTempPlan(t, body)

	_, err := runImport(db, path, "", false, "alice")
	if err == nil {
		t.Fatal("expected error")
	}
	want := `tasks[2]: blockedBy[0] "Write tests" matches multiple tasks; use a ref or a short ID to disambiguate`
	if err.Error() != want {
		t.Errorf("error:\n  got:  %q\n  want: %q", err.Error(), want)
	}
}

func TestImport_BlockedByUsingExistingDBShortID(t *testing.T) {
	db := setupTestDB(t)
	existing := mustAdd(t, db, "", "Pre-existing")

	body := "```yaml\n" +
		"tasks:\n" +
		"  - title: New root\n" +
		"    blockedBy: [" + existing + "]\n" +
		"```\n"
	path := writeTempPlan(t, body)

	res, err := runImport(db, path, "", false, "alice")
	if err != nil {
		t.Fatalf("runImport: %v", err)
	}
	if len(res.Tasks) != 1 {
		t.Fatalf("tasks: got %d, want 1", len(res.Tasks))
	}

	newTask, _ := getTaskByShortID(db, res.Tasks[0].ID)
	prev, _ := getTaskByShortID(db, existing)
	var n int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM blocks WHERE blocker_id = ? AND blocked_id = ?",
		prev.ID, newTask.ID,
	).Scan(&n); err != nil {
		t.Fatalf("query blocks: %v", err)
	}
	if n != 1 {
		t.Errorf("block not created; got %d rows", n)
	}
}

func TestImport_CrossSubtreeRefForwardReference(t *testing.T) {
	db := setupTestDB(t)
	body := "```yaml\n" +
		"tasks:\n" +
		"  - title: Alpha\n" +
		"  - title: Beta\n" +
		"    blockedBy: [gamma-ref]\n" +
		"  - title: Gamma\n" +
		"    ref: gamma-ref\n" +
		"```\n"
	path := writeTempPlan(t, body)

	res, err := runImport(db, path, "", false, "alice")
	if err != nil {
		t.Fatalf("runImport: %v", err)
	}
	if len(res.Tasks) != 3 {
		t.Fatalf("tasks: got %d, want 3", len(res.Tasks))
	}

	beta, _ := getTaskByShortID(db, res.Tasks[1].ID)
	gamma, _ := getTaskByShortID(db, res.Tasks[2].ID)
	var n int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM blocks WHERE blocker_id = ? AND blocked_id = ?",
		gamma.ID, beta.ID,
	).Scan(&n); err != nil {
		t.Fatalf("query blocks: %v", err)
	}
	if n != 1 {
		t.Errorf("Beta should be blocked by Gamma; got %d block rows", n)
	}
}

func TestImport_DryRun(t *testing.T) {
	db := setupTestDB(t)
	before := countRows(t, db, "tasks")
	beforeEvents := countRows(t, db, "events")

	body := "```yaml\n" +
		"tasks:\n" +
		"  - title: First\n" +
		"  - title: Second\n" +
		"```\n"
	path := writeTempPlan(t, body)

	res, err := runImport(db, path, "", true, "alice")
	if err != nil {
		t.Fatalf("runImport dry: %v", err)
	}
	if !res.DryRun {
		t.Error("DryRun flag must be true")
	}
	if len(res.Tasks) != 2 {
		t.Fatalf("tasks: got %d, want 2", len(res.Tasks))
	}
	if res.Tasks[0].ID != "<new-1>" || res.Tasks[1].ID != "<new-2>" {
		t.Errorf("placeholders: got %v / %v", res.Tasks[0].ID, res.Tasks[1].ID)
	}

	if got := countRows(t, db, "tasks"); got != before {
		t.Errorf("dry-run wrote tasks: %d vs %d", got, before)
	}
	if got := countRows(t, db, "events"); got != beforeEvents {
		t.Errorf("dry-run wrote events: %d vs %d", got, beforeEvents)
	}

	// Real follow-up run creates actual IDs, distinct from placeholders.
	real, err := runImport(db, path, "", false, "alice")
	if err != nil {
		t.Fatalf("real runImport: %v", err)
	}
	for _, rt := range real.Tasks {
		if strings.HasPrefix(rt.ID, "<new-") {
			t.Errorf("real run should not use placeholders, got %q", rt.ID)
		}
	}
}

func TestImport_Parent(t *testing.T) {
	db := setupTestDB(t)
	parent := mustAdd(t, db, "", "Existing root")

	body := "```yaml\n" +
		"tasks:\n" +
		"  - title: Imported root A\n" +
		"  - title: Imported root B\n" +
		"```\n"
	path := writeTempPlan(t, body)

	res, err := runImport(db, path, parent, false, "alice")
	if err != nil {
		t.Fatalf("runImport: %v", err)
	}
	parentTask, _ := getTaskByShortID(db, parent)
	for _, rt := range res.Tasks {
		ta, _ := getTaskByShortID(db, rt.ID)
		if ta.ParentID == nil || *ta.ParentID != parentTask.ID {
			t.Errorf("%s parent not set to %s", rt.Title, parent)
		}
		if rt.Parent != parent {
			t.Errorf("result Parent = %q, want %q", rt.Parent, parent)
		}
	}
}

func TestImport_ParentNotFound(t *testing.T) {
	db := setupTestDB(t)
	before := countRows(t, db, "tasks")

	body := "```yaml\n" +
		"tasks:\n" +
		"  - title: Any\n" +
		"```\n"
	path := writeTempPlan(t, body)

	_, err := runImport(db, path, "bogus", false, "alice")
	if err == nil {
		t.Fatal("expected error")
	}
	want := `parent task "bogus" not found`
	if err.Error() != want {
		t.Errorf("error:\n  got:  %q\n  want: %q", err.Error(), want)
	}
	if got := countRows(t, db, "tasks"); got != before {
		t.Errorf("wrote tasks on validation fail: %d vs %d", got, before)
	}
}

func TestImport_FirstTasksBlockWins(t *testing.T) {
	db := setupTestDB(t)
	body := "Some doc.\n\n" +
		"```yaml\n" +
		"foo: bar\n" +
		"```\n\n" +
		"More text.\n\n" +
		"```yaml\n" +
		"tasks:\n" +
		"  - title: Real one\n" +
		"```\n\n" +
		"```yaml\n" +
		"tasks:\n" +
		"  - title: Ignored trailing block\n" +
		"```\n"

	path := writeTempPlan(t, body)
	res, err := runImport(db, path, "", false, "alice")
	if err != nil {
		t.Fatalf("runImport: %v", err)
	}
	if len(res.Tasks) != 1 {
		t.Fatalf("tasks: got %d, want 1 (only first matching block)", len(res.Tasks))
	}
	if res.Tasks[0].Title != "Real one" {
		t.Errorf("title = %q, want %q", res.Tasks[0].Title, "Real one")
	}
}

func TestImport_UnlabeledFenceAccepted(t *testing.T) {
	db := setupTestDB(t)
	body := "# Plan\n\n" +
		"```\n" +
		"tasks:\n" +
		"  - title: OK\n" +
		"```\n"

	path := writeTempPlan(t, body)
	res, err := runImport(db, path, "", false, "alice")
	if err != nil {
		t.Fatalf("runImport: %v", err)
	}
	if len(res.Tasks) != 1 || res.Tasks[0].Title != "OK" {
		t.Fatalf("unexpected result: %#v", res.Tasks)
	}
}

func TestImport_NoTasksBlock_Errors(t *testing.T) {
	db := setupTestDB(t)
	body := "# Prose only. No fences.\n"
	path := writeTempPlan(t, body)

	_, err := runImport(db, path, "", false, "alice")
	if err == nil {
		t.Fatal("expected error")
	}
	want := "no YAML `tasks:` block found in " + path
	if err.Error() != want {
		t.Errorf("error:\n  got:  %q\n  want: %q", err.Error(), want)
	}
}
