package job

import (
	"testing"
	"time"
)

// withFrozenNow drives CurrentNowFunc forward step-by-step so closed events
// have monotonically increasing timestamps we can predict.
func withFrozenNow(t *testing.T, base int64) (advance func(seconds int64)) {
	t.Helper()
	cur := base
	prev := CurrentNowFunc
	CurrentNowFunc = func() time.Time { return time.Unix(cur, 0) }
	t.Cleanup(func() { CurrentNowFunc = prev })
	return func(seconds int64) { cur += seconds }
}

func TestRunListWithTail_EmptyDB(t *testing.T) {
	db := SetupTestDB(t)
	res, err := RunListWithTail(db, ListFilter{})
	if err != nil {
		t.Fatalf("RunListWithTail: %v", err)
	}
	if len(res.Open) != 0 {
		t.Errorf("expected 0 open nodes, got %d", len(res.Open))
	}
	if len(res.ClosedTail) != 0 {
		t.Errorf("expected 0 closed tail rows, got %d", len(res.ClosedTail))
	}
	if res.ClosedTotal != 0 {
		t.Errorf("expected ClosedTotal=0, got %d", res.ClosedTotal)
	}
}

func TestRunListWithTail_SortsByClosedAtDesc(t *testing.T) {
	db := SetupTestDB(t)
	advance := withFrozenNow(t, 1_700_000_000)
	a := MustAdd(t, db, "", "alpha")
	b := MustAdd(t, db, "", "bravo")
	c := MustAdd(t, db, "", "charlie")

	advance(10)
	MustDone(t, db, a)
	advance(10)
	MustDone(t, db, c)
	advance(10)
	MustDone(t, db, b)

	res, err := RunListWithTail(db, ListFilter{})
	if err != nil {
		t.Fatalf("RunListWithTail: %v", err)
	}
	if len(res.ClosedTail) != 3 {
		t.Fatalf("expected 3 closed rows, got %d", len(res.ClosedTail))
	}
	got := []string{
		res.ClosedTail[0].Task.ShortID,
		res.ClosedTail[1].Task.ShortID,
		res.ClosedTail[2].Task.ShortID,
	}
	want := []string{b, c, a}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("row %d: got %q, want %q", i, got[i], want[i])
		}
	}
	// closed-at descending
	if !(res.ClosedTail[0].ClosedAt > res.ClosedTail[1].ClosedAt &&
		res.ClosedTail[1].ClosedAt > res.ClosedTail[2].ClosedAt) {
		t.Errorf("ClosedAt not descending: %d, %d, %d",
			res.ClosedTail[0].ClosedAt, res.ClosedTail[1].ClosedAt, res.ClosedTail[2].ClosedAt)
	}
}

func TestRunListWithTail_DefaultCapTen(t *testing.T) {
	db := SetupTestDB(t)
	advance := withFrozenNow(t, 1_700_000_000)
	var ids []string
	for range 12 {
		ids = append(ids, MustAdd(t, db, "", "task"))
	}
	for _, id := range ids {
		advance(1)
		MustDone(t, db, id)
	}

	res, err := RunListWithTail(db, ListFilter{})
	if err != nil {
		t.Fatalf("RunListWithTail: %v", err)
	}
	if len(res.ClosedTail) != 10 {
		t.Errorf("default cap should yield 10 rows, got %d", len(res.ClosedTail))
	}
	if res.ClosedTotal != 12 {
		t.Errorf("ClosedTotal: want 12, got %d", res.ClosedTotal)
	}
}

func TestRunListWithTail_NoCap(t *testing.T) {
	db := SetupTestDB(t)
	advance := withFrozenNow(t, 1_700_000_000)
	for range 12 {
		id := MustAdd(t, db, "", "task")
		advance(1)
		MustDone(t, db, id)
	}

	res, err := RunListWithTail(db, ListFilter{ClosedTailCap: -1})
	if err != nil {
		t.Fatalf("RunListWithTail: %v", err)
	}
	if len(res.ClosedTail) != 12 {
		t.Errorf("ClosedTailCap=-1 should yield all 12 rows, got %d", len(res.ClosedTail))
	}
	if res.ClosedTotal != 12 {
		t.Errorf("ClosedTotal: want 12, got %d", res.ClosedTotal)
	}
}

func TestRunListWithTail_RespectsLabel(t *testing.T) {
	db := SetupTestDB(t)
	advance := withFrozenNow(t, 1_700_000_000)
	withLabel, err := RunAdd(db, "", "with label", "", "", []string{"p0"}, TestActor)
	if err != nil {
		t.Fatal(err)
	}
	noLabel := MustAdd(t, db, "", "no label")
	advance(1)
	MustDone(t, db, noLabel)
	advance(1)
	MustDone(t, db, withLabel.ShortID)

	res, err := RunListWithTail(db, ListFilter{Label: "p0"})
	if err != nil {
		t.Fatalf("RunListWithTail: %v", err)
	}
	if len(res.ClosedTail) != 1 || res.ClosedTail[0].Task.ShortID != withLabel.ShortID {
		t.Errorf("expected 1 labeled row %s, got %#v", withLabel.ShortID, res.ClosedTail)
	}
	if res.ClosedTotal != 1 {
		t.Errorf("ClosedTotal: want 1, got %d", res.ClosedTotal)
	}
}

func TestRunListWithTail_RespectsGrep(t *testing.T) {
	db := SetupTestDB(t)
	advance := withFrozenNow(t, 1_700_000_000)
	a := MustAdd(t, db, "", "fix the cache bug")
	b := MustAdd(t, db, "", "polish docs")
	advance(1)
	MustDone(t, db, a)
	advance(1)
	MustDone(t, db, b)

	res, err := RunListWithTail(db, ListFilter{GrepPattern: "cache"})
	if err != nil {
		t.Fatalf("RunListWithTail: %v", err)
	}
	if len(res.ClosedTail) != 1 || res.ClosedTail[0].Task.ShortID != a {
		t.Errorf("grep cache should match only %s, got %#v", a, res.ClosedTail)
	}
}

func TestRunListWithTail_RespectsStatusDone(t *testing.T) {
	db := SetupTestDB(t)
	advance := withFrozenNow(t, 1_700_000_000)
	a := MustAdd(t, db, "", "done one")
	b := MustAdd(t, db, "", "canceled one")
	advance(1)
	MustDone(t, db, a)
	advance(1)
	if _, _, _, err := RunCancel(db, []string{b}, "no longer needed", false, false, false, TestActor); err != nil {
		t.Fatal(err)
	}

	res, err := RunListWithTail(db, ListFilter{Status: "done"})
	if err != nil {
		t.Fatalf("RunListWithTail: %v", err)
	}
	if len(res.ClosedTail) != 1 || res.ClosedTail[0].Task.ShortID != a {
		t.Errorf("status=done should match only %s, got %#v", a, res.ClosedTail)
	}
}

func TestRunListWithTail_RespectsStatusCanceled(t *testing.T) {
	db := SetupTestDB(t)
	advance := withFrozenNow(t, 1_700_000_000)
	a := MustAdd(t, db, "", "done one")
	b := MustAdd(t, db, "", "canceled one")
	advance(1)
	MustDone(t, db, a)
	advance(1)
	if _, _, _, err := RunCancel(db, []string{b}, "no longer needed", false, false, false, TestActor); err != nil {
		t.Fatal(err)
	}

	res, err := RunListWithTail(db, ListFilter{Status: "canceled"})
	if err != nil {
		t.Fatalf("RunListWithTail: %v", err)
	}
	if len(res.ClosedTail) != 1 || res.ClosedTail[0].Task.ShortID != b {
		t.Errorf("status=canceled should match only %s, got %#v", b, res.ClosedTail)
	}
}

func TestRunListWithTail_SubtreeScoped(t *testing.T) {
	db := SetupTestDB(t)
	advance := withFrozenNow(t, 1_700_000_000)
	parent := MustAdd(t, db, "", "parent")
	other := MustAdd(t, db, "", "outside")
	child := MustAdd(t, db, parent, "child")
	advance(1)
	MustDone(t, db, child)
	advance(1)
	MustDone(t, db, other)

	res, err := RunListWithTail(db, ListFilter{ParentID: parent})
	if err != nil {
		t.Fatalf("RunListWithTail: %v", err)
	}
	// Parent itself auto-closes when its only child closes; both should be in
	// scope of `ParentID=parent` for the closed tail (child + auto-closed
	// parent), but `other` must not appear.
	for _, row := range res.ClosedTail {
		if row.Task.ShortID == other {
			t.Errorf("subtree scope leaked: %s appears in tail", other)
		}
	}
	if res.ClosedTotal == 0 {
		t.Errorf("expected at least one closed row in subtree scope")
	}
}

func TestRunListWithTail_ClaimedByFiltersByCloser(t *testing.T) {
	db := SetupTestDB(t)
	advance := withFrozenNow(t, 1_700_000_000)
	a := MustAdd(t, db, "", "alice's task")
	b := MustAdd(t, db, "", "bob's task")
	advance(1)
	if _, _, err := RunDone(db, []string{a}, false, "", nil, "alice", false, ""); err != nil {
		t.Fatal(err)
	}
	advance(1)
	if _, _, err := RunDone(db, []string{b}, false, "", nil, "bob", false, ""); err != nil {
		t.Fatal(err)
	}

	res, err := RunListWithTail(db, ListFilter{ClaimedByActor: "alice"})
	if err != nil {
		t.Fatalf("RunListWithTail: %v", err)
	}
	if len(res.ClosedTail) != 1 || res.ClosedTail[0].Task.ShortID != a {
		t.Errorf("ClaimedByActor=alice should match only %s, got %#v", a, res.ClosedTail)
	}
}

func TestRunListWithTail_OpenSetMatchesPriorBehavior(t *testing.T) {
	db := SetupTestDB(t)
	advance := withFrozenNow(t, 1_700_000_000)
	open1 := MustAdd(t, db, "", "open one")
	open2 := MustAdd(t, db, "", "open two")
	closed := MustAdd(t, db, "", "to close")
	advance(1)
	MustDone(t, db, closed)

	res, err := RunListWithTail(db, ListFilter{})
	if err != nil {
		t.Fatalf("RunListWithTail: %v", err)
	}
	if len(res.Open) != 2 {
		t.Fatalf("expected 2 open nodes, got %d", len(res.Open))
	}
	gotIDs := []string{res.Open[0].Task.ShortID, res.Open[1].Task.ShortID}
	wantSet := map[string]bool{open1: true, open2: true}
	for _, id := range gotIDs {
		if !wantSet[id] {
			t.Errorf("unexpected open id %s", id)
		}
	}
}
