package main

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

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
	cmd.PersistentFlags().StringVar(&dbPath, "db", "", "path to job database (default: .jobs.db)")
	cmd.AddCommand(newLoginCmd())
	cmd.AddCommand(newLogoutCmd())
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
	cmd.AddCommand(newClaimCmd())
	cmd.AddCommand(newReleaseCmd())
	cmd.AddCommand(newNextCmd())
	cmd.AddCommand(newClaimNextCmd())
	cmd.AddCommand(newLogCmd())
	cmd.AddCommand(newTailCmd())
	return cmd
}

func openDBFromCmd() (*sql.DB, error) {
	path := resolveDBPath(dbPath)
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("no job database found in %s. Run `job init` or specify a database with --db", path)
	}
	return openDB(path)
}

func newInitCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new job database",
		Long:  "Initialize a new .jobs.db in the current directory. Errors if one already exists unless --force is used.",
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
		Long:  "Add a new task. If parent is provided, the task is added as a child. Use --desc for a description and --before to insert before a specific sibling.",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			user, err := requireAuth(db)
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

			id, err := runAdd(db, parentShortID, title, desc, before, user.Name)
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
		Long:  "List tasks. By default shows only actionable (available, unblocked, unclaimed) tasks. Use 'all' to include done, claimed, and blocked tasks. Use --format=json for machine-readable output.",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			user, err := requireAuth(db)
			if err != nil {
				return err
			}

			var parentShortID string
			showAll := false
			for _, arg := range args {
				if arg == "all" {
					showAll = true
				} else {
					parentShortID = arg
				}
			}

			nodes, err := runList(db, parentShortID, user.Name, showAll)
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
		Long:  "Mark a task as done. Requires all subtasks to be done unless --force is used. An optional note is recorded (e.g. a git commit hash). Idempotent: already-done tasks report success.",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			user, err := requireAuth(db)
			if err != nil {
				return err
			}

			shortID := args[0]
			var note string
			if len(args) == 2 {
				note = args[1]
			}

			forced, alreadyDone, err := runDone(db, shortID, force, note, user.Name)
			if err != nil {
				return err
			}

			if alreadyDone {
				fmt.Fprintf(cmd.OutOrStdout(), "Already done: %s\n", shortID)
			} else if len(forced) > 0 {
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
		Long:  "Reopen a completed task, setting it back to available. If closed with --force, also reopens all force-closed descendants.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			user, err := requireAuth(db)
			if err != nil {
				return err
			}

			reopened, err := runReopen(db, args[0], user.Name)
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
		Long:  "Change a task's title to the provided value.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			user, err := requireAuth(db)
			if err != nil {
				return err
			}

			if err := runEdit(db, args[0], args[1], user.Name); err != nil {
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
		Long:  "Append text to a task's description, prefixed with a timestamp. The description becomes an append-only scratchpad for progress notes.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			user, err := requireAuth(db)
			if err != nil {
				return err
			}

			if err := runNote(db, args[0], args[1], user.Name); err != nil {
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
		Long:  "Soft-delete a task. Requires interactive confirmation unless --force is used. If the task has children, specify 'all' to remove them as well.",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			user, err := requireAuth(db)
			if err != nil {
				return err
			}

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

			count, err := runRemove(db, shortID, removeAll, force, user.Name)
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
		Long:  "Move a task before or after a sibling task. Both tasks must share the same parent.",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			user, err := requireAuth(db)
			if err != nil {
				return err
			}

			direction := args[1]
			if direction != "before" && direction != "after" {
				return fmt.Errorf("direction must be 'before' or 'after', got %q", direction)
			}

			if err := runMove(db, args[0], direction, args[2], user.Name); err != nil {
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
		Long:  "Show ID, title, description, status, claim info, blockers, children summary, and creation time. Use --format=json for machine-readable output.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			if _, err := requireAuth(db); err != nil {
				return err
			}

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
		Long:  "Declare that the blocked task cannot proceed until the blocker task is done. Circular dependencies are detected and rejected.",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			user, err := requireAuth(db)
			if err != nil {
				return err
			}

			if args[1] != "by" {
				return fmt.Errorf("usage: job block <blocked> by <blocker>")
			}

			if err := runBlock(db, args[0], args[2], user.Name); err != nil {
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
		Long:  "Manually remove a blocking relationship. Blocking relationships are also auto-removed when the blocker task is marked done.",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			user, err := requireAuth(db)
			if err != nil {
				return err
			}

			if args[1] != "from" {
				return fmt.Errorf("usage: job unblock <blocked> from <blocker>")
			}

			if err := runUnblock(db, args[0], args[2], user.Name); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Unblocked: %s (was blocked by %s)\n", args[0], args[2])
			return nil
		},
	}
	return cmd
}

func newClaimCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "claim <id> [duration]",
		Short: "Claim a task",
		Long:  "Claim a task, marking it as in-progress. Duration defaults to 1h. Supported units: s, m, h, d. Use --force to override an existing claim.",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			user, err := requireAuth(db)
			if err != nil {
				return err
			}

			shortID := args[0]
			var duration string
			if len(args) >= 2 {
				duration = args[1]
			}

			prevClaimedBy := ""
			if force {
				task, _ := getTaskByShortID(db, shortID)
				if task != nil && task.Status == "claimed" && task.ClaimedBy != nil {
					prevClaimedBy = *task.ClaimedBy
				}
			}

			if err := runClaim(db, shortID, duration, user.Name, force); err != nil {
				return err
			}

			durStr := "1h"
			if duration != "" {
				durStr = duration
			}

			if force && prevClaimedBy != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Claimed: %s (overrode previous claim by %s, expires in %s)\n", shortID, prevClaimedBy, durStr)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Claimed: %s (expires in %s)\n", shortID, durStr)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "override an existing claim")
	return cmd
}

func newReleaseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "release <id>",
		Short: "Release a claimed task",
		Long:  "Release a claim, returning the task to available status.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			user, err := requireAuth(db)
			if err != nil {
				return err
			}

			if err := runRelease(db, args[0], user.Name); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Released: %s\n", args[0])
			return nil
		},
	}
	return cmd
}

func newNextCmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "next [parent]",
		Short: "Show the next available task",
		Long:  "Show the next available (unblocked, unclaimed, not done) task. Without a parent, searches root-level tasks. Use --format=json for machine-readable output.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			user, err := requireAuth(db)
			if err != nil {
				return err
			}

			var parentShortID string
			if len(args) == 1 {
				parentShortID = args[0]
			}

			task, err := runNext(db, parentShortID, user.Name)
			if err != nil {
				return err
			}

			if format == "json" {
				renderTaskJSON(cmd.OutOrStdout(), task)
				fmt.Fprintln(cmd.OutOrStdout())
			} else {
				renderNextText(cmd.OutOrStdout(), task)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "md", "output format (md|json)")
	return cmd
}

func newClaimNextCmd() *cobra.Command {
	var force bool
	var format string
	cmd := &cobra.Command{
		Use:   "claim-next [parent] [duration]",
		Short: "Find and claim the next available task",
		Long:  "Find the next available task and claim it in one step. Without a parent, searches root-level tasks.",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			user, err := requireAuth(db)
			if err != nil {
				return err
			}

			var parentShortID, duration string
			if len(args) == 0 {
				parentShortID, duration = "", ""
			} else if isDuration(args[0]) {
				duration = args[0]
			} else {
				parentShortID = args[0]
				if len(args) > 1 {
					duration = args[1]
				}
			}

			task, err := runClaimNext(db, parentShortID, duration, user.Name, force)
			if err != nil {
				return err
			}

			if format == "json" {
				renderTaskJSON(cmd.OutOrStdout(), task)
				fmt.Fprintln(cmd.OutOrStdout())
			} else {
				durStr := "1h"
				if duration != "" {
					durStr = duration
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Claimed: %s %q (expires in %s)\n", task.ShortID, task.Title, durStr)
				if task.Description != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "\n  %s\n", task.Description)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "override an existing claim")
	cmd.Flags().StringVar(&format, "format", "md", "output format (md|json)")
	return cmd
}

func newLogCmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "log <id>",
		Short: "Show event history for a task and its descendants",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			if _, err := requireAuth(db); err != nil {
				return err
			}

			events, err := runLog(db, args[0])
			if err != nil {
				return err
			}

			if format == "json" {
				b, err := formatEventLogJSON(events)
				if err != nil {
					return err
				}
				cmd.OutOrStdout().Write(b)
				fmt.Fprintln(cmd.OutOrStdout())
			} else {
				renderEventLogMarkdown(cmd.OutOrStdout(), events)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "md", "output format (md|json)")
	return cmd
}

func newTailCmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "tail <id>",
		Short: "Stream events in real-time for a task and its descendants",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			if _, err := requireAuth(db); err != nil {
				return err
			}

			shortID := args[0]
			task, err := getTaskByShortID(db, shortID)
			if err != nil {
				return err
			}
			if task == nil {
				return fmt.Errorf("task %q not found", shortID)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Tailing events for %s (Ctrl+C to stop)...\n", shortID)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			go func() {
				<-cmd.Context().Done()
				cancel()
			}()

			return runTail(ctx, db, shortID, 1*time.Second, func(events []EventEntry) error {
				if format == "json" {
					for _, e := range events {
						b, err := formatEventLogJSON([]EventEntry{e})
						if err != nil {
							return err
						}
						cmd.OutOrStdout().Write(b)
						fmt.Fprintln(cmd.OutOrStdout())
					}
				} else {
					renderEventLogMarkdown(cmd.OutOrStdout(), events)
				}
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&format, "format", "md", "output format (md|json)")
	return cmd
}

func newLoginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login [name] [key]",
		Short: "Log in or create a user",
		Long: `Log in or create a user.

  job login              Create a new user with a random name and key
  job login <name>       Create user <name> if it doesn't exist, or log in if it does
  job login <name> <key> Log in as <name> with the given key

Use: eval $(job login) to set environment variables in your shell.`,
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			var name, key string
			if len(args) >= 1 {
				name = args[0]
			}
			if len(args) >= 2 {
				key = args[1]
			}

			if name != "" && key == "" {
				existing, err := getUserByName(db, name)
				if err != nil {
					return err
				}
				if existing != nil {
					fmt.Fprintf(cmd.OutOrStderr(), "Enter key for %s: ", name)
					reader := bufio.NewReader(os.Stdin)
					input, err := reader.ReadString('\n')
					if err != nil {
						return fmt.Errorf("failed to read key: %w", err)
					}
					key = strings.TrimSpace(input)
				}
			}

			result, err := runLogin(db, name, key)
			if err != nil {
				return err
			}

			if result.IsNew {
				fmt.Fprintf(cmd.OutOrStderr(), "Created user %s with key %s\n", result.Name, result.Key)
			} else {
				fmt.Fprintf(cmd.OutOrStderr(), "Logged in as %s\n", result.Name)
			}
			fmt.Fprintln(cmd.OutOrStdout(), formatLoginExport(result.Name, result.Key))
			return nil
		},
	}
	return cmd
}

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Log out the current user",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), formatLogoutExport())
			return nil
		},
	}
}
