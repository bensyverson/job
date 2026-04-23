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

func TestStatus_CallerHoldsOneClaim_ShowsCount(t *testing.T) {
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
	got := buf.String()
	if !strings.Contains(got, "1 claimed") {
		t.Errorf("render missing '1 claimed':\n%s", got)
	}
	// The older "claimed by you" phrasing was replaced in P5 — reject
	// any regression.
	if strings.Contains(got, "claimed by you") {
		t.Errorf("render should not include the old 'claimed by you' phrasing:\n%s", got)
	}
}

func TestStatus_CallerHoldsTwoClaims_ShowsCount(t *testing.T) {
	db := job.SetupTestDB(t)
	a := job.MustAdd(t, db, "", "A")
	b := job.MustAdd(t, db, "", "B")
	c := job.MustAdd(t, db, "", "C")
	job.MustDone(t, db, a)
	if err := job.RunClaim(db, b, "1h", "alice", false); err != nil {
		t.Fatalf("claim b: %v", err)
	}
	if err := job.RunClaim(db, c, "1h", "alice", false); err != nil {
		t.Fatalf("claim c: %v", err)
	}

	s, err := job.RunStatus(db, "alice")
	if err != nil {
		t.Fatalf("job.RunStatus: %v", err)
	}
	var buf bytes.Buffer
	job.RenderStatus(&buf, s)
	got := buf.String()
	if !strings.Contains(got, "2 claimed, 0 open, 1 done") {
		t.Errorf("status missing '2 claimed, 0 open, 1 done':\n%s", got)
	}
}

func TestStatus_CallerHoldsZero_NoClaimedTerm(t *testing.T) {
	db := job.SetupTestDB(t)
	job.MustAdd(t, db, "", "A")

	s, err := job.RunStatus(db, "alice")
	if err != nil {
		t.Fatalf("job.RunStatus: %v", err)
	}
	var buf bytes.Buffer
	job.RenderStatus(&buf, s)
	got := buf.String()
	if strings.Contains(got, "claimed") {
		t.Errorf("expected no 'claimed' term when caller holds zero:\n%s", got)
	}
}

func TestStatus_NoCaller_ShowsGlobalClaimed(t *testing.T) {
	db := job.SetupTestDB(t)
	a := job.MustAdd(t, db, "", "A")
	b := job.MustAdd(t, db, "", "B")
	job.MustAdd(t, db, "", "C")
	if err := job.RunClaim(db, a, "1h", "alice", false); err != nil {
		t.Fatalf("claim a: %v", err)
	}
	if err := job.RunClaim(db, b, "1h", "bob", false); err != nil {
		t.Fatalf("claim b: %v", err)
	}

	s, err := job.RunStatus(db, "")
	if err != nil {
		t.Fatalf("job.RunStatus: %v", err)
	}
	var buf bytes.Buffer
	job.RenderStatus(&buf, s)
	got := buf.String()
	if !strings.Contains(got, "2 claimed") {
		t.Errorf("expected '2 claimed' from global count:\n%s", got)
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
	if !strings.Contains(got, "0 open, 0 done") {
		t.Errorf("missing counts on empty DB:\n%s", got)
	}
	if !strings.Contains(got, "Identity: none set") {
		t.Errorf("missing identity line on empty DB:\n%s", got)
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
	if !strings.Contains(stdout, "1 claimed") {
		t.Errorf("want '1 claimed':\n%s", stdout)
	}
	if strings.Contains(stdout, "claimed by you") {
		t.Errorf("output should not include old 'claimed by you' phrasing:\n%s", stdout)
	}
}

func TestStatus_ExpiredClaims_NotCounted(t *testing.T) {
	origNow := job.CurrentNowFunc
	defer func() { job.CurrentNowFunc = origNow }()
	base := time.Unix(1_700_000_000, 0)
	job.CurrentNowFunc = func() time.Time { return base }

	db := job.SetupTestDB(t)
	a := job.MustAdd(t, db, "", "A")
	b := job.MustAdd(t, db, "", "B")
	if err := job.RunClaim(db, a, "1h", "alice", false); err != nil {
		t.Fatalf("claim a: %v", err)
	}
	if err := job.RunClaim(db, b, "1h", "alice", false); err != nil {
		t.Fatalf("claim b: %v", err)
	}

	// Jump past both expirations.
	job.CurrentNowFunc = func() time.Time { return base.Add(2 * time.Hour) }

	s, err := job.RunStatus(db, "alice")
	if err != nil {
		t.Fatalf("job.RunStatus: %v", err)
	}
	if s.ClaimedByYou != 0 {
		t.Errorf("ClaimedByYou after expiry: got %d, want 0", s.ClaimedByYou)
	}
	if s.Claimed != 0 {
		t.Errorf("Claimed after expiry: got %d, want 0", s.Claimed)
	}
	var buf bytes.Buffer
	job.RenderStatus(&buf, s)
	if strings.Contains(buf.String(), "claimed") {
		t.Errorf("expired claims must not appear in status:\n%s", buf.String())
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

func TestStatus_Identity_DefaultSet(t *testing.T) {
	db := job.SetupTestDB(t)
	if err := job.SetDefaultIdentity(db, "claude"); err != nil {
		t.Fatalf("SetDefaultIdentity: %v", err)
	}

	s, err := job.RunStatus(db, "")
	if err != nil {
		t.Fatalf("job.RunStatus: %v", err)
	}
	if s.IdentityDefault != "claude" {
		t.Errorf("IdentityDefault: got %q, want %q", s.IdentityDefault, "claude")
	}
	if s.Strict {
		t.Errorf("Strict: got true, want false")
	}

	var buf bytes.Buffer
	job.RenderStatus(&buf, s)
	got := buf.String()
	if !strings.Contains(got, "Identity: claude (default) · strict mode off") {
		t.Errorf("render missing default-set line:\n%s", got)
	}
}

func TestStatus_Identity_NoDefault(t *testing.T) {
	db := job.SetupTestDB(t)

	s, err := job.RunStatus(db, "")
	if err != nil {
		t.Fatalf("job.RunStatus: %v", err)
	}
	if s.IdentityDefault != "" {
		t.Errorf("IdentityDefault: got %q, want empty", s.IdentityDefault)
	}

	var buf bytes.Buffer
	job.RenderStatus(&buf, s)
	got := buf.String()
	if !strings.Contains(got, "Identity: none set · --as required on writes") {
		t.Errorf("render missing no-default line:\n%s", got)
	}
}

func TestStatus_Identity_DefaultSet_StrictOn(t *testing.T) {
	db := job.SetupTestDB(t)
	if err := job.SetDefaultIdentity(db, "claude"); err != nil {
		t.Fatalf("SetDefaultIdentity: %v", err)
	}
	if err := job.SetStrict(db, true); err != nil {
		t.Fatalf("SetStrict: %v", err)
	}

	s, err := job.RunStatus(db, "")
	if err != nil {
		t.Fatalf("job.RunStatus: %v", err)
	}
	if !s.Strict {
		t.Errorf("Strict: got false, want true")
	}

	var buf bytes.Buffer
	job.RenderStatus(&buf, s)
	got := buf.String()
	if !strings.Contains(got, "Identity: claude (default) · strict mode on") {
		t.Errorf("render missing strict-on line:\n%s", got)
	}
}

func TestStatus_Identity_NoDefault_StrictOn(t *testing.T) {
	db := job.SetupTestDB(t)
	if err := job.SetStrict(db, true); err != nil {
		t.Fatalf("SetStrict: %v", err)
	}

	s, err := job.RunStatus(db, "")
	if err != nil {
		t.Fatalf("job.RunStatus: %v", err)
	}

	var buf bytes.Buffer
	job.RenderStatus(&buf, s)
	got := buf.String()
	if !strings.Contains(got, "Identity: none set · --as required on writes") {
		t.Errorf("render missing no-default line:\n%s", got)
	}
}

func TestStatus_Identity_RenderedOnSecondLine(t *testing.T) {
	db := job.SetupTestDB(t)
	if err := job.SetDefaultIdentity(db, "claude"); err != nil {
		t.Fatalf("SetDefaultIdentity: %v", err)
	}
	job.MustAdd(t, db, "", "A")

	s, err := job.RunStatus(db, "")
	if err != nil {
		t.Fatalf("job.RunStatus: %v", err)
	}
	var buf bytes.Buffer
	job.RenderStatus(&buf, s)
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d:\n%s", len(lines), buf.String())
	}
	if !strings.Contains(lines[0], "1 open") {
		t.Errorf("line 1 should be the counts summary:\n%s", lines[0])
	}
	if !strings.HasPrefix(lines[1], "Identity: ") {
		t.Errorf("line 2 should start with 'Identity: ':\n%s", lines[1])
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
