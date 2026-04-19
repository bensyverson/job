package main

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var dbPath string

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "job",
		Short:         "A lightweight CLI task manager",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.PersistentFlags().StringVar(&dbPath, "db", "", "path to jobs database (default: .jobs.db)")
	cmd.AddCommand(newInitCmd())
	cmd.AddCommand(newAddCmd())
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newDoneCmd())
	cmd.AddCommand(newReopenCmd())
	return cmd
}

func openDBFromCmd() (*sql.DB, error) {
	path := resolveDBPath(dbPath)
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("no Jobs database found in %s. Run `job init` or specify a database with --db", path)
	}
	return openDB(path)
}

func newInitCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new Jobs database",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := resolveDBPath(dbPath)
			if _, err := os.Stat(path); err == nil && !force {
				return fmt.Errorf("%s already exists. Use --force to overwrite", path)
			}
			if force {
				os.Remove(path)
			}
			db, err := createDB(path)
			if err != nil {
				return err
			}
			db.Close()
			if force {
				fmt.Fprintf(cmd.OutOrStdout(), "Initialized %s (overwrote existing database)\n", path)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Initialized %s\n", path)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing database")
	return cmd
}

func newAddCmd() *cobra.Command {
	var desc string
	var before string
	cmd := &cobra.Command{
		Use:   "add [parent] <title>",
		Short: "Add a new task",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			var parentShortID, title string
			if len(args) == 2 {
				parentShortID = args[0]
				title = args[1]
			} else {
				title = args[0]
			}

			id, err := runAdd(db, parentShortID, title, desc, before)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), id)
			return nil
		},
	}
	cmd.Flags().StringVar(&desc, "desc", "", "task description")
	cmd.Flags().StringVar(&before, "before", "", "insert before this sibling task ID")
	return cmd
}

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list [parent] [all]",
		Short: "List tasks",
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

			nodes, err := runList(db, parentShortID, showAll)
			if err != nil {
				return err
			}

			renderMarkdown(cmd, nodes, 0)
			return nil
		},
	}
	return cmd
}

func renderMarkdown(cmd *cobra.Command, nodes []*TaskNode, depth int) {
	indent := strings.Repeat("  ", depth)
	for _, node := range nodes {
		fmt.Fprintf(cmd.OutOrStdout(), "%s- %s  %s", indent, node.Task.ShortID, node.Task.Title)
		if node.Task.Status == "done" {
			if node.Task.CompletionNote != nil && *node.Task.CompletionNote != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  [done, %s]", *node.Task.CompletionNote)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "  [done]")
			}
		}
		fmt.Fprintln(cmd.OutOrStdout())
		renderMarkdown(cmd, node.Children, depth+1)
	}
}

func newDoneCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "done <id> [note]",
		Short: "Mark a task as done",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			shortID := args[0]
			var note string
			if len(args) == 2 {
				note = args[1]
			}

			forced, err := runDone(db, shortID, force, note)
			if err != nil {
				return err
			}

			if len(forced) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Done: %s (and %d subtasks)\n", shortID, len(forced))
			} else {
				if note != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "Done: %s (note: %s)\n", shortID, note)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "Done: %s\n", shortID)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "force mark all incomplete subtasks as done")
	return cmd
}

func newReopenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reopen <id>",
		Short: "Reopen a completed task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			reopened, err := runReopen(db, args[0])
			if err != nil {
				return err
			}

			if len(reopened) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Reopened: %s (and %d subtasks)\n", args[0], len(reopened))
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Reopened: %s\n", args[0])
			}
			return nil
		},
	}
	return cmd
}
