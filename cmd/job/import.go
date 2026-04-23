package main

import (
	"encoding/json"
	"fmt"
	job "github.com/bensyverson/job/internal/job"
	"github.com/spf13/cobra"
)

func newImportCmd() *cobra.Command {
	var parent string
	var dryRun bool
	var format string
	cmd := &cobra.Command{
		Use:   "import <file.md>",
		Short: "Import tasks from a Markdown plan with a YAML tasks: block",
		Long:  "Parse the first fenced YAML block whose top-level key is tasks: and create every task atomically. Use --dry-run to validate without writing. Use --parent <id> to nest the import under an existing task.",
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

			res, err := job.RunImport(db, args[0], parent, dryRun, actor)
			if err != nil {
				return err
			}

			if format == "json" {
				b, err := json.Marshal(res)
				if err != nil {
					return err
				}
				cmd.OutOrStdout().Write(b)
				fmt.Fprintln(cmd.OutOrStdout())
				return nil
			}

			for _, t := range res.Tasks {
				fmt.Fprintf(cmd.OutOrStdout(), "%s  %s\n", t.ID, t.Title)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&parent, "parent", "p", "", "make imported roots children of this task")
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "validate the plan without writing to the database")
	cmd.Flags().StringVar(&format, "format", "md", "output format (md|json)")
	return cmd
}
