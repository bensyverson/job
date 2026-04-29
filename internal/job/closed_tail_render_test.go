package job

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderClosedTail_HeaderShowsNofM(t *testing.T) {
	rows := []ClosedTailRow{
		{Task: &Task{ShortID: "abc", Title: "first", Status: "done"}, ClosedAt: 100},
	}
	var buf bytes.Buffer
	RenderClosedTail(&buf, rows, 7, nil, true)
	out := buf.String()
	if !strings.Contains(out, "Recently closed (1 of 7)") {
		t.Errorf("expected header 'Recently closed (1 of 7)', got:\n%s", out)
	}
}

func TestRenderClosedTail_OmittedWhenZero(t *testing.T) {
	var buf bytes.Buffer
	RenderClosedTail(&buf, nil, 0, nil, true)
	if buf.String() != "" {
		t.Errorf("expected empty render, got:\n%s", buf.String())
	}
}

func TestRenderClosedTail_BreadcrumbForUnscoped(t *testing.T) {
	rows := []ClosedTailRow{
		{Task: &Task{ShortID: "abc", Title: "fix bug", Status: "done", ParentID: new(int64(42))}, ClosedAt: 100},
	}
	parents := map[int64]ParentInfo{
		42: {ShortID: "par01", Title: "Parent feature"},
	}
	var buf bytes.Buffer
	RenderClosedTail(&buf, rows, 1, parents, false)
	out := buf.String()
	if !strings.Contains(out, "par01") || !strings.Contains(out, "Parent feature") {
		t.Errorf("expected breadcrumb with par01 + Parent feature, got:\n%s", out)
	}
}

func TestRenderClosedTail_BreadcrumbOmittedForSubtreeScope(t *testing.T) {
	rows := []ClosedTailRow{
		{Task: &Task{ShortID: "abc", Title: "fix bug", Status: "done", ParentID: new(int64(42))}, ClosedAt: 100},
	}
	parents := map[int64]ParentInfo{
		42: {ShortID: "par01", Title: "Parent feature"},
	}
	var buf bytes.Buffer
	RenderClosedTail(&buf, rows, 1, parents, true)
	out := buf.String()
	if strings.Contains(out, "par01") || strings.Contains(out, "Parent feature") {
		t.Errorf("expected no breadcrumb in subtree scope, got:\n%s", out)
	}
}

func TestRenderClosedTail_DoneAndCanceledGlyphs(t *testing.T) {
	rows := []ClosedTailRow{
		{Task: &Task{ShortID: "ddd", Title: "shipped", Status: "done"}, ClosedAt: 200},
		{Task: &Task{ShortID: "ccc", Title: "abandoned", Status: "canceled"}, ClosedAt: 100},
	}
	var buf bytes.Buffer
	RenderClosedTail(&buf, rows, 2, nil, true)
	out := buf.String()
	if !strings.Contains(out, "[x] `ddd`") {
		t.Errorf("expected '[x] `ddd`' for done, got:\n%s", out)
	}
	if !strings.Contains(out, "[-] `ccc`") {
		t.Errorf("expected '[-] `ccc`' for canceled, got:\n%s", out)
	}
}

func TestRenderClosedTail_NoBreadcrumbForRootTasks(t *testing.T) {
	rows := []ClosedTailRow{
		{Task: &Task{ShortID: "abc", Title: "root task", Status: "done"}, ClosedAt: 100},
	}
	var buf bytes.Buffer
	RenderClosedTail(&buf, rows, 1, map[int64]ParentInfo{}, false)
	out := buf.String()
	if strings.Contains(out, "(in") {
		t.Errorf("expected no '(in ...)' breadcrumb on root task, got:\n%s", out)
	}
}

//go:fix inline
func int64Ptr(v int64) *int64 { return new(v) }
