package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/bensyverson/jobs/internal/web/server"
	"github.com/spf13/cobra"
)

// portWalkAttempts is how many adjacent ports `job serve` walks when
// the default is in use and no --bind was passed. Twenty headroom
// covers "I left another instance running" without hiding the case
// where something has truly gone wrong.
const portWalkAttempts = 20

// resolveServeAddr applies the bind-flag default: empty → loopback.
// Kept as a seam so tests can assert the default-is-loopback rule
// without standing up a server.
func resolveServeAddr(bindFlag string) string {
	if bindFlag == "" {
		return server.DefaultAddr
	}
	return bindFlag
}

func newServeCmd() *cobra.Command {
	var bindFlag string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the read-only web dashboard",
		Long: "Run the Jobs web dashboard as a foreground process.\n\n" +
			"Binds 127.0.0.1:7823 by default — loopback only. Use --bind to pick a\n" +
			"different address; binding to all interfaces requires passing it\n" +
			"explicitly (for example, --bind 0.0.0.0:7823).\n\n" +
			"When the default port is already in use and --bind was not passed,\n" +
			"serve walks the next 20 ports upward and binds the first free one.\n" +
			"With an explicit --bind, a port collision fails loud — the user asked\n" +
			"for that port specifically.\n\n" +
			"The dashboard is read-only: no writes to the task store. Stop with Ctrl-C.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			cfg := server.Config{Addr: resolveServeAddr(bindFlag), DB: db}
			explicitBind := cmd.Flags().Changed("bind")

			ln, walked, err := bindForServe(cfg.Addr, explicitBind)
			if err != nil {
				return fmt.Errorf("bind %s: %w", cfg.Addr, err)
			}
			srv := server.New(ctx, cfg)

			if walked {
				fmt.Fprintf(cmd.OutOrStdout(),
					"Note: default %s in use; using %s instead.\n",
					cfg.Addr, ln.Addr())
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Jobs dashboard: http://%s/\n", ln.Addr())
			fmt.Fprintln(cmd.OutOrStdout(), "Press Ctrl-C to stop.")

			return server.Serve(ctx, srv, ln)
		},
	}
	cmd.Flags().StringVar(&bindFlag, "bind", "", "address to bind (default 127.0.0.1:7823)")
	return cmd
}

// bindForServe encapsulates the walk-vs-fail-loud policy. When the
// user accepted the default (--bind not passed), a port collision
// triggers ListenWalk; with an explicit --bind, a collision is fatal
// and the original error is returned. Returns the bound listener and
// a `walked` flag the caller uses to decide whether to log the
// "default was in use" hint.
func bindForServe(addr string, explicitBind bool) (net.Listener, bool, error) {
	if explicitBind {
		ln, err := net.Listen("tcp", addr)
		return ln, false, err
	}
	ln, err := server.ListenWalk(addr, portWalkAttempts)
	if err != nil {
		return nil, false, err
	}
	walked := ln.Addr().String() != addr
	return ln, walked, nil
}
