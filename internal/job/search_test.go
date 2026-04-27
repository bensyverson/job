package job

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func setupSearchTestDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "search.db")
	db, err := CreateDB(path)
	if err != nil {
		t.Fatalf("CreateDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func searchMustAdd(t *testing.T, db *sql.DB, actor, title, desc string, parent *string, labels []string) string {
	t.Helper()
	parentID := ""
	if parent != nil {
		parentID = *parent
	}
	res, err := RunAdd(db, parentID, title, desc, "", labels, actor)
	if err != nil {
		t.Fatalf("RunAdd(%q): %v", title, err)
	}
	return res.ShortID
}

func searchMustNote(t *testing.T, db *sql.DB, shortID, text, actor string) {
	t.Helper()
	if err := RunNote(db, shortID, text, nil, actor); err != nil {
		t.Fatalf("RunNote(%q): %v", shortID, err)
	}
}

func searchMustClaim(t *testing.T, db *sql.DB, shortID, actor string) {
	t.Helper()
	if err := RunClaim(db, shortID, "", actor, false); err != nil {
		t.Fatalf("RunClaim(%q): %v", shortID, err)
	}
}

func searchMustDone(t *testing.T, db *sql.DB, shortID, actor string) {
	t.Helper()
	if _, _, err := RunDone(db, []string{shortID}, false, "", nil, actor); err != nil {
		t.Fatalf("RunDone(%q): %v", shortID, err)
	}
}

func TestRunSearch_MatchesByShortID(t *testing.T) {
	db := setupSearchTestDB(t)
	id := searchMustAdd(t, db, "alice", "Some task", "", nil, nil)

	hits, err := RunSearch(db, id, 10)
	if err != nil {
		t.Fatalf("RunSearch: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	if hits[0].ShortID != id {
		t.Errorf("expected short_id %q, got %q", id, hits[0].ShortID)
	}
}

func TestRunSearch_MatchesByTitle(t *testing.T) {
	db := setupSearchTestDB(t)
	searchMustAdd(t, db, "alice", "Dashboard polish", "", nil, nil)

	hits, err := RunSearch(db, "polish", 10)
	if err != nil {
		t.Fatalf("RunSearch: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	if hits[0].Title != "Dashboard polish" {
		t.Errorf("expected title %q, got %q", "Dashboard polish", hits[0].Title)
	}
}

func TestRunSearch_MatchesByDescription(t *testing.T) {
	db := setupSearchTestDB(t)
	searchMustAdd(t, db, "alice", "Task", "Build the search index", nil, nil)

	hits, err := RunSearch(db, "index", 10)
	if err != nil {
		t.Fatalf("RunSearch: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
}

func TestRunSearch_MatchesByNote(t *testing.T) {
	db := setupSearchTestDB(t)
	id := searchMustAdd(t, db, "alice", "Task", "", nil, nil)
	searchMustNote(t, db, id, "Remember to update docs", "alice")

	hits, err := RunSearch(db, "docs", 10)
	if err != nil {
		t.Fatalf("RunSearch: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
}

func TestRunSearch_MatchesByLabel(t *testing.T) {
	db := setupSearchTestDB(t)
	searchMustAdd(t, db, "alice", "Task", "", nil, []string{"search"})

	hits, err := RunSearch(db, "search", 10)
	if err != nil {
		t.Fatalf("RunSearch: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
}

func TestRunSearch_Deduplicates(t *testing.T) {
	db := setupSearchTestDB(t)
	id := searchMustAdd(t, db, "alice", "Search task", "search desc", nil, []string{"search"})
	searchMustNote(t, db, id, "search note", "alice")

	hits, err := RunSearch(db, "search", 10)
	if err != nil {
		t.Fatalf("RunSearch: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 deduplicated hit, got %d", len(hits))
	}
}

func TestRunSearch_PrioritySort(t *testing.T) {
	db := setupSearchTestDB(t)
	claimed := searchMustAdd(t, db, "alice", "Claimed task", "", nil, nil)
	_ = searchMustAdd(t, db, "alice", "Available task", "", nil, nil)
	done := searchMustAdd(t, db, "alice", "Done task", "", nil, nil)
	canceled := searchMustAdd(t, db, "alice", "Canceled task", "", nil, nil)

	searchMustClaim(t, db, claimed, "alice")
	searchMustDone(t, db, done, "alice")
	_, _, _, err := RunCancel(db, []string{canceled}, "test", false, false, false, "alice")
	if err != nil {
		t.Fatalf("RunCancel: %v", err)
	}

	hits, err := RunSearch(db, "task", 10)
	if err != nil {
		t.Fatalf("RunSearch: %v", err)
	}
	if len(hits) != 4 {
		t.Fatalf("expected 4 hits, got %d", len(hits))
	}
	want := []string{"claimed", "available", "done", "canceled"}
	for i, h := range hits {
		if h.Status != want[i] {
			t.Errorf("hit[%d].status = %q, want %q", i, h.Status, want[i])
		}
	}
}

func TestRunSearch_Limit(t *testing.T) {
	db := setupSearchTestDB(t)
	for i := range 5 {
		searchMustAdd(t, db, "alice", "Task "+string(rune('A'+i)), "", nil, nil)
	}

	hits, err := RunSearch(db, "Task", 3)
	if err != nil {
		t.Fatalf("RunSearch: %v", err)
	}
	if len(hits) != 3 {
		t.Fatalf("expected 3 hits (limit), got %d", len(hits))
	}
}

func TestRunSearch_EmptyQuery(t *testing.T) {
	db := setupSearchTestDB(t)
	searchMustAdd(t, db, "alice", "Task", "", nil, nil)

	hits, err := RunSearch(db, "", 10)
	if err != nil {
		t.Fatalf("RunSearch: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("expected 0 hits for empty query, got %d", len(hits))
	}

	hits, err = RunSearch(db, "   ", 10)
	if err != nil {
		t.Fatalf("RunSearch: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("expected 0 hits for whitespace query, got %d", len(hits))
	}
}

func TestRunSearch_ExcludesDeleted(t *testing.T) {
	db := setupSearchTestDB(t)
	id := searchMustAdd(t, db, "alice", "Gone", "", nil, nil)
	now := time.Now()
	if _, err := db.Exec(`UPDATE tasks SET deleted_at = ? WHERE short_id = ?`, now.Unix(), id); err != nil {
		t.Fatalf("soft-delete: %v", err)
	}

	hits, err := RunSearch(db, "Gone", 10)
	if err != nil {
		t.Fatalf("RunSearch: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("expected 0 hits for deleted task, got %d", len(hits))
	}
}
