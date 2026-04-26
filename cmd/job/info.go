package main

import (
	"fmt"
	job "github.com/bensyverson/jobs/internal/job"
	"github.com/spf13/cobra"
)

func newInfoCmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:     "show <id> [id ...]",
		Aliases: []string{"info"},
		Short:   "Show full details of one or more tasks",
		Long:    "Show ID, title, description, status, claim info, blockers, children summary, and creation time. Accepts multiple IDs; tasks are separated by a blank line. Use --format=json for machine-readable output (returns a JSON array).",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			out := cmd.OutOrStdout()

			if format == "json" {
				fmt.Fprint(out, "[")
				for i, id := range args {
					info, err := job.RunInfo(db, id)
					if err != nil {
						return err
					}
					if i > 0 {
						fmt.Fprint(out, ",")
					}
					job.RenderInfoJSON(out, info)
				}
				fmt.Fprintln(out, "]")
				return nil
			}

			for i, id := range args {
				info, err := job.RunInfo(db, id)
				if err != nil {
					return err
				}
				if i > 0 {
					fmt.Fprintln(out)
				}
				job.RenderInfoMarkdown(out, info)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "md", "output format (md|json)")
	return cmd
}
