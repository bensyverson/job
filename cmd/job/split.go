package main

import (
	"fmt"
	job "github.com/bensyverson/jobs/internal/job"
	"github.com/spf13/cobra"
)

func newSplitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "split <id> <title> [<title> ...]",
		Short: "Split a leaf task into children with the given titles",
		Long: "Take an existing leaf and open one child under it for each supplied title. " +
			"The parent must currently have no children — split is for subdividing a leaf, " +
			"not for piling children onto an existing phase. After split, the parent will " +
			"auto-close once all new children close (leaf-frontier cascade).",
		Args: cobra.MinimumNArgs(2),
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

			parentID := args[0]
			titles := args[1:]

			res, err := job.RunSplit(db, parentID, titles, actor)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Split: %s into %d children\n", res.ParentShortID, len(res.ChildShortIDs))
			for i, cid := range res.ChildShortIDs {
				fmt.Fprintf(out, "  %s %q\n", cid, titles[i])
			}
			if res.AutoReleasedParent != "" {
				fmt.Fprintf(out, "Released: %s (prior claim by %s auto-released — parent now has open children)\n",
					res.AutoReleasedParent, res.AutoReleasedByActor)
			}
			return nil
		},
	}
	return cmd
}
