package main

import (
	"fmt"
	job "github.com/bensyverson/job/internal/job"
	"github.com/spf13/cobra"
)

func newAddCmd() *cobra.Command {
	var desc string
	var before string
	cmd := &cobra.Command{
		Use:   "add [parent] <title>",
		Short: "Add a new task",
		Long:  "Add a new task. If parent is provided, the task is added as a child. Use --desc for a description and --before to insert before a specific sibling.",
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

			res, err := job.RunAdd(db, parentShortID, title, desc, before, actor)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), res.ShortID)
			if res.AutoReleasedParent != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Released: %s (prior claim by %s auto-released — parent now has open children)\n",
					res.AutoReleasedParent, res.AutoReleasedByActor)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&desc, "desc", "d", "", "task description")
	cmd.Flags().StringVarP(&before, "before", "b", "", "insert before this sibling task ID")
	return cmd
}
