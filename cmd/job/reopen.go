package main

import (
	"fmt"
	job "github.com/bensyverson/jobs/internal/job"
	"github.com/spf13/cobra"
)

func newReopenCmd() *cobra.Command {
	var cascade bool
	cmd := &cobra.Command{
		Use:   "reopen <id>",
		Short: "Reopen a completed or canceled task",
		Long:  "Reopen a completed or canceled task, setting it back to available. Use --cascade to also reopen all done/canceled descendants.",
		Args:  cobra.ExactArgs(1),
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

			reopened, err := job.RunReopen(db, args[0], cascade, actor)
			if err != nil {
				return err
			}

			if len(reopened) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Reopened: %s (and %d subtasks)\n", args[0], len(reopened))
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Reopened: %s\n", args[0])
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&cascade, "cascade", false, "also reopen all done descendants")
	return cmd
}
