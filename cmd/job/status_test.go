package main

import (
	"bytes"
	job "github.com/bensyverson/job/internal/job"
	"strings"
	"testing"
	"time"
)

func TestStatus_Counts(t *testing.T) {
	db := job.SetupTestDB(t)
	a := job.MustAdd(t, db, "", "A")
	b := job.MustAdd(t, db, "", "B")
	c := job.MustAdd(t, db, "", "C")
	job.MustDone(t, db, a)
	if err := job.RunClaim(db, b, "1h", "alice", false); err != nil {
		t.Fatalf("claim: %v", err)
	}
	_ = c

	s, err := job.RunStatus(db, "")
	if err != nil {
		t.Fatalf("job.RunStatus: %v", err)
	}
	if s.Done != 1 {
		t.Errorf("Done: got %d, want 1", s.Done)
	}
	if s.Claimed != 1 {
		t.Errorf("Claimed: got %d, want 1", s.Claimed)
	}
	if s.Open != 1 {
		t.Errorf("Open: got %d, want 1", s.Open)
	}
}

func TestStatus_LastActivity(t *testing.T) {
	db := job.SetupTestDB(t)
	job.MustAdd(t, db, "", "A")

	s, err := job.RunStatus(db, "")
	if err != nil {
		t.Fatalf("job.RunStatus: %v", err)
	}
	if s.LastActivity == 0 {
		t.Errorf("LastActivity should be set after add")
	}
}

func TestStatus_ClaimedByYou(t *testing.T) {
	db := job.SetupTestDB(t)
	id := job.MustAdd(t, db, "", "A")
	if err := job.RunClaim(db, id, "1h", "alice", false); err != nil {
		t.Fatalf("claim: %v", err)
	}

	s, err := job.RunStatus(db, "alice")
	if err != nil {
		t.Fatalf("job.RunStatus: %v", err)
	}
	if s.ClaimedByYou != 1 {
		t.Errorf("ClaimedByYou: got %d, want 1", s.ClaimedByYou)
	}

	var buf bytes.Buffer
	job.RenderStatus(&buf, s)
	if !strings.Contains(buf.String(), "1 claimed by you") {
		t.Errorf("render missing 'claimed by you':\n%s", buf.String())
	}
}

func TestStatus_ClaimedByYou_OmittedWithoutAs(t *testing.T) {
	db := job.SetupTestDB(t)
	id := job.MustAdd(t, db, "", "A")
	if err := job.RunClaim(db, id, "1h", "alice", false); err != nil {
		t.Fatalf("claim: %v", err)
	}

	s, err := job.RunStatus(db, "")
	if err != nil {
		t.Fatalf("job.RunStatus: %v", err)
	}
	var buf bytes.Buffer
	job.RenderStatus(&buf, s)
	if strings.Contains(buf.String(), "claimed by you") {
		t.Errorf("should not mention 'claimed by you' when no --as:\n%s", buf.String())
	}
}

func TestStatus_EmptyDB(t *testing.T) {
	db := job.SetupTestDB(t)
	s, err := job.RunStatus(db, "")
	if err != nil {
		t.Fatalf("job.RunStatus: %v", err)
	}
	var buf bytes.Buffer
	job.RenderStatus(&buf, s)
	got := buf.String()
	want := "0 open, 0 done\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStatus_CLI_NoAs(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	job.MustAdd(t, db, "", "A")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "status")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(stdout, "1 open") {
		t.Errorf("want '1 open' in output:\n%s", stdout)
	}
	if strings.Contains(stdout, "claimed by you") {
		t.Errorf("should not include 'claimed by you' without --as:\n%s", stdout)
	}
}

func TestStatus_CLI_WithAs(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := job.MustAdd(t, db, "", "A")
	if err := job.RunClaim(db, id, "1h", "alice", false); err != nil {
		t.Fatalf("claim: %v", err)
	}
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "status")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(stdout, "1 claimed by you") {
		t.Errorf("want '1 claimed by you':\n%s", stdout)
	}
}

func TestStatus_CountsCanceled(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := job.MustAdd(t, db, "", "X")
	db.Close()

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "cancel", id, "--reason", "x"); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	stdout, _, err := runCLI(t, dbFile, "status")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(stdout, "1 canceled") {
		t.Errorf("status missing '1 canceled':\n%s", stdout)
	}
}

func TestStatus_OmitsCanceled_WhenZero(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	job.MustAdd(t, db, "", "X")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "status")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if strings.Contains(stdout, "canceled") {
		t.Errorf("status should not include 'canceled' when zero:\n%s", stdout)
	}
}

func TestList_HidesCanceled_Default(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	keep := job.MustAdd(t, db, "", "Keep")
	cancel := job.MustAdd(t, db, "", "Cancel")
	db.Close()

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "cancel", cancel, "--reason", "x"); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	stdout, _, err := runCLI(t, dbFile, "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(stdout, keep) {
		t.Errorf("expected to see %s:\n%s", keep, stdout)
	}
	if strings.Contains(stdout, cancel) {
		t.Errorf("canceled task %s should not appear in default list:\n%s", cancel, stdout)
	}
}

func TestList_ShowsCanceled_InListAll(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := job.MustAdd(t, db, "", "Bye")
	db.Close()

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "cancel", id, "--reason", "x"); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	stdout, _, err := runCLI(t, dbFile, "list", "all")
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if !strings.Contains(stdout, id) {
		t.Errorf("canceled task should appear in list all:\n%s", stdout)
	}
	if !strings.Contains(stdout, "(canceled)") {
		t.Errorf("expected '(canceled)' marker:\n%s", stdout)
	}
}

func TestStatus_LastActivityPhrase(t *testing.T) {
	origNow := job.CurrentNowFunc
	defer func() { job.CurrentNowFunc = origNow }()
	baseTime := time.Unix(1_700_000_000, 0)
	job.CurrentNowFunc = func() time.Time { return baseTime }

	db := job.SetupTestDB(t)
	job.MustAdd(t, db, "", "A")

	job.CurrentNowFunc = func() time.Time { return baseTime.Add(4 * time.Hour) }
	s, err := job.RunStatus(db, "")
	if err != nil {
		t.Fatalf("job.RunStatus: %v", err)
	}
	var buf bytes.Buffer
	job.RenderStatus(&buf, s)
	if !strings.Contains(buf.String(), "last activity:") {
		t.Errorf("missing last activity phrase:\n%s", buf.String())
	}
}
