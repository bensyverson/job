package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestHeartbeat_Single_Happy(t *testing.T) {
	origNow := currentNowFunc
	defer func() { currentNowFunc = origNow }()

	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	base := time.Now()
	currentNowFunc = func() time.Time { return base }
	if err := runClaim(db, id, "1h", "alice", false); err != nil {
		t.Fatalf("claim: %v", err)
	}

	currentNowFunc = func() time.Time { return base.Add(30 * time.Minute) }
	results, err := runHeartbeat(db, []string{id}, "alice")
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if len(results) != 1 || results[0].ShortID != id {
		t.Fatalf("results: %+v", results)
	}

	task := mustGet(t, db, id)
	if task.ClaimExpiresAt == nil {
		t.Fatal("claim_expires_at should be set")
	}
	wantExpiresAt := base.Add(30*time.Minute).Unix() + defaultClaimTTLSeconds
	if *task.ClaimExpiresAt != wantExpiresAt {
		t.Errorf("claim_expires_at: got %d, want %d", *task.ClaimExpiresAt, wantExpiresAt)
	}

	detail, derr := getLatestEventDetail(db, task.ID, "heartbeat")
	if derr != nil || detail == nil {
		t.Fatalf("heartbeat event missing: err=%v detail=%v", derr, detail)
	}
	if got, _ := detail["new_expires_at"].(float64); int64(got) != wantExpiresAt {
		t.Errorf("new_expires_at: got %v, want %d", detail["new_expires_at"], wantExpiresAt)
	}
}

func TestHeartbeat_Variadic(t *testing.T) {
	db := setupTestDB(t)
	a := mustAdd(t, db, "", "A")
	b := mustAdd(t, db, "", "B")
	c := mustAdd(t, db, "", "C")
	for _, id := range []string{a, b, c} {
		if err := runClaim(db, id, "1h", "alice", false); err != nil {
			t.Fatalf("claim %s: %v", id, err)
		}
	}

	results, err := runHeartbeat(db, []string{a, b, c}, "alice")
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
	for _, id := range []string{a, b, c} {
		task := mustGet(t, db, id)
		detail, derr := getLatestEventDetail(db, task.ID, "heartbeat")
		if derr != nil || detail == nil {
			t.Errorf("missing heartbeat for %s: err=%v detail=%v", id, derr, detail)
		}
	}
}

func TestHeartbeat_AllOrNothing(t *testing.T) {
	db := setupTestDB(t)
	a := mustAdd(t, db, "", "A")
	b := mustAdd(t, db, "", "B")
	if err := runClaim(db, a, "1h", "alice", false); err != nil {
		t.Fatalf("claim a: %v", err)
	}
	// b is not claimed - should cause failure.
	_, err := runHeartbeat(db, []string{a, b}, "alice")
	if err == nil {
		t.Fatal("expected error when one target is unclaimed")
	}

	// Verify a has no heartbeat event.
	task := mustGet(t, db, a)
	detail, _ := getLatestEventDetail(db, task.ID, "heartbeat")
	if detail != nil {
		t.Errorf("heartbeat should not have been recorded on a (rolled back): %v", detail)
	}
}

func TestHeartbeat_ClaimedByOther_Errors(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")
	if err := runClaim(db, id, "1h", "alice", false); err != nil {
		t.Fatalf("claim: %v", err)
	}
	_, err := runHeartbeat(db, []string{id}, "bob")
	if err == nil {
		t.Fatal("expected error")
	}
	want := "task " + id + " is claimed by alice, not you. 'heartbeat' refreshes only your own claims."
	if err.Error() != want {
		t.Errorf("error:\n  got:  %q\n  want: %q", err.Error(), want)
	}
}

func TestHeartbeat_ExpiredWasMine_NowUnclaimed_Errors(t *testing.T) {
	origNow := currentNowFunc
	defer func() { currentNowFunc = origNow }()

	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	base := time.Now()
	currentNowFunc = func() time.Time { return base }
	if err := runClaim(db, id, "1h", "alice", false); err != nil {
		t.Fatalf("claim: %v", err)
	}

	// Advance past TTL; claim will be expireStaled by runHeartbeat.
	currentNowFunc = func() time.Time { return base.Add(2 * time.Hour) }
	_, err := runHeartbeat(db, []string{id}, "alice")
	if err == nil {
		t.Fatal("expected error")
	}
	want := "your claim on " + id + " expired; reclaim with 'job claim " + id + "'"
	if err.Error() != want {
		t.Errorf("error:\n  got:  %q\n  want: %q", err.Error(), want)
	}
}

func TestHeartbeat_ExpiredWasMine_NowOther_Errors(t *testing.T) {
	origNow := currentNowFunc
	defer func() { currentNowFunc = origNow }()

	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	base := time.Now()
	currentNowFunc = func() time.Time { return base }
	if err := runClaim(db, id, "1h", "alice", false); err != nil {
		t.Fatalf("alice claim: %v", err)
	}

	currentNowFunc = func() time.Time { return base.Add(2 * time.Hour) }
	if err := runClaim(db, id, "1h", "bob", false); err != nil {
		t.Fatalf("bob claim: %v", err)
	}

	currentNowFunc = func() time.Time { return base.Add(2*time.Hour + 5*time.Minute) }
	_, err := runHeartbeat(db, []string{id}, "alice")
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "your claim on "+id+" expired") {
		t.Errorf("error should mention expired: %v", err)
	}
	if !strings.Contains(msg, "now held by bob") {
		t.Errorf("error should name bob: %v", err)
	}
}

func TestHeartbeat_OnUnclaimed_Errors(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")
	_, err := runHeartbeat(db, []string{id}, "alice")
	if err == nil {
		t.Fatal("expected error")
	}
	want := "task " + id + " is not claimed (status: available); heartbeat refreshes a live claim."
	if err.Error() != want {
		t.Errorf("error:\n  got:  %q\n  want: %q", err.Error(), want)
	}
}

func TestHeartbeat_OnDone_Errors(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")
	mustDone(t, db, id)
	_, err := runHeartbeat(db, []string{id}, "alice")
	if err == nil {
		t.Fatal("expected error")
	}
	want := "task " + id + " is done; heartbeat refreshes only live claims."
	if err.Error() != want {
		t.Errorf("error:\n  got:  %q\n  want: %q", err.Error(), want)
	}
}

func TestHeartbeat_OnCanceled_Errors(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")
	if _, _, _, err := runCancel(db, []string{id}, "nope", false, false, false, "alice"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	_, err := runHeartbeat(db, []string{id}, "alice")
	if err == nil {
		t.Fatal("expected error")
	}
	want := "task " + id + " is canceled; heartbeat refreshes only live claims."
	if err.Error() != want {
		t.Errorf("error:\n  got:  %q\n  want: %q", err.Error(), want)
	}
}

func TestHeartbeat_TtlIsAlways15m(t *testing.T) {
	origNow := currentNowFunc
	defer func() { currentNowFunc = origNow }()

	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	base := time.Now()
	currentNowFunc = func() time.Time { return base }
	// Claim with a long explicit duration.
	if err := runClaim(db, id, "8h", "alice", false); err != nil {
		t.Fatalf("claim: %v", err)
	}

	// Heartbeat 1h later.
	currentNowFunc = func() time.Time { return base.Add(1 * time.Hour) }
	if _, err := runHeartbeat(db, []string{id}, "alice"); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}

	task := mustGet(t, db, id)
	want := base.Add(1*time.Hour).Unix() + 900
	if task.ClaimExpiresAt == nil || *task.ClaimExpiresAt != want {
		got := int64(0)
		if task.ClaimExpiresAt != nil {
			got = *task.ClaimExpiresAt
		}
		t.Errorf("claim_expires_at: got %d, want %d (always +15m)", got, want)
	}
}

func TestHeartbeat_Md_Single_Shape(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := mustAdd(t, db, "", "Task")
	if err := runClaim(db, id, "1h", "alice", false); err != nil {
		t.Fatalf("claim: %v", err)
	}
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "heartbeat", id)
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	want := "Heartbeat: " + id + " (expires in 15m)\n"
	if stdout != want {
		t.Errorf("got %q, want %q", stdout, want)
	}
}

func TestHeartbeat_Md_Multi_Shape(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	a := mustAdd(t, db, "", "A")
	b := mustAdd(t, db, "", "B")
	for _, id := range []string{a, b} {
		if err := runClaim(db, id, "1h", "alice", false); err != nil {
			t.Fatalf("claim %s: %v", id, err)
		}
	}
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "heartbeat", a, b)
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if !strings.HasPrefix(stdout, "Heartbeat 2 tasks:\n") {
		t.Errorf("multi headline:\n%s", stdout)
	}
	for _, id := range []string{a, b} {
		line := "- " + id + " (expires in 15m)"
		if !strings.Contains(stdout, line) {
			t.Errorf("missing %q in:\n%s", line, stdout)
		}
	}
}

func TestHeartbeat_Json_Shape(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := mustAdd(t, db, "", "Task")
	if err := runClaim(db, id, "1h", "alice", false); err != nil {
		t.Fatalf("claim: %v", err)
	}
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "heartbeat", id, "--format=json")
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, stdout)
	}
	arr, ok := got["heartbeat"].([]any)
	if !ok || len(arr) != 1 {
		t.Fatalf("heartbeat array: %v", got)
	}
	entry := arr[0].(map[string]any)
	if entry["id"] != id {
		t.Errorf("id: %v", entry["id"])
	}
	if s, _ := entry["expires_in_seconds"].(float64); int64(s) != 900 {
		t.Errorf("expires_in_seconds: %v, want 900", entry["expires_in_seconds"])
	}
	if _, ok := entry["expires_at"].(float64); !ok {
		t.Errorf("expires_at missing or wrong type: %v", entry["expires_at"])
	}
}

func TestHeartbeat_HiddenFromDefaultTail(t *testing.T) {
	events := []EventEntry{
		{EventType: "heartbeat", Actor: "alice"},
		{EventType: "claimed", Actor: "alice"},
	}
	defaultOut := filterEvents(events, EventFilter{})
	for _, e := range defaultOut {
		if e.EventType == "heartbeat" {
			t.Errorf("heartbeat should be hidden by default")
		}
	}
	optedIn := filterEvents(events, EventFilter{Types: parseFilterList("heartbeat")})
	if len(optedIn) != 1 || optedIn[0].EventType != "heartbeat" {
		t.Errorf("--events heartbeat should opt in: %+v", optedIn)
	}
}
