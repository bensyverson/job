package main

import (
	"bufio"
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
	cmd.AddCommand(newEditCmd())
	cmd.AddCommand(newNoteCmd())
	cmd.AddCommand(newRemoveCmd())
	cmd.AddCommand(newMoveCmd())
	cmd.AddCommand(newInfoCmd())
	cmd.AddCommand(newBlockCmd())
	cmd.AddCommand(newUnblockCmd())
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
	var format string
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

			if format == "json" {
				blockers, err := collectBlockers(db, nodes)
				if err != nil {
					return err
				}
				_ = blockers
				b, err := formatTaskNodesJSON(nodes)
				if err != nil {
					return err
				}
				cmd.OutOrStdout().Write(b)
				fmt.Fprintln(cmd.OutOrStdout())
			} else {
				blockers, err := collectBlockers(db, nodes)
				if err != nil {
					return err
				}
				renderMarkdownList(cmd.OutOrStdout(), nodes, blockers, 0)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "md", "output format (md|json)")
	return cmd
}

func collectBlockers(db *sql.DB, nodes []*TaskNode) (map[string][]string, error) {
	result := make(map[string][]string)
	var walk func([]*TaskNode)
	walk = func(nodes []*TaskNode) {
		for _, node := range nodes {
			blockers, err := getBlockers(db, node.Task.ShortID)
			if err != nil {
				return
			}
			if len(blockers) > 0 {
				var ids []string
				for _, b := range blockers {
					ids = append(ids, b.ShortID)
				}
				result[node.Task.ShortID] = ids
			}
			walk(node.Children)
		}
	}
	walk(nodes)
	return result, nil
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

func newEditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit <id> <title>",
		Short: "Change a task's title",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			if err := runEdit(db, args[0], args[1]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Edited: %s\n", args[0])
			return nil
		},
	}
	return cmd
}

func newNoteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "note <id> <text>",
		Short: "Append a note to a task's description",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			if err := runNote(db, args[0], args[1]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Noted: %s\n", args[0])
			return nil
		},
	}
	return cmd
}

func newRemoveCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "remove <id> [all]",
		Short: "Remove a task",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			shortID := args[0]
			removeAll := false
			if len(args) == 2 && args[1] == "all" {
				removeAll = true
			}

			if !force {
				task, _ := getTaskByShortID(db, shortID)
				if task == nil {
					return fmt.Errorf("task %q not found", shortID)
				}
				fmt.Fprintf(cmd.OutOrStderr(), "Remove %s? [y/N] ", task.Title)
				reader := bufio.NewReader(os.Stdin)
				input, _ := reader.ReadString('\n')
				input = strings.TrimSpace(input)
				if input != "y" && input != "Y" {
					fmt.Fprintln(cmd.OutOrStdout(), "Cancelled")
					return nil
				}
			}

			count, err := runRemove(db, shortID, removeAll, force)
			if err != nil {
				return err
			}

			if count > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Removed: %s (and %d subtasks)\n", shortID, count)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Removed: %s\n", shortID)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "skip confirmation prompt")
	return cmd
}

func newMoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "move <id> before|after <sibling>",
		Short: "Move a task relative to a sibling",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			direction := args[1]
			if direction != "before" && direction != "after" {
				return fmt.Errorf("direction must be 'before' or 'after', got %q", direction)
			}

			if err := runMove(db, args[0], direction, args[2]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Moved: %s %s %s\n", args[0], direction, args[2])
			return nil
		},
	}
	return cmd
}

func newInfoCmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "info <id>",
		Short: "Show full details of a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			info, err := runInfo(db, args[0])
			if err != nil {
				return err
			}

			if format == "json" {
				renderInfoJSON(cmd.OutOrStdout(), info)
				fmt.Fprintln(cmd.OutOrStdout())
			} else {
				renderInfoMarkdown(cmd.OutOrStdout(), info)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "md", "output format (md|json)")
	return cmd
}

func newBlockCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "block <blocked> by <blocker>",
		Short: "Block a task until another is complete",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			if args[1] != "by" {
				return fmt.Errorf("usage: job block <blocked> by <blocker>")
			}

			if err := runBlock(db, args[0], args[2]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Blocked: %s (blocked by %s)\n", args[0], args[2])
			return nil
		},
	}
	return cmd
}

func newUnblockCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unblock <blocked> from <blocker>",
		Short: "Remove a blocking relationship",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			if args[1] != "from" {
				return fmt.Errorf("usage: job unblock <blocked> from <blocker>")
			}

			if err := runUnblock(db, args[0], args[2]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Unblocked: %s (was blocked by %s)\n", args[0], args[2])
			return nil
		},
	}
	return cmd
}
