package main

import (
	"fmt"

	job "github.com/bensyverson/job/internal/job"
	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "status [id]",
		Aliases: []string{"summary"},
		Short:   "Show a session preamble and work landscape, or a single subtree",
		Long:    "Without an argument, prints a session preamble (claimed/open/done counts, time since last event, identity) followed by the forest-level rollup — one row per top-level task with its own subtree counts. With an id, scopes the renderer to the subtree rooted at that task and skips the session preamble (the preamble is DB-wide metadata and doesn't belong on a subtree view). `job summary [id]` is a deprecated alias and emits a stderr notice on every call. No --as required.",
		Args:    cobra.MaximumNArgs(1),
		PreRun: func(cmd *cobra.Command, args []string) {
			if cmd.CalledAs() == "summary" {
				fmt.Fprintln(cmd.ErrOrStderr(), "note: `job summary` is a deprecated alias for `job status`; prefer the canonical form.")
			}
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			out := cmd.OutOrStdout()

			var target *job.Task
			if len(args) == 1 {
				target, err = job.GetTaskByShortID(db, args[0])
				if err != nil {
					return err
				}
				if target == nil {
					return fmt.Errorf("task %q not found", args[0])
				}
			}

			// Snapshot stale claims BEFORE running anything that
			// auto-expires them (RunStatus below, plus any downstream
			// code paths). Without this, RunStatus clears stale claims
			// as a side effect and there's nothing left to surface.
			var scopeID *int64
			if target != nil {
				scopeID = &target.ID
			}
			stales, err := job.FindStaleClaims(db, scopeID)
			if err != nil {
				return err
			}

			if target != nil {
				rollup, err := job.BuildRollup(db, target)
				if err != nil {
					return err
				}
				job.RenderSummary(out, rollup)
			} else {
				s, err := job.RunStatus(db, asFlag)
				if err != nil {
					return err
				}
				job.RenderStatus(out, s)

				rollup, err := job.BuildRollup(db, nil)
				if err != nil {
					return err
				}
				if len(rollup.DirectChildren) > 0 {
					fmt.Fprintln(out)
					job.RenderSummary(out, rollup)
				}
			}

			if len(stales) > 0 {
				fmt.Fprintln(out)
				job.RenderStaleClaims(out, stales)
			}
			return nil
		},
	}
	return cmd
}
