package main

import (
	"fmt"
	job "github.com/bensyverson/job/internal/job"
	"github.com/spf13/cobra"
)

func newUnblockCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unblock <blocked> from <blocker>",
		Short: "Remove a blocking relationship",
		Long:  "Manually remove a blocking relationship. Blocking relationships are also auto-removed when the blocker task is marked done.",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			actor, err := requireAs(db)
			if err != nil {
				return err
			}

			if args[1] != "from" {
				return fmt.Errorf("usage: job unblock <blocked> from <blocker>")
			}

			if err := job.RunUnblock(db, args[0], args[2], actor); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Unblocked: %s (was blocked by %s)\n", args[0], args[2])
			return nil
		},
	}
	return cmd
}
