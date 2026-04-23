package main

import (
	"strings"
	"testing"

	job "github.com/bensyverson/jobs/internal/job"
)

// R4 — Echo note body on success.
//
// Format (two-line, aligned with done/cancel):
//
//	Noted: <id> "<Title>"
//	  note: <N chars> · "<preview>"
//
//   - <N chars> is the raw character count of the stored body.
//   - <preview> is the first 60 chars (word-boundary-snapped), with
//     newlines/tabs collapsed to spaces. Elided with `…` only when the
//     body exceeds 60 chars.

func TestNote_EchoesShortBody(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := job.MustAdd(t, db, "", "My Task")
	db.Close()

	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "note", id, "-m", "hello world")
	if err != nil {
		t.Fatalf("note: %v", err)
	}
	wantLine1 := "Noted: " + id + " \"My Task\"\n"
	wantLine2 := "  note: 11 chars · \"hello world\"\n"
	want := wantLine1 + wantLine2
	if stdout != want {
		t.Errorf("stdout:\n  got:  %q\n  want: %q", stdout, want)
	}
}

func TestNote_EchoesAtSixtyCharsExactly(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := job.MustAdd(t, db, "", "Task")
	db.Close()

	body := strings.Repeat("a", 60)
	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "note", id, "-m", body)
	if err != nil {
		t.Fatalf("note: %v", err)
	}
	if !strings.HasPrefix(stdout, "Noted: "+id) {
		t.Errorf("expected 'Noted: <id>' header:\n%s", stdout)
	}
	noteLine := ""
	for l := range strings.SplitSeq(stdout, "\n") {
		if strings.HasPrefix(l, "  note:") {
			noteLine = l
		}
	}
	if noteLine == "" {
		t.Fatalf("missing '  note:' sub-line:\n%s", stdout)
	}
	if !strings.Contains(noteLine, "60 chars") {
		t.Errorf("expected '60 chars' in note sub-line:\n%s", noteLine)
	}
	if strings.Contains(noteLine, "…") {
		t.Errorf("60-char body should not be elided:\n%s", noteLine)
	}
	if !strings.Contains(noteLine, "\""+body+"\"") {
		t.Errorf("expected full body in quotes in note sub-line:\n%s", noteLine)
	}
}

func TestNote_ElidesLongBody(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := job.MustAdd(t, db, "", "Task")
	db.Close()

	body := "Implemented sticky chrome layout with body height:100vh and main overflow-y:auto for the home dashboard"
	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "note", id, "-m", body)
	if err != nil {
		t.Fatalf("note: %v", err)
	}
	if !strings.Contains(stdout, "…") {
		t.Errorf("long body should be elided with …:\n%s", stdout)
	}
	expectedCount := len(body)
	if !strings.Contains(stdout, " "+itoa(expectedCount)+" chars ") {
		t.Errorf("expected raw count %d in output:\n%s", expectedCount, stdout)
	}
	// Word-boundary snap — the preview must not end mid-word. Pull the
	// preview between the quotes and check that the last visible char
	// (before …) is followed by a space in the original.
	preview := extractQuotedPreview(t, stdout)
	prevTrim := strings.TrimSuffix(preview, "…")
	if strings.HasSuffix(prevTrim, " ") {
		t.Errorf("preview should not end with trailing space: %q", preview)
	}
	// The trimmed preview should be a prefix of the body up to a space.
	if !strings.HasPrefix(body, prevTrim) {
		t.Errorf("preview is not a prefix of body: %q", preview)
	}
}

func TestNote_CollapsesNewlinesInPreview(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := job.MustAdd(t, db, "", "Task")
	db.Close()

	body := "first line\nsecond line\tthird"
	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "note", id, "-m", body)
	if err != nil {
		t.Fatalf("note: %v", err)
	}
	preview := extractQuotedPreview(t, stdout)
	if strings.ContainsAny(preview, "\n\t") {
		t.Errorf("preview should not contain newlines or tabs: %q", preview)
	}
}

func TestNote_RoundTripsBackticks(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := job.MustAdd(t, db, "", "Task")
	db.Close()

	body := "tap `border-bottom-color` at (0,1,1)"
	stdout, _, err := runCLI(t, dbFile, "--as", "alice", "note", id, "-m", body)
	if err != nil {
		t.Fatalf("note: %v", err)
	}
	preview := extractQuotedPreview(t, stdout)
	if !strings.Contains(preview, "`border-bottom-color`") {
		t.Errorf("preview should preserve backticks: %q", preview)
	}
}

func TestNote_StoresFullBody_NotPreview(t *testing.T) {
	dbFile := setupCLI(t)
	db := openTestDB(t, dbFile)
	id := job.MustAdd(t, db, "", "Task")
	db.Close()

	body := strings.Repeat("xy ", 40)
	if _, _, err := runCLI(t, dbFile, "--as", "alice", "note", id, "-m", body); err != nil {
		t.Fatalf("note: %v", err)
	}

	db = openTestDB(t, dbFile)
	defer db.Close()
	info, err := job.RunInfo(db, id)
	if err != nil {
		t.Fatalf("RunInfo: %v", err)
	}
	if !strings.Contains(info.Task.Description, strings.TrimSpace(body)) {
		t.Errorf("description missing full body — preview must not affect storage:\n%s", info.Task.Description)
	}
}

// itoa avoids pulling strconv into one tiny site.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// extractQuotedPreview pulls the quoted preview from the "  note: N chars · "…"" sub-line.
func extractQuotedPreview(t *testing.T, output string) string {
	t.Helper()
	for line := range strings.SplitSeq(output, "\n") {
		if strings.HasPrefix(line, "  note:") {
			start := strings.IndexByte(line, '"')
			end := strings.LastIndexByte(line, '"')
			if start >= 0 && end > start {
				return line[start+1 : end]
			}
		}
	}
	t.Fatalf("could not find '  note:' sub-line in: %q", output)
	return ""
}
