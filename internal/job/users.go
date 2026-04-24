package job

import (
	"database/sql"
	"fmt"
)

func createUser(db *sql.DB, name string) (*User, error) {
	now := CurrentNowFunc().Unix()
	result, err := db.Exec(
		"INSERT INTO users (name, created_at) VALUES (?, ?)",
		name, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create user %q: %w", name, err)
	}
	id, _ := result.LastInsertId()
	return &User{ID: id, Name: name, CreatedAt: now}, nil
}

func GetUserByName(db *sql.DB, name string) (*User, error) {
	var u User
	err := db.QueryRow("SELECT id, name, created_at FROM users WHERE name = ?", name).
		Scan(&u.ID, &u.Name, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func EnsureUser(db *sql.DB, name string) (*User, error) {
	existing, err := GetUserByName(db, name)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}
	return createUser(db, name)
}
