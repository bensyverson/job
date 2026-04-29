package main

import (
	"fmt"
	job "github.com/bensyverson/jobs/internal/job"
	"github.com/spf13/cobra"
)

func newMoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "move <id> (before|after <sibling> | under <new-parent> [before|after <sibling>])",
		Short: "Move a task — reorder among siblings or reparent under a new parent",
		Long: "Move a task. Two forms:\n" +
			"  job move <id> before|after <sibling>                  — reorder among siblings\n" +
			"  job move <id> under <new-parent>                      — reparent (placed at end)\n" +
			"  job move <id> under <new-parent> before|after <sib>   — reparent and position",
		Args: cobra.RangeArgs(3, 5),
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

			id := args[0]

			switch args[1] {
			case "before", "after":
				if len(args) != 3 {
					return fmt.Errorf("usage: job move <id> %s <sibling>", args[1])
				}
				if err := job.RunMove(db, id, args[1], args[2], actor); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Moved: %s %s %s\n", id, args[1], args[2])
				return nil
			case "under":
				if len(args) != 3 && len(args) != 5 {
					return fmt.Errorf("usage: job move %s under <new-parent> [before|after <sibling>]", id)
				}
				newParent := args[2]
				direction := ""
				relativeTo := ""
				if len(args) == 5 {
					direction = args[3]
					if direction != "before" && direction != "after" {
						return fmt.Errorf("direction must be 'before' or 'after', got %q", direction)
					}
					relativeTo = args[4]
				}
				if err := job.RunReparent(db, id, newParent, direction, relativeTo, actor); err != nil {
					return err
				}
				if direction == "" {
					fmt.Fprintf(cmd.OutOrStdout(), "Moved: %s under %s\n", id, newParent)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "Moved: %s under %s %s %s\n", id, newParent, direction, relativeTo)
				}
				return nil
			default:
				return fmt.Errorf("expected 'before', 'after', or 'under' as second argument, got %q", args[1])
			}
		},
	}
	return cmd
}
