package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newUnblockCmd is the legacy alias for `block remove`. It still works
// but emits a one-line stderr deprecation notice on every invocation.
func newUnblockCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unblock <blocked> from <blocker>",
		Short: "Remove a blocking relationship (alias for `block remove`)",
		Long:  "Manually remove a blocking relationship. Deprecated alias for `job block remove <blocked> by <blocker>` — emits a one-line notice on every call. Blocking relationships are also auto-removed when the blocker task is marked done.",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 3 || args[1] != "from" {
				return fmt.Errorf("usage: job unblock <blocked> from <blocker>")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.ErrOrStderr(), "note: `job unblock <blocked> from <blocker>` is an alias for `job block remove <blocked> by <blocker>`; prefer the canonical form.")
			return runBlockRemove(cmd, args[0], []string{args[2]})
		},
	}
}
