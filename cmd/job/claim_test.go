package main

import (
	"strings"
	"testing"

	job "github.com/bensyverson/job/internal/job"
)

func TestClaim_Md_Shape_EchoesTitleAndDefaultTTL(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := job.MustAdd(t, db, "", "Write the thing")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "claim", id)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	want := "Claimed: " + id + " \"Write the thing\" (expires in 30m)\n"
	if stdout != want {
		t.Errorf("got %q, want %q", stdout, want)
	}
}

func TestClaim_Md_Shape_WithExplicitDuration(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := job.MustAdd(t, db, "", "Long task")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "claim", id, "2h")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	want := "Claimed: " + id + " \"Long task\" (expires in 2h)\n"
	if stdout != want {
		t.Errorf("got %q, want %q", stdout, want)
	}
}

func TestClaim_Md_Shape_ForceOverrideEchoesTitle(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := job.MustAdd(t, db, "", "Contended task")
	db.Close()

	if _, _, err := runCLI(t, dbFile, "--as", "bob", "claim", id, "1h"); err != nil {
		t.Fatalf("bob claim: %v", err)
	}
	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "claim", id, "--force")
	if err != nil {
		t.Fatalf("alice claim --force: %v", err)
	}
	want := "Claimed: " + id + " \"Contended task\" (overrode previous claim by bob, expires in 30m)\n"
	if stdout != want {
		t.Errorf("got %q, want %q", stdout, want)
	}
	if !strings.Contains(stdout, `"Contended task"`) {
		t.Errorf("force-override ack must echo title:\n%s", stdout)
	}
}
