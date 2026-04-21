package main

import (
	"fmt"
	job "github.com/bensyverson/job/internal/job"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var format string
	var labelFilter string
	cmd := &cobra.Command{
		Use:   "list [parent] [all]",
		Short: "List tasks",
		Long:  "List tasks. By default shows only actionable (available, unblocked, unclaimed) tasks. Use 'all' to include done, claimed, and blocked tasks. Use --label <name> to filter to tasks carrying that label. Use --format=json for machine-readable output.",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			var parentShortID string
			showAll := false
			for _, arg := range args {
				if arg == "all" {
					showAll = true
				} else {
					parentShortID = arg
				}
			}

			nodes, err := job.RunListFiltered(db, parentShortID, "", showAll, labelFilter)
			if err != nil {
				return err
			}

			if format == "json" {
				blockers, err := job.CollectBlockers(db, nodes)
				if err != nil {
					return err
				}
				_ = blockers
				b, err := job.FormatTaskNodesJSON(nodes)
				if err != nil {
					return err
				}
				cmd.OutOrStdout().Write(b)
				fmt.Fprintln(cmd.OutOrStdout())
			} else {
				if len(nodes) == 0 {
					total, done, cerr := countTasks(db)
					if cerr != nil {
						return cerr
					}
					job.RenderListEmpty(cmd.OutOrStdout(), total, done)
					return nil
				}
				blockers, err := job.CollectBlockers(db, nodes)
				if err != nil {
					return err
				}
				labels, err := collectLabels(db, nodes)
				if err != nil {
					return err
				}
				job.RenderMarkdownList(cmd.OutOrStdout(), nodes, blockers, labels, 0)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "md", "output format (md|json)")
	cmd.Flags().StringVar(&labelFilter, "label", "", "filter to tasks carrying this label")
	return cmd
}
