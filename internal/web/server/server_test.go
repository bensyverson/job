package server_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	job "github.com/bensyverson/jobs/internal/job"
	"github.com/bensyverson/jobs/internal/web/assets"
	"github.com/bensyverson/jobs/internal/web/server"
)

func TestListen_BindsToRequestedAddr(t *testing.T) {
	srv, ln, err := server.Listen(server.Config{Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer ln.Close()
	if srv == nil {
		t.Fatal("Listen: nil server")
	}
	tcp, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("Listen: addr %v is not TCP", ln.Addr())
	}
	if !tcp.IP.IsLoopback() {
		t.Errorf("Listen: want loopback, got %v", tcp.IP)
	}
	if tcp.Port == 0 {
		t.Error("Listen: want bound port, got 0")
	}
}

func TestListen_BindError(t *testing.T) {
	_, _, err := server.Listen(server.Config{Addr: "127.0.0.1:not-a-port"})
	if err == nil {
		t.Fatal("Listen: expected error for malformed addr, got nil")
	}
}

func TestServe_RespondsToRequests(t *testing.T) {
	srv, ln, err := server.Listen(server.Config{Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, srv, ln) }()

	url := "http://" + ln.Addr().String() + "/"
	resp, err := httpGet(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /: status %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("GET /: Content-Type %q, want text/html", ct)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Serve returned %v, want nil after context cancel", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return within 2s of context cancel")
	}
}

func TestServe_ShutsDownOnContextCancel(t *testing.T) {
	srv, ln, err := server.Listen(server.Config{Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, srv, ln) }()

	// Give Serve a moment to enter its select.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Serve: got %v, want nil on graceful shutdown", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not shut down within 2s")
	}
}

func httpGet(url string) (*http.Response, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// TestMux_FullRoutingMatrix spins up the real mux via NewMux and
// verifies the shape of the response for every route-class: root,
// content pages, unknown paths, fingerprinted static assets. It's
// the integration-level companion to the handler-package unit tests.
func TestMux_FullRoutingMatrix(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routing.db")
	db, err := job.CreateDB(path)
	if err != nil {
		t.Fatalf("CreateDB: %v", err)
	}
	defer db.Close()

	mux := server.NewMux(server.Config{DB: db})

	type check struct {
		name        string
		method      string
		path        string
		wantStatus  int
		wantHTML    bool
		containsAny []string
	}
	cases := []check{
		{
			name:        "root renders Home",
			method:      "GET",
			path:        "/",
			wantStatus:  200,
			wantHTML:    true,
			containsAny: []string{"Home · Jobs"},
		},
		{
			name:        "log view renders",
			method:      "GET",
			path:        "/log",
			wantStatus:  200,
			wantHTML:    true,
			containsAny: []string{"c-filter-bar", `class="c-log"`},
		},
		{
			name:        "unknown path returns templated 404",
			method:      "GET",
			path:        "/nope",
			wantStatus:  404,
			wantHTML:    true,
			containsAny: []string{"Error 404", "Page not found"},
		},
		{
			name:        "unknown task id returns templated 404",
			method:      "GET",
			path:        "/tasks/zzzzz",
			wantStatus:  404,
			wantHTML:    true,
			containsAny: []string{"Task not found"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(c.method, c.path, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != c.wantStatus {
				t.Errorf("%s %s: status %d, want %d", c.method, c.path, w.Code, c.wantStatus)
			}
			if c.wantHTML {
				ct := w.Header().Get("Content-Type")
				if !strings.HasPrefix(ct, "text/html") {
					t.Errorf("%s %s: Content-Type %q, want text/html", c.method, c.path, ct)
				}
			}
			body := w.Body.String()
			for _, needle := range c.containsAny {
				if !strings.Contains(body, needle) {
					t.Errorf("%s %s: body missing %q\n---\n%s", c.method, c.path, needle, body)
				}
			}
		})
	}
}

func TestMux_StaticAssetsServedWithImmutableCache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "static.db")
	db, err := job.CreateDB(path)
	if err != nil {
		t.Fatalf("CreateDB: %v", err)
	}
	defer db.Close()

	m, err := assets.BuildManifest()
	if err != nil {
		t.Fatalf("BuildManifest: %v", err)
	}
	tokensURL := m.URL("css/tokens.css")
	if tokensURL == "" {
		t.Fatal("manifest missing tokens.css")
	}

	mux := server.NewMux(server.Config{DB: db})

	req := httptest.NewRequest("GET", tokensURL, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("GET %s: status %d, want 200", tokensURL, w.Code)
	}
	if cc := w.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("GET %s: Cache-Control %q, want immutable", tokensURL, cc)
	}
}

// Compile-time check that DefaultAddr is loopback — a regression fence
// against someone quietly changing the default to 0.0.0.0.
func TestDefaultAddr_IsLoopback(t *testing.T) {
	host, _, err := net.SplitHostPort(server.DefaultAddr)
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", server.DefaultAddr, err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		t.Errorf("DefaultAddr host = %q, want a loopback IP", host)
	}
}
