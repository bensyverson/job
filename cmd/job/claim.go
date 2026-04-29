package main

import (
	"database/sql"
	"fmt"

	job "github.com/bensyverson/jobs/internal/job"
	"github.com/spf13/cobra"
)

func newClaimCmd() *cobra.Command {
	var force bool
	var quiet bool
	var next bool
	var includeParents bool
	var format string
	cmd := &cobra.Command{
		Use:   "claim <id> [duration]",
		Short: "Claim a task (duration optional, default 30m)",
		Long: `Claim a task, marking it as in-progress. Duration defaults to 30m. Supported units: s, m, h, d. Use --force to override an existing claim.

The ack's first line is the scriptable signal ("Claimed: <id> '<title>' (expires in <dur>) as=<actor>") followed by the full 'show <id>' briefing — claiming is the moment you want every detail you'd otherwise have to fetch with a follow-up 'show'.

Long-running claim example: ` + "`job claim abc12 2h`" + ` extends the default 30m TTL to 2 hours for work that genuinely needs the lock held longer than a typical session.

Tip: use 'job claim --next [parent] [duration]' to find and claim the next available leaf in one step, and 'job done <id> --claim-next' for the close-and-advance loop ('done <id>' followed atomically by claiming the next leaf).`,
		Args: cobra.RangeArgs(0, 2),
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

			if next {
				return runClaimNext(cmd, db, args, actor, force, includeParents, quiet, format)
			}

			if len(args) < 1 {
				return fmt.Errorf("claim requires <id>, or pass --next [parent] to pick the next available leaf")
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
				fmt.Fprintf(cmd.OutOrStdout(), "Claimed: %s %q (overrode previous claim by %s, expires in %s) as=%s\n", shortID, title, prevClaimedBy, durStr, actor)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Claimed: %s %q (expires in %s) as=%s\n", shortID, title, durStr, actor)
			}

			if pre != nil {
				if hint := autoCloseCascadeHint(db, pre); hint != "" {
					fmt.Fprintln(cmd.OutOrStdout(), hint)
				}
			}

			if !quiet {
				renderClaimBriefing(cmd.OutOrStdout(), db, shortID)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "override an existing claim")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "suppress the trailing show briefing — keep only the one-line confirm")
	cmd.Flags().BoolVar(&next, "next", false, "find and claim the next available leaf (replaces standalone `claim-next`)")
	cmd.Flags().BoolVar(&includeParents, "include-parents", false, "with --next: permit claiming tasks with open children")
	cmd.Flags().StringVar(&format, "format", "md", "with --next: output format (md|json)")
	return cmd
}

// runClaimNext is the body of `claim --next [parent] [duration]` — folds
// the prior standalone `claim-next` into `claim` per pVUoZ.
func runClaimNext(cmd *cobra.Command, db *sql.DB, args []string, actor string, force, includeParents, quiet bool, format string) error {
	var parentShortID, duration string
	if len(args) > 0 {
		if job.IsDuration(args[0]) {
			duration = args[0]
		} else {
			parentShortID = args[0]
			if len(args) > 1 {
				duration = args[1]
			}
		}
	}
	task, err := job.RunClaimNextFiltered(db, parentShortID, duration, actor, force, includeParents)
	if err != nil {
		return err
	}
	if format == "json" {
		job.RenderTaskJSON(cmd.OutOrStdout(), task)
		fmt.Fprintln(cmd.OutOrStdout())
		return nil
	}
	durStr := job.FormatDuration(job.DefaultClaimTTLSeconds)
	if duration != "" {
		durStr = duration
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Claimed: %s %q (expires in %s) as=%s\n", task.ShortID, task.Title, durStr, actor)
	if !quiet {
		renderClaimBriefing(cmd.OutOrStdout(), db, task.ShortID)
	}
	return nil
}

// autoCloseCascadeHint returns a one-line hint when claiming `task` would,
// on close, cascade-close the parent (because `task` is the last
// not-yet-done child of its parent).
func autoCloseCascadeHint(db *sql.DB, task *job.Task) string {
	if task.ParentID == nil {
		return ""
	}
	var openSiblings int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM tasks
		 WHERE parent_id = ? AND status NOT IN ('done', 'canceled') AND deleted_at IS NULL AND id != ?`,
		*task.ParentID, task.ID,
	).Scan(&openSiblings); err != nil {
		return ""
	}
	if openSiblings != 0 {
		return ""
	}
	var parentShort string
	if err := db.QueryRow("SELECT short_id FROM tasks WHERE id = ?", *task.ParentID).Scan(&parentShort); err != nil {
		return ""
	}
	return fmt.Sprintf("  Closing this task will auto-close parent %s. Verify scope first.", parentShort)
}
