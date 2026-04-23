package main

import (
	"fmt"
	job "github.com/bensyverson/jobs/internal/job"
	"github.com/spf13/cobra"
)

func newMoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "move <id> before|after <sibling>",
		Short: "Move a task relative to a sibling",
		Long:  "Move a task before or after a sibling task. Both tasks must share the same parent.",
		Args:  cobra.ExactArgs(3),
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

			direction := args[1]
			if direction != "before" && direction != "after" {
				return fmt.Errorf("direction must be 'before' or 'after', got %q", direction)
			}

			if err := job.RunMove(db, args[0], direction, args[2], actor); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Moved: %s %s %s\n", args[0], direction, args[2])
			return nil
		},
	}
	return cmd
}
