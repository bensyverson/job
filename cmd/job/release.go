package main

import (
	"fmt"
	job "github.com/bensyverson/jobs/internal/job"
	"github.com/spf13/cobra"
)

func newReleaseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "release <id>",
		Short: "Release a claimed task",
		Long:  "Release a claim, returning the task to available status.",
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

			if err := job.RunRelease(db, args[0], actor); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Released: %s\n", args[0])
			return nil
		},
	}
	return cmd
}
