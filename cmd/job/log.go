package main

import (
	"fmt"
	job "github.com/bensyverson/job/internal/job"
	"github.com/spf13/cobra"
	"time"
)

func newLogCmd() *cobra.Command {
	var format string
	var since string
	cmd := &cobra.Command{
		Use:   "log [id|all]",
		Short: "Show event history for a task and its descendants (or all trees)",
		Long:  "Show event history for a task and its descendants. With no positional arg (or the literal 'all'), streams events from every top-level task in the database. Filters (--since) compose with the chosen scope.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			var sincePtr *int64
			if since != "" {
				ts, perr := time.Parse(time.RFC3339, since)
				if perr != nil {
					return fmt.Errorf("--since: invalid RFC3339 timestamp: %s", since)
				}
				u := ts.Unix()
				sincePtr = &u
			}

			shortID := ""
			if len(args) == 1 && args[0] != "all" {
				shortID = args[0]
			}

			events, err := job.RunLog(db, shortID, sincePtr)
			if err != nil {
				return err
			}

			if format == "json" {
				b, err := job.FormatEventLogJSON(events)
				if err != nil {
					return err
				}
				cmd.OutOrStdout().Write(b)
				fmt.Fprintln(cmd.OutOrStdout())
			} else {
				job.RenderEventLogMarkdown(cmd.OutOrStdout(), events)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "md", "output format (md|json)")
	cmd.Flags().StringVarP(&since, "since", "s", "", "only events at or after this RFC3339 timestamp")
	return cmd
}
