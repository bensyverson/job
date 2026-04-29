package main

import (
	"fmt"
	job "github.com/bensyverson/jobs/internal/job"
	"github.com/spf13/cobra"
)

func newAddCmd() *cobra.Command {
	var desc string
	var before string
	var labels []string
	var criteria []string
	var parentFlag string
	cmd := &cobra.Command{
		Use:   "add [parent] <title>",
		Short: "Add a new task",
		Long:  "Add a new task. If parent is provided, the task is added as a child. Use --desc for a description, --before to insert before a specific sibling, and --criterion (repeatable) to attach acceptance criteria.",
		Args:  cobra.RangeArgs(1, 2),
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

			var parentShortID, title string
			if len(args) == 2 {
				parentShortID = args[0]
				title = args[1]
			} else {
				title = args[0]
			}
			if parentFlag != "" {
				if parentShortID != "" && parentShortID != parentFlag {
					return fmt.Errorf("add: --parent %q conflicts with positional parent %q", parentFlag, parentShortID)
				}
				parentShortID = parentFlag
			}

			var priorChildCount int
			if parentShortID != "" {
				priorChildCount, _, _ = job.CountOpenChildrenOfShortID(db, parentShortID)
			}

			res, err := job.RunAdd(db, parentShortID, title, desc, before, labels, actor)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), res.ShortID)
			if res.AutoReleasedParent != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Released: %s (prior claim by %s auto-released — parent now has open children)\n",
					res.AutoReleasedParent, res.AutoReleasedByActor)
			}
			if parentShortID != "" && priorChildCount > 0 {
				fmt.Fprintf(cmd.OutOrStdout(),
					"  %s now has %d children; complete them all to auto-close the parent.\n",
					parentShortID, priorChildCount+1)
			}
			if len(criteria) > 0 {
				items := make([]job.Criterion, 0, len(criteria))
				for _, label := range criteria {
					items = append(items, job.Criterion{Label: label})
				}
				if _, err := job.RunAddCriteria(db, res.ShortID, items, actor); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&desc, "desc", "d", "", "task description")
	cmd.Flags().StringVarP(&before, "before", "b", "", "insert before this sibling task ID")
	cmd.Flags().StringArrayVarP(&labels, "label", "l", nil, "label to attach (repeatable)")
	cmd.Flags().StringArrayVar(&criteria, "criterion", nil, "acceptance criterion to attach, defaults to pending state (repeatable)")
	cmd.Flags().StringVar(&parentFlag, "parent", "", "parent task ID (alias for the positional parent argument)")
	return cmd
}
