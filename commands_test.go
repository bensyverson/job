package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setupCLI(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	db, err := createDB(path)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	t.Cleanup(resetFlags)
	return path
}

func resetFlags() {
	dbPath = ""
	asFlag = ""
}

func runCLI(t *testing.T, dbFile string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	resetFlags()
	root := newRootCmd()
	var outBuf, errBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	full := append([]string{"--db", dbFile}, args...)
	root.SetArgs(full)
	err = root.Execute()
	return outBuf.String(), errBuf.String(), err
}

func runCLIWithStdin(t *testing.T, dbFile, stdin string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	resetFlags()
	root := newRootCmd()
	var outBuf, errBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SetIn(bytes.NewBufferString(stdin))
	full := append([]string{"--db", dbFile}, args...)
	root.SetArgs(full)
	err = root.Execute()
	return outBuf.String(), errBuf.String(), err
}

func openTestDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := openDB(path)
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

const wantIdentityRequired = "identity required. Pass --as <name> before the verb."

func TestWriteRequiresAs(t *testing.T) {
	dbFile := setupCLI(t)

	// Seed: create a task using direct API, so we have a target id
	db := openTestDB(t, dbFile)
	idRes, err := runAdd(db, "", "Seed", "", "", "seeder")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	id := idRes.ShortID
	otherRes, err := runAdd(db, "", "Other", "", "", "seeder")
	if err != nil {
		t.Fatalf("seed 2: %v", err)
	}
	other := otherRes.ShortID
	db.Close()

	cases := []struct {
		name string
		args []string
	}{
		{"add", []string{"add", "New"}},
		{"done", []string{"done", id}},
		{"reopen", []string{"reopen", id}},
		{"edit", []string{"edit", id, "--title", "New title"}},
		{"note", []string{"note", id, "-m", "hello"}},
		{"cancel", []string{"cancel", id, "--reason", "x"}},
		{"move", []string{"move", id, "before", other}},
		{"block", []string{"block", id, "by", other}},
		{"unblock", []string{"unblock", id, "from", other}},
		{"claim", []string{"claim", id}},
		{"release", []string{"release", id}},
		{"claim-next", []string{"claim-next"}},
		{"heartbeat", []string{"heartbeat", id}},
		{"label add", []string{"label", "add", id, "foo"}},
		{"label remove", []string{"label", "remove", id, "foo"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := runCLI(t, dbFile, tc.args...)
			if err == nil {
				t.Fatalf("expected error for %s without --as", tc.name)
			}
			if err.Error() != wantIdentityRequired {
				t.Errorf("error:\n  got:  %q\n  want: %q", err.Error(), wantIdentityRequired)
			}
		})
	}
}

func TestReadDoesNotRequireAs(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	res, err := runAdd(db, "", "Seed", "", "", "seeder")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	id := res.ShortID
	db.Close()

	cases := [][]string{
		{"list"},
		{"list", "all"},
		{"info", id},
		{"log", id},
		{"next"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, _, err := runCLI(t, dbFile, args...)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestClaimNextRequiresAsEvenWhenReadPartSucceeds(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	if _, err := runAdd(db, "", "Seed", "", "", "seeder"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	db.Close()

	_, _, err := runCLI(t, dbFile, "claim-next")
	if err == nil {
		t.Fatal("expected claim-next without --as to error")
	}
	if err.Error() != wantIdentityRequired {
		t.Errorf("got %q, want %q", err.Error(), wantIdentityRequired)
	}
}

func TestWriteCreatesUserLazily(t *testing.T) {
	dbFile := setupCLI(t)

	_, _, err := runCLI(t, dbFile, "--as", "newname", "add", "x")
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	db := openTestDB(t, dbFile)
	user, err := getUserByName(db, "newname")
	if err != nil {
		t.Fatalf("getUserByName: %v", err)
	}
	if user == nil {
		t.Fatal("user should have been created lazily")
	}
}

func TestWriteAttributesEvent(t *testing.T) {
	dbFile := setupCLI(t)

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "add", "x")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	id := strings.TrimSpace(stdout)

	db := openTestDB(t, dbFile)
	task, err := getTaskByShortID(db, id)
	if err != nil || task == nil {
		t.Fatalf("get task: %v", err)
	}
	detail, err := getLatestEventDetail(db, task.ID, "created")
	if err != nil {
		t.Fatalf("getLatestEventDetail: %v", err)
	}
	if detail == nil {
		t.Fatal("expected created event")
	}

	var actor string
	if err := db.QueryRow("SELECT actor FROM events WHERE task_id = ? AND event_type = 'created'", task.ID).Scan(&actor); err != nil {
		t.Fatalf("select actor: %v", err)
	}
	if actor != "alice" {
		t.Errorf("actor: got %q, want %q", actor, "alice")
	}
}

func TestClaimConflict(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	if err := runClaim(db, id, "1h", "alice", false); err != nil {
		t.Fatalf("alice claim: %v", err)
	}

	_, _, err := runDone(db, []string{id}, false, "", nil, "bob")
	if err == nil {
		t.Fatal("expected bob done to error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "claimed by alice") {
		t.Errorf("error should name alice: %v", err)
	}
	if !strings.Contains(msg, "Wait for expiry, or ask alice to release") {
		t.Errorf("error should guide user: %v", err)
	}
	if !strings.Contains(msg, "expires in") {
		t.Errorf("error should mention expiry: %v", err)
	}
}

func TestStolenClaim(t *testing.T) {
	origNow := currentNowFunc
	defer func() { currentNowFunc = origNow }()

	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	baseTime := time.Now()
	currentNowFunc = func() time.Time { return baseTime }
	if err := runClaim(db, id, "1h", "alice", false); err != nil {
		t.Fatalf("alice claim: %v", err)
	}

	// Advance past TTL so alice's claim expires; bob claims.
	currentNowFunc = func() time.Time { return baseTime.Add(2 * time.Hour) }
	if err := runClaim(db, id, "1h", "bob", false); err != nil {
		t.Fatalf("bob claim: %v", err)
	}

	// Move time forward a bit so "claimed X ago" reads non-zero.
	currentNowFunc = func() time.Time { return baseTime.Add(2*time.Hour + 5*time.Minute) }

	_, _, err := runDone(db, []string{id}, false, "", nil, "alice")
	if err == nil {
		t.Fatal("expected alice done to error (stolen claim)")
	}
	msg := err.Error()
	if !strings.Contains(msg, "your claim on "+id+" expired") {
		t.Errorf("error should mention expired claim: %v", err)
	}
	if !strings.Contains(msg, "now held by bob") {
		t.Errorf("error should name bob: %v", err)
	}
	if !strings.Contains(msg, "Run 'claim "+id+"' to take it back") {
		t.Errorf("error should suggest reclaim: %v", err)
	}
}

func TestReleaseWrongHolder(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	if err := runClaim(db, id, "1h", "alice", false); err != nil {
		t.Fatalf("alice claim: %v", err)
	}
	err := runRelease(db, id, "bob")
	if err == nil {
		t.Fatal("expected bob release to error")
	}
	msg := err.Error()
	want := "task " + id + " is claimed by alice, not you. 'release' operates only on your own claims."
	if msg != want {
		t.Errorf("error:\n  got:  %q\n  want: %q", msg, want)
	}
}

func TestNoEnvFallback(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	res, err := runAdd(db, "", "Seed", "", "", "seeder")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	id := res.ShortID
	db.Close()

	t.Setenv("JOBS_USER", "bob")
	t.Setenv("JOBS_KEY", "somekey")

	_, _, err = runCLI(t, dbFile, "done", id)
	if err == nil {
		t.Fatal("expected error; env vars should not satisfy identity")
	}
	if err.Error() != wantIdentityRequired {
		t.Errorf("got %q, want %q", err.Error(), wantIdentityRequired)
	}
}

func TestLoginVerbRemoved(t *testing.T) {
	dbFile := setupCLI(t)
	for _, verb := range []string{"login", "logout"} {
		t.Run(verb, func(t *testing.T) {
			_, _, err := runCLI(t, dbFile, verb)
			if err == nil {
				t.Fatalf("expected %q to be unknown", verb)
			}
			if !strings.Contains(err.Error(), "unknown command") {
				t.Errorf("error: got %v, want unknown command", err)
			}
		})
	}
}

func TestImport_RequiresAs(t *testing.T) {
	dbFile := setupCLI(t)
	planPath := filepath.Join(filepath.Dir(dbFile), "plan.md")
	body := "```yaml\ntasks:\n  - title: x\n```\n"
	if err := os.WriteFile(planPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	_, _, err := runCLI(t, dbFile, "import", planPath)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != wantIdentityRequired {
		t.Errorf("error:\n  got:  %q\n  want: %q", err.Error(), wantIdentityRequired)
	}
}

func TestImport_CLI_HappyPath(t *testing.T) {
	dbFile := setupCLI(t)
	planPath := filepath.Join(filepath.Dir(dbFile), "plan.md")
	body := "# Plan\n\n```yaml\n" +
		"tasks:\n" +
		"  - title: Root A\n" +
		"  - title: Root B\n" +
		"```\n"
	if err := os.WriteFile(planPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "import", planPath)
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2:\n%s", len(lines), stdout)
	}
	if !strings.HasSuffix(lines[0], "Root A") {
		t.Errorf("line 0 title: %q", lines[0])
	}
	if !strings.HasSuffix(lines[1], "Root B") {
		t.Errorf("line 1 title: %q", lines[1])
	}

	db := openTestDB(t, dbFile)
	fields := strings.Fields(lines[0])
	ta, _ := getTaskByShortID(db, fields[0])
	if ta == nil || ta.Title != "Root A" {
		t.Errorf("Root A not in db")
	}
}

func TestImport_CLI_DryRunJSON(t *testing.T) {
	dbFile := setupCLI(t)
	planPath := filepath.Join(filepath.Dir(dbFile), "plan.md")
	body := "```yaml\ntasks:\n  - title: One\n  - title: Two\n```\n"
	if err := os.WriteFile(planPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "import", planPath, "--dry-run", "--format=json")
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, stdout)
	}
	if got["dry_run"] != true {
		t.Errorf("dry_run flag: %v", got["dry_run"])
	}
	tasks, ok := got["tasks"].([]any)
	if !ok || len(tasks) != 2 {
		t.Fatalf("tasks: %v", got["tasks"])
	}
	for _, tt := range tasks {
		m := tt.(map[string]any)
		id, _ := m["id"].(string)
		if !strings.HasPrefix(id, "<new-") {
			t.Errorf("dry-run id should be a placeholder, got %q", id)
		}
	}

	// No rows written.
	db := openTestDB(t, dbFile)
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM tasks").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("dry-run wrote %d tasks", n)
	}
}

func TestSchema_CLI_NoIdentityRequired(t *testing.T) {
	dbFile := setupCLI(t)

	stdout, _, err := runCLI(t, dbFile, "schema")
	if err != nil {
		t.Fatalf("schema: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("CLI output not valid JSON: %v\n%s", err, stdout)
	}

	var buf bytes.Buffer
	if err := runSchema(&buf); err != nil {
		t.Fatal(err)
	}
	if stdout != buf.String() {
		t.Errorf("CLI and runSchema differ:\n CLI:  %q\n func: %q", stdout, buf.String())
	}
}

func TestInit_StillUsesCwd_EvenUnderAncestor(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a")
	b := filepath.Join(a, "b")
	if err := os.MkdirAll(b, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Pre-existing ancestor db.
	ancestor := filepath.Join(a, ".jobs.db")
	if _, err := createDB(ancestor); err != nil {
		t.Fatalf("createDB ancestor: %v", err)
	}

	t.Chdir(b)
	t.Setenv("JOBS_DB", "")
	resetFlags()
	t.Cleanup(resetFlags)

	root2 := newRootCmd()
	var outBuf, errBuf bytes.Buffer
	root2.SetOut(&outBuf)
	root2.SetErr(&errBuf)
	root2.SetArgs([]string{"init"})
	if err := root2.Execute(); err != nil {
		t.Fatalf("init: %v (stderr=%s)", err, errBuf.String())
	}

	cwdDB := filepath.Join(b, ".jobs.db")
	if _, err := os.Stat(cwdDB); err != nil {
		t.Errorf("init did not create %s: %v", cwdDB, err)
	}
}

func TestDone_EnrichedAck_MidPhase(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	parent := mustAdd(t, db, "", "Phase 1")
	c1 := mustAdd(t, db, parent, "Child 1")
	c2 := mustAdd(t, db, parent, "Child 2")
	_ = mustAdd(t, db, parent, "Child 3")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "done", c1)
	if err != nil {
		t.Fatalf("done: %v", err)
	}
	wantHead := `Done: ` + c1 + ` "Child 1"`
	if !strings.Contains(stdout, wantHead) {
		t.Errorf("missing headline:\n%s", stdout)
	}
	wantNext := `  Next: ` + c2 + ` "Child 2"`
	if !strings.Contains(stdout, wantNext) {
		t.Errorf("missing Next line:\n%s", stdout)
	}
	wantParent := "  Parent " + parent + ": 1 of 3 complete"
	if !strings.Contains(stdout, wantParent) {
		t.Errorf("missing Parent line:\n%s", stdout)
	}
}

func TestDone_EnrichedAck_SkipBlocked(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	parent := mustAdd(t, db, "", "Phase")
	c1 := mustAdd(t, db, parent, "C1")
	c2 := mustAdd(t, db, parent, "C2")
	c3 := mustAdd(t, db, parent, "C3")
	// Block C2 by some external blocker.
	blocker := mustAdd(t, db, "", "Blocker")
	if err := runBlock(db, c2, blocker, testActor); err != nil {
		t.Fatalf("block: %v", err)
	}
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "done", c1)
	if err != nil {
		t.Fatalf("done: %v", err)
	}
	want := "  Next sibling " + c2 + " is blocked on " + blocker + ". Skipping to " + c3 + "."
	if !strings.Contains(stdout, want) {
		t.Errorf("missing skip-blocked line:\n%s", stdout)
	}
}

func TestDone_EnrichedAck_LastChild_WithParentSibling(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	// Root -> Phase1, Phase2 each with child
	p1 := mustAdd(t, db, "", "Phase 1")
	p2 := mustAdd(t, db, "", "Phase 2")
	c1 := mustAdd(t, db, p1, "C1 only child")
	_ = p2
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "done", c1)
	if err != nil {
		t.Fatalf("done: %v", err)
	}
	wantAuto := `  Auto-closed: ` + p1 + ` "Phase 1"`
	if !strings.Contains(stdout, wantAuto) {
		t.Errorf("missing auto-closed line:\n%s", stdout)
	}
	wantNext := `  Next: ` + p2 + ` "Phase 2"`
	if !strings.Contains(stdout, wantNext) {
		t.Errorf("missing Next line:\n%s", stdout)
	}
}

func TestDone_EnrichedAck_WholeTreeDone(t *testing.T) {
	// Under leaf-frontier semantics, closing the last open child auto-closes
	// every ancestor up to the root. The whole-tree ack fires on that single
	// close, not on a subsequent manual close of the root.
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	root := mustAdd(t, db, "", "Root")
	c1 := mustAdd(t, db, root, "C1")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "done", c1)
	if err != nil {
		t.Fatalf("done: %v", err)
	}
	want := "  All tasks in " + root + " complete. (2 done, 0 open)"
	if !strings.Contains(stdout, want) {
		t.Errorf("missing whole-tree line:\n%s", stdout)
	}
}

func TestDone_AlreadyDone_Unchanged(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := mustAdd(t, db, "", "Task")
	mustDone(t, db, id)
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "done", id)
	if err != nil {
		t.Fatalf("done: %v", err)
	}
	want := "Already done: " + id + "\n"
	if stdout != want {
		t.Errorf("got %q, want %q", stdout, want)
	}
}

func TestDone_Cascade_IncludesContext(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	root := mustAdd(t, db, "", "Root")
	_ = mustAdd(t, db, root, "C1")
	_ = mustAdd(t, db, root, "C2")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "done", root, "--cascade")
	if err != nil {
		t.Fatalf("done: %v", err)
	}
	if !strings.Contains(stdout, "and 2 subtasks") {
		t.Errorf("missing cascade count:\n%s", stdout)
	}
	if !strings.Contains(stdout, "All tasks in "+root+" complete") {
		t.Errorf("missing whole-tree context on cascade:\n%s", stdout)
	}
}

func TestList_EmptyState_HasDoneTasks(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := mustAdd(t, db, "", "X")
	mustDone(t, db, id)
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	want := "Nothing actionable. 1 tasks done. Run 'list all' to see the full tree.\n"
	if stdout != want {
		t.Errorf("got %q, want %q", stdout, want)
	}
}

func TestList_EmptyState_FreshDB(t *testing.T) {
	dbFile := setupCLI(t)

	stdout, _, err := runCLI(t, dbFile, "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(stdout, "No tasks.") {
		t.Errorf("want fresh-db message:\n%s", stdout)
	}
	if !strings.Contains(stdout, "job import plan.md") {
		t.Errorf("want import hint:\n%s", stdout)
	}
}

func TestNext_Empty_WordingUpdated(t *testing.T) {
	dbFile := setupCLI(t)
	_, _, err := runCLI(t, dbFile, "next")
	if err == nil {
		t.Fatal("expected error when no available tasks")
	}
	want := "No available tasks. Run 'list all' to see blocked or claimed work."
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestInit_DefaultOutput_IncludesGitignoreHint(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("JOBS_DB", "")
	resetFlags()
	t.Cleanup(resetFlags)

	root := newRootCmd()
	var outBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&outBuf)
	root.SetArgs([]string{"init"})
	if err := root.Execute(); err != nil {
		t.Fatalf("init: %v", err)
	}
	out := outBuf.String()
	if !strings.Contains(out, "Recommended .gitignore entries") {
		t.Errorf("expected hint:\n%s", out)
	}
	if !strings.Contains(out, ".jobs.db-shm") {
		t.Errorf("expected shm entry:\n%s", out)
	}
}

func TestInit_GitignoreFlag_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("JOBS_DB", "")
	resetFlags()
	t.Cleanup(resetFlags)

	root := newRootCmd()
	var outBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&outBuf)
	root.SetArgs([]string{"init", "--gitignore"})
	if err := root.Execute(); err != nil {
		t.Fatalf("init: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, ".jobs.db-shm") {
		t.Errorf(".gitignore missing -shm entry:\n%s", content)
	}
	if !strings.Contains(content, ".jobs.db-wal") {
		t.Errorf(".gitignore missing -wal entry:\n%s", content)
	}
	if strings.Contains(content, "\n.jobs.db\n") {
		t.Errorf(".gitignore should not include .jobs.db itself:\n%s", content)
	}
	out := outBuf.String()
	if !strings.Contains(out, "Wrote 2 entries to .gitignore") {
		t.Errorf("missing success output:\n%s", out)
	}
}

func TestInit_GitignoreFlag_AppendsExisting(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("JOBS_DB", "")
	resetFlags()
	t.Cleanup(resetFlags)

	existing := "node_modules/\n.env\n"
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(existing), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	root := newRootCmd()
	var outBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&outBuf)
	root.SetArgs([]string{"init", "--gitignore"})
	if err := root.Execute(); err != nil {
		t.Fatalf("init: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	content := string(data)
	if !strings.HasPrefix(content, existing) {
		t.Errorf("original content clobbered:\n%s", content)
	}
	if !strings.Contains(content, ".jobs.db-shm") {
		t.Errorf("missing appended entry:\n%s", content)
	}
	if !strings.Contains(content, "# job\n") {
		t.Errorf("missing section header:\n%s", content)
	}
}

func TestInit_GitignoreFlag_Idempotent(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("JOBS_DB", "")
	resetFlags()
	t.Cleanup(resetFlags)

	// First run
	root1 := newRootCmd()
	root1.SetOut(&bytes.Buffer{})
	root1.SetErr(&bytes.Buffer{})
	root1.SetArgs([]string{"init", "--gitignore"})
	if err := root1.Execute(); err != nil {
		t.Fatalf("init 1: %v", err)
	}
	first, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))

	// Second run: overwrite db, gitignore should be unchanged.
	resetFlags()
	root2 := newRootCmd()
	var outBuf bytes.Buffer
	root2.SetOut(&outBuf)
	root2.SetErr(&outBuf)
	root2.SetArgs([]string{"init", "--force", "--gitignore"})
	if err := root2.Execute(); err != nil {
		t.Fatalf("init 2: %v", err)
	}
	second, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if string(first) != string(second) {
		t.Errorf("gitignore changed on second run:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
	if !strings.Contains(outBuf.String(), "already includes") {
		t.Errorf("expected 'already includes':\n%s", outBuf.String())
	}
}

func TestInit_GitignoreFlag_PartialPresent(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("JOBS_DB", "")
	resetFlags()
	t.Cleanup(resetFlags)

	existing := ".jobs.db-shm\n"
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(existing), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	root := newRootCmd()
	var outBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&outBuf)
	root.SetArgs([]string{"init", "--gitignore"})
	if err := root.Execute(); err != nil {
		t.Fatalf("init: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	content := string(data)
	if !strings.Contains(content, ".jobs.db-wal") {
		t.Errorf("missing -wal append:\n%s", content)
	}
	// Only one -shm entry (not duplicated).
	if strings.Count(content, ".jobs.db-shm") != 1 {
		t.Errorf("duplicated -shm entry:\n%s", content)
	}
	out := outBuf.String()
	if !strings.Contains(out, "Wrote 1 entries") && !strings.Contains(out, ".jobs.db-wal") {
		t.Errorf("missing wrote message:\n%s", out)
	}
}

func TestHelp_MentionsCurrentVerbs(t *testing.T) {
	dbFile := setupCLI(t)
	_ = dbFile
	resetFlags()
	t.Cleanup(resetFlags)

	root := newRootCmd()
	var outBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&outBuf)
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("help: %v", err)
	}
	out := outBuf.String()

	// Verbs that are registered AND should appear in help.
	// `remove` was retired in Phase 5 in favor of `cancel`.
	wantVerbs := []string{
		"init", "schema", "add", "import", "edit", "block", "unblock",
		"move", "claim", "claim-next", "release", "note", "done", "reopen",
		"cancel", "list", "info", "log", "status", "next", "tail",
		"heartbeat", "label",
	}
	for _, v := range wantVerbs {
		if !strings.Contains(out, v) {
			t.Errorf("help missing verb %q", v)
		}
	}
}

func TestHelp_PhaseGatedVerbsAnnotated(t *testing.T) {
	resetFlags()
	t.Cleanup(resetFlags)

	root := newRootCmd()
	var outBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&outBuf)
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("help: %v", err)
	}
	out := outBuf.String()
	for _, phrase := range []string{
		"next all",
		"tail --until-close",
	} {
		if !strings.Contains(out, phrase) {
			t.Errorf("help missing orchestration phrase %q", phrase)
		}
	}
	if strings.Contains(out, "(in next release)") {
		t.Errorf("help should not contain '(in next release)' annotations:\n%s", out)
	}
}

func TestHelp_Snapshot(t *testing.T) {
	// Snapshot test: lock the top-level help output.
	// When verbs are added/removed in future phases, update the golden
	// string deliberately.
	resetFlags()
	t.Cleanup(resetFlags)

	root := newRootCmd()
	var outBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&outBuf)
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("help: %v", err)
	}
	out := outBuf.String()
	for _, mustHave := range []string{
		"QUICKSTART",
		"IDENTITY",
		"VERBS (grouped by role)",
		"OUTPUT",
		"ORCHESTRATION",
		"job import plan.md",
		"claim-next",
		"--format=json",
	} {
		if !strings.Contains(out, mustHave) {
			t.Errorf("help missing anchor phrase %q", mustHave)
		}
	}
}

func TestReadSideEffectsUseEmptyActor(t *testing.T) {
	origNow := currentNowFunc
	defer func() { currentNowFunc = origNow }()

	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	res, err := runAdd(db, "", "Seed", "", "", "seeder")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	id := res.ShortID
	baseTime := time.Now()
	currentNowFunc = func() time.Time { return baseTime }
	if err := runClaim(db, id, "1h", "alice", false); err != nil {
		t.Fatalf("alice claim: %v", err)
	}
	db.Close()

	// Advance past TTL. List (a read) should expire alice's claim with actor="".
	currentNowFunc = func() time.Time { return baseTime.Add(2 * time.Hour) }

	if _, _, err := runCLI(t, dbFile, "list"); err != nil {
		t.Fatalf("list: %v", err)
	}

	db = openTestDB(t, dbFile)
	task, err := getTaskByShortID(db, id)
	if err != nil || task == nil {
		t.Fatalf("get task: %v", err)
	}

	var actor string
	if err := db.QueryRow(
		"SELECT actor FROM events WHERE task_id = ? AND event_type = 'claim_expired' ORDER BY id DESC LIMIT 1",
		task.ID,
	).Scan(&actor); err != nil {
		t.Fatalf("select actor: %v", err)
	}
	if actor != "" {
		t.Errorf("actor: got %q, want empty", actor)
	}
}
