package job

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// R5 — Include notes in `job info`. Notes are stored as `noted` events
// with `text` in the detail JSON; surfacing them in info turns the
// "everything about this task" view into actually that.

func TestRunInfo_IncludesNotesChronological(t *testing.T) {
	origNow := CurrentNowFunc
	defer func() { CurrentNowFunc = origNow }()
	base := time.Unix(1_700_000_000, 0)
	CurrentNowFunc = func() time.Time { return base }

	db := SetupTestDB(t)
	id := MustAdd(t, db, "", "Task")
	if err := RunNote(db, id, "first note", nil, TestActor); err != nil {
		t.Fatalf("note 1: %v", err)
	}
	CurrentNowFunc = func() time.Time { return base.Add(15 * time.Minute) }
	if err := RunNote(db, id, "second note", nil, "ben"); err != nil {
		t.Fatalf("note 2: %v", err)
	}

	info, err := RunInfo(db, id)
	if err != nil {
		t.Fatalf("RunInfo: %v", err)
	}
	if len(info.Notes) != 2 {
		t.Fatalf("expected 2 notes, got %d", len(info.Notes))
	}
	if info.Notes[0].Text != "first note" {
		t.Errorf("note[0].Text: got %q, want %q", info.Notes[0].Text, "first note")
	}
	if info.Notes[1].Text != "second note" {
		t.Errorf("note[1].Text: got %q, want %q", info.Notes[1].Text, "second note")
	}
	if info.Notes[0].Actor != TestActor {
		t.Errorf("note[0].Actor: got %q, want %q", info.Notes[0].Actor, TestActor)
	}
	if info.Notes[1].Actor != "ben" {
		t.Errorf("note[1].Actor: got %q, want %q", info.Notes[1].Actor, "ben")
	}
	// Order is preserved by SQL `id ASC` tiebreak even when wall-clock
	// timestamps tie (RunNote uses time.Now() directly, so CurrentNowFunc
	// doesn't influence event timestamps). The Text-order assertions above
	// are the real check; we just confirm timestamps don't go backwards.
	if info.Notes[0].CreatedAt > info.Notes[1].CreatedAt {
		t.Errorf("notes out of order: %d > %d", info.Notes[0].CreatedAt, info.Notes[1].CreatedAt)
	}
}

func TestRunInfo_NoNotes_NotesIsEmpty(t *testing.T) {
	db := SetupTestDB(t)
	id := MustAdd(t, db, "", "Task")

	info, err := RunInfo(db, id)
	if err != nil {
		t.Fatalf("RunInfo: %v", err)
	}
	if len(info.Notes) != 0 {
		t.Errorf("expected 0 notes, got %d", len(info.Notes))
	}
}

func TestRenderInfoMarkdown_RendersNotesSection(t *testing.T) {
	origNow := CurrentNowFunc
	defer func() { CurrentNowFunc = origNow }()
	base := time.Unix(1_700_000_000, 0)
	CurrentNowFunc = func() time.Time { return base }

	db := SetupTestDB(t)
	id := MustAdd(t, db, "", "Task")
	if err := RunNote(db, id, "first note body", nil, TestActor); err != nil {
		t.Fatalf("note: %v", err)
	}

	info, err := RunInfo(db, id)
	if err != nil {
		t.Fatalf("RunInfo: %v", err)
	}
	var buf bytes.Buffer
	RenderInfoMarkdown(&buf, info)
	got := buf.String()
	if !strings.Contains(got, "Notes:") {
		t.Errorf("missing Notes: section:\n%s", got)
	}
	if !strings.Contains(got, "first note body") {
		t.Errorf("note body not rendered:\n%s", got)
	}
	if !strings.Contains(got, "@"+TestActor) {
		t.Errorf("actor not rendered:\n%s", got)
	}
}

func TestRenderInfoMarkdown_OmitsNotesSection_WhenNone(t *testing.T) {
	db := SetupTestDB(t)
	id := MustAdd(t, db, "", "Task")

	info, err := RunInfo(db, id)
	if err != nil {
		t.Fatalf("RunInfo: %v", err)
	}
	var buf bytes.Buffer
	RenderInfoMarkdown(&buf, info)
	got := buf.String()
	if strings.Contains(got, "Notes:") {
		t.Errorf("Notes: section should be omitted when no notes:\n%s", got)
	}
}

func TestRenderInfoJSON_IncludesNotesArray(t *testing.T) {
	db := SetupTestDB(t)
	id := MustAdd(t, db, "", "Task")
	if err := RunNote(db, id, "alpha", nil, TestActor); err != nil {
		t.Fatalf("note: %v", err)
	}
	if err := RunNote(db, id, "beta", nil, "ben"); err != nil {
		t.Fatalf("note: %v", err)
	}

	info, err := RunInfo(db, id)
	if err != nil {
		t.Fatalf("RunInfo: %v", err)
	}
	var buf bytes.Buffer
	RenderInfoJSON(&buf, info)

	var got struct {
		Notes []struct {
			Text      string `json:"text"`
			Actor     string `json:"actor"`
			CreatedAt int64  `json:"created_at"`
		} `json:"notes"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(got.Notes) != 2 {
		t.Fatalf("expected 2 notes in json, got %d", len(got.Notes))
	}
	if got.Notes[0].Text != "alpha" || got.Notes[1].Text != "beta" {
		t.Errorf("notes order: got %q,%q want alpha,beta", got.Notes[0].Text, got.Notes[1].Text)
	}
}

func TestRenderInfoJSON_NoNotes_EmptyArray(t *testing.T) {
	db := SetupTestDB(t)
	id := MustAdd(t, db, "", "Task")

	info, err := RunInfo(db, id)
	if err != nil {
		t.Fatalf("RunInfo: %v", err)
	}
	var buf bytes.Buffer
	RenderInfoJSON(&buf, info)

	if !strings.Contains(buf.String(), `"notes": []`) {
		t.Errorf("expected empty notes array in JSON:\n%s", buf.String())
	}
}
