package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestTail_FormatJson_EmitsJSONLines(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "X")
	mustClaim(t, db, id, "1h")

	events, err := getEventsForTaskTree(db, id)
	if err != nil {
		t.Fatalf("getEventsForTaskTree: %v", err)
	}
	var buf bytes.Buffer
	if err := formatEventLogJSONLines(&buf, events); err != nil {
		t.Fatalf("format: %v", err)
	}

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != len(events) {
		t.Fatalf("got %d lines for %d events:\n%s", len(lines), len(events), buf.String())
	}
	for i, line := range lines {
		var got map[string]any
		if err := json.Unmarshal([]byte(line), &got); err != nil {
			t.Errorf("line %d not valid JSON: %v\n%s", i, err, line)
		}
	}
}

func TestTail_FormatJson_NoTrailingArray(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "X")

	events, err := getEventsForTaskTree(db, id)
	if err != nil {
		t.Fatalf("getEventsForTaskTree: %v", err)
	}
	var buf bytes.Buffer
	if err := formatEventLogJSONLines(&buf, events); err != nil {
		t.Fatalf("format: %v", err)
	}
	out := buf.String()
	if strings.HasPrefix(out, "[") {
		t.Errorf("JSON-lines should not be wrapped in []:\n%s", out)
	}
	if strings.Contains(out, "},\n{") {
		t.Errorf("should not have comma separators between objects:\n%s", out)
	}
}

func TestTail_Events_Filter(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "X")
	mustClaim(t, db, id, "1h")
	if _, _, err := runDone(db, []string{id}, false, "", nil, testActor); err != nil {
		t.Fatalf("done: %v", err)
	}

	events, err := getEventsForTaskTree(db, id)
	if err != nil {
		t.Fatalf("getEventsForTaskTree: %v", err)
	}
	filter := EventFilter{Types: parseFilterList("claimed,done")}
	out := filterEvents(events, filter)
	for _, e := range out {
		if e.EventType != "claimed" && e.EventType != "done" {
			t.Errorf("unexpected event type %q passed filter", e.EventType)
		}
	}
	if len(out) != 2 {
		t.Errorf("expected 2 filtered events, got %d", len(out))
	}
}

func TestTail_Users_Filter(t *testing.T) {
	db := setupTestDB(t)
	id, err := runAdd(db, "", "X", "", "", "alice")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := runAdd(db, "", "Y", "", "", "bob"); err != nil {
		t.Fatalf("add bob: %v", err)
	}

	events, err := getEventsForTaskTree(db, id)
	if err != nil {
		t.Fatalf("getEventsForTaskTree: %v", err)
	}
	filter := EventFilter{Users: parseFilterList("alice")}
	out := filterEvents(events, filter)
	for _, e := range out {
		if e.Actor != "alice" {
			t.Errorf("unexpected actor %q passed filter", e.Actor)
		}
	}
	if len(out) == 0 {
		t.Errorf("expected at least 1 alice event")
	}
}

func TestTail_Events_Intersection_Users(t *testing.T) {
	db := setupTestDB(t)
	id, err := runAdd(db, "", "X", "", "", "alice")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := runClaim(db, id, "1h", "bob", false); err != nil {
		t.Fatalf("claim: %v", err)
	}

	events, err := getEventsForTaskTree(db, id)
	if err != nil {
		t.Fatalf("getEventsForTaskTree: %v", err)
	}
	filter := EventFilter{
		Types: parseFilterList("claimed"),
		Users: parseFilterList("bob"),
	}
	out := filterEvents(events, filter)
	if len(out) != 1 {
		t.Fatalf("expected 1 event, got %d", len(out))
	}
	if out[0].EventType != "claimed" || out[0].Actor != "bob" {
		t.Errorf("got %+v", out[0])
	}
}

func TestTail_Hides_Heartbeat_Default(t *testing.T) {
	events := []EventEntry{
		{EventType: "heartbeat", Actor: "alice"},
		{EventType: "claimed", Actor: "alice"},
	}
	out := filterEvents(events, EventFilter{})
	if len(out) != 1 || out[0].EventType != "claimed" {
		t.Errorf("default should hide heartbeat: %+v", out)
	}
}

func TestTail_Events_Heartbeat_OptIn(t *testing.T) {
	events := []EventEntry{
		{EventType: "heartbeat", Actor: "alice"},
		{EventType: "claimed", Actor: "alice"},
	}
	out := filterEvents(events, EventFilter{Types: parseFilterList("heartbeat")})
	if len(out) != 1 || out[0].EventType != "heartbeat" {
		t.Errorf("--events heartbeat should opt in: %+v", out)
	}
}

func TestTail_UntilClose_AlreadyDone_ExitsImmediately(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")
	mustDone(t, db, id)

	var buf bytes.Buffer
	ctx := context.Background()
	err := runTailUntilClose(ctx, db, id, []string{id}, 0, 20*time.Millisecond, false, "md", EventFilter{}, &buf)
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	want := "Closed: " + id + " (already done)\n"
	if buf.String() != want {
		t.Errorf("got %q, want %q", buf.String(), want)
	}
}

func TestTail_UntilClose_AlreadyCanceled_ExitsImmediately(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")
	if _, _, _, err := runCancel(db, []string{id}, "nope", false, false, false, "alice"); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	var buf bytes.Buffer
	err := runTailUntilClose(context.Background(), db, id, []string{id}, 0, 20*time.Millisecond, false, "md", EventFilter{}, &buf)
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	want := "Closed: " + id + " (already canceled)\n"
	if buf.String() != want {
		t.Errorf("got %q, want %q", buf.String(), want)
	}
}

func TestTail_UntilClose_BlocksUntilDone(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	var buf bytes.Buffer
	var mu sync.Mutex
	var sw safeWriter
	sw.buf = &buf
	sw.mu = &mu

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- runTailUntilClose(ctx, db, id, []string{id}, 0, 20*time.Millisecond, true, "md", EventFilter{}, &sw)
	}()

	// Give goroutine a moment to enter the poll loop.
	time.Sleep(100 * time.Millisecond)
	if _, _, err := runDone(db, []string{id}, false, "", nil, "alice"); err != nil {
		t.Fatalf("done: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runTailUntilClose: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for runTailUntilClose to return")
	}

	mu.Lock()
	out := buf.String()
	mu.Unlock()
	if !strings.Contains(out, "Closed: "+id+" (done)") {
		t.Errorf("missing close line:\n%s", out)
	}
}

func TestTail_UntilClose_BlocksUntilCanceled(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	var buf bytes.Buffer
	var mu sync.Mutex
	sw := safeWriter{buf: &buf, mu: &mu}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- runTailUntilClose(ctx, db, id, []string{id}, 0, 20*time.Millisecond, true, "md", EventFilter{}, &sw)
	}()

	time.Sleep(100 * time.Millisecond)
	if _, _, _, err := runCancel(db, []string{id}, "nope", false, false, false, "alice"); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runTailUntilClose: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}

	mu.Lock()
	out := buf.String()
	mu.Unlock()
	if !strings.Contains(out, "Closed: "+id+" (canceled)") {
		t.Errorf("missing close line:\n%s", out)
	}
}

func TestTail_UntilClose_Multi_Conjunction(t *testing.T) {
	db := setupTestDB(t)
	a := mustAdd(t, db, "", "A")
	b := mustAdd(t, db, "", "B")
	c := mustAdd(t, db, "", "C")

	var buf bytes.Buffer
	var mu sync.Mutex
	sw := safeWriter{buf: &buf, mu: &mu}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- runTailUntilClose(ctx, db, a, []string{a, b, c}, 0, 20*time.Millisecond, true, "md", EventFilter{}, &sw)
	}()

	time.Sleep(100 * time.Millisecond)
	if _, _, err := runDone(db, []string{a}, false, "", nil, "alice"); err != nil {
		t.Fatalf("done a: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if _, _, err := runDone(db, []string{b}, false, "", nil, "alice"); err != nil {
		t.Fatalf("done b: %v", err)
	}

	// After 1.5s, c is still open - still blocking.
	select {
	case err := <-done:
		t.Fatalf("runTailUntilClose returned early with err=%v", err)
	case <-time.After(500 * time.Millisecond):
	}

	if _, _, err := runDone(db, []string{c}, false, "", nil, "alice"); err != nil {
		t.Fatalf("done c: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runTailUntilClose: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestTail_UntilClose_Quiet_SuppressesStream(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	var buf bytes.Buffer
	var mu sync.Mutex
	sw := safeWriter{buf: &buf, mu: &mu}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- runTailUntilClose(ctx, db, id, []string{id}, 0, 20*time.Millisecond, true, "md", EventFilter{}, &sw)
	}()

	time.Sleep(100 * time.Millisecond)
	// Fire a non-terminal event.
	if err := runClaim(db, id, "1h", "alice", false); err != nil {
		t.Fatalf("claim: %v", err)
	}
	time.Sleep(1200 * time.Millisecond)
	if _, _, err := runDone(db, []string{id}, false, "", nil, "alice"); err != nil {
		t.Fatalf("done: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runTailUntilClose: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
	mu.Lock()
	out := buf.String()
	mu.Unlock()

	if strings.Contains(out, "claimed") {
		t.Errorf("quiet mode should suppress claim event:\n%s", out)
	}
	if strings.Contains(out, "Tailing events for") {
		t.Errorf("quiet mode should suppress preamble:\n%s", out)
	}
	if !strings.Contains(out, "Closed: "+id+" (done)") {
		t.Errorf("should still emit close line:\n%s", out)
	}
}

func TestTail_UntilClose_EventsFilter_DoesNotHideTermination(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	var buf bytes.Buffer
	var mu sync.Mutex
	sw := safeWriter{buf: &buf, mu: &mu}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := EventFilter{Types: parseFilterList("claimed")}

	done := make(chan error, 1)
	go func() {
		done <- runTailUntilClose(ctx, db, id, []string{id}, 0, 20*time.Millisecond, false, "md", filter, &sw)
	}()

	time.Sleep(100 * time.Millisecond)
	if _, _, err := runDone(db, []string{id}, false, "", nil, "alice"); err != nil {
		t.Fatalf("done: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runTailUntilClose: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout; filter should not block terminal detection")
	}

	mu.Lock()
	out := buf.String()
	mu.Unlock()
	if !strings.Contains(out, "Closed: "+id+" (done)") {
		t.Errorf("missing close line:\n%s", out)
	}
}

func TestTail_Timeout_FiresAndExitCode2(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	var buf bytes.Buffer
	ctx := context.Background()
	err := runTailUntilClose(ctx, db, id, []string{id}, 100*time.Millisecond, 20*time.Millisecond, true, "md", EventFilter{}, &buf)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errTailTimeout) {
		t.Errorf("errors.Is(err, errTailTimeout) = false: %v", err)
	}
	if !strings.Contains(buf.String(), "Timeout:") {
		t.Errorf("missing Timeout summary line:\n%s", buf.String())
	}
}

func TestTail_Timeout_NotFired_ExitsCleanlyOnClose(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	var buf bytes.Buffer
	var mu sync.Mutex
	sw := safeWriter{buf: &buf, mu: &mu}

	done := make(chan error, 1)
	go func() {
		done <- runTailUntilClose(context.Background(), db, id, []string{id}, 3*time.Second, 20*time.Millisecond, true, "md", EventFilter{}, &sw)
	}()
	time.Sleep(100 * time.Millisecond)
	if _, _, err := runDone(db, []string{id}, false, "", nil, "alice"); err != nil {
		t.Fatalf("done: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected nil err, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestTail_Timeout_QuietJSON_ShapeOnExpiry(t *testing.T) {
	db := setupTestDB(t)
	id := mustAdd(t, db, "", "Task")

	var buf bytes.Buffer
	err := runTailUntilClose(context.Background(), db, id, []string{id}, 100*time.Millisecond, 20*time.Millisecond, true, "json", EventFilter{}, &buf)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errTailTimeout) {
		t.Errorf("errors.Is(err, errTailTimeout) = false: %v", err)
	}
	out := strings.TrimSpace(buf.String())
	if out == "" {
		t.Fatal("expected JSON output")
	}
	lines := strings.Split(out, "\n")
	var got map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &got); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, out)
	}
	if got["timeout"] != true {
		t.Errorf("timeout field: %v", got["timeout"])
	}
	arr, ok := got["still_open"].([]any)
	if !ok || len(arr) != 1 || arr[0] != id {
		t.Errorf("still_open: %v", got["still_open"])
	}
}

// safeWriter is a mutex-guarded bytes.Buffer used by the tail tests so the
// test goroutine and the tail goroutine don't race on the buffer.
type safeWriter struct {
	buf *bytes.Buffer
	mu  *sync.Mutex
}

func (s *safeWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}
