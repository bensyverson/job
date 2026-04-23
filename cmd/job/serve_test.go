package main

import (
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
