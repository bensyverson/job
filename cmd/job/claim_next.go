package main

import (
	"fmt"
	job "github.com/bensyverson/jobs/internal/job"
	"github.com/spf13/cobra"
)

func newClaimNextCmd() *cobra.Command {
	var force bool
	var format string
	var includeParents bool
	cmd := &cobra.Command{
		Use:   "claim-next [parent] [duration]",
		Short: "Find and claim the next available task",
		Long:  "Find the next available task and claim it in one step. By default only leaves (tasks with no open children) are claimable — the search descends through parents to find work. Pass --include-parents to permit claiming any available task. Duration defaults to 30m. Supported units: s, m, h, d. Without a parent, searches the entire tree.",
		Args:  cobra.MaximumNArgs(2),
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

			var parentShortID, duration string
			if len(args) == 0 {
				parentShortID, duration = "", ""
			} else if job.IsDuration(args[0]) {
				duration = args[0]
			} else {
				parentShortID = args[0]
				if len(args) > 1 {
					duration = args[1]
				}
			}

			task, err := job.RunClaimNextFiltered(db, parentShortID, duration, actor, force, includeParents)
			if err != nil {
				return err
			}

			if format == "json" {
				job.RenderTaskJSON(cmd.OutOrStdout(), task)
				fmt.Fprintln(cmd.OutOrStdout())
			} else {
				durStr := job.FormatDuration(job.DefaultClaimTTLSeconds)
				if duration != "" {
					durStr = duration
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Claimed: %s %q (expires in %s)\n", task.ShortID, task.Title, durStr)
				if task.Description != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "\n  %s\n", task.Description)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "override an existing claim")
	cmd.Flags().StringVar(&format, "format", "md", "output format (md|json)")
	cmd.Flags().BoolVar(&includeParents, "include-parents", false, "permit claiming tasks with open children (legacy behavior)")
	return cmd
}
