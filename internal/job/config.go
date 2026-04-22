package job

import (
	"database/sql"
	"fmt"
)

const (
	configKeyDefaultIdentity = "default_identity"
	configKeyStrict          = "strict"
)

// GetConfig returns the string value stored for key, or "" if unset.
func GetConfig(db dbtx, key string) (string, error) {
	var value string
	err := db.QueryRow("SELECT value FROM config WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("config get %s: %w", key, err)
	}
	return value, nil
}

// SetConfig upserts a config row. Empty value is stored as-is (use
// DeleteConfig to remove).
func SetConfig(db dbtx, key, value string) error {
	_, err := db.Exec(
		"INSERT INTO config (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
		key, value,
	)
	if err != nil {
		return fmt.Errorf("config set %s: %w", key, err)
	}
	return nil
}

// GetDefaultIdentity returns the configured default writer identity, or
// "" if unset.
func GetDefaultIdentity(db dbtx) (string, error) {
	return GetConfig(db, configKeyDefaultIdentity)
}

// SetDefaultIdentity sets the configured default writer identity.
func SetDefaultIdentity(db dbtx, name string) error {
	return SetConfig(db, configKeyDefaultIdentity, name)
}

// IsStrict reports whether strict mode is enabled. Default is permissive
// (false) when the key is unset.
func IsStrict(db dbtx) (bool, error) {
	v, err := GetConfig(db, configKeyStrict)
	if err != nil {
		return false, err
	}
	return v == "true", nil
}

// SetStrict toggles strict mode.
func SetStrict(db dbtx, on bool) error {
	v := "false"
	if on {
		v = "true"
	}
	return SetConfig(db, configKeyStrict, v)
}

// ResolveIdentity applies the P3 resolution chain for writes:
//  1. flagValue — the --as flag value (wins if non-empty).
//  2. default_identity from config, unless strict mode is on.
//  3. "" — caller must error with "identity required".
func ResolveIdentity(db dbtx, flagValue string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	strict, err := IsStrict(db)
	if err != nil {
		return "", err
	}
	if strict {
		return "", nil
	}
	return GetDefaultIdentity(db)
}
