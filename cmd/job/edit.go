package main

import (
	"fmt"
	job "github.com/bensyverson/jobs/internal/job"
	"github.com/spf13/cobra"
)

func newEditCmd() *cobra.Command {
	var title string
	var desc string
	cmd := &cobra.Command{
		Use:   "edit <id>",
		Short: "Change a task's title and/or description",
		Long:  "Replace a task's title and/or description. At least one of --title or --desc must be provided. Use --desc \"\" to clear the description.",
		Args:  cobra.ExactArgs(1),
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

			var titlePtr, descPtr *string
			if cmd.Flags().Changed("title") {
				t := title
				titlePtr = &t
			}
			if cmd.Flags().Changed("desc") {
				d := desc
				descPtr = &d
			}
			if titlePtr == nil && descPtr == nil {
				return fmt.Errorf("edit requires --title and/or --desc")
			}

			if err := job.RunEdit(db, args[0], titlePtr, descPtr, actor); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Edited: %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVarP(&title, "title", "t", "", "new title (replaces current)")
	cmd.Flags().StringVarP(&desc, "desc", "d", "", "new description (replaces current; pass \"\" to clear)")
	return cmd
}
