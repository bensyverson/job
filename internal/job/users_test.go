package job

import (
	"testing"
)

func TestCreateUser_Basic(t *testing.T) {
	db := SetupTestDB(t)
	user, err := createUser(db, "TestUser")
	if err != nil {
		t.Fatalf("createUser: %v", err)
	}
	if user.ID < 1 {
		t.Errorf("user ID: got %d, want >= 1", user.ID)
	}
	if user.Name != "TestUser" {
		t.Errorf("name: got %q, want %q", user.Name, "TestUser")
	}
}

func TestCreateUser_DuplicateName(t *testing.T) {
	db := SetupTestDB(t)
	_, err := createUser(db, "TestUser")
	if err != nil {
		t.Fatalf("first createUser: %v", err)
	}
	_, err = createUser(db, "TestUser")
	if err == nil {
		t.Fatal("expected error for duplicate user name")
	}
}

func TestGetUserByName_Exists(t *testing.T) {
	db := SetupTestDB(t)
	created, err := createUser(db, "TestUser")
	if err != nil {
		t.Fatalf("createUser: %v", err)
	}
	user, err := GetUserByName(db, "TestUser")
	if err != nil {
		t.Fatalf("GetUserByName: %v", err)
	}
	if user == nil {
		t.Fatal("expected user, got nil")
	}
	if user.ID != created.ID {
		t.Errorf("ID: got %d, want %d", user.ID, created.ID)
	}
}

func TestGetUserByName_NotFound(t *testing.T) {
	db := SetupTestDB(t)
	user, err := GetUserByName(db, "Nobody")
	if err != nil {
		t.Fatalf("GetUserByName: %v", err)
	}
	if user != nil {
		t.Error("expected nil for non-existent user")
	}
}

func TestInitSchema_UsersTable(t *testing.T) {
	db := SetupTestDB(t)
	var count int
	err := db.QueryRow("SELECT count(*) FROM users").Scan(&count)
	if err != nil {
		t.Fatalf("users table should exist: %v", err)
	}
}

func TestInitSchema_UsersTableHasNoKeyColumn(t *testing.T) {
	db := SetupTestDB(t)
	_, err := db.Exec("SELECT key FROM users LIMIT 1")
	if err == nil {
		t.Fatal("users.key column should not exist")
	}
}

func TestInitSchema_EventsActorColumn(t *testing.T) {
	db := SetupTestDB(t)
	_, err := db.Exec("SELECT actor FROM events LIMIT 1")
	if err != nil {
		t.Fatalf("events.actor column should exist: %v", err)
	}
}

func TestEnsureUser_CreatesNewUser(t *testing.T) {
	db := SetupTestDB(t)
	user, err := EnsureUser(db, "alice")
	if err != nil {
		t.Fatalf("EnsureUser: %v", err)
	}
	if user == nil {
		t.Fatal("expected user, got nil")
	}
	if user.Name != "alice" {
		t.Errorf("name: got %q, want %q", user.Name, "alice")
	}
	got, _ := GetUserByName(db, "alice")
	if got == nil {
		t.Fatal("user should be persisted")
	}
}

func TestEnsureUser_Idempotent(t *testing.T) {
	db := SetupTestDB(t)
	first, err := EnsureUser(db, "alice")
	if err != nil {
		t.Fatalf("first EnsureUser: %v", err)
	}
	second, err := EnsureUser(db, "alice")
	if err != nil {
		t.Fatalf("second EnsureUser: %v", err)
	}
	if first.ID != second.ID {
		t.Errorf("ID changed between calls: first %d, second %d", first.ID, second.ID)
	}

	var count int
	if err := db.QueryRow("SELECT count(*) FROM users WHERE name = ?", "alice").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 row for alice, got %d", count)
	}
}
