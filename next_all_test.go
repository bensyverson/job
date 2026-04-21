package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNextAll_ReturnsAllAvailable(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	a := mustAdd(t, db, "", "A")
	b := mustAdd(t, db, "", "B")
	c := mustAdd(t, db, "", "C")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "next", "all")
	if err != nil {
		t.Fatalf("next all: %v", err)
	}
	for _, id := range []string{a, b, c} {
		if !strings.Contains(stdout, "- "+id+" ") {
			t.Errorf("missing %s:\n%s", id, stdout)
		}
	}
}

func TestNextAll_EmptyIsNotError(t *testing.T) {
	dbFile := setupCLI(t)
	stdout, _, err := runCLI(t, dbFile, "next", "all")
	if err != nil {
		t.Fatalf("next all on empty db should not error: %v", err)
	}
	if !strings.Contains(stdout, "No available tasks.") {
		t.Errorf("expected friendly empty message:\n%s", stdout)
	}
}

func TestNextAll_EmptyJSON(t *testing.T) {
	dbFile := setupCLI(t)
	stdout, _, err := runCLI(t, dbFile, "next", "all", "--format=json")
	if err != nil {
		t.Fatalf("next all: %v", err)
	}
	var got []any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, stdout)
	}
	if len(got) != 0 {
		t.Errorf("expected empty array, got %v", got)
	}
}

func TestNextAll_ScopesToParent_Either_Order(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	parent := mustAdd(t, db, "", "Parent")
	c1 := mustAdd(t, db, parent, "C1")
	c2 := mustAdd(t, db, parent, "C2")
	other := mustAdd(t, db, "", "Other")
	db.Close()

	for _, args := range [][]string{
		{"next", parent, "all"},
		{"next", "all", parent},
	} {
		stdout, _, err := runCLI(t, dbFile, args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		for _, id := range []string{c1, c2} {
			if !strings.Contains(stdout, "- "+id+" ") {
				t.Errorf("%v: missing %s:\n%s", args, id, stdout)
			}
		}
		if strings.Contains(stdout, "- "+other+" ") {
			t.Errorf("%v: should not include sibling-of-parent %s:\n%s", args, other, stdout)
		}
	}
}

func TestNextAll_ExcludesBlocked(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	a := mustAdd(t, db, "", "A")
	b := mustAdd(t, db, "", "B")
	if err := runBlock(db, b, a, testActor); err != nil {
		t.Fatalf("block: %v", err)
	}
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "next", "all")
	if err != nil {
		t.Fatalf("next all: %v", err)
	}
	if strings.Contains(stdout, b) {
		t.Errorf("blocked task %s should not appear:\n%s", b, stdout)
	}
	if !strings.Contains(stdout, a) {
		t.Errorf("blocker %s should appear:\n%s", a, stdout)
	}
}

func TestNextAll_ExcludesClaimed(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	a := mustAdd(t, db, "", "A")
	b := mustAdd(t, db, "", "B")
	if err := runClaim(db, a, "1h", "alice", false); err != nil {
		t.Fatalf("claim: %v", err)
	}
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "next", "all")
	if err != nil {
		t.Fatalf("next all: %v", err)
	}
	if strings.Contains(stdout, "- "+a+" ") {
		t.Errorf("claimed task should not appear:\n%s", stdout)
	}
	if !strings.Contains(stdout, "- "+b+" ") {
		t.Errorf("available task should appear:\n%s", stdout)
	}
}

func TestNextAll_Md_Shape(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	a := mustAdd(t, db, "", "Alpha")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "next", "all")
	if err != nil {
		t.Fatalf("next all: %v", err)
	}
	want := "- " + a + " \"Alpha\"\n"
	if stdout != want {
		t.Errorf("got %q, want %q", stdout, want)
	}
}

func TestNextAll_Json_Shape(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	a := mustAdd(t, db, "", "Alpha")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "next", "all", "--format=json")
	if err != nil {
		t.Fatalf("next all: %v", err)
	}
	var got []map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, stdout)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 task, got %d", len(got))
	}
	if got[0]["id"] != a {
		t.Errorf("id: got %v, want %s", got[0]["id"], a)
	}
	if got[0]["title"] != "Alpha" {
		t.Errorf("title: got %v", got[0]["title"])
	}
}
