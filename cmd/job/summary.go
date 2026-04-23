package main

import (
	job "github.com/bensyverson/job/internal/job"
	"github.com/spf13/cobra"
)

func newSummaryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "summary <id>",
		Short: "Two-level rollup of a task and its direct children",
		Long:  "Print a two-level rollup: the target's overall progress (done/blocked/available/in-flight counts) plus one rollup line per direct child. Fills the gap between `status` (whole DB) and `list ... all` (full subtree). Works against any task. No --as required.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()
			s, err := job.RunSummary(db, args[0])
			if err != nil {
				return err
			}
			job.RenderSummary(cmd.OutOrStdout(), s)
			return nil
		},
	}
}
