package main

import (
	"fmt"
	job "github.com/bensyverson/jobs/internal/job"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var format string
	var labelFilter string
	var mine bool
	var claimedBy string
	var grepPattern string
	var allFlag bool
	cmd := &cobra.Command{
		Use:     "list [parent]",
		Aliases: []string{"ls"},
		Short:   "List tasks",
		Long: `List tasks. By default shows only actionable (available, unblocked, unclaimed) tasks.

Use --all to include done, claimed, and blocked tasks.
Use --label <name> to filter to tasks carrying that label.
Use --mine to show only tasks claimed by the caller (via --as or default identity).
Use --claimed-by <name> to show tasks claimed by a specific agent.
Use --grep <pattern> for case-insensitive title search.
Composes: 'list --mine --label p0', 'list --claimed-by alice --all'.
Use --format=json for machine-readable output.`,
		Args: cobra.MaximumNArgs(2),
		PreRun: func(cmd *cobra.Command, args []string) {
			if cmd.CalledAs() == "ls" {
				fmt.Fprintln(cmd.ErrOrStderr(), "note: `job ls` is an alias for `job list`; prefer the canonical form.")
			}
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			var parentShortID string
			showAll := allFlag
			for _, arg := range args {
				if arg == "all" {
					showAll = true
				} else {
					parentShortID = arg
				}
			}

			if mine && claimedBy != "" {
				return fmt.Errorf("cannot use both --mine and --claimed-by")
			}

			var claimedByFilter string
			if mine {
				name, err := job.ResolveIdentity(db, asFlag)
				if err != nil {
					return err
				}
				if name == "" {
					return fmt.Errorf("no identity to scope to. Use --as <name> or set a default identity.")
				}
				claimedByFilter = name
			} else if claimedBy != "" {
				claimedByFilter = claimedBy
			}

			nodes, err := job.RunListFiltered(db, job.ListFilter{
				ParentID:       parentShortID,
				ShowAll:        showAll,
				Label:          labelFilter,
				ClaimedByActor: claimedByFilter,
				GrepPattern:    grepPattern,
			})
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
					if grepPattern != "" {
						return nil
					}
					if claimedByFilter != "" {
						fmt.Fprintf(cmd.OutOrStdout(), "No tasks claimed by %s.\n", claimedByFilter)
						return nil
					}
					total, done, cerr := countTasks(db, parentShortID)
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
	cmd.Flags().StringVarP(&labelFilter, "label", "l", "", "filter to tasks carrying this label")
	cmd.Flags().BoolVar(&mine, "mine", false, "show only tasks claimed by the caller")
	cmd.Flags().StringVar(&claimedBy, "claimed-by", "", "show only tasks claimed by this agent")
	cmd.Flags().StringVar(&grepPattern, "grep", "", "case-insensitive substring filter on title")
	cmd.Flags().BoolVar(&allFlag, "all", false, "include done, claimed, and blocked tasks")
	return cmd
}
