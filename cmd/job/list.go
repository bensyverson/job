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
	var statusFilter string
	var openFlag bool
	cmd := &cobra.Command{
		Use:     "ls [parent]",
		Aliases: []string{"list", "tree"},
		Short:   "List tasks in tree format",
		Long: `List tasks. By default shows only actionable (available, unblocked, unclaimed) tasks.

Scope: when given a parent, ls returns the full subtree under that parent (not just direct children). Combine with the filters below to narrow the recursive walk.

Use --all to include done, claimed, and blocked tasks.
Use --label <name> to filter to tasks carrying that label.
Use --mine to show only tasks claimed by the caller (via --as or default identity).
Use --claimed-by <name> to show tasks claimed by a specific agent.
Use --grep <pattern> for case-insensitive title search.
Composes: 'ls --mine --label p0', 'ls --claimed-by alice --all'.
Use --format=json for machine-readable output.`,
		Args: cobra.MaximumNArgs(2),
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
			if openFlag && statusFilter != "" && statusFilter != "open" {
				return fmt.Errorf("cannot use --open with --status=%s", statusFilter)
			}
			effectiveStatus := statusFilter
			if openFlag {
				effectiveStatus = "open"
			}
			if effectiveStatus != "" {
				normalized, verr := job.ValidateStatusFilter(effectiveStatus)
				if verr != nil {
					return verr
				}
				effectiveStatus = normalized
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
				Status:         effectiveStatus,
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
						fmt.Fprintf(cmd.OutOrStdout(), "No tasks match `%s`. Try --all to include blocked / done / canceled.\n", grepPattern)
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

				// Sparse-results hint: when an unscoped, unfiltered ls returns
				// fewer than 5 nodes, append a one-liner so first-time users
				// understand the actionable-only default and how to broaden.
				unscopedUnfiltered := parentShortID == "" && !showAll &&
					labelFilter == "" && claimedByFilter == "" && grepPattern == "" &&
					effectiveStatus == ""
				if unscopedUnfiltered && countTopNodes(nodes) < 5 {
					fmt.Fprintln(cmd.OutOrStdout(), "Showing actionable tasks only. Use --all to include blocked / done / canceled tasks.")
				}
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
	cmd.Flags().StringVar(&statusFilter, "status", "", "filter to one status (available|claimed|done|canceled|open)")
	cmd.Flags().BoolVar(&openFlag, "open", false, "shortcut for --status=open (anything not done or canceled)")
	return cmd
}

// countTopNodes counts only the top-level returned nodes — children are
// not double-counted, since the sparse-results hint is about how few roots
// the user sees on screen.
func countTopNodes(nodes []*job.TaskNode) int {
	return len(nodes)
}
