package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	dbPath string
	asFlag string
)

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "job",
		Short:         "A lightweight CLI task manager",
		Long:          rootLongHelp,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.PersistentFlags().StringVar(&dbPath, "db", "", "path to job database (default: .jobs.db)")
	cmd.PersistentFlags().StringVar(&asFlag, "as", "", "identity to use for writes (e.g. --as alice)")
	cmd.AddCommand(newInitCmd())
	cmd.AddCommand(newAddCmd())
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newDoneCmd())
	cmd.AddCommand(newReopenCmd())
	cmd.AddCommand(newEditCmd())
	cmd.AddCommand(newNoteCmd())
	cmd.AddCommand(newCancelCmd())
	cmd.AddCommand(newMoveCmd())
	cmd.AddCommand(newInfoCmd())
	cmd.AddCommand(newBlockCmd())
	cmd.AddCommand(newUnblockCmd())
	cmd.AddCommand(newClaimCmd())
	cmd.AddCommand(newHeartbeatCmd())
	cmd.AddCommand(newReleaseCmd())
	cmd.AddCommand(newLabelCmd())
	cmd.AddCommand(newNextCmd())
	cmd.AddCommand(newClaimNextCmd())
	cmd.AddCommand(newLogCmd())
	cmd.AddCommand(newTailCmd())
	cmd.AddCommand(newImportCmd())
	cmd.AddCommand(newSchemaCmd())
	cmd.AddCommand(newStatusCmd())
	return cmd
}

const rootLongHelp = `job — a lightweight task tracker for multi-phase, multi-agent work.

Use job for any task with more than a few steps, work that benefits from
a durable audit trail, or work that may involve multiple agents
coordinating. For ad-hoc one-off todos, built-in session notes are fine;
use job when persistence, attribution, or coordination matter.

QUICKSTART

  1. Plan in a Markdown doc with a YAML code fence:

       ` + "```" + `yaml
       tasks:
         - title: Root task
           children:
             - title: First subtask
             - title: Second subtask
       ` + "```" + `

  2. Import:  job import plan.md
     (Use --dry-run first if you want to preview without creating.)

  3. Work:    job --as claude claim-next
              job --as claude done <id> "notes on what was done"

  4. Observe: job list         (actionable tasks)
              job status       (one-line summary)
              job log <id>     (history of a task and its subtree)

IDENTITY

  Writes require --as <name>. Reads (list, info, log, status, next,
  tail, schema) work without it.

    job --as alice claim 87TNz     # explicit identity per write

  For "set once, forget" ergonomics, shell-alias it:
    alias job='job --as alice'     # in .zshrc, .bashrc, etc.

  Identity is free-form. Pick a stable name per agent or user; if two
  agents use the same name they share attribution, so choose unique
  names in multi-agent workflows.

VERBS (grouped by role)

  Setup:        init, schema
  Planning:     add, import, edit, block, unblock, move, label
  Execution:    claim, claim-next, release, note, done, reopen, cancel, heartbeat
  Observation:  list, info, log, status, next, next all, tail

  For full options on any verb:  job <verb> --help

OUTPUT

  Dense Markdown by default, token-efficient for both human and LLM
  readers. --format=json on any read verb for deterministic parsers
  or subscriber agents on live streams.

  List output uses GFM checkboxes so you can paste ` + "`job list all`" + `
  straight into a PR or issue and have it render as a task list.

ORCHESTRATION

  For multi-agent workflows, see:
    job next all                       # full claimable frontier
    job tail <id> --format=json        # streaming JSON-lines event stream
    job tail --until-close <id>        # block until <id> closes
    job --as <name> cancel <id>        # non-destructively stop work
    job --as <name> heartbeat <id>     # refresh a long-running claim
`

func openDBFromCmd() (*sql.DB, error) {
	path := resolveDBPath(dbPath)
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("no job database found in %s. Run `job init` or specify a database with --db", path)
	}
	return openDB(path)
}

func newInitCmd() *cobra.Command {
	var force bool
	var writeGitignore bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new job database",
		Long:  "Initialize a new .jobs.db in the current directory. Errors if one already exists unless --force is used. Use --gitignore to append recommended entries to .gitignore.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := resolveDBPathForInit(dbPath)
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

			if writeGitignore {
				dir := filepath.Dir(path)
				if dir == "" {
					dir = "."
				}
				written, alreadyPresent, gerr := writeGitignoreEntries(dir)
				if gerr != nil {
					return gerr
				}
				if len(written) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "Wrote %d entries to .gitignore: %s\n", len(written), strings.Join(written, ", "))
				} else if len(alreadyPresent) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), ".gitignore already includes %s\n", humanJoin(alreadyPresent))
				}
			} else {
				fmt.Fprintln(cmd.OutOrStdout())
				fmt.Fprintln(cmd.OutOrStdout(), gitignoreHint)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing database")
	cmd.Flags().BoolVar(&writeGitignore, "gitignore", false, "append recommended entries to .gitignore")
	return cmd
}

func humanJoin(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	case 2:
		return items[0] + " and " + items[1]
	default:
		return strings.Join(items[:len(items)-1], ", ") + ", and " + items[len(items)-1]
	}
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

			res, err := runAdd(db, parentShortID, title, desc, before, actor)
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
	cmd.Flags().StringVar(&desc, "desc", "", "task description")
	cmd.Flags().StringVar(&before, "before", "", "insert before this sibling task ID")
	return cmd
}

func newListCmd() *cobra.Command {
	var format string
	var labelFilter string
	cmd := &cobra.Command{
		Use:   "list [parent] [all]",
		Short: "List tasks",
		Long:  "List tasks. By default shows only actionable (available, unblocked, unclaimed) tasks. Use 'all' to include done, claimed, and blocked tasks. Use --label <name> to filter to tasks carrying that label. Use --format=json for machine-readable output.",
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

			nodes, err := runListFiltered(db, parentShortID, "", showAll, labelFilter)
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
				if len(nodes) == 0 {
					total, done, cerr := countTasks(db)
					if cerr != nil {
						return cerr
					}
					renderListEmpty(cmd.OutOrStdout(), total, done)
					return nil
				}
				blockers, err := collectBlockers(db, nodes)
				if err != nil {
					return err
				}
				labels, err := collectLabels(db, nodes)
				if err != nil {
					return err
				}
				renderMarkdownList(cmd.OutOrStdout(), nodes, blockers, labels, 0)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "md", "output format (md|json)")
	cmd.Flags().StringVar(&labelFilter, "label", "", "filter to tasks carrying this label")
	return cmd
}

func countTasks(db *sql.DB) (total, done int, err error) {
	if err = db.QueryRow("SELECT COUNT(*) FROM tasks WHERE deleted_at IS NULL").Scan(&total); err != nil {
		return 0, 0, err
	}
	if err = db.QueryRow("SELECT COUNT(*) FROM tasks WHERE deleted_at IS NULL AND status = 'done'").Scan(&done); err != nil {
		return 0, 0, err
	}
	return total, done, nil
}

func collectLabels(db *sql.DB, nodes []*TaskNode) (map[int64][]string, error) {
	var ids []int64
	var walk func([]*TaskNode)
	walk = func(nodes []*TaskNode) {
		for _, node := range nodes {
			ids = append(ids, node.Task.ID)
			walk(node.Children)
		}
	}
	walk(nodes)
	return getLabelsForTaskIDs(db, ids)
}

func collectBlockers(db *sql.DB, nodes []*TaskNode) (map[string][]string, error) {
	var ids []int64
	shortByID := make(map[int64]string)
	var walk func([]*TaskNode)
	walk = func(nodes []*TaskNode) {
		for _, node := range nodes {
			ids = append(ids, node.Task.ID)
			shortByID[node.Task.ID] = node.Task.ShortID
			walk(node.Children)
		}
	}
	walk(nodes)

	byID, err := getBlockersForTaskIDs(db, ids)
	if err != nil {
		return nil, err
	}
	result := make(map[string][]string, len(byID))
	for id, blockers := range byID {
		if short, ok := shortByID[id]; ok && len(blockers) > 0 {
			result[short] = blockers
		}
	}
	return result, nil
}

func newDoneCmd() *cobra.Command {
	var cascade bool
	var note string
	var resultStr string
	var format string
	cmd := &cobra.Command{
		Use:   "done <id> [<id>...]",
		Short: "Mark one or more tasks as done",
		Long:  "Mark one or more tasks as done, atomically. Use --cascade to close a task and all open descendants in one call. Use -m to record a completion note, and --result for structured JSON output. Idempotent: already-done tasks are reported, not re-recorded.",
		Args:  cobra.MinimumNArgs(1),
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

			var resultRaw json.RawMessage
			if resultStr != "" {
				if !json.Valid([]byte(resultStr)) {
					return fmt.Errorf("--result: invalid JSON: %s", resultStr)
				}
				resultRaw = json.RawMessage(resultStr)
			}

			closed, alreadyDone, err := runDone(db, args, cascade, note, resultRaw, actor)
			if err != nil {
				return err
			}

			// Determine last-named input id still targetable for the context block.
			lastCtxID := ""
			for i := len(args) - 1; i >= 0; i-- {
				lastCtxID = args[i]
				break
			}

			// Collect all auto-closed ancestor IDs across all closed results so
			// computeDoneContext can distinguish "already-done parent" from
			// "just-auto-closed parent".
			autoClosedSet := make(map[string]bool)
			for _, c := range closed {
				for _, anc := range c.AutoClosedAncestors {
					autoClosedSet[anc.ShortID] = true
				}
			}

			var ctx *DoneContext
			if lastCtxID != "" {
				c, cerr := computeDoneContext(db, lastCtxID, autoClosedSet)
				if cerr != nil {
					return cerr
				}
				ctx = c
			}

			if format == "json" {
				return renderDoneJSON(cmd.OutOrStdout(), closed, alreadyDone, ctx)
			}

			// Idempotent single-ID already-done: preserve Phase 3 wording.
			if len(closed) == 0 && len(alreadyDone) == 1 && len(args) == 1 {
				fmt.Fprintf(cmd.OutOrStdout(), "Already done: %s\n", alreadyDone[0])
				return nil
			}

			renderDoneAck(cmd.OutOrStdout(), closed, alreadyDone, ctx)
			return nil
		},
	}
	cmd.Flags().BoolVar(&cascade, "cascade", false, "close the target and all open descendants")
	cmd.Flags().StringVarP(&note, "message", "m", "", "record a completion note")
	cmd.Flags().StringVar(&resultStr, "result", "", "structured JSON result recorded on the done event")
	cmd.Flags().StringVar(&format, "format", "md", "output format (md|json)")
	return cmd
}

func renderDoneAck(w io.Writer, closed []*ClosedResult, alreadyDone []string, finalCtx *DoneContext) {
	single := len(closed) == 1 && len(alreadyDone) == 0 && len(closed[0].CascadeClosed) == 0

	if single {
		c := closed[0]
		fmt.Fprintf(w, "Done: %s %q\n", c.ShortID, c.Title)
	} else if len(closed) == 1 && len(closed[0].CascadeClosed) > 0 && len(alreadyDone) == 0 {
		c := closed[0]
		fmt.Fprintf(w, "Done: %s %q (and %d subtasks)\n", c.ShortID, c.Title, len(c.CascadeClosed))
	} else if len(closed) > 0 {
		fmt.Fprintf(w, "Closed %d tasks:\n", len(closed))
		for _, c := range closed {
			if len(c.CascadeClosed) > 0 {
				fmt.Fprintf(w, "- Done: %s %q (and %d subtasks)\n", c.ShortID, c.Title, len(c.CascadeClosed))
			} else {
				fmt.Fprintf(w, "- Done: %s %q\n", c.ShortID, c.Title)
			}
		}
	}
	if len(alreadyDone) > 0 {
		fmt.Fprintf(w, "  already done: %s\n", strings.Join(alreadyDone, ", "))
	}

	// Surface auto-closed ancestors (leaf-frontier cascade) on every closed
	// result, regardless of final-context state. One line per ancestor.
	for _, c := range closed {
		for _, anc := range c.AutoClosedAncestors {
			fmt.Fprintf(w, "  Auto-closed: %s %q\n", anc.ShortID, anc.Title)
		}
	}

	if finalCtx == nil {
		return
	}

	// Only render the trailing context block when we actually closed something,
	// since an already-done-only call won't have a meaningful "next" context
	// beyond whole-tree completion.
	if len(closed) == 0 {
		return
	}

	ctx := finalCtx

	if ctx.WholeTreeComplete {
		fmt.Fprintf(w, "  All tasks in %s complete. (%d done, 0 open)\n", ctx.WholeTreeRootID, ctx.WholeTreeDoneCount)
		// Fall through so we can also surface NextAfterParent if there's more
		// work at a higher scope (another root-level subtree).
	}

	if ctx.SkippedBlocked != nil && ctx.NextSibling != nil {
		fmt.Fprintf(w, "  Next sibling %s is blocked on %s. Skipping to %s.\n",
			ctx.SkippedBlocked.ShortID, ctx.SkippedBlockedBy, ctx.NextSibling.ShortID)
	} else if ctx.NextSibling != nil {
		fmt.Fprintf(w, "  Next: %s %q\n", ctx.NextSibling.ShortID, ctx.NextSibling.Title)
	} else if ctx.NextAfterParent != nil {
		// Parent auto-closed; surface the next work past it.
		fmt.Fprintf(w, "  Next: %s %q\n", ctx.NextAfterParent.ShortID, ctx.NextAfterParent.Title)
	}

	// Only show "Parent X: N of M complete" when the parent is still open.
	// An auto-closed parent has nothing meaningful to report here.
	if ctx.ParentID != "" && !ctx.ParentWasDone && !ctx.ParentAutoClosed {
		fmt.Fprintf(w, "  Parent %s: %d of %d complete\n", ctx.ParentID, ctx.ParentDoneCount, ctx.ParentTotalCount)
	}
}

type doneJSONClosed struct {
	ID                  string               `json:"id"`
	Title               string               `json:"title"`
	CascadeClosed       []string             `json:"cascade_closed"`
	AutoClosedAncestors []doneJSONAutoClosed `json:"auto_closed_ancestors,omitempty"`
}

type doneJSONAutoClosed struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type doneJSONNext struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type doneJSONParent struct {
	ID    string `json:"id"`
	Done  int    `json:"done"`
	Total int    `json:"total"`
}

type doneJSON struct {
	Closed      []doneJSONClosed `json:"closed"`
	AlreadyDone []string         `json:"already_done"`
	Next        *doneJSONNext    `json:"next"`
	Parent      *doneJSONParent  `json:"parent"`
}

func renderDoneJSON(w io.Writer, closed []*ClosedResult, alreadyDone []string, ctx *DoneContext) error {
	out := doneJSON{
		AlreadyDone: alreadyDone,
	}
	if out.AlreadyDone == nil {
		out.AlreadyDone = []string{}
	}
	for _, c := range closed {
		jc := doneJSONClosed{ID: c.ShortID, Title: c.Title, CascadeClosed: c.CascadeClosed}
		if jc.CascadeClosed == nil {
			jc.CascadeClosed = []string{}
		}
		for _, anc := range c.AutoClosedAncestors {
			jc.AutoClosedAncestors = append(jc.AutoClosedAncestors, doneJSONAutoClosed{ID: anc.ShortID, Title: anc.Title})
		}
		out.Closed = append(out.Closed, jc)
	}
	if out.Closed == nil {
		out.Closed = []doneJSONClosed{}
	}
	if ctx != nil && len(closed) > 0 {
		if ctx.NextSibling != nil {
			out.Next = &doneJSONNext{ID: ctx.NextSibling.ShortID, Title: ctx.NextSibling.Title}
		} else if ctx.NextAfterParent != nil && ctx.ParentAutoClosed {
			out.Next = &doneJSONNext{ID: ctx.NextAfterParent.ShortID, Title: ctx.NextAfterParent.Title}
		}
		if ctx.ParentID != "" {
			out.Parent = &doneJSONParent{ID: ctx.ParentID, Done: ctx.ParentDoneCount, Total: ctx.ParentTotalCount}
		}
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	w.Write(b)
	fmt.Fprintln(w)
	return nil
}

func newReopenCmd() *cobra.Command {
	var cascade bool
	cmd := &cobra.Command{
		Use:   "reopen <id>",
		Short: "Reopen a completed or canceled task",
		Long:  "Reopen a completed or canceled task, setting it back to available. Use --cascade to also reopen all done/canceled descendants.",
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

			reopened, err := runReopen(db, args[0], cascade, actor)
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
	cmd.Flags().BoolVar(&cascade, "cascade", false, "also reopen all done descendants")
	return cmd
}

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

			if err := runEdit(db, args[0], titlePtr, descPtr, actor); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Edited: %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "new title (replaces current)")
	cmd.Flags().StringVar(&desc, "desc", "", "new description (replaces current; pass \"\" to clear)")
	return cmd
}

func newNoteCmd() *cobra.Command {
	var message string
	var resultStr string
	cmd := &cobra.Command{
		Use:   "note <id>",
		Short: "Append a note to a task's description",
		Long:  "Append text to a task's description, prefixed with a timestamp. Pass the body via -m or read from stdin with `-`. Use --result to attach a structured JSON blob to the event without touching the description.",
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

			shortID := args[0]
			stdinForm := len(args) == 2 && args[1] == "-"
			if len(args) == 2 && !stdinForm {
				return fmt.Errorf("note: unexpected argument %q (use -m \"<text>\" or stdin via -)", args[1])
			}

			hasMessage := cmd.Flags().Changed("message")
			if !hasMessage && !stdinForm {
				return fmt.Errorf("note requires -m \"<text>\" or stdin via -")
			}
			if hasMessage && stdinForm {
				return fmt.Errorf("note -m and stdin form are mutually exclusive")
			}

			text := message
			if stdinForm {
				b, rerr := io.ReadAll(cmd.InOrStdin())
				if rerr != nil {
					return rerr
				}
				text = strings.TrimRight(string(b), "\n\r")
			}
			if text == "" {
				return fmt.Errorf("note text is empty")
			}

			var resultRaw json.RawMessage
			if resultStr != "" {
				if !json.Valid([]byte(resultStr)) {
					return fmt.Errorf("--result: invalid JSON: %s", resultStr)
				}
				resultRaw = json.RawMessage(resultStr)
			}

			if err := runNote(db, shortID, text, resultRaw, actor); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Noted: %s\n", shortID)
			return nil
		},
	}
	cmd.Flags().StringVarP(&message, "message", "m", "", "note text to append")
	cmd.Flags().StringVar(&resultStr, "result", "", "structured JSON result recorded on the noted event")
	return cmd
}

func newCancelCmd() *cobra.Command {
	var reason string
	var cascade bool
	var purge bool
	var yes bool
	var format string
	cmd := &cobra.Command{
		Use:   "cancel <id> [<id>...]",
		Short: "Non-destructively stop work on one or more tasks",
		Long:  "Mark one or more tasks as canceled, atomically. --reason is required. --cascade also cancels open descendants. --purge erases the task and its events instead of transitioning state; --purge --cascade requires --yes.",
		Args:  cobra.MinimumNArgs(1),
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

			canceled, alreadyCanceled, purged, err := runCancel(db, args, reason, cascade, purge, yes, actor)
			if err != nil {
				return err
			}

			if format == "json" {
				return renderCancelJSON(cmd.OutOrStdout(), canceled, alreadyCanceled, purged, reason)
			}

			if purge {
				renderPurgeAck(cmd.OutOrStdout(), purged, reason)
				return nil
			}

			if len(canceled) == 0 && len(alreadyCanceled) == 1 && len(args) == 1 {
				fmt.Fprintf(cmd.OutOrStdout(), "Already canceled: %s\n", alreadyCanceled[0])
				return nil
			}

			renderCancelAck(cmd.OutOrStdout(), canceled, alreadyCanceled, reason)
			return nil
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "human-readable reason (required)")
	cmd.Flags().BoolVar(&cascade, "cascade", false, "also cancel/purge open descendants")
	cmd.Flags().BoolVar(&purge, "purge", false, "erase the task row and its events instead of transitioning state")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm irrecoverable purge of a subtree (required with --purge --cascade)")
	cmd.Flags().StringVar(&format, "format", "md", "output format (md|json)")
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

			actor, err := requireAs(db)
			if err != nil {
				return err
			}

			direction := args[1]
			if direction != "before" && direction != "after" {
				return fmt.Errorf("direction must be 'before' or 'after', got %q", direction)
			}

			if err := runMove(db, args[0], direction, args[2], actor); err != nil {
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

			actor, err := requireAs(db)
			if err != nil {
				return err
			}

			if args[1] != "by" {
				return fmt.Errorf("usage: job block <blocked> by <blocker>")
			}

			if err := runBlock(db, args[0], args[2], actor); err != nil {
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

			actor, err := requireAs(db)
			if err != nil {
				return err
			}

			if args[1] != "from" {
				return fmt.Errorf("usage: job unblock <blocked> from <blocker>")
			}

			if err := runUnblock(db, args[0], args[2], actor); err != nil {
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
		Long:  "Claim a task, marking it as in-progress. Duration defaults to 15m. Supported units: s, m, h, d. Use --force to override an existing claim.",
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

			if err := runClaim(db, shortID, duration, actor, force); err != nil {
				return err
			}

			durStr := formatDuration(defaultClaimTTLSeconds)
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

func newHeartbeatCmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "heartbeat <id> [<id>...]",
		Short: "Extend your live claim(s) by 15 minutes",
		Long:  "Refresh one or more live claims held by the caller. Extends claim_expires_at by 15 minutes and emits a heartbeat event. All targets must currently be claimed by the caller; any other state errors and rolls back the whole call.",
		Args:  cobra.MinimumNArgs(1),
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

			results, err := runHeartbeat(db, args, actor)
			if err != nil {
				return err
			}

			if format == "json" {
				return renderHeartbeatJSON(cmd.OutOrStdout(), results)
			}
			renderHeartbeatAck(cmd.OutOrStdout(), results)
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "md", "output format (md|json)")
	return cmd
}

func newLabelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "label",
		Short: "Add or remove labels on a task",
		Long:  "Manage flat, free-form labels on a task. Subcommands: 'add' and 'remove'. Names are variadic per call, idempotent, and atomic. Labels are local to each task (no inheritance).",
	}
	cmd.AddCommand(newLabelAddCmd())
	cmd.AddCommand(newLabelRemoveCmd())
	return cmd
}

func newLabelAddCmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "add <id> <name> [<name>...]",
		Short: "Add one or more labels to a task",
		Long:  "Add one or more labels to a task. Idempotent: names that are already attached are reported, not re-recorded. Emits a single 'labeled' event per call when at least one new label is added.",
		Args:  cobra.MinimumNArgs(2),
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

			res, err := runLabelAdd(db, args[0], args[1:], actor)
			if err != nil {
				return err
			}

			if format == "json" {
				return renderLabelJSON(cmd.OutOrStdout(), res)
			}
			renderLabelAck(cmd.OutOrStdout(), res)
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "md", "output format (md|json)")
	return cmd
}

func newLabelRemoveCmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "remove <id> <name> [<name>...]",
		Short: "Remove one or more labels from a task",
		Long:  "Remove one or more labels from a task. Idempotent: names that are not present are reported, not re-recorded. Emits a single 'unlabeled' event per call when at least one label is actually removed.",
		Args:  cobra.MinimumNArgs(2),
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

			res, err := runLabelRemove(db, args[0], args[1:], actor)
			if err != nil {
				return err
			}

			if format == "json" {
				return renderUnlabelJSON(cmd.OutOrStdout(), res)
			}
			renderUnlabelAck(cmd.OutOrStdout(), res)
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "md", "output format (md|json)")
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

			actor, err := requireAs(db)
			if err != nil {
				return err
			}

			if err := runRelease(db, args[0], actor); err != nil {
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
	var labelFilter string
	var includeParents bool
	cmd := &cobra.Command{
		Use:   "next [parent] [all]",
		Short: "Show the next available task (or all of them with `all`)",
		Long:  "Show the next available (unblocked, unclaimed, not done) task. By default only leaves (tasks with no open children) are surfaced — tasks with open children are descended through, not returned. Pass --include-parents to surface any available task regardless of whether it has open children. With 'all' (in either position), returns the full claimable frontier instead. Use --label <name> to filter to tasks carrying that label. Without a parent, searches the entire tree.",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			var parentShortID string
			showAll := false
			for _, a := range args {
				if a == "all" {
					showAll = true
				} else {
					parentShortID = a
				}
			}

			if showAll {
				tasks, err := runNextAllFiltered(db, parentShortID, "", labelFilter, includeParents)
				if err != nil {
					return err
				}
				if format == "json" {
					return renderNextAllJSON(cmd.OutOrStdout(), tasks)
				}
				renderNextAllText(cmd.OutOrStdout(), tasks)
				return nil
			}

			task, err := runNextFiltered(db, parentShortID, "", labelFilter, includeParents)
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
	cmd.Flags().StringVar(&labelFilter, "label", "", "filter to tasks carrying this label")
	cmd.Flags().BoolVar(&includeParents, "include-parents", false, "surface tasks with open children (legacy behavior)")
	return cmd
}

func newClaimNextCmd() *cobra.Command {
	var force bool
	var format string
	var includeParents bool
	cmd := &cobra.Command{
		Use:   "claim-next [parent] [duration]",
		Short: "Find and claim the next available task",
		Long:  "Find the next available task and claim it in one step. By default only leaves (tasks with no open children) are claimable — the search descends through parents to find work. Pass --include-parents to permit claiming any available task. Duration defaults to 15m. Supported units: s, m, h, d. Without a parent, searches the entire tree.",
		Args:  cobra.MaximumNArgs(2),
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

			task, err := runClaimNextFiltered(db, parentShortID, duration, actor, force, includeParents)
			if err != nil {
				return err
			}

			if format == "json" {
				renderTaskJSON(cmd.OutOrStdout(), task)
				fmt.Fprintln(cmd.OutOrStdout())
			} else {
				durStr := formatDuration(defaultClaimTTLSeconds)
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
	cmd.Flags().BoolVar(&includeParents, "include-parents", false, "permit claiming tasks with open children (legacy behavior)")
	return cmd
}

func newLogCmd() *cobra.Command {
	var format string
	var since string
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

			var sincePtr *int64
			if since != "" {
				ts, perr := time.Parse(time.RFC3339, since)
				if perr != nil {
					return fmt.Errorf("--since: invalid RFC3339 timestamp: %s", since)
				}
				u := ts.Unix()
				sincePtr = &u
			}

			events, err := runLog(db, args[0], sincePtr)
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
	cmd.Flags().StringVar(&since, "since", "", "only events at or after this RFC3339 timestamp")
	return cmd
}

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

			res, err := runImport(db, args[0], parent, dryRun, actor)
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
	cmd.Flags().StringVar(&parent, "parent", "", "make imported roots children of this task")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "validate the plan without writing to the database")
	cmd.Flags().StringVar(&format, "format", "md", "output format (md|json)")
	return cmd
}

func newSchemaCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schema",
		Short: "Print the JSON Schema for `job import`",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSchema(cmd.OutOrStdout())
		},
	}
	return cmd
}

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show a one-line summary of task counts and recent activity",
		Long:  "Print a one-line summary: open / claimed by you (if --as is set) / done, plus the time since the last event. No --as required.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			s, err := runStatus(db, asFlag)
			if err != nil {
				return err
			}
			renderStatus(cmd.OutOrStdout(), s)
			return nil
		},
	}
	return cmd
}

func newTailCmd() *cobra.Command {
	var format string
	var eventsFlag string
	var usersFlag string
	var untilClose []string
	var timeoutStr string
	var quiet bool
	cmd := &cobra.Command{
		Use:   "tail <id>",
		Short: "Stream events in real-time for a task and its descendants",
		Long:  "Stream events for a task and its descendants. Use --until-close <id> (repeatable) to block until each named task reaches done/canceled, then exit 0. --until-close with no value watches the positional task. --timeout <duration> bounds the wait; on expiry exits 2. --quiet suppresses the streamed event output while preserving close/exit messages.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			shortID := args[0]
			task, err := getTaskByShortID(db, shortID)
			if err != nil {
				return err
			}
			if task == nil {
				return fmt.Errorf("task %q not found", shortID)
			}

			filter := EventFilter{
				Types: parseFilterList(eventsFlag),
				Users: parseFilterList(usersFlag),
			}

			// --until-close was passed (flag changed) but only self-sentinel
			// entries, or no value: default to the positional id.
			if cmd.Flags().Changed("until-close") {
				cleaned := make([]string, 0, len(untilClose))
				sawSelf := false
				for _, id := range untilClose {
					if id == "" || id == "_" {
						sawSelf = true
						continue
					}
					cleaned = append(cleaned, id)
				}
				if sawSelf || len(cleaned) == 0 {
					cleaned = append(cleaned, shortID)
				}
				untilClose = cleaned
			}

			var timeout time.Duration
			if timeoutStr != "" {
				secs, perr := parseDuration(timeoutStr)
				if perr != nil {
					return perr
				}
				timeout = time.Duration(secs) * time.Second
			}

			if len(untilClose) > 0 {
				return runTailUntilClose(
					cmd.Context(), db, shortID, untilClose, timeout,
					defaultTailUntilClosePollInterval,
					quiet, format, filter, cmd.OutOrStdout(),
				)
			}

			if format != "json" {
				fmt.Fprintf(cmd.OutOrStdout(), "Tailing events for %s (Ctrl+C to stop)...\n", shortID)
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			go func() {
				<-cmd.Context().Done()
				cancel()
			}()

			return runTail(ctx, db, shortID, 1*time.Second, func(events []EventEntry) error {
				events = filterEvents(events, filter)
				if len(events) == 0 {
					return nil
				}
				if format == "json" {
					return formatEventLogJSONLines(cmd.OutOrStdout(), events)
				}
				renderEventLogMarkdown(cmd.OutOrStdout(), events)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&format, "format", "md", "output format (md|json)")
	cmd.Flags().StringVar(&eventsFlag, "events", "", "comma-separated list of event types to include (default: all except heartbeat)")
	cmd.Flags().StringVar(&usersFlag, "users", "", "comma-separated list of actor names to include")
	cmd.Flags().StringSliceVar(&untilClose, "until-close", nil, "block until the named task closes; repeatable; use --until-close=_ to default to the positional id")
	cmd.Flags().Lookup("until-close").NoOptDefVal = "_"
	cmd.Flags().StringVar(&timeoutStr, "timeout", "", "exit 2 if no close occurs in this duration (e.g. 30s, 5m)")
	cmd.Flags().BoolVar(&quiet, "quiet", false, "suppress event stream while waiting; preserves close and timeout messages")
	return cmd
}
