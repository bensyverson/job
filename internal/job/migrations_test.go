package job

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// openFreshSqlite opens a raw sqlite connection without any migrations — so
// each test case controls what the migrator sees.
func openFreshSqlite(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mig.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// initialFS is the minimal baseline migration used by several tests.
var initialFS = fstest.MapFS{
	"0001_initial.sql": &fstest.MapFile{Data: []byte(`
CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT);
`)},
}

func TestRunMigrations_FreshDBAppliesBaseline(t *testing.T) {
	db := openFreshSqlite(t)
	if err := RunMigrations(db, initialFS); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	// schema_migrations table must exist.
	var name string
	err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='schema_migrations'").Scan(&name)
	if err != nil {
		t.Fatalf("schema_migrations table missing: %v", err)
	}
	// The 0001 migration must be recorded.
	var version int
	if err := db.QueryRow("SELECT version FROM schema_migrations WHERE version=1").Scan(&version); err != nil {
		t.Fatalf("expected schema_migrations to contain version=1: %v", err)
	}
	if version != 1 {
		t.Errorf("version = %d, want 1", version)
	}
	// The baseline actually ran (table 't' exists).
	if err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='t'").Scan(&name); err != nil {
		t.Errorf("0001 migration did not apply: %v", err)
	}
}

func TestRunMigrations_Idempotent(t *testing.T) {
	db := openFreshSqlite(t)
	if err := RunMigrations(db, initialFS); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if err := RunMigrations(db, initialFS); err != nil {
		t.Fatalf("second run: %v", err)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("schema_migrations row count = %d after idempotent re-run, want 1", count)
	}
}

func TestRunMigrations_AppliesNewMigration(t *testing.T) {
	db := openFreshSqlite(t)
	if err := RunMigrations(db, initialFS); err != nil {
		t.Fatalf("first: %v", err)
	}
	extendedFS := fstest.MapFS{
		"0001_initial.sql": initialFS["0001_initial.sql"],
		"0002_add_column.sql": &fstest.MapFile{Data: []byte(`
ALTER TABLE t ADD COLUMN extra TEXT;
`)},
	}
	if err := RunMigrations(db, extendedFS); err != nil {
		t.Fatalf("second: %v", err)
	}
	// The new migration's column must exist.
	rows, err := db.Query("PRAGMA table_info(t)")
	if err != nil {
		t.Fatalf("pragma: %v", err)
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan: %v", err)
		}
		seen[name] = true
	}
	if !seen["extra"] {
		t.Errorf("0002 migration did not apply; columns=%v", seen)
	}
	// schema_migrations now has version=2.
	var version int
	if err := db.QueryRow("SELECT version FROM schema_migrations WHERE version=2").Scan(&version); err != nil {
		t.Errorf("expected version=2 row: %v", err)
	}
}

func TestRunMigrations_BrokenMigrationErrorsAndDoesNotMark(t *testing.T) {
	db := openFreshSqlite(t)
	brokenFS := fstest.MapFS{
		"0001_initial.sql": initialFS["0001_initial.sql"],
		"0002_broken.sql":  &fstest.MapFile{Data: []byte(`this is not valid SQL at all;`)},
	}
	err := RunMigrations(db, brokenFS)
	if err == nil {
		t.Fatalf("expected error from broken migration, got nil")
	}
	if !strings.Contains(err.Error(), "0002") {
		t.Errorf("error should name the failing migration; got: %v", err)
	}
	// 0001 should still be recorded; 0002 must not be.
	var has1, has2 int
	db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version=1").Scan(&has1)
	db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version=2").Scan(&has2)
	if has1 != 1 {
		t.Errorf("0001 should be applied, got count=%d", has1)
	}
	if has2 != 0 {
		t.Errorf("0002 must not be marked applied after failure, got count=%d", has2)
	}
}

func TestRunMigrations_NumericGapsAllowed(t *testing.T) {
	db := openFreshSqlite(t)
	gappyFS := fstest.MapFS{
		"0001_a.sql": &fstest.MapFile{Data: []byte("CREATE TABLE a (id INTEGER);")},
		"0005_b.sql": &fstest.MapFile{Data: []byte("CREATE TABLE b (id INTEGER);")},
		"0042_c.sql": &fstest.MapFile{Data: []byte("CREATE TABLE c (id INTEGER);")},
	}
	if err := RunMigrations(db, gappyFS); err != nil {
		t.Fatalf("gappy: %v", err)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 migrations recorded, got %d", count)
	}
	for _, tbl := range []string{"a", "b", "c"} {
		var n string
		if err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", tbl).Scan(&n); err != nil {
			t.Errorf("table %s missing: %v", tbl, err)
		}
	}
	// Versions recorded: 1, 5, 42.
	rows, err := db.Query("SELECT version FROM schema_migrations ORDER BY version")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []int
	for rows.Next() {
		var v int
		rows.Scan(&v)
		got = append(got, v)
	}
	if len(got) != 3 || got[0] != 1 || got[1] != 5 || got[2] != 42 {
		t.Errorf("versions = %v, want [1 5 42]", got)
	}
}

func TestRunMigrations_LexicographicOrder(t *testing.T) {
	// Two migrations whose combined effect depends on order: second depends on
	// a column the first adds. If the runner applied them alphabetically by
	// filename (which the plan specifies), this succeeds.
	db := openFreshSqlite(t)
	orderedFS := fstest.MapFS{
		"0001_create.sql": &fstest.MapFile{Data: []byte(`CREATE TABLE t (id INTEGER);`)},
		"0002_alter.sql":  &fstest.MapFile{Data: []byte(`ALTER TABLE t ADD COLUMN v TEXT;`)},
	}
	if err := RunMigrations(db, orderedFS); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	// Verify the alter worked.
	if _, err := db.Exec("INSERT INTO t (id, v) VALUES (1, 'hi')"); err != nil {
		t.Errorf("ordered apply failed: %v", err)
	}
}
