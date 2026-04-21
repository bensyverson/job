package main

import (
	"encoding/json"
	"os"
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
	wantParent := "  Auto-closed: " + parent + " \"P\""
	if !strings.Contains(stdout, wantParent) {
		t.Errorf("missing auto-closed line:\n%s", stdout)
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

// --- P2: done --claim-next ---------------------------------------------

// The flag closes the target AND claims the next available leaf in one
// call, collapsing the most common close-then-advance flow.
func TestDone_ClaimNext_ClosesAndClaims(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	root := mustAdd(t, db, "", "Root")
	a := mustAdd(t, db, root, "A")
	b := mustAdd(t, db, root, "B")
	db.Close()

	// alice claims a, then done --claim-next: should close a, then claim b.
	if _, _, err := runCLI(t, dbFile, "--as", "alice", "claim", a); err != nil {
		t.Fatalf("claim a: %v", err)
	}
	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "done", a, "--claim-next")
	if err != nil {
		t.Fatalf("done --claim-next: %v", err)
	}
	if !strings.Contains(stdout, "Done: "+a) {
		t.Errorf("ack should include Done line for closed task:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Claimed: "+b) {
		t.Errorf("ack should include Claimed line for next leaf:\n%s", stdout)
	}

	// Verify DB state: b is claimed by alice.
	db2 := openTestDB(t, dbFile)
	bt := mustGet(t, db2, b)
	if bt.Status != "claimed" || bt.ClaimedBy == nil || *bt.ClaimedBy != "alice" {
		t.Errorf("b should be claimed by alice; status=%q, claimed_by=%v", bt.Status, bt.ClaimedBy)
	}
}

// Output shape matches bare claim's ack: each line is greppable and the
// Claimed line starts with the literal "Claimed:" so tailing agents can
// use `^Claimed:` regardless of how the claim was acquired.
func TestDone_ClaimNext_OutputShapeMatchesClaim(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	root := mustAdd(t, db, "", "Root")
	a := mustAdd(t, db, root, "A")
	_ = mustAdd(t, db, root, "B")
	db.Close()

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "claim", a); err != nil {
		t.Fatalf("claim a: %v", err)
	}
	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "done", a, "--claim-next")
	if err != nil {
		t.Fatalf("done --claim-next: %v", err)
	}
	lines := strings.Split(stdout, "\n")
	foundClaimed := false
	for _, line := range lines {
		if strings.HasPrefix(line, "Claimed: ") {
			foundClaimed = true
			if !strings.Contains(line, "expires in") {
				t.Errorf("Claimed line should include expiry, got: %q", line)
			}
		}
	}
	if !foundClaimed {
		t.Errorf("expected a line starting with 'Claimed:' for greppability:\n%s", stdout)
	}
}

// When there is no next leaf (the closed task was the last work), the
// done succeeds and the ack simply omits the Claimed line (the existing
// "All tasks in X complete" ack still fires).
func TestDone_ClaimNext_NoNextLeaf_Succeeds(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	root := mustAdd(t, db, "", "Root")
	a := mustAdd(t, db, root, "A")
	db.Close()

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "claim", a); err != nil {
		t.Fatalf("claim a: %v", err)
	}
	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "done", a, "--claim-next")
	if err != nil {
		t.Fatalf("done --claim-next should not error when no next leaf: %v", err)
	}
	if !strings.Contains(stdout, "Done: "+a) {
		t.Errorf("ack should include Done line:\n%s", stdout)
	}
	if strings.Contains(stdout, "Claimed:") {
		t.Errorf("ack should NOT include Claimed line when no next leaf:\n%s", stdout)
	}
}

// If the next leaf got claimed by someone else between the done and the
// auto-claim, done still succeeds; a status line names the taken leaf
// instead of erroring (done is irreversible, claim is opportunistic).
func TestDone_ClaimNext_RaceLostReportsStatus(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	root := mustAdd(t, db, "", "Root")
	a := mustAdd(t, db, root, "A")
	b := mustAdd(t, db, root, "B")
	db.Close()

	// alice claims a; bob claims b before alice finishes.
	if _, _, err := runCLI(t, dbFile, "--as", "alice", "claim", a); err != nil {
		t.Fatalf("claim a: %v", err)
	}
	if _, _, err := runCLI(t, dbFile, "--as", "bob", "claim", b); err != nil {
		t.Fatalf("claim b: %v", err)
	}
	// alice: done a --claim-next. b is gone; no other leaf.
	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "done", a, "--claim-next")
	if err != nil {
		t.Fatalf("done --claim-next should not error on race: %v", err)
	}
	if !strings.Contains(stdout, "Done: "+a) {
		t.Errorf("ack should include Done line:\n%s", stdout)
	}
	if strings.Contains(stdout, "Claimed: "+b) {
		t.Errorf("should not claim b, which bob already has:\n%s", stdout)
	}
}

// --- P4: positional-prose detection -------------------------------------

// A multi-word second positional to `done` is prose, not an ID. Suggest -m.
func TestDone_PositionalProse_MultiWord_SuggestsDashM(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := mustAdd(t, db, "", "Task")
	db.Close()

	_, _, err := runCLI(t, dbFile, "--as", "alice", "done", id, "wrote the red tests")
	if err == nil {
		t.Fatal("expected error for prose second positional")
	}
	if !strings.Contains(err.Error(), "-m") {
		t.Errorf("error should suggest -m: %v", err)
	}
}

// A single-word non-ID second positional is ambiguous. Heuristic: err on
// the side of suggesting -m (typoed IDs are rarer than forgotten -m).
func TestDone_PositionalProse_SingleWordNonID_SuggestsDashM(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := mustAdd(t, db, "", "Task")
	db.Close()

	// "done" is 4 chars, not a valid 5-char short-ID shape.
	_, _, err := runCLI(t, dbFile, "--as", "alice", "done", id, "done")
	if err == nil {
		t.Fatal("expected error for non-ID-shaped second positional")
	}
	if !strings.Contains(err.Error(), "-m") {
		t.Errorf("error should suggest -m: %v", err)
	}
}

// A valid 5-char short-ID-shaped second positional is treated as a task
// ID (multi-done), not as prose.
func TestDone_TwoValidIDs_StillWorks(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	a := mustAdd(t, db, "", "A")
	b := mustAdd(t, db, "", "B")
	db.Close()

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "done", a, b); err != nil {
		t.Fatalf("done of two valid IDs should work: %v", err)
	}
}

// claim with a prose second positional: suggest -m.
func TestClaim_PositionalProse_SuggestsDashM(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := mustAdd(t, db, "", "Task")
	db.Close()

	_, _, err := runCLI(t, dbFile, "--as", "alice", "claim", id, "heading into this now")
	if err == nil {
		t.Fatal("expected error for prose second positional on claim")
	}
	// claim's second arg is a duration, not a note. Suggest -m isn't the
	// right pointer here — the right pointer is "that's not a duration".
	// But for the feedback author's muscle memory, suggest a helpful hint.
	if !strings.Contains(err.Error(), "duration") && !strings.Contains(err.Error(), "-m") {
		t.Errorf("error should explain the second arg shape: %v", err)
	}
}

// --- P3: -m @file / -m - stdin support ---------------------------------

// -m @path reads the note body from a file.
func TestDone_DashM_AtFile_ReadsFromFile(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := mustAdd(t, db, "", "Task")
	db.Close()

	notePath := t.TempDir() + "/note.txt"
	contents := "multi-line\nevidence payload\nwith backticks ```and stuff```"
	if err := os.WriteFile(notePath, []byte(contents), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "done", id, "-m", "@"+notePath); err != nil {
		t.Fatalf("done -m @file: %v", err)
	}

	db2 := openTestDB(t, dbFile)
	task := mustGet(t, db2, id)
	if task.CompletionNote == nil || *task.CompletionNote != contents {
		t.Errorf("note: got %v, want %q", task.CompletionNote, contents)
	}
}

// -m - reads the note body from stdin.
func TestDone_DashM_DashReadsStdin(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := mustAdd(t, db, "", "Task")
	db.Close()

	contents := "stdin piped note\nwith newlines"
	if _, _, err := runCLIWithStdin(t, dbFile, contents, "--as", "alice", "done", id, "-m", "-"); err != nil {
		t.Fatalf("done -m -: %v", err)
	}

	db2 := openTestDB(t, dbFile)
	task := mustGet(t, db2, id)
	if task.CompletionNote == nil || *task.CompletionNote != contents {
		t.Errorf("note: got %v, want %q", task.CompletionNote, contents)
	}
}

// Literal strings starting with anything other than @ or - are unchanged.
func TestDone_DashM_LiteralStringStillWorks(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := mustAdd(t, db, "", "Task")
	db.Close()

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "done", id, "-m", "plain literal"); err != nil {
		t.Fatalf("done -m: %v", err)
	}

	db2 := openTestDB(t, dbFile)
	task := mustGet(t, db2, id)
	if task.CompletionNote == nil || *task.CompletionNote != "plain literal" {
		t.Errorf("note: got %v, want 'plain literal'", task.CompletionNote)
	}
}

// @nonexistent errors with a clear message — don't silently treat as
// literal; the user meant a file.
func TestDone_DashM_AtNonexistentFile_Errors(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := mustAdd(t, db, "", "Task")
	db.Close()

	_, _, err := runCLI(t, dbFile, "--as", "alice", "done", id, "-m", "@/does/not/exist.txt")
	if err == nil {
		t.Fatal("expected error for @nonexistent file")
	}
	if !strings.Contains(err.Error(), "-m @") && !strings.Contains(err.Error(), "read note file") {
		t.Errorf("error should explain @-file failure: %v", err)
	}
}

// File contents preserve internal newlines verbatim.
func TestDone_DashM_AtFile_PreservesNewlines(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := mustAdd(t, db, "", "Task")
	db.Close()

	notePath := t.TempDir() + "/note.txt"
	contents := "line 1\nline 2\nline 3"
	if err := os.WriteFile(notePath, []byte(contents), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "done", id, "-m", "@"+notePath); err != nil {
		t.Fatalf("done: %v", err)
	}

	db2 := openTestDB(t, dbFile)
	task := mustGet(t, db2, id)
	if task.CompletionNote == nil {
		t.Fatal("no completion note")
	}
	if !strings.Contains(*task.CompletionNote, "\n") {
		t.Errorf("newlines not preserved: %q", *task.CompletionNote)
	}
	if *task.CompletionNote != contents {
		t.Errorf("note mismatch:\n got: %q\nwant: %q", *task.CompletionNote, contents)
	}
}

// The same resolution applies to `note -m`.
func TestNote_DashM_AtFile_ReadsFromFile(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := mustAdd(t, db, "", "Task")
	db.Close()

	notePath := t.TempDir() + "/note.txt"
	contents := "note body from file"
	if err := os.WriteFile(notePath, []byte(contents), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "note", id, "-m", "@"+notePath); err != nil {
		t.Fatalf("note -m @file: %v", err)
	}

	db2 := openTestDB(t, dbFile)
	task := mustGet(t, db2, id)
	if !strings.Contains(task.Description, contents) {
		t.Errorf("note body not appended: description=%q", task.Description)
	}
}

// -m "<note>" composes with --claim-next: the note attaches to the
// closed task and the claim still fires.
func TestDone_ClaimNext_CombinesWithNote(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	root := mustAdd(t, db, "", "Root")
	a := mustAdd(t, db, root, "A")
	b := mustAdd(t, db, root, "B")
	db.Close()

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "claim", a); err != nil {
		t.Fatalf("claim a: %v", err)
	}
	if _, _, err := runCLI(t, dbFile, "--as", "alice", "done", a, "-m", "wrapped up the red tests", "--claim-next"); err != nil {
		t.Fatalf("done -m --claim-next: %v", err)
	}

	// Verify note recorded on a, and b claimed by alice.
	db2 := openTestDB(t, dbFile)
	at := mustGet(t, db2, a)
	if at.CompletionNote == nil || *at.CompletionNote != "wrapped up the red tests" {
		t.Errorf("completion note not recorded; got %v", at.CompletionNote)
	}
	bt := mustGet(t, db2, b)
	if bt.Status != "claimed" || bt.ClaimedBy == nil || *bt.ClaimedBy != "alice" {
		t.Errorf("b should be claimed; got status=%q, claimed_by=%v", bt.Status, bt.ClaimedBy)
	}
}
