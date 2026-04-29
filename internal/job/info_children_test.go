package job

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// `show <id>` lists children inline as a markdown checklist when the task
// has direct children. The cap rule:
//   - 1..10 children: render every child as one line under a `Children:`
//     header, using the same shape as `ls` output.
//   - >10 children: collapse to `Children: N (use 'job ls <id>' to see all)`.

func TestRenderInfoMarkdown_ListsChildrenInline(t *testing.T) {
	db := SetupTestDB(t)
	parent := MustAdd(t, db, "", "Parent task")
	doneChild := MustAdd(t, db, parent, "First child")
	MustDone(t, db, doneChild)
	openChild := MustAdd(t, db, parent, "Second child")
	_ = openChild

	info, err := RunInfo(db, parent)
	if err != nil {
		t.Fatalf("RunInfo: %v", err)
	}
	var buf bytes.Buffer
	RenderInfoMarkdown(&buf, info)
	got := buf.String()

	if !strings.Contains(got, "Children:\n") {
		t.Errorf("missing `Children:` header line:\n%s", got)
	}
	if !strings.Contains(got, fmt.Sprintf("- [x] `%s` First child", doneChild)) {
		t.Errorf("done child missing or wrong shape:\n%s", got)
	}
	if !strings.Contains(got, fmt.Sprintf("- [ ] `%s` Second child", openChild)) {
		t.Errorf("open child missing or wrong shape:\n%s", got)
	}
}

func TestRenderInfoMarkdown_NoChildren_OmitsChildrenSection(t *testing.T) {
	db := SetupTestDB(t)
	id := MustAdd(t, db, "", "Lonely")

	info, err := RunInfo(db, id)
	if err != nil {
		t.Fatalf("RunInfo: %v", err)
	}
	var buf bytes.Buffer
	RenderInfoMarkdown(&buf, info)
	got := buf.String()
	if strings.Contains(got, "Children:") {
		t.Errorf("Children: section should be omitted when no children:\n%s", got)
	}
}

func TestRenderInfoMarkdown_ManyChildren_FallbackCount(t *testing.T) {
	db := SetupTestDB(t)
	parent := MustAdd(t, db, "", "Big parent")
	for i := range 11 {
		MustAdd(t, db, parent, fmt.Sprintf("Child %d", i))
	}

	info, err := RunInfo(db, parent)
	if err != nil {
		t.Fatalf("RunInfo: %v", err)
	}
	var buf bytes.Buffer
	RenderInfoMarkdown(&buf, info)
	got := buf.String()

	if !strings.Contains(got, "Children:     11") {
		t.Errorf("expected fallback count line `Children:     11 ...`:\n%s", got)
	}
	if !strings.Contains(got, "`job ls") {
		t.Errorf("expected fallback hint to point at `job ls <id>`:\n%s", got)
	}
	// Must not list individual children when over the cap.
	if strings.Contains(got, "- [ ] `") {
		t.Errorf(">10 children should collapse to a count line, not list them:\n%s", got)
	}
}

func TestRenderInfoMarkdown_ChildShowsBlockerAndLabel(t *testing.T) {
	db := SetupTestDB(t)
	parent := MustAdd(t, db, "", "Parent")
	blocker := MustAdd(t, db, parent, "Gate")
	gated := MustAdd(t, db, parent, "Needs gate")
	if err := RunBlock(db, gated, blocker, TestActor); err != nil {
		t.Fatalf("RunBlock: %v", err)
	}
	if _, err := RunLabelAdd(db, gated, []string{"p0"}, TestActor); err != nil {
		t.Fatalf("RunLabelAdd: %v", err)
	}

	info, err := RunInfo(db, parent)
	if err != nil {
		t.Fatalf("RunInfo: %v", err)
	}
	var buf bytes.Buffer
	RenderInfoMarkdown(&buf, info)
	got := buf.String()

	gatedLine := ""
	for line := range strings.SplitSeq(got, "\n") {
		if strings.Contains(line, gated) {
			gatedLine = line
			break
		}
	}
	if gatedLine == "" {
		t.Fatalf("could not find gated child line in:\n%s", got)
	}
	if !strings.Contains(gatedLine, "blocked on "+blocker) {
		t.Errorf("gated child line missing `blocked on %s`:\n%s", blocker, gatedLine)
	}
	if !strings.Contains(gatedLine, "labels: p0") {
		t.Errorf("gated child line missing `labels: p0`:\n%s", gatedLine)
	}
}

func TestRenderInfoJSON_IncludesChildrenArray(t *testing.T) {
	db := SetupTestDB(t)
	parent := MustAdd(t, db, "", "Parent")
	c1 := MustAdd(t, db, parent, "First")
	c2 := MustAdd(t, db, parent, "Second")
	MustDone(t, db, c1)

	info, err := RunInfo(db, parent)
	if err != nil {
		t.Fatalf("RunInfo: %v", err)
	}
	var buf bytes.Buffer
	RenderInfoJSON(&buf, info)

	var obj struct {
		Children []struct {
			ID     string `json:"id"`
			Title  string `json:"title"`
			Status string `json:"status"`
		} `json:"children"`
	}
	if err := json.Unmarshal(buf.Bytes(), &obj); err != nil {
		t.Fatalf("unmarshal info JSON: %v\npayload=%s", err, buf.String())
	}
	if len(obj.Children) != 2 {
		t.Fatalf("expected 2 children in JSON, got %d (payload=%s)", len(obj.Children), buf.String())
	}
	if obj.Children[0].ID != c1 || obj.Children[0].Status != "done" {
		t.Errorf("children[0] mismatch: got %+v", obj.Children[0])
	}
	if obj.Children[1].ID != c2 || obj.Children[1].Status != "available" {
		t.Errorf("children[1] mismatch: got %+v", obj.Children[1])
	}
}
