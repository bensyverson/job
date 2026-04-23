package main

import (
	"bytes"
	job "github.com/bensyverson/jobs/internal/job"
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

// u7 — A claim that is past its TTL but hasn't yet been auto-expired
// must surface as a Stale line in the status output, giving a recovery
// signal for a crashed agent's abandoned work.
func TestStatus_CLI_SurfacesStaleClaims_ForestScope(t *testing.T) {
	origNow := job.CurrentNowFunc
	defer func() { job.CurrentNowFunc = origNow }()
	base := time.Unix(1_700_000_000, 0)
	job.CurrentNowFunc = func() time.Time { return base }

	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := job.MustAdd(t, db, "", "CrashedAgentWork")
	if err := job.RunClaim(db, id, "30m", "alice", false); err != nil {
		t.Fatalf("claim: %v", err)
	}
	db.Close()

	// Two hours later — well past the 30m claim.
	job.CurrentNowFunc = func() time.Time { return base.Add(2 * time.Hour) }

	stdout, _, err := runCLI(t, dbFile, "status")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	wantPrefix := "Stale: " + id + " \"CrashedAgentWork\" — claimed by alice, expired"
	if !strings.Contains(stdout, wantPrefix) {
		t.Errorf("stale line missing. want prefix %q\ngot:\n%s", wantPrefix, stdout)
	}
	if !strings.Contains(stdout, " ago") {
		t.Errorf("stale line should end with ' ago':\n%s", stdout)
	}
}

// u7 — Subtree scope only surfaces stale claims under the argument
// task. Unrelated stale claims elsewhere stay out.
func TestStatus_CLI_SurfacesStaleClaims_SubtreeScope(t *testing.T) {
	origNow := job.CurrentNowFunc
	defer func() { job.CurrentNowFunc = origNow }()
	base := time.Unix(1_700_000_000, 0)
	job.CurrentNowFunc = func() time.Time { return base }

	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	inside := job.MustAdd(t, db, "", "InsideRoot")
	insideLeaf := job.MustAdd(t, db, inside, "InsideLeaf")
	_ = job.MustAdd(t, db, "", "OutsideRoot") // otherwise unused
	outsideLeaf := job.MustAdd(t, db, "", "OutsideLeaf")
	if err := job.RunClaim(db, insideLeaf, "30m", "alice", false); err != nil {
		t.Fatalf("claim inside: %v", err)
	}
	if err := job.RunClaim(db, outsideLeaf, "30m", "bob", false); err != nil {
		t.Fatalf("claim outside: %v", err)
	}
	db.Close()

	job.CurrentNowFunc = func() time.Time { return base.Add(2 * time.Hour) }

	stdout, _, err := runCLI(t, dbFile, "status", inside)
	if err != nil {
		t.Fatalf("status <id>: %v", err)
	}
	if !strings.Contains(stdout, "Stale: "+insideLeaf) {
		t.Errorf("subtree stale claim must surface:\n%s", stdout)
	}
	if strings.Contains(stdout, "Stale: "+outsideLeaf) {
		t.Errorf("out-of-subtree stale claim must NOT surface:\n%s", stdout)
	}
}

// u5 — `job status <id>` scopes the shared renderer to the subtree,
// behaviour-identical to `summary <id>` (which becomes an alias under
// u8). No session preamble at the node level.
func TestStatus_CLI_WithID_RendersSubtreeRollup(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	root := job.MustAdd(t, db, "", "ProjectX")
	phase := job.MustAdd(t, db, root, "PhaseA")
	job.MustAdd(t, db, phase, "leaf1")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "status", root)
	if err != nil {
		t.Fatalf("status <id>: %v", err)
	}
	if !strings.Contains(stdout, "ProjectX") {
		t.Errorf("subtree rollup missing title:\n%s", stdout)
	}
	// No session preamble on the node-level form.
	if strings.Contains(stdout, "Identity:") {
		t.Errorf("status <id> must not include the Identity preamble:\n%s", stdout)
	}
	if strings.Contains(stdout, "last activity:") {
		t.Errorf("status <id> must not include the last-activity preamble:\n%s", stdout)
	}
}

// u5 — Parity: `status <id>` and `summary <id>` produce the same stdout.
func TestStatus_CLI_WithID_MatchesSummary(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	root := job.MustAdd(t, db, "", "Root")
	phase := job.MustAdd(t, db, root, "Phase")
	job.MustAdd(t, db, phase, "leaf")
	db.Close()

	statusOut, _, err := runCLI(t, dbFile, "status", root)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	summaryOut, _, err := runCLI(t, dbFile, "summary", root)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if statusOut != summaryOut {
		t.Errorf("status <id> and summary <id> must match:\n--status--\n%s\n--summary--\n%s",
			statusOut, summaryOut)
	}
}

// u4 — `job status` (no id) now renders the forest-level rollup under
// the session preamble: one row per top-level task, each with its own
// subtree counts. Lets an agent see the landscape at session start
// instead of just an opaque "N open" integer.
func TestStatus_CLI_RendersForestRollup(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	r1 := job.MustAdd(t, db, "", "Alpha")
	job.MustAdd(t, db, r1, "alpha-leaf") // makes Alpha a non-leaf so its row shows rollup
	r2 := job.MustAdd(t, db, "", "Beta")
	job.MustAdd(t, db, r2, "beta-leaf")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "status")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	// Preamble still there.
	if !strings.Contains(stdout, "open") {
		t.Errorf("preamble counts missing:\n%s", stdout)
	}
	// Both roots surfaced via the shared renderer, each with their own
	// rollup tail ("0 of 1 done · next ...").
	if !strings.Contains(stdout, "Alpha") {
		t.Errorf("Alpha root missing from forest block:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Beta") {
		t.Errorf("Beta root missing from forest block:\n%s", stdout)
	}
}
