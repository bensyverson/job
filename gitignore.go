package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
)

const gitignoreHint = `Recommended .gitignore entries:
  .jobs.db-shm      # SQLite WAL index (always local)
  .jobs.db-wal      # SQLite WAL journal (always local)

To also keep the tracker local (don't check in the tree):
  .jobs.db

Or run: job init --gitignore  to write these for you.`

type gitignoreEntry struct {
	name    string
	comment string
}

var jobGitignoreEntries = []gitignoreEntry{
	{".jobs.db-shm", "SQLite WAL index (always local)"},
	{".jobs.db-wal", "SQLite WAL journal (always local)"},
}

func writeGitignoreEntries(dir string) (written []string, alreadyPresent []string, err error) {
	path := filepath.Join(dir, ".gitignore")
	existing := ""
	if data, readErr := os.ReadFile(path); readErr == nil {
		existing = string(data)
	} else if !os.IsNotExist(readErr) {
		return nil, nil, readErr
	}

	for _, e := range jobGitignoreEntries {
		if gitignoreHasEntry(existing, e.name) {
			alreadyPresent = append(alreadyPresent, e.name)
		} else {
			written = append(written, e.name)
		}
	}

	if len(written) == 0 {
		return written, alreadyPresent, nil
	}

	var buf bytes.Buffer
	buf.WriteString(existing)
	if existing != "" && !strings.HasSuffix(existing, "\n") {
		buf.WriteString("\n")
	}
	if existing != "" {
		buf.WriteString("\n")
	}
	buf.WriteString("# job\n")
	for _, e := range jobGitignoreEntries {
		if gitignoreHasEntry(existing, e.name) {
			continue
		}
		buf.WriteString(e.name)
		buf.WriteString("\t# ")
		buf.WriteString(e.comment)
		buf.WriteString("\n")
	}

	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return nil, nil, err
	}

	return written, alreadyPresent, nil
}

func gitignoreHasEntry(content, name string) bool {
	for line := range strings.SplitSeq(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) > 0 && fields[0] == name {
			return true
		}
	}
	return false
}
