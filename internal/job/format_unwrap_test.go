package job

import (
	"bytes"
	"strings"
	"testing"
)

// R6 — Stop hard-wrapping prose in `job info`. The wrap actually comes
// from the source text (YAML imports preserve author-supplied `\n`s),
// not from the renderer. Fix: unwrap at render time — single newlines
// become spaces, paragraph breaks (\n\n) are preserved, bullet lines
// are kept structural.

func TestUnwrapProse_SingleLine(t *testing.T) {
	got := unwrapProse("hello world")
	if got != "hello world" {
		t.Errorf("got %q, want %q", got, "hello world")
	}
}

func TestUnwrapProse_CollapsesSingleNewlines(t *testing.T) {
	in := "Eight agent-facing CLI ergonomics fixes grouped under one\numbrella, drawn from the R0–R7 recommendations."
	want := "Eight agent-facing CLI ergonomics fixes grouped under one umbrella, drawn from the R0–R7 recommendations."
	if got := unwrapProse(in); got != want {
		t.Errorf("\n got: %q\nwant: %q", got, want)
	}
}

func TestUnwrapProse_PreservesParagraphBreaks(t *testing.T) {
	in := "first paragraph\nstill first.\n\nsecond paragraph\nstill second."
	want := "first paragraph still first.\n\nsecond paragraph still second."
	if got := unwrapProse(in); got != want {
		t.Errorf("\n got: %q\nwant: %q", got, want)
	}
}

func TestUnwrapProse_PreservesBullets(t *testing.T) {
	in := "Implemented:\n- sticky body layout\n- mini-graph hover\n- log view\nRemaining:\n- actor view"
	got := unwrapProse(in)
	// Each bullet should be on its own line.
	for _, want := range []string{"- sticky body layout", "- mini-graph hover", "- log view", "- actor view"} {
		if !strings.Contains(got, "\n"+want) && !strings.HasPrefix(got, want) {
			t.Errorf("missing bullet %q in:\n%s", want, got)
		}
	}
	// "Implemented:" and "Remaining:" introduce bullet groups; should not
	// collapse into the bullets they precede.
	if !strings.Contains(got, "Implemented:") {
		t.Errorf("missing Implemented: in:\n%s", got)
	}
	if !strings.Contains(got, "Remaining:") {
		t.Errorf("missing Remaining: in:\n%s", got)
	}
}

func TestUnwrapProse_TrailingNewlinesTrimmed(t *testing.T) {
	got := unwrapProse("hello\n\n\n")
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestRenderInfoMarkdown_UnwrapsDescription(t *testing.T) {
	db := SetupTestDB(t)
	id, err := RunAdd(db, "", "Task",
		"line one of description\nline two\nline three with words", "", nil, TestActor)
	if err != nil {
		t.Fatalf("RunAdd: %v", err)
	}

	info, err := RunInfo(db, id.ShortID)
	if err != nil {
		t.Fatalf("RunInfo: %v", err)
	}
	var buf bytes.Buffer
	RenderInfoMarkdown(&buf, info)
	got := buf.String()
	// The whole description should now appear on a single Description: line
	// (no embedded newlines).
	if !strings.Contains(got, "Description:  line one of description line two line three with words") {
		t.Errorf("description not unwrapped:\n%s", got)
	}
}

func TestRenderInfoMarkdown_UnwrapsNoteBody(t *testing.T) {
	db := SetupTestDB(t)
	id := MustAdd(t, db, "", "Task")
	body := "Implemented sticky chrome layout\nbody/page/main use 100vh\nmain has overflow-y: auto"
	if err := RunNote(db, id, body, nil, TestActor); err != nil {
		t.Fatalf("note: %v", err)
	}

	info, err := RunInfo(db, id)
	if err != nil {
		t.Fatalf("RunInfo: %v", err)
	}
	var buf bytes.Buffer
	RenderInfoMarkdown(&buf, info)
	got := buf.String()
	if strings.Contains(got, "Implemented sticky chrome layout\n    body/page/main") {
		t.Errorf("note body should be unwrapped, not preserved as multi-line:\n%s", got)
	}
}
