package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	job "github.com/bensyverson/jobs/internal/job"
)

// initCLI drives a fresh `init` through the cobra layer in a temp dir.
// Returns the db path and captured stdout.
func initCLI(t *testing.T, extra ...string) (dbFile, stdout string) {
	t.Helper()
	dir := t.TempDir()
	dbFile = filepath.Join(dir, "test.db")
	resetFlags()
	t.Cleanup(resetFlags)
	root := newRootCmd()
	var outBuf, errBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	args := append([]string{"--db", dbFile, "init"}, extra...)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("init %v: %v\nstderr: %s", args, err, errBuf.String())
	}
	return dbFile, outBuf.String()
}

// --- Default identity (P3) ---

func TestInit_NoFlags_SetsDefaultFromUserEnv(t *testing.T) {
	t.Setenv("USER", "envuser")
	dbFile, stdout := initCLI(t)

	db := openTestDB(t, dbFile)
	got, err := job.GetDefaultIdentity(db)
	if err != nil {
		t.Fatalf("GetDefaultIdentity: %v", err)
	}
	if got != "envuser" {
		t.Errorf("default identity = %q, want %q", got, "envuser")
	}
	if !strings.Contains(stdout, "Default identity: envuser") {
		t.Errorf("init output should announce default identity:\n%s", stdout)
	}
	if !strings.Contains(stdout, "from $USER") {
		t.Errorf("init output should name source ($USER):\n%s", stdout)
	}
}

func TestInit_WithDefaultIdentityFlag_OverridesEnv(t *testing.T) {
	t.Setenv("USER", "envuser")
	dbFile, stdout := initCLI(t, "--default-identity", "claude")

	db := openTestDB(t, dbFile)
	got, _ := job.GetDefaultIdentity(db)
	if got != "claude" {
		t.Errorf("default identity = %q, want claude", got)
	}
	if !strings.Contains(stdout, "Default identity: claude") {
		t.Errorf("output should announce 'Default identity: claude':\n%s", stdout)
	}
	if strings.Contains(stdout, "$USER") {
		t.Errorf("explicit flag should not cite $USER as source:\n%s", stdout)
	}
}

func TestInit_Strict_LeavesDefaultUnset(t *testing.T) {
	t.Setenv("USER", "envuser")
	dbFile, stdout := initCLI(t, "--strict")

	db := openTestDB(t, dbFile)
	got, _ := job.GetDefaultIdentity(db)
	if got != "" {
		t.Errorf("--strict should leave default unset; got %q", got)
	}
	strict, _ := job.IsStrict(db)
	if !strict {
		t.Errorf("--strict should enable strict mode")
	}
	if strings.Contains(stdout, "Default identity:") {
		t.Errorf("--strict should not print a default-identity note:\n%s", stdout)
	}
}

// --- Write resolution (P3) ---

func TestWrite_NoAs_UsesDefaultIdentity(t *testing.T) {
	t.Setenv("USER", "alice")
	dbFile, _ := initCLI(t)

	// No --as on the add call.
	stdout, stderr, err := runCLI(t, dbFile, "add", "hello")
	if err != nil {
		t.Fatalf("add without --as: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "Added") && !strings.Contains(stdout, "Created") {
		// Different ack shapes — at minimum, no error and the task exists.
	}

	// Verify the task was attributed to alice via the created event.
	db := openTestDB(t, dbFile)
	events, err := job.RunLog(db, "", nil)
	if err != nil {
		t.Fatalf("RunLog: %v", err)
	}
	var sawAlice bool
	for _, e := range events {
		if e.Actor == "alice" && e.EventType == "created" {
			sawAlice = true
			break
		}
	}
	if !sawAlice {
		t.Errorf("expected a 'created' event attributed to alice (via default identity)")
	}
}

func TestWrite_NoAs_Strict_Errors(t *testing.T) {
	dbFile, _ := initCLI(t, "--strict")

	_, stderr, err := runCLI(t, dbFile, "add", "hello")
	if err == nil {
		t.Fatalf("expected error: write without --as under strict mode")
	}
	msg := err.Error() + stderr
	if !strings.Contains(msg, "identity required") {
		t.Errorf("error should say 'identity required', got: %s", msg)
	}
}

func TestWrite_AsFlag_OverridesDefaultIdentity(t *testing.T) {
	t.Setenv("USER", "alice")
	dbFile, _ := initCLI(t)

	_, _, err := runCLI(t, dbFile, "--as", "bob", "add", "hi")
	if err != nil {
		t.Fatalf("add --as bob: %v", err)
	}
	db := openTestDB(t, dbFile)
	events, _ := job.RunLog(db, "", nil)
	for _, e := range events {
		if e.EventType == "created" && e.Actor != "bob" {
			t.Errorf("--as bob should override default (alice); got actor=%q", e.Actor)
		}
	}
}

// --- identity verb (P3) ---

func TestIdentity_Set_RequiresAs(t *testing.T) {
	t.Setenv("USER", "alice")
	dbFile, _ := initCLI(t)

	// No --as on identity set — bootstrap discipline: still require it.
	_, stderr, err := runCLI(t, dbFile, "identity", "set", "bob")
	if err == nil {
		t.Fatalf("identity set without --as: expected error")
	}
	if !strings.Contains(err.Error()+stderr, "identity required") {
		t.Errorf("error should say 'identity required', got: %s %s", err.Error(), stderr)
	}
}

func TestIdentity_Set_UpdatesDefault(t *testing.T) {
	t.Setenv("USER", "alice")
	dbFile, _ := initCLI(t)

	_, _, err := runCLI(t, dbFile, "--as", "alice", "identity", "set", "claude")
	if err != nil {
		t.Fatalf("identity set: %v", err)
	}
	db := openTestDB(t, dbFile)
	got, _ := job.GetDefaultIdentity(db)
	if got != "claude" {
		t.Errorf("default identity = %q, want claude", got)
	}
}

func TestIdentity_Strict_On_DisablesDefault(t *testing.T) {
	t.Setenv("USER", "alice")
	dbFile, _ := initCLI(t)

	// Turn strict on.
	_, _, err := runCLI(t, dbFile, "--as", "alice", "identity", "strict", "on")
	if err != nil {
		t.Fatalf("identity strict on: %v", err)
	}
	// Now writes without --as should error.
	_, _, err = runCLI(t, dbFile, "add", "x")
	if err == nil {
		t.Fatalf("expected error: write without --as under strict mode")
	}
	if !strings.Contains(err.Error(), "identity required") {
		t.Errorf("error should say 'identity required', got: %v", err)
	}
}

func TestIdentity_Strict_OffAfterStrictInit_DefaultRemainsUnset(t *testing.T) {
	t.Setenv("USER", "alice")
	dbFile, _ := initCLI(t, "--strict")

	// Turn strict off — but default was never set, so per the P3 rule it
	// stays unset until explicitly set.
	_, _, err := runCLI(t, dbFile, "--as", "alice", "identity", "strict", "off")
	if err != nil {
		t.Fatalf("identity strict off: %v", err)
	}
	db := openTestDB(t, dbFile)
	got, _ := job.GetDefaultIdentity(db)
	if got != "" {
		t.Errorf("default identity after strict off should remain unset; got %q", got)
	}
	// Writes without --as still error (no default).
	_, _, err = runCLI(t, dbFile, "add", "x")
	if err == nil {
		t.Fatalf("expected error: no default identity")
	}
}

// Preserved expectation: with no config at all (pre-P3 behaviour still works
// for databases that predate the migration), writes without --as error with
// the standard message. A fresh DB under strict mode is the equivalent state.
func TestWrite_Strict_EmptyDB_IdentityRequiredMessageUnchanged(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	if err := job.SetStrict(db, true); err != nil {
		t.Fatalf("SetStrict: %v", err)
	}
	db.Close()

	_, _, err := runCLI(t, dbFile, "add", "x")
	if err == nil {
		t.Fatalf("expected error under strict with no default")
	}
	if !strings.Contains(err.Error(), "identity required") {
		t.Errorf("message should match legacy wording; got %v", err)
	}
}

// Defensive: don't accidentally leak the USER env var into resolution at
// write time — P3 spec explicitly rules out env-var fallback. If default is
// empty and strict is off, writes without --as still error.
func TestWrite_NoDefault_PermissiveMode_StillErrors(t *testing.T) {
	// Note: setupCLI uses CreateDB which does NOT set a default identity.
	// Strict mode is off by default. Write without --as should still error
	// because nothing is configured.
	dbFile := setupCLI(t)
	t.Setenv("USER", "envuser") // must NOT be used.

	_, _, err := runCLI(t, dbFile, "add", "x")
	if err == nil {
		t.Fatalf("expected error: no default identity configured, $USER should NOT back-fill")
	}
	if !strings.Contains(err.Error(), "identity required") {
		t.Errorf("got %v", err)
	}

	// Verify nothing was written.
	db := openTestDB(t, dbFile)
	got, _ := job.GetDefaultIdentity(db)
	if got != "" {
		t.Errorf("default identity leaked in from $USER: %q", got)
	}
	_ = os.Getenv // silence unused if refactor drops env above
}
