package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCancel_Single_Happy(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := mustAdd(t, db, "", "Write red tests")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "cancel", id, "--reason", "no longer needed")
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	wantHead := `Canceled: ` + id + ` "Write red tests"`
	if !strings.Contains(stdout, wantHead) {
		t.Errorf("missing headline:\n%s", stdout)
	}
	if !strings.Contains(stdout, "  reason: no longer needed") {
		t.Errorf("missing reason line:\n%s", stdout)
	}

	db = openTestDB(t, dbFile)
	task := mustGet(t, db, id)
	if task.Status != "canceled" {
		t.Errorf("status: got %q, want canceled", task.Status)
	}
	detail, _ := getLatestEventDetail(db, task.ID, "canceled")
	if detail["reason"] != "no longer needed" {
		t.Errorf("event reason: got %v", detail["reason"])
	}
}

func TestCancel_Variadic(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	a := mustAdd(t, db, "", "A")
	b := mustAdd(t, db, "", "B")
	c := mustAdd(t, db, "", "C")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "cancel", a, b, c, "--reason", "scope cut")
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if !strings.HasPrefix(stdout, "Canceled 3 tasks:\n") {
		t.Errorf("multi headline:\n%s", stdout)
	}
	for _, id := range []string{a, b, c} {
		if !strings.Contains(stdout, "- Canceled: "+id) {
			t.Errorf("missing per-id line for %s:\n%s", id, stdout)
		}
	}

	db = openTestDB(t, dbFile)
	for _, id := range []string{a, b, c} {
		task := mustGet(t, db, id)
		if task.Status != "canceled" {
			t.Errorf("%s: status=%q, want canceled", id, task.Status)
		}
	}
}

func TestCancel_AllOrNothing(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	a := mustAdd(t, db, "", "A")
	b := mustAdd(t, db, "", "B")
	db.Close()

	_, _, err := runCLI(t, dbFile, "--as", "alice", "cancel", a, "noExs", b, "--reason", "x")
	if err == nil {
		t.Fatal("expected error with invalid id")
	}

	db = openTestDB(t, dbFile)
	for _, id := range []string{a, b} {
		task := mustGet(t, db, id)
		if task.Status == "canceled" {
			t.Errorf("%s was canceled despite failure", id)
		}
	}
}

func TestCancel_Cascade_ClosesOpenDescendants(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	parent := mustAdd(t, db, "", "Parent")
	c1 := mustAdd(t, db, parent, "C1")
	gc := mustAdd(t, db, c1, "GC")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "cancel", parent, "--reason", "pivot", "--cascade")
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if !strings.Contains(stdout, "and 2 subtasks") {
		t.Errorf("missing cascade count:\n%s", stdout)
	}

	db = openTestDB(t, dbFile)
	for _, id := range []string{parent, c1, gc} {
		task := mustGet(t, db, id)
		if task.Status != "canceled" {
			t.Errorf("%s: status=%q, want canceled", id, task.Status)
		}
	}
}

func TestCancel_Cascade_SkipsAlreadyDone(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	parent := mustAdd(t, db, "", "Parent")
	c1 := mustAdd(t, db, parent, "C1")
	c2 := mustAdd(t, db, parent, "C2")
	mustDone(t, db, c1)
	db.Close()

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "cancel", parent, "--reason", "x", "--cascade"); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	db = openTestDB(t, dbFile)
	if t1 := mustGet(t, db, c1); t1.Status != "done" {
		t.Errorf("done child should remain done, got %q", t1.Status)
	}
	if t2 := mustGet(t, db, c2); t2.Status != "canceled" {
		t.Errorf("open child should be canceled, got %q", t2.Status)
	}
}

func TestCancel_MissingReason_Errors(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := mustAdd(t, db, "", "X")
	db.Close()

	_, _, err := runCLI(t, dbFile, "--as", "alice", "cancel", id)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `cancel requires --reason "<text>"`) {
		t.Errorf("err: %v", err)
	}
}

func TestCancel_OnDone_Errors(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := mustAdd(t, db, "", "X")
	mustDone(t, db, id)
	db.Close()

	_, _, err := runCLI(t, dbFile, "--as", "alice", "cancel", id, "--reason", "x")
	if err == nil {
		t.Fatal("expected error")
	}
	want := "task " + id + " is already done; cancel only applies to open work"
	if err.Error() != want {
		t.Errorf("err:\n  got:  %q\n  want: %q", err.Error(), want)
	}
}

func TestCancel_Idempotent_Single(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := mustAdd(t, db, "", "X")
	db.Close()

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "cancel", id, "--reason", "x"); err != nil {
		t.Fatalf("first cancel: %v", err)
	}

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "cancel", id, "--reason", "y")
	if err != nil {
		t.Fatalf("second cancel: %v", err)
	}
	want := "Already canceled: " + id + "\n"
	if stdout != want {
		t.Errorf("got %q, want %q", stdout, want)
	}
}

func TestCancel_Idempotent_InVariadic(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	a := mustAdd(t, db, "", "A")
	b := mustAdd(t, db, "", "B")
	db.Close()

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "cancel", a, "--reason", "first"); err != nil {
		t.Fatalf("seed cancel: %v", err)
	}

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "cancel", a, b, "--reason", "second")
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if !strings.Contains(stdout, "already canceled: "+a) {
		t.Errorf("missing already-canceled line:\n%s", stdout)
	}
	if !strings.Contains(stdout, "- Canceled: "+b) {
		t.Errorf("missing per-id line for %s:\n%s", b, stdout)
	}
}

func TestCancel_UnblocksDependents(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	a := mustAdd(t, db, "", "A")
	dep := mustAdd(t, db, "", "Dep")
	if err := runBlock(db, dep, a, testActor); err != nil {
		t.Fatalf("block: %v", err)
	}
	db.Close()

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "cancel", a, "--reason", "drop"); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	db = openTestDB(t, dbFile)
	bls, err := getBlockers(db, dep)
	if err != nil {
		t.Fatalf("getBlockers: %v", err)
	}
	if len(bls) != 0 {
		t.Errorf("expected dep unblocked, got %d blockers", len(bls))
	}
	depTask := mustGet(t, db, dep)
	detail, _ := getLatestEventDetail(db, depTask.ID, "unblocked")
	if detail["reason"] != "blocker_canceled" {
		t.Errorf("unblock reason: got %v, want blocker_canceled", detail["reason"])
	}
}

func TestCancel_ClaimedByOtherErrors(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := mustAdd(t, db, "", "X")
	if err := runClaim(db, id, "1h", "alice", false); err != nil {
		t.Fatalf("claim: %v", err)
	}
	db.Close()

	_, _, err := runCLI(t, dbFile, "--as", "bob", "cancel", id, "--reason", "x")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "claimed by alice") {
		t.Errorf("err: %v", err)
	}
}

func TestCancel_ClaimedByCallerSucceeds_DropsClaim(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := mustAdd(t, db, "", "X")
	if err := runClaim(db, id, "1h", "alice", false); err != nil {
		t.Fatalf("claim: %v", err)
	}
	db.Close()

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "cancel", id, "--reason", "x"); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	db = openTestDB(t, dbFile)
	task := mustGet(t, db, id)
	if task.Status != "canceled" {
		t.Errorf("status: got %q, want canceled", task.Status)
	}
	if task.ClaimedBy != nil {
		t.Errorf("claim should be dropped, got %v", *task.ClaimedBy)
	}
	if task.ClaimExpiresAt != nil {
		t.Errorf("claim_expires_at should be cleared, got %v", *task.ClaimExpiresAt)
	}
}

func TestCancel_Md_Single_Shape(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := mustAdd(t, db, "", "Write red tests")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "cancel", id, "--reason", "no longer needed")
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	want := "Canceled: " + id + " \"Write red tests\"\n  reason: no longer needed\n"
	if stdout != want {
		t.Errorf("got %q, want %q", stdout, want)
	}
}

func TestCancel_Md_Multi_Shape(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	a := mustAdd(t, db, "", "A")
	b := mustAdd(t, db, "", "B")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "cancel", a, b, "--reason", "scope cut")
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if !strings.HasPrefix(stdout, "Canceled 2 tasks:\n") {
		t.Errorf("headline:\n%s", stdout)
	}
	if !strings.Contains(stdout, "- Canceled: "+a+" \"A\"\n") {
		t.Errorf("missing a line:\n%s", stdout)
	}
	if !strings.Contains(stdout, "- Canceled: "+b+" \"B\"\n") {
		t.Errorf("missing b line:\n%s", stdout)
	}
	if !strings.Contains(stdout, "  reason: scope cut\n") {
		t.Errorf("missing reason line:\n%s", stdout)
	}
}

func TestCancel_Json_Shape(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := mustAdd(t, db, "", "X")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "cancel", id, "--reason", "x", "--format=json")
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, stdout)
	}
	if got["reason"] != "x" {
		t.Errorf("reason: %v", got["reason"])
	}
	if got["purged"] != false {
		t.Errorf("purged: %v", got["purged"])
	}
	canceled, ok := got["canceled"].([]any)
	if !ok || len(canceled) != 1 {
		t.Fatalf("canceled: %v", got["canceled"])
	}
	first := canceled[0].(map[string]any)
	if first["id"] != id || first["title"] != "X" {
		t.Errorf("canceled[0]: %v", first)
	}
}

func TestCancel_Purge_EraseTaskAndEvents(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	parent := mustAdd(t, db, "", "Parent")
	child := mustAdd(t, db, parent, "Child to purge")
	parentTask := mustGet(t, db, parent)
	childTask := mustGet(t, db, child)
	childID := childTask.ID
	db.Close()

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "cancel", "--purge", child, "--reason", "garbage"); err != nil {
		t.Fatalf("cancel --purge: %v", err)
	}

	db = openTestDB(t, dbFile)
	row, _ := getTaskByShortID(db, child)
	if row != nil {
		t.Errorf("child task row should be erased")
	}
	var nEvents int
	db.QueryRow("SELECT COUNT(*) FROM events WHERE task_id = ?", childID).Scan(&nEvents)
	if nEvents != 0 {
		t.Errorf("child events should be erased, got %d", nEvents)
	}
	// Purged event should appear on the parent.
	detail, err := getLatestEventDetail(db, parentTask.ID, "purged")
	if err != nil {
		t.Fatalf("getLatestEventDetail: %v", err)
	}
	if detail == nil {
		t.Fatal("expected purged event on parent")
	}
	if detail["purged_id"] != child {
		t.Errorf("purged_id: %v", detail["purged_id"])
	}
}

func TestCancel_Purge_RootTask_EventOnNullTaskID(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := mustAdd(t, db, "", "Root")
	db.Close()

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "cancel", "--purge", id, "--reason", "drop"); err != nil {
		t.Fatalf("cancel --purge: %v", err)
	}

	db = openTestDB(t, dbFile)
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM events WHERE task_id IS NULL AND event_type = 'purged'").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("expected 1 orphan purged event, got %d", n)
	}
}

func TestCancel_Purge_MissingReason_Errors(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := mustAdd(t, db, "", "X")
	db.Close()

	_, _, err := runCLI(t, dbFile, "--as", "alice", "cancel", "--purge", id)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `cancel --purge requires --reason "<text>"`) {
		t.Errorf("err: %v", err)
	}
}

func TestCancel_Purge_RequiresCascade_WhenChildrenPresent(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	parent := mustAdd(t, db, "", "Parent")
	_ = mustAdd(t, db, parent, "Child")
	db.Close()

	_, _, err := runCLI(t, dbFile, "--as", "alice", "cancel", "--purge", parent, "--reason", "x")
	if err == nil {
		t.Fatal("expected error")
	}
	want := "task " + parent + " has subtasks; add --cascade --yes to purge the subtree"
	if err.Error() != want {
		t.Errorf("err:\n  got:  %q\n  want: %q", err.Error(), want)
	}
}

func TestCancel_Purge_Cascade_Yes_Erases(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	parent := mustAdd(t, db, "", "Parent")
	c1 := mustAdd(t, db, parent, "C1")
	gc := mustAdd(t, db, c1, "GC")
	db.Close()

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "cancel", "--purge", "--cascade", "--yes", parent, "--reason", "drop"); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	db = openTestDB(t, dbFile)
	for _, id := range []string{parent, c1, gc} {
		task, _ := getTaskByShortID(db, id)
		if task != nil {
			t.Errorf("%s should be erased", id)
		}
	}
}

func TestCancel_Purge_Cascade_WithoutYes_Errors(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	parent := mustAdd(t, db, "", "Parent")
	_ = mustAdd(t, db, parent, "C1")
	db.Close()

	_, _, err := runCLI(t, dbFile, "--as", "alice", "cancel", "--purge", "--cascade", parent, "--reason", "drop")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "requires --yes") {
		t.Errorf("err: %v", err)
	}
	if !strings.Contains(err.Error(), "irrecoverable erasure of 2 tasks") {
		t.Errorf("err should name count: %v", err)
	}
}

func TestCancel_Purge_RemovesBlocksCleanly(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	a := mustAdd(t, db, "", "A")
	b := mustAdd(t, db, "", "B")
	if err := runBlock(db, b, a, testActor); err != nil {
		t.Fatalf("block: %v", err)
	}
	db.Close()

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "cancel", "--purge", a, "--reason", "x"); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	db = openTestDB(t, dbFile)
	bls, err := getBlockers(db, b)
	if err != nil {
		t.Fatalf("getBlockers: %v", err)
	}
	if len(bls) != 0 {
		t.Errorf("expected no blockers, got %d", len(bls))
	}
}

func TestRemove_Unknown(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := mustAdd(t, db, "", "X")
	db.Close()

	_, _, err := runCLI(t, dbFile, "remove", id)
	if err == nil {
		t.Fatal("expected unknown command error")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("err: %v", err)
	}
}
