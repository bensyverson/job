package main

import (
	"fmt"
	job "github.com/bensyverson/job/internal/job"
	"github.com/spf13/cobra"
)

func newClaimCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "claim <id> [duration]",
		Short: "Claim a task",
		Long:  "Claim a task, marking it as in-progress. Duration defaults to 15m. Supported units: s, m, h, d. Use --force to override an existing claim.",
		Args:  cobra.RangeArgs(1, 2),
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

			shortID := args[0]
			var duration string
			if len(args) >= 2 {
				duration = args[1]
			}

			prevClaimedBy := ""
			if force {
				task, _ := job.GetTaskByShortID(db, shortID)
				if task != nil && task.Status == "claimed" && task.ClaimedBy != nil {
					prevClaimedBy = *task.ClaimedBy
				}
			}

			if err := job.RunClaim(db, shortID, duration, actor, force); err != nil {
				return err
			}

			durStr := job.FormatDuration(job.DefaultClaimTTLSeconds)
			if duration != "" {
				durStr = duration
			}

			if force && prevClaimedBy != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Claimed: %s (overrode previous claim by %s, expires in %s)\n", shortID, prevClaimedBy, durStr)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Claimed: %s (expires in %s)\n", shortID, durStr)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "override an existing claim")
	return cmd
}
