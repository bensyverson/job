package main

import (
	job "github.com/bensyverson/job/internal/job"
	"github.com/spf13/cobra"
)

func newLabelAddCmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "add <id> <name> [<name>...]",
		Short: "Add one or more labels to a task",
		Long:  "Add one or more labels to a task. Idempotent: names that are already attached are reported, not re-recorded. Emits a single 'labeled' event per call when at least one new label is added.",
		Args:  cobra.MinimumNArgs(2),
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

			res, err := job.RunLabelAdd(db, args[0], args[1:], actor)
			if err != nil {
				return err
			}

			if format == "json" {
				return job.RenderLabelJSON(cmd.OutOrStdout(), res)
			}
			job.RenderLabelAck(cmd.OutOrStdout(), res)
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "md", "output format (md|json)")
	return cmd
}
