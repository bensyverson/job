package main

import (
	job "github.com/bensyverson/jobs/internal/job"
	"github.com/spf13/cobra"
)

func newLabelRemoveCmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "remove <id> <name> [<name>...]",
		Short: "Remove one or more labels from a task",
		Long:  "Remove one or more labels from a task. Idempotent: names that are not present are reported, not re-recorded. Emits a single 'unlabeled' event per call when at least one label is actually removed.",
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

			res, err := job.RunLabelRemove(db, args[0], args[1:], actor)
			if err != nil {
				return err
			}

			if format == "json" {
				return job.RenderUnlabelJSON(cmd.OutOrStdout(), res)
			}
			job.RenderUnlabelAck(cmd.OutOrStdout(), res)
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "md", "output format (md|json)")
	return cmd
}
