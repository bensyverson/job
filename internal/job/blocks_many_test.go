package job

import (
	"strings"
	"testing"
)

// R1 — Multi-value `block add` (N blockers in one call).
//
// `RunBlockMany` accepts a list of blocker short-IDs and applies them
// atomically. Cycle detection runs across the full input set. One
// "blocked" event is recorded per edge. Duplicates in the input collapse
// to a single edge (idempotent on repeats). On any failure (missing task,
// self-block, cycle), nothing persists.
//
// `RunUnblockMany` mirrors the shape for removing N edges atomically.

func TestRunBlockMany_AddsAllInOneCall(t *testing.T) {
	db := SetupTestDB(t)
	blocked := MustAdd(t, db, "", "Gate")
	a := MustAdd(t, db, "", "A")
	b := MustAdd(t, db, "", "B")
	c := MustAdd(t, db, "", "C")

	if err := RunBlockMany(db, blocked, []string{a, b, c}, TestActor); err != nil {
		t.Fatalf("RunBlockMany: %v", err)
	}

	bls, err := GetBlockers(db, blocked)
	if err != nil {
		t.Fatalf("GetBlockers: %v", err)
	}
	if len(bls) != 3 {
		t.Fatalf("expected 3 blockers, got %d", len(bls))
	}
}

func TestRunBlockMany_RecordsOneEventPerEdge(t *testing.T) {
	db := SetupTestDB(t)
	blocked := MustAdd(t, db, "", "Gate")
	a := MustAdd(t, db, "", "A")
	b := MustAdd(t, db, "", "B")

	if err := RunBlockMany(db, blocked, []string{a, b}, TestActor); err != nil {
		t.Fatalf("RunBlockMany: %v", err)
	}

	blockedTask := MustGet(t, db, blocked)
	rows, err := db.Query("SELECT detail FROM events WHERE task_id = ? AND event_type = 'blocked'", blockedTask.ID)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var detail string
		if err := rows.Scan(&detail); err != nil {
			t.Fatalf("scan: %v", err)
		}
		count++
	}
	if count != 2 {
		t.Errorf("expected 2 'blocked' events, got %d", count)
	}
}

func TestRunBlockMany_AtomicOnMissingBlocker(t *testing.T) {
	db := SetupTestDB(t)
	blocked := MustAdd(t, db, "", "Gate")
	a := MustAdd(t, db, "", "A")
	b := MustAdd(t, db, "", "B")

	// "noExs" is the canonical 5-char nonexistent ID used elsewhere.
	err := RunBlockMany(db, blocked, []string{a, "noExs", b}, TestActor)
	if err == nil {
		t.Fatal("expected error for missing blocker")
	}

	bls, err := GetBlockers(db, blocked)
	if err != nil {
		t.Fatalf("GetBlockers: %v", err)
	}
	if len(bls) != 0 {
		t.Errorf("atomic: expected 0 blockers persisted on failure, got %d", len(bls))
	}
}

func TestRunBlockMany_RejectsSelfBlock(t *testing.T) {
	db := SetupTestDB(t)
	a := MustAdd(t, db, "", "A")
	b := MustAdd(t, db, "", "B")

	err := RunBlockMany(db, a, []string{b, a}, TestActor)
	if err == nil {
		t.Fatal("expected error for self-block")
	}
	if !strings.Contains(err.Error(), "cannot block itself") {
		t.Errorf("error should mention self-block, got: %v", err)
	}

	bls, err := GetBlockers(db, a)
	if err != nil {
		t.Fatalf("GetBlockers: %v", err)
	}
	if len(bls) != 0 {
		t.Errorf("atomic: expected 0 blockers persisted, got %d", len(bls))
	}
}

func TestRunBlockMany_RejectsCycleAcrossInput(t *testing.T) {
	db := SetupTestDB(t)
	a := MustAdd(t, db, "", "A")
	b := MustAdd(t, db, "", "B")
	c := MustAdd(t, db, "", "C")

	// First, b is blocked by a.
	if err := RunBlock(db, b, a, TestActor); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Now try to block a by [c, b]. The b edge would close a → b → a.
	err := RunBlockMany(db, a, []string{c, b}, TestActor)
	if err == nil {
		t.Fatal("expected cycle error")
	}

	// a should have no blockers — atomic rollback.
	bls, err := GetBlockers(db, a)
	if err != nil {
		t.Fatalf("GetBlockers: %v", err)
	}
	if len(bls) != 0 {
		t.Errorf("atomic: expected no blockers on a, got %d", len(bls))
	}
}

func TestRunBlockMany_DuplicatesCollapse(t *testing.T) {
	db := SetupTestDB(t)
	blocked := MustAdd(t, db, "", "Gate")
	a := MustAdd(t, db, "", "A")

	if err := RunBlockMany(db, blocked, []string{a, a, a}, TestActor); err != nil {
		t.Fatalf("RunBlockMany with duplicates: %v", err)
	}

	bls, err := GetBlockers(db, blocked)
	if err != nil {
		t.Fatalf("GetBlockers: %v", err)
	}
	if len(bls) != 1 {
		t.Errorf("expected 1 blocker after dup collapse, got %d", len(bls))
	}

	// Only one event recorded for a single distinct edge.
	blockedTask := MustGet(t, db, blocked)
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM events WHERE task_id = ? AND event_type = 'blocked'", blockedTask.ID).Scan(&count); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 'blocked' event for collapsed dups, got %d", count)
	}
}

func TestRunUnblockMany_RemovesAll(t *testing.T) {
	db := SetupTestDB(t)
	blocked := MustAdd(t, db, "", "Gate")
	a := MustAdd(t, db, "", "A")
	b := MustAdd(t, db, "", "B")
	c := MustAdd(t, db, "", "C")

	if err := RunBlockMany(db, blocked, []string{a, b, c}, TestActor); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := RunUnblockMany(db, blocked, []string{a, b, c}, TestActor); err != nil {
		t.Fatalf("RunUnblockMany: %v", err)
	}

	bls, err := GetBlockers(db, blocked)
	if err != nil {
		t.Fatalf("GetBlockers: %v", err)
	}
	if len(bls) != 0 {
		t.Errorf("expected 0 blockers after unblock-many, got %d", len(bls))
	}
}

func TestRunUnblockMany_AtomicOnMissingEdge(t *testing.T) {
	db := SetupTestDB(t)
	blocked := MustAdd(t, db, "", "Gate")
	a := MustAdd(t, db, "", "A")
	b := MustAdd(t, db, "", "B")
	c := MustAdd(t, db, "", "C")

	// Only blocked-by-a exists; b and c were never added.
	if err := RunBlock(db, blocked, a, TestActor); err != nil {
		t.Fatalf("seed: %v", err)
	}
	err := RunUnblockMany(db, blocked, []string{a, b, c}, TestActor)
	if err == nil {
		t.Fatal("expected error when an edge is missing")
	}

	// Atomic — a-edge must still be present.
	bls, err := GetBlockers(db, blocked)
	if err != nil {
		t.Fatalf("GetBlockers: %v", err)
	}
	if len(bls) != 1 {
		t.Errorf("atomic: expected blocked-by-a still present, got %d blockers", len(bls))
	}
}
