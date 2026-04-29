package main

import (
	"strings"
	"testing"
	"time"

	job "github.com/bensyverson/jobs/internal/job"
)

func TestListAll_SinceDuration_LimitsTimeWindow(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)

	// Two old closed tasks (40d ago) and one fresh closed task (1d ago).
	// --since 7d should drop the old ones from the footer.
	now := time.Now().Unix()
	prev := job.CurrentNowFunc
	defer func() { job.CurrentNowFunc = prev }()
	job.CurrentNowFunc = func() time.Time { return time.Unix(now-40*86400, 0) }

	old1 := job.MustAdd(t, db, "", "old one")
	old2 := job.MustAdd(t, db, "", "old two")
	job.MustDone(t, db, old1)
	job.MustDone(t, db, old2)

	job.CurrentNowFunc = func() time.Time { return time.Unix(now-1*86400, 0) }
	fresh := job.MustAdd(t, db, "", "fresh task")
	job.MustDone(t, db, fresh)

	job.CurrentNowFunc = func() time.Time { return time.Unix(now, 0) }
	// Add an open task so the open tree is non-empty.
	job.MustAdd(t, db, "", "still open")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "list", "--all", "--since", "7d")
	if err != nil {
		t.Fatalf("ls --all --since 7d: %v", err)
	}
	if !strings.Contains(stdout, "fresh task") {
		t.Errorf("expected fresh task in tail:\n%s", stdout)
	}
	if strings.Contains(stdout, "old one") || strings.Contains(stdout, "old two") {
		t.Errorf("--since 7d should drop tasks closed 40d ago:\n%s", stdout)
	}
}

func TestListAll_SinceCount_OverridesDefaultCap(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	now := time.Now().Unix()
	prev := job.CurrentNowFunc
	defer func() { job.CurrentNowFunc = prev }()
	for i := range 15 {
		ts := now - int64(15-i)
		job.CurrentNowFunc = func() time.Time { return time.Unix(ts, 0) }
		id := job.MustAdd(t, db, "", "closed task")
		job.MustDone(t, db, id)
	}
	job.CurrentNowFunc = func() time.Time { return time.Unix(now, 0) }
	job.MustAdd(t, db, "", "open one")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "list", "--all", "--since", "12")
	if err != nil {
		t.Fatalf("ls --all --since 12: %v", err)
	}
	if !strings.Contains(stdout, "Recently closed (12 of 15)") {
		t.Errorf("expected 'Recently closed (12 of 15)', got:\n%s", stdout)
	}
}

func TestListAll_NoTruncate_ReturnsAll(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	now := time.Now().Unix()
	prev := job.CurrentNowFunc
	defer func() { job.CurrentNowFunc = prev }()
	for i := range 25 {
		ts := now - int64(25-i)
		job.CurrentNowFunc = func() time.Time { return time.Unix(ts, 0) }
		id := job.MustAdd(t, db, "", "closed task")
		job.MustDone(t, db, id)
	}
	job.CurrentNowFunc = func() time.Time { return time.Unix(now, 0) }
	job.MustAdd(t, db, "", "open one")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "list", "--all", "--no-truncate")
	if err != nil {
		t.Fatalf("ls --all --no-truncate: %v", err)
	}
	if !strings.Contains(stdout, "Recently closed (25 of 25)") {
		t.Errorf("expected 'Recently closed (25 of 25)', got:\n%s", stdout)
	}
}

func TestListAll_SinceAndNoTruncate_AreMutuallyExclusive(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	job.MustAdd(t, db, "", "anything")
	db.Close()

	_, _, err := runCLI(t, dbFile, "--as", "alice", "list", "--all", "--since", "7d", "--no-truncate")
	if err == nil {
		t.Fatal("expected error when both --since and --no-truncate are passed")
	}
	if !strings.Contains(err.Error(), "since") || !strings.Contains(err.Error(), "no-truncate") {
		t.Errorf("error should name both flags, got: %v", err)
	}
}

func TestListAll_SinceComposesWithLabel(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	now := time.Now().Unix()
	prev := job.CurrentNowFunc
	defer func() { job.CurrentNowFunc = prev }()
	job.CurrentNowFunc = func() time.Time { return time.Unix(now-1*86400, 0) }
	withLabel, err := job.RunAdd(db, "", "with label", "", "", []string{"p0"}, job.TestActor)
	if err != nil {
		t.Fatal(err)
	}
	noLabel := job.MustAdd(t, db, "", "no label")
	job.MustDone(t, db, withLabel.ShortID)
	job.MustDone(t, db, noLabel)
	job.CurrentNowFunc = func() time.Time { return time.Unix(now, 0) }
	job.MustAdd(t, db, "", "open one")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "list", "--all", "--label", "p0", "--since", "7d")
	if err != nil {
		t.Fatalf("ls --all --label p0 --since 7d: %v", err)
	}
	if !strings.Contains(stdout, "with label") {
		t.Errorf("expected labeled task in tail:\n%s", stdout)
	}
	if strings.Contains(stdout, "no label") {
		t.Errorf("--label p0 should drop non-labeled tasks from tail:\n%s", stdout)
	}
}
