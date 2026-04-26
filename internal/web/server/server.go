// Package server wires the Jobs web dashboard's http.Server: config,
// lifecycle, and the top-level route table. Handlers themselves live in
// sibling package [github.com/bensyverson/jobs/internal/web/handlers].
package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"syscall"
	"time"
)

// DefaultAddr is the loopback bind address documented in the vision doc.
const DefaultAddr = "127.0.0.1:7823"

// Config captures everything the web server needs at startup.
type Config struct {
	Addr string
	DB   *sql.DB
}

// New constructs an *http.Server with the dashboard routes mounted but
// no listener attached. The ctx governs background goroutines started
// inside the mux (notably the broadcaster's poll loop); canceling it
// stops them. Use [Listen] to bind a port and [Serve] to run.
func New(ctx context.Context, cfg Config) *http.Server {
	return &http.Server{
		Addr:              cfg.Addr,
		Handler:           NewMux(ctx, cfg),
		ReadHeaderTimeout: 5 * time.Second,
	}
}

// Listen builds the server and binds a listener to cfg.Addr. The ctx
// governs background goroutines started inside the mux. Splitting
// this out from [Serve] lets callers (notably `job serve`) report the
// bound address — including the OS-assigned port when Addr ends in ":0"
// — before entering the blocking serve loop, and surfaces bind errors
// before any "server is up" messaging.
func Listen(ctx context.Context, cfg Config) (*http.Server, net.Listener, error) {
	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return nil, nil, err
	}
	return New(ctx, cfg), ln, nil
}

// Serve runs srv on ln until ctx is canceled, then performs a graceful
// shutdown. Returns nil on clean shutdown; any other error from Serve
// or Shutdown is returned as-is.
func Serve(ctx context.Context, srv *http.Server, ln net.Listener) error {
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

// Run is the [Listen] + [Serve] convenience for callers that don't
// need the bound address.
func Run(ctx context.Context, cfg Config) error {
	srv, ln, err := Listen(ctx, cfg)
	if err != nil {
		return err
	}
	return Serve(ctx, srv, ln)
}

// ListenWalk binds to addr; if the port is already in use, walks the
// port number upward by one and tries again, up to maxAttempts times
// total. Returns the first successful listener. Non-EADDRINUSE errors
// (malformed addr, missing host, etc.) are surfaced immediately
// without walking.
//
// Used by `job serve` so a stale dashboard on the default port doesn't
// block a fresh `job serve` invocation. The CLI only walks when the
// user accepted the default; an explicit --bind fails loud on conflict.
func ListenWalk(addr string, maxAttempts int) (net.Listener, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	startPort, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("port %q: %w", portStr, err)
	}
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	var lastErr error
	for i := 0; i < maxAttempts; i++ {
		cand := net.JoinHostPort(host, strconv.Itoa(startPort+i))
		ln, err := net.Listen("tcp", cand)
		if err == nil {
			return ln, nil
		}
		if !errors.Is(err, syscall.EADDRINUSE) {
			return nil, err
		}
		lastErr = err
	}
	return nil, fmt.Errorf("no free port in %d attempts starting at %s: %w", maxAttempts, addr, lastErr)
}
