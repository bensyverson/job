package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDone_Variadic_Happy(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	parent := mustAdd(t, db, "", "Parent")
	c1 := mustAdd(t, db, parent, "C1")
	c2 := mustAdd(t, db, parent, "C2")
	c3 := mustAdd(t, db, parent, "C3")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "done", c1, c2, c3)
	if err != nil {
		t.Fatalf("done: %v", err)
	}
	if !strings.HasPrefix(stdout, "Closed 3 tasks:\n") {
		t.Errorf("headline:\n%s", stdout)
	}
	for _, id := range []string{c1, c2, c3} {
		if !strings.Contains(stdout, "- Done: "+id) {
			t.Errorf("missing per-id line for %s:\n%s", id, stdout)
		}
	}

	db = openTestDB(t, dbFile)
	for _, id := range []string{c1, c2, c3} {
		task := mustGet(t, db, id)
		if task.Status != "done" {
			t.Errorf("%s: status=%q, want done", id, task.Status)
		}
	}
}

func TestDone_Variadic_AllOrNothing(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	a := mustAdd(t, db, "", "A")
	b := mustAdd(t, db, "", "B")
	db.Close()

	_, _, err := runCLI(t, dbFile, "--as", "alice", "done", a, "noExs", b)
	if err == nil {
		t.Fatal("expected error with invalid id")
	}

	db = openTestDB(t, dbFile)
	for _, id := range []string{a, b} {
		task := mustGet(t, db, id)
		if task.Status == "done" {
			t.Errorf("%s was closed despite failure", id)
		}
	}
}

func TestDone_Cascade_ClosesDescendants(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	parent := mustAdd(t, db, "", "Parent")
	c1 := mustAdd(t, db, parent, "C1")
	gc := mustAdd(t, db, c1, "GC")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "done", parent, "--cascade")
	if err != nil {
		t.Fatalf("done: %v", err)
	}
	if !strings.Contains(stdout, "and 2 subtasks") {
		t.Errorf("missing cascade count:\n%s", stdout)
	}

	db = openTestDB(t, dbFile)
	for _, id := range []string{parent, c1, gc} {
		task := mustGet(t, db, id)
		if task.Status != "done" {
			t.Errorf("%s: status=%q, want done", id, task.Status)
		}
	}
}

func TestDone_Cascade_Variadic_Composes(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	p1 := mustAdd(t, db, "", "P1")
	c1 := mustAdd(t, db, p1, "C1")
	p2 := mustAdd(t, db, "", "P2")
	c2 := mustAdd(t, db, p2, "C2")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "done", "--cascade", p1, p2)
	if err != nil {
		t.Fatalf("done: %v", err)
	}
	if !strings.HasPrefix(stdout, "Closed 2 tasks:\n") {
		t.Errorf("headline:\n%s", stdout)
	}

	db = openTestDB(t, dbFile)
	for _, id := range []string{p1, c1, p2, c2} {
		task := mustGet(t, db, id)
		if task.Status != "done" {
			t.Errorf("%s: status=%q, want done", id, task.Status)
		}
	}
}

func TestDone_WithoutCascade_OpenChildrenErrors(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	p := mustAdd(t, db, "", "Parent")
	_ = mustAdd(t, db, p, "Child")
	db.Close()

	_, _, err := runCLI(t, dbFile, "--as", "alice", "done", p)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--cascade "+p) {
		t.Errorf("missing --cascade hint: %v", err)
	}
}

func TestDone_ForceFlag_Gone(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := mustAdd(t, db, "", "Task")
	db.Close()

	_, _, err := runCLI(t, dbFile, "--as", "alice", "done", id, "--force")
	if err == nil {
		t.Fatal("expected --force to be unknown")
	}
	if !strings.Contains(err.Error(), "unknown flag") {
		t.Errorf("expected unknown flag error: %v", err)
	}
}

func TestDone_Note_Inline(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := mustAdd(t, db, "", "Task")
	db.Close()

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "done", id, "-m", "shipped"); err != nil {
		t.Fatalf("done: %v", err)
	}

	db = openTestDB(t, dbFile)
	task := mustGet(t, db, id)
	if task.CompletionNote == nil || *task.CompletionNote != "shipped" {
		t.Errorf("completion_note: got %v, want %q", task.CompletionNote, "shipped")
	}
	detail, _ := getLatestEventDetail(db, task.ID, "done")
	if detail["note"] != "shipped" {
		t.Errorf("event note: got %v", detail["note"])
	}
}

func TestDone_Result_Json(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := mustAdd(t, db, "", "Task")
	db.Close()

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "done", id, "--result", `{"k":1}`); err != nil {
		t.Fatalf("done: %v", err)
	}

	db = openTestDB(t, dbFile)
	task := mustGet(t, db, id)
	detail, _ := getLatestEventDetail(db, task.ID, "done")
	result, ok := detail["result"].(map[string]any)
	if !ok {
		t.Fatalf("result: got %T, want map", detail["result"])
	}
	if result["k"] != float64(1) {
		t.Errorf("result[k]: got %v", result["k"])
	}
}

func TestDone_Result_InvalidJson(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := mustAdd(t, db, "", "Task")
	db.Close()

	_, _, err := runCLI(t, dbFile, "--as", "alice", "done", id, "--result", "not-json")
	if err == nil {
		t.Fatal("expected invalid JSON error")
	}
	if !strings.Contains(err.Error(), "invalid JSON") {
		t.Errorf("err: %v", err)
	}
}

func TestDone_NoteAndResult_Both(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := mustAdd(t, db, "", "Task")
	db.Close()

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "done", id, "-m", "n", "--result", `{"x":2}`); err != nil {
		t.Fatalf("done: %v", err)
	}

	db = openTestDB(t, dbFile)
	task := mustGet(t, db, id)
	if task.CompletionNote == nil || *task.CompletionNote != "n" {
		t.Errorf("note: %v", task.CompletionNote)
	}
	detail, _ := getLatestEventDetail(db, task.ID, "done")
	if detail["note"] != "n" {
		t.Errorf("event note: %v", detail["note"])
	}
	result, _ := detail["result"].(map[string]any)
	if result["x"] != float64(2) {
		t.Errorf("result[x]: %v", result)
	}
}

func TestDone_Note_Positional_Gone(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := mustAdd(t, db, "", "Task")
	db.Close()

	// `done <id> "some note"` is now treated as two ids; the second ("some note") is not a task.
	_, _, err := runCLI(t, dbFile, "--as", "alice", "done", id, "some note")
	if err == nil {
		t.Fatal("expected error: second positional arg is not a valid id")
	}
}

func TestDone_Single_Uses_Phase3_Render(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	parent := mustAdd(t, db, "", "P")
	c1 := mustAdd(t, db, parent, "C1")
	_ = mustAdd(t, db, parent, "C2")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "done", c1)
	if err != nil {
		t.Fatalf("done: %v", err)
	}
	if !strings.HasPrefix(stdout, "Done: "+c1+" \"C1\"") {
		t.Errorf("want Phase 3 single headline:\n%s", stdout)
	}
	if strings.Contains(stdout, "Closed ") {
		t.Errorf("single-ID call should not use multi-headline:\n%s", stdout)
	}
}

func TestDone_Multi_Md_Shape(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	parent := mustAdd(t, db, "", "P")
	c1 := mustAdd(t, db, parent, "C1")
	c2 := mustAdd(t, db, parent, "C2")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "done", c1, c2)
	if err != nil {
		t.Fatalf("done: %v", err)
	}
	if !strings.Contains(stdout, "Closed 2 tasks:") {
		t.Errorf("missing multi headline:\n%s", stdout)
	}
	if !strings.Contains(stdout, "- Done: "+c1) {
		t.Errorf("missing c1 line:\n%s", stdout)
	}
	if !strings.Contains(stdout, "- Done: "+c2) {
		t.Errorf("missing c2 line:\n%s", stdout)
	}
}

func TestDone_Multi_Json_Shape(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	parent := mustAdd(t, db, "", "P")
	c1 := mustAdd(t, db, parent, "C1")
	c2 := mustAdd(t, db, parent, "C2")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "done", c1, c2, "--format=json")
	if err != nil {
		t.Fatalf("done: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, stdout)
	}
	closed, ok := got["closed"].([]any)
	if !ok || len(closed) != 2 {
		t.Fatalf("closed: %v", got["closed"])
	}
	first := closed[0].(map[string]any)
	if first["id"] != c1 || first["title"] != "C1" {
		t.Errorf("closed[0]: %v", first)
	}
	if _, ok := got["already_done"]; !ok {
		t.Errorf("missing already_done key")
	}
}

func TestDone_AlreadyDone_InVariadic(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	parent := mustAdd(t, db, "", "P")
	c1 := mustAdd(t, db, parent, "C1")
	c2 := mustAdd(t, db, parent, "C2")
	mustDone(t, db, c1)
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "done", c1, c2)
	if err != nil {
		t.Fatalf("done: %v", err)
	}
	if !strings.Contains(stdout, "already done: "+c1) {
		t.Errorf("missing already-done line:\n%s", stdout)
	}
	if !strings.Contains(stdout, "- Done: "+c2) {
		t.Errorf("missing c2 close line:\n%s", stdout)
	}
}

func TestDone_EnrichedAck_MultiID_FinalContext(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	parent := mustAdd(t, db, "", "P")
	c1 := mustAdd(t, db, parent, "C1")
	c2 := mustAdd(t, db, parent, "C2")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "done", c1, c2)
	if err != nil {
		t.Fatalf("done: %v", err)
	}
	wantParent := "  Parent " + parent + " complete — run 'job done " + parent + "' to close it."
	if !strings.Contains(stdout, wantParent) {
		t.Errorf("missing parent-closeable line:\n%s", stdout)
	}
}

func TestFormatEvent_LegacyForce_Renders(t *testing.T) {
	detailJSON := `{"force":true,"note":"","force_closed_children":["abc12"]}`
	out := formatEventDescription("done", detailJSON)
	if !strings.Contains(out, "done --cascade") {
		t.Errorf("legacy force should render as done --cascade: %q", out)
	}
	if !strings.Contains(out, "and 1 subtasks") {
		t.Errorf("legacy force should surface cascade count: %q", out)
	}
}
