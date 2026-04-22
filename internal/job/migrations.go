package job

import (
	"database/sql"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
)

// RunMigrations applies any unapplied *.sql files from migrationsFS to db.
//
// Contract:
//   - Creates schema_migrations(version INTEGER PRIMARY KEY, applied_at INTEGER)
//     if missing.
//   - Files are ordered lexicographically by filename. The numeric prefix
//     before the first underscore is the migration version.
//   - Each migration runs in its own transaction. On failure, that
//     migration is NOT recorded, and RunMigrations returns an error
//     naming it. Previously-applied migrations keep their rows.
//   - Running with no new migrations is a no-op.
func RunMigrations(db *sql.DB, migrationsFS fs.FS) error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    INTEGER PRIMARY KEY,
			applied_at INTEGER NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := fs.Glob(migrationsFS, "*.sql")
	if err != nil {
		return fmt.Errorf("glob migrations: %w", err)
	}
	sort.Strings(entries)

	applied, err := loadAppliedVersions(db)
	if err != nil {
		return err
	}

	for _, name := range entries {
		version, err := parseMigrationVersion(name)
		if err != nil {
			return err
		}
		if applied[version] {
			continue
		}
		data, err := fs.ReadFile(migrationsFS, name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if err := applyMigration(db, version, name, string(data)); err != nil {
			return err
		}
	}
	return nil
}

func loadAppliedVersions(db *sql.DB) (map[int]bool, error) {
	rows, err := db.Query("SELECT version FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("query schema_migrations: %w", err)
	}
	defer rows.Close()
	applied := map[int]bool{}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

var migrationVersionRE = regexp.MustCompile(`^(\d+)`)

func parseMigrationVersion(name string) (int, error) {
	base := filepath.Base(name)
	m := migrationVersionRE.FindString(base)
	if m == "" {
		return 0, fmt.Errorf("migration %s: missing numeric prefix", name)
	}
	v, err := strconv.Atoi(m)
	if err != nil {
		return 0, fmt.Errorf("migration %s: bad version %q", name, m)
	}
	return v, nil
}

func applyMigration(db *sql.DB, version int, name, sqlText string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin %s: %w", name, err)
	}
	if _, err := tx.Exec(sqlText); err != nil {
		tx.Rollback()
		return fmt.Errorf("migration %s: %w", name, err)
	}
	if _, err := tx.Exec(
		"INSERT INTO schema_migrations (version, applied_at) VALUES (?, strftime('%s','now'))",
		version,
	); err != nil {
		tx.Rollback()
		return fmt.Errorf("migration %s: record: %w", name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("migration %s: commit: %w", name, err)
	}
	return nil
}
