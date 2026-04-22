package main

import (
	job "github.com/bensyverson/job/internal/job"
	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show a one-line summary of task counts and recent activity",
		Long:  "Print a one-line summary: claimed / open / done, plus the time since the last event. With --as, the claimed count is scoped to the caller; without --as, it counts all live claims. The claimed term is suppressed entirely when zero. No --as required.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			s, err := job.RunStatus(db, asFlag)
			if err != nil {
				return err
			}
			job.RenderStatus(cmd.OutOrStdout(), s)
			return nil
		},
	}
	return cmd
}
