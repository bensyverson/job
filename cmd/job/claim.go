package main

import (
	"fmt"
	job "github.com/bensyverson/jobs/internal/job"
	"github.com/spf13/cobra"
)

func newClaimCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "claim <id> [duration]",
		Short: "Claim a task (duration optional, default 30m)",
		Long: `Claim a task, marking it as in-progress. Duration defaults to 30m. Supported units: s, m, h, d. Use --force to override an existing claim.

The ack's first line is the scriptable signal ("Claimed: <id> '<title>'
(expires in <dur>)") followed by the full 'show <id>' briefing — claiming
is the moment you want every detail you'd otherwise have to fetch with a
follow-up 'show'.

Tip: use 'job claim-next [parent] [duration]' to find and claim the next
available leaf in one step, and 'job done <id> --claim-next' for the
close-and-advance loop ('done <id>' followed atomically by claiming the
next leaf).`,
		Args: cobra.RangeArgs(1, 2),
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

			pre, _ := job.GetTaskByShortID(db, shortID)
			prevClaimedBy := ""
			if force && pre != nil && pre.Status == "claimed" && pre.ClaimedBy != nil {
				prevClaimedBy = *pre.ClaimedBy
			}

			if err := job.RunClaim(db, shortID, duration, actor, force); err != nil {
				return err
			}

			title := ""
			if pre != nil {
				title = pre.Title
			}

			durStr := job.FormatDuration(job.DefaultClaimTTLSeconds)
			if duration != "" {
				durStr = duration
			}

			if force && prevClaimedBy != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Claimed: %s %q (overrode previous claim by %s, expires in %s)\n", shortID, title, prevClaimedBy, durStr)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Claimed: %s %q (expires in %s)\n", shortID, title, durStr)
			}
			renderClaimBriefing(cmd.OutOrStdout(), db, shortID)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "override an existing claim")
	return cmd
}
