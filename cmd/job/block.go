package main

import (
	"fmt"
	job "github.com/bensyverson/job/internal/job"
	"github.com/spf13/cobra"
)

func newBlockCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "block <blocked> by <blocker>",
		Short: "Block a task until another is complete",
		Long:  "Declare that the blocked task cannot proceed until the blocker task is done. Circular dependencies are detected and rejected.",
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

			if args[1] != "by" {
				return fmt.Errorf("usage: job block <blocked> by <blocker>")
			}

			if err := job.RunBlock(db, args[0], args[2], actor); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Blocked: %s (blocked by %s)\n", args[0], args[2])
			return nil
		},
	}
	return cmd
}
