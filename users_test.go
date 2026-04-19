package main

import (
	"os"
	"strings"
	"testing"
)

func TestGenerateRandomKey_Length(t *testing.T) {
	key, err := generateRandomKey()
	if err != nil {
		t.Fatalf("generateRandomKey: %v", err)
	}
	if len(key) != 8 {
		t.Errorf("key length: got %d, want 8", len(key))
	}
}

func TestGenerateRandomKey_Charset(t *testing.T) {
	key, err := generateRandomKey()
	if err != nil {
		t.Fatalf("generateRandomKey: %v", err)
	}
	for _, c := range key {
		if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyz0123456789", c) {
			t.Errorf("key contains invalid char: %c", c)
		}
	}
}

func TestGenerateRandomKey_Uniqueness(t *testing.T) {
	keys := make(map[string]bool)
	for range 100 {
		key, err := generateRandomKey()
		if err != nil {
			t.Fatalf("generateRandomKey: %v", err)
		}
		keys[key] = true
	}
	if len(keys) < 95 {
		t.Errorf("expected ~100 unique keys, got %d", len(keys))
	}
}

func TestGenerateRandomName_Format(t *testing.T) {
	name, err := generateRandomName()
	if err != nil {
		t.Fatalf("generateRandomName: %v", err)
	}
	if len(name) < 4 {
		t.Errorf("name too short: %q", name)
	}
	if name[0] < 'A' || name[0] > 'Z' {
		t.Errorf("name should start with uppercase: %q", name)
	}
}

func TestGenerateRandomName_IsAdjectiveAnimal(t *testing.T) {
	name, err := generateRandomName()
	if err != nil {
		t.Fatalf("generateRandomName: %v", err)
	}
	found := false
	for _, adj := range adjectives {
		for _, animal := range animals {
			if name == adj+animal {
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		t.Errorf("name %q is not a valid adjective+animal combination", name)
	}
}

func TestGenerateRandomName_Uniqueness(t *testing.T) {
	names := make(map[string]bool)
	for range 200 {
		name, err := generateRandomName()
		if err != nil {
			t.Fatalf("generateRandomName: %v", err)
		}
		names[name] = true
	}
	if len(names) < 100 {
		t.Errorf("expected diverse names, got %d unique out of 200", len(names))
	}
}

func TestCreateUser_Basic(t *testing.T) {
	db := setupTestDB(t)
	user, err := createUser(db, "TestUser", "abc12345")
	if err != nil {
		t.Fatalf("createUser: %v", err)
	}
	if user.ID < 1 {
		t.Errorf("user ID: got %d, want >= 1", user.ID)
	}
	if user.Name != "TestUser" {
		t.Errorf("name: got %q, want %q", user.Name, "TestUser")
	}
	if user.Key != "abc12345" {
		t.Errorf("key: got %q, want %q", user.Key, "abc12345")
	}
}

func TestCreateUser_DuplicateName(t *testing.T) {
	db := setupTestDB(t)
	_, err := createUser(db, "TestUser", "key1")
	if err != nil {
		t.Fatalf("first createUser: %v", err)
	}
	_, err = createUser(db, "TestUser", "key2")
	if err == nil {
		t.Fatal("expected error for duplicate user name")
	}
}

func TestGetUserByName_Exists(t *testing.T) {
	db := setupTestDB(t)
	created, err := createUser(db, "TestUser", "abc12345")
	if err != nil {
		t.Fatalf("createUser: %v", err)
	}
	user, err := getUserByName(db, "TestUser")
	if err != nil {
		t.Fatalf("getUserByName: %v", err)
	}
	if user == nil {
		t.Fatal("expected user, got nil")
	}
	if user.ID != created.ID {
		t.Errorf("ID: got %d, want %d", user.ID, created.ID)
	}
	if user.Key != "abc12345" {
		t.Errorf("key: got %q, want %q", user.Key, "abc12345")
	}
}

func TestGetUserByName_NotFound(t *testing.T) {
	db := setupTestDB(t)
	user, err := getUserByName(db, "Nobody")
	if err != nil {
		t.Fatalf("getUserByName: %v", err)
	}
	if user != nil {
		t.Error("expected nil for non-existent user")
	}
}

func TestValidateUser_Valid(t *testing.T) {
	db := setupTestDB(t)
	_, err := createUser(db, "TestUser", "abc12345")
	if err != nil {
		t.Fatalf("createUser: %v", err)
	}
	user, err := validateUser(db, "TestUser", "abc12345")
	if err != nil {
		t.Fatalf("validateUser: %v", err)
	}
	if user.Name != "TestUser" {
		t.Errorf("name: got %q, want %q", user.Name, "TestUser")
	}
}

func TestValidateUser_WrongKey(t *testing.T) {
	db := setupTestDB(t)
	_, err := createUser(db, "TestUser", "abc12345")
	if err != nil {
		t.Fatalf("createUser: %v", err)
	}
	_, err = validateUser(db, "TestUser", "wrongkey")
	if err == nil {
		t.Fatal("expected error for wrong key")
	}
}

func TestValidateUser_NotFound(t *testing.T) {
	db := setupTestDB(t)
	_, err := validateUser(db, "Nobody", "abc12345")
	if err == nil {
		t.Fatal("expected error for non-existent user")
	}
}

func TestGenerateUniqueName(t *testing.T) {
	db := setupTestDB(t)
	name, err := generateUniqueName(db)
	if err != nil {
		t.Fatalf("generateUniqueName: %v", err)
	}
	if len(name) < 4 {
		t.Errorf("name too short: %q", name)
	}
}

func TestGenerateUniqueName_NoCollisions(t *testing.T) {
	db := setupTestDB(t)
	names := make(map[string]bool)
	for range 50 {
		name, err := generateUniqueName(db)
		if err != nil {
			t.Fatalf("generateUniqueName: %v", err)
		}
		if names[name] {
			t.Fatalf("duplicate name generated: %q", name)
		}
		names[name] = true
		_, _ = createUser(db, name, "key")
	}
}

func TestGenerateUniqueName_RetriesOnCollision(t *testing.T) {
	db := setupTestDB(t)
	for _, adj := range adjectives[:5] {
		for _, animal := range animals[:5] {
			_, _ = createUser(db, adj+animal, "key")
		}
	}
	name, err := generateUniqueName(db)
	if err != nil {
		t.Fatalf("generateUniqueName: %v", err)
	}
	for _, adj := range adjectives[:5] {
		for _, animal := range animals[:5] {
			if name == adj+animal {
				t.Fatalf("should not return a colliding name, got %q", name)
			}
		}
	}
}

func TestInitSchema_UsersTable(t *testing.T) {
	db := setupTestDB(t)
	var count int
	err := db.QueryRow("SELECT count(*) FROM users").Scan(&count)
	if err != nil {
		t.Fatalf("users table should exist: %v", err)
	}
}

func TestInitSchema_EventsActorColumn(t *testing.T) {
	db := setupTestDB(t)
	_, err := db.Exec("SELECT actor FROM events LIMIT 1")
	if err != nil {
		t.Fatalf("events.actor column should exist: %v", err)
	}
}

// --- Session management ---

func TestGetSession_Present(t *testing.T) {
	os.Setenv("JOBS_USER", "TestUser")
	os.Setenv("JOBS_KEY", "abc12345")
	defer os.Unsetenv("JOBS_USER")
	defer os.Unsetenv("JOBS_KEY")

	name, key := getSession()
	if name != "TestUser" {
		t.Errorf("name: got %q, want %q", name, "TestUser")
	}
	if key != "abc12345" {
		t.Errorf("key: got %q, want %q", key, "abc12345")
	}
}

func TestGetSession_Missing(t *testing.T) {
	os.Unsetenv("JOBS_USER")
	os.Unsetenv("JOBS_KEY")

	name, key := getSession()
	if name != "" {
		t.Errorf("name: got %q, want empty", name)
	}
	if key != "" {
		t.Errorf("key: got %q, want empty", key)
	}
}

func TestRequireAuth_Valid(t *testing.T) {
	db := setupTestDB(t)
	_, err := createUser(db, "TestUser", "abc12345")
	if err != nil {
		t.Fatalf("createUser: %v", err)
	}

	os.Setenv("JOBS_USER", "TestUser")
	os.Setenv("JOBS_KEY", "abc12345")
	defer os.Unsetenv("JOBS_USER")
	defer os.Unsetenv("JOBS_KEY")

	user, err := requireAuth(db)
	if err != nil {
		t.Fatalf("requireAuth: %v", err)
	}
	if user.Name != "TestUser" {
		t.Errorf("name: got %q, want %q", user.Name, "TestUser")
	}
}

func TestRequireAuth_NotLoggedIn(t *testing.T) {
	db := setupTestDB(t)
	os.Unsetenv("JOBS_USER")
	os.Unsetenv("JOBS_KEY")

	_, err := requireAuth(db)
	if err == nil {
		t.Fatal("expected error when not logged in")
	}
	if !strings.Contains(err.Error(), "login") {
		t.Errorf("error should mention login: %v", err)
	}
}

func TestRequireAuth_InvalidKey(t *testing.T) {
	db := setupTestDB(t)
	_, err := createUser(db, "TestUser", "abc12345")
	if err != nil {
		t.Fatalf("createUser: %v", err)
	}

	os.Setenv("JOBS_USER", "TestUser")
	os.Setenv("JOBS_KEY", "wrongkey")
	defer os.Unsetenv("JOBS_USER")
	defer os.Unsetenv("JOBS_KEY")

	_, err = requireAuth(db)
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
}

func TestRequireAuth_UserNotFound(t *testing.T) {
	db := setupTestDB(t)

	os.Setenv("JOBS_USER", "Nobody")
	os.Setenv("JOBS_KEY", "abc12345")
	defer os.Unsetenv("JOBS_USER")
	defer os.Unsetenv("JOBS_KEY")

	_, err := requireAuth(db)
	if err == nil {
		t.Fatal("expected error for unknown user")
	}
}

// --- Login/Logout ---

func TestRunLogin_NoArgs(t *testing.T) {
	db := setupTestDB(t)
	result, err := runLogin(db, "", "")
	if err != nil {
		t.Fatalf("runLogin: %v", err)
	}
	if result.Name == "" {
		t.Error("expected non-empty name")
	}
	if len(result.Key) != 8 {
		t.Errorf("key length: got %d, want 8", len(result.Key))
	}
	if !result.IsNew {
		t.Error("expected IsNew=true")
	}

	user, err := getUserByName(db, result.Name)
	if err != nil {
		t.Fatalf("getUserByName: %v", err)
	}
	if user == nil {
		t.Fatal("user should exist in DB")
	}
	if user.Key != result.Key {
		t.Errorf("key mismatch: DB has %q, result has %q", user.Key, result.Key)
	}
}

func TestRunLogin_NewName(t *testing.T) {
	db := setupTestDB(t)
	result, err := runLogin(db, "JazzHands", "")
	if err != nil {
		t.Fatalf("runLogin: %v", err)
	}
	if result.Name != "JazzHands" {
		t.Errorf("name: got %q, want %q", result.Name, "JazzHands")
	}
	if len(result.Key) != 8 {
		t.Errorf("key length: got %d, want 8", len(result.Key))
	}
	if !result.IsNew {
		t.Error("expected IsNew=true")
	}
}

func TestRunLogin_ExistingNameNoKey(t *testing.T) {
	db := setupTestDB(t)
	_, _ = createUser(db, "JazzHands", "abc12345")

	_, err := runLogin(db, "JazzHands", "")
	if err == nil {
		t.Fatal("expected error when logging into existing user without key")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error should mention existing: %v", err)
	}
}

func TestRunLogin_ExistingNameWithValidKey(t *testing.T) {
	db := setupTestDB(t)
	_, _ = createUser(db, "JazzHands", "abc12345")

	result, err := runLogin(db, "JazzHands", "abc12345")
	if err != nil {
		t.Fatalf("runLogin: %v", err)
	}
	if result.Name != "JazzHands" {
		t.Errorf("name: got %q, want %q", result.Name, "JazzHands")
	}
	if result.Key != "abc12345" {
		t.Errorf("key: got %q, want %q", result.Key, "abc12345")
	}
	if result.IsNew {
		t.Error("expected IsNew=false")
	}
}

func TestRunLogin_ExistingNameWithWrongKey(t *testing.T) {
	db := setupTestDB(t)
	_, _ = createUser(db, "JazzHands", "abc12345")

	_, err := runLogin(db, "JazzHands", "wrongkey")
	if err == nil {
		t.Fatal("expected error for wrong key")
	}
}

func TestRunLogin_DuplicateNameRejected(t *testing.T) {
	db := setupTestDB(t)
	_, _ = runLogin(db, "JazzHands", "")

	_, err := runLogin(db, "JazzHands", "")
	if err == nil {
		t.Fatal("expected error when trying to create duplicate user")
	}
}

func TestFormatLoginExport(t *testing.T) {
	result := formatLoginExport("JazzHands", "abc12345")
	if result != `export JOBS_USER="JazzHands" JOBS_KEY="abc12345"` {
		t.Errorf("unexpected export format: %q", result)
	}
}

func TestFormatLogoutExport(t *testing.T) {
	result := formatLogoutExport()
	if result != "unset JOBS_USER JOBS_KEY" {
		t.Errorf("unexpected logout format: %q", result)
	}
}
