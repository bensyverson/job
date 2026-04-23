package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/bensyverson/jobs/internal/web/server"
	"github.com/spf13/cobra"
)

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
			"The dashboard is read-only: no writes to the task store. Stop with Ctrl-C.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			cfg := server.Config{Addr: resolveServeAddr(bindFlag), DB: db}
			srv, ln, err := server.Listen(cfg)
			if err != nil {
				return fmt.Errorf("bind %s: %w", cfg.Addr, err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Jobs dashboard: http://%s/\n", ln.Addr())
			fmt.Fprintln(cmd.OutOrStdout(), "Press Ctrl-C to stop.")

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return server.Serve(ctx, srv, ln)
		},
	}
	cmd.Flags().StringVar(&bindFlag, "bind", "", "address to bind (default 127.0.0.1:7823)")
	return cmd
}
