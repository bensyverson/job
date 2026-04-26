package main

import (
	"net"
	"strconv"
	"testing"

	"github.com/bensyverson/jobs/internal/web/server"
)

func TestResolveServeAddr_DefaultsToLoopback(t *testing.T) {
	got := resolveServeAddr("")
	if got != server.DefaultAddr {
		t.Errorf("resolveServeAddr(\"\") = %q, want %q", got, server.DefaultAddr)
	}
}

func TestResolveServeAddr_FlagOverrides(t *testing.T) {
	got := resolveServeAddr("127.0.0.1:9090")
	if got != "127.0.0.1:9090" {
		t.Errorf("resolveServeAddr: got %q, want 127.0.0.1:9090", got)
	}
}

func TestResolveServeAddr_AllInterfacesOnlyViaExplicitFlag(t *testing.T) {
	// When the user passes --bind 0.0.0.0:N, we trust them. The regression
	// fence is that the *default* (empty flag) never resolves to 0.0.0.0.
	got := resolveServeAddr("0.0.0.0:7823")
	if got != "0.0.0.0:7823" {
		t.Errorf("resolveServeAddr(\"0.0.0.0:7823\") = %q, want it passed through unchanged", got)
	}
	def := resolveServeAddr("")
	if def == "0.0.0.0:7823" {
		t.Errorf("resolveServeAddr(\"\") = %q — default must never bind 0.0.0.0", def)
	}
}

func TestBindForServe_DefaultBindWalksPastInUsePort(t *testing.T) {
	// Pre-bind a free port; bindForServe with explicitBind=false must
	// walk to the next port and report walked=true.
	first, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("seed listen: %v", err)
	}
	defer first.Close()
	port := first.Addr().(*net.TCPAddr).Port
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))

	ln, walked, err := bindForServe(addr, false)
	if err != nil {
		t.Fatalf("bindForServe: %v", err)
	}
	defer ln.Close()
	if !walked {
		t.Error("walked = false, want true (port was in use, walk should have advanced)")
	}
	got := ln.Addr().(*net.TCPAddr).Port
	if got == port {
		t.Errorf("bound port = %d (== seed); walk did not advance", got)
	}
}

func TestBindForServe_ExplicitBindFailsLoudOnCollision(t *testing.T) {
	// With explicitBind=true, a port collision must surface the error
	// rather than walk silently — the user asked for that exact port.
	first, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("seed listen: %v", err)
	}
	defer first.Close()
	port := first.Addr().(*net.TCPAddr).Port
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))

	if _, _, err := bindForServe(addr, true); err == nil {
		t.Fatalf("bindForServe(%q, explicitBind=true): expected EADDRINUSE, got nil", addr)
	}
}

func TestBindForServe_DefaultBindHappyPath(t *testing.T) {
	// A free port: walked=false because we landed on the first try.
	tmp, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	port := tmp.Addr().(*net.TCPAddr).Port
	if err := tmp.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))

	ln, walked, err := bindForServe(addr, false)
	if err != nil {
		t.Fatalf("bindForServe: %v", err)
	}
	defer ln.Close()
	if walked {
		t.Errorf("walked = true on free port; want false")
	}
	if got := ln.Addr().(*net.TCPAddr).Port; got != port {
		t.Errorf("bound port = %d, want %d", got, port)
	}
}

// Compile-time check: server.DefaultAddr is referenced so the test
// keeps a hard dependency on the constant. Catches accidental rename
// or move (the walk policy hinges on knowing the default).
var _ = server.DefaultAddr
