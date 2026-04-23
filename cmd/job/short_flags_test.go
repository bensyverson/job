package main

import (
	"strings"
	"testing"

	job "github.com/bensyverson/jobs/internal/job"
)

// 7MFho — Short-flag conventions audit. -m = free-text body across
// commands that take one (note already had it; cancel and done now too).
// Other unambiguous short flags added where the mapping is obvious and
// no overloaded letter (-r, -f, -v, -h) gets reused for a non-matching
// semantic.

func TestCancel_DashM_RoutesToReason(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := job.MustAdd(t, db, "", "Task")
	db.Close()

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "cancel", id, "-m", "lost interest"); err != nil {
		t.Fatalf("cancel -m: %v", err)
	}

	db = openTestDB(t, dbFile)
	defer db.Close()
	task := job.MustGet(t, db, id)
	if task.Status != "canceled" {
		t.Errorf("status: got %q, want canceled", task.Status)
	}
}

func TestCancel_DashY_AllowsPurgeCascade(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	root := job.MustAdd(t, db, "", "Root")
	job.MustAdd(t, db, root, "Child")
	db.Close()

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "cancel", root, "-m", "wipe", "--purge", "--cascade", "-y"); err != nil {
		t.Fatalf("cancel -y --purge --cascade: %v", err)
	}
}

func TestAdd_DashD_RoutesToDesc(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	defer db.Close()

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "add", "Task", "-d", "the description"); err != nil {
		t.Fatalf("add -d: %v", err)
	}
	// fetch the only task and check description
	tasks, err := job.RunListFiltered(db, job.ListFilter{ShowAll: true})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("want 1 task, got %d", len(tasks))
	}
	if tasks[0].Task.Description != "the description" {
		t.Errorf("desc: got %q, want %q", tasks[0].Task.Description, "the description")
	}
}

func TestEdit_DashT_RoutesToTitle(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := job.MustAdd(t, db, "", "Old")
	db.Close()

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "edit", id, "-t", "New"); err != nil {
		t.Fatalf("edit -t: %v", err)
	}
	db = openTestDB(t, dbFile)
	defer db.Close()
	task := job.MustGet(t, db, id)
	if task.Title != "New" {
		t.Errorf("title: got %q, want New", task.Title)
	}
}

func TestList_DashL_RoutesToLabel(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	a := job.MustAdd(t, db, "", "A")
	job.MustAdd(t, db, "", "B")
	if _, err := job.RunLabelAdd(db, a, []string{"p0"}, "alice"); err != nil {
		t.Fatalf("label: %v", err)
	}
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "list", "-l", "p0")
	if err != nil {
		t.Fatalf("list -l: %v", err)
	}
	if !strings.Contains(stdout, a) {
		t.Errorf("expected to see %s:\n%s", a, stdout)
	}
	if strings.Contains(stdout, "`B`") {
		t.Errorf("did not expect to see B:\n%s", stdout)
	}
}

func TestImport_DashN_RoutesToDryRun(t *testing.T) {
	dbFile := setupCLI(t)
	tmpDir := t.TempDir()
	planPath := tmpDir + "/p.md"
	if err := writeFile(planPath, "```yaml\ntasks:\n  - title: T1\n```\n"); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	if _, _, err := runCLI(t, dbFile, "--as", "alice", "import", planPath, "-n"); err != nil {
		t.Fatalf("import -n: %v", err)
	}
	db := openTestDB(t, dbFile)
	defer db.Close()
	tasks, err := job.RunListFiltered(db, job.ListFilter{ShowAll: true})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("dry-run should not write: got %d tasks", len(tasks))
	}
}

func TestRootHelp_ShortFlagConvention(t *testing.T) {
	resetFlags()
	t.Cleanup(resetFlags)
	root := newRootCmd()
	help := root.Long
	if !strings.Contains(help, "-m") {
		t.Errorf("root help should mention -m convention:\n%s", help)
	}
}
