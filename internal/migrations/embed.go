// Package migrations bundles the forward-only SQL migrations that define
// the job database schema. Consumers read via FS(); the runner lives in
// internal/job.
package migrations

import (
	"embed"
	"io/fs"
)

//go:embed *.sql
var files embed.FS

// FS returns the embedded migrations directory as an fs.FS.
func FS() fs.FS { return files }
