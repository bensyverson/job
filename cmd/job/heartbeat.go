package main

import (
	job "github.com/bensyverson/job/internal/job"
	"github.com/spf13/cobra"
)

func newHeartbeatCmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "heartbeat <id> [<id>...]",
		Short: "Extend your live claim(s) by 30 minutes",
		Long:  "Refresh one or more live claims held by the caller. Extends claim_expires_at by 30 minutes and emits a heartbeat event. All targets must currently be claimed by the caller; any other state errors and rolls back the whole call.",
		Args:  cobra.MinimumNArgs(1),
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

			results, err := job.RunHeartbeat(db, args, actor)
			if err != nil {
				return err
			}

			if format == "json" {
				return job.RenderHeartbeatJSON(cmd.OutOrStdout(), results)
			}
			job.RenderHeartbeatAck(cmd.OutOrStdout(), results)
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "md", "output format (md|json)")
	return cmd
}
