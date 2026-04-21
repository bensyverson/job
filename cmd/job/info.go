package main

import (
	"fmt"
	job "github.com/bensyverson/job/internal/job"
	"github.com/spf13/cobra"
)

func newInfoCmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "info <id>",
		Short: "Show full details of a task",
		Long:  "Show ID, title, description, status, claim info, blockers, children summary, and creation time. Use --format=json for machine-readable output.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			info, err := job.RunInfo(db, args[0])
			if err != nil {
				return err
			}

			if format == "json" {
				job.RenderInfoJSON(cmd.OutOrStdout(), info)
				fmt.Fprintln(cmd.OutOrStdout())
			} else {
				job.RenderInfoMarkdown(cmd.OutOrStdout(), info)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "md", "output format (md|json)")
	return cmd
}
