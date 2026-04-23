package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	job "github.com/bensyverson/job/internal/job"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var (
	dbPath string
	asFlag string
)

// looksLikeShortID returns true if s has the shape of a job short-ID:
// exactly 5 characters, each in [a-zA-Z0-9]. Used to detect when a user
// passed prose where an ID was expected.
func looksLikeShortID(s string) bool {
	if len(s) != 5 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

// resolveMessage expands a -m flag value. Three forms:
//   - "@path": read the file at path. Errors clearly if the file is missing.
//   - "-": read from stdin.
//   - anything else: returned as the literal string.
//
// Rationale: shell-quoting multi-line evidence payloads into -m "..." is
// painful (backticks, nested quotes); file and stdin forms sidestep it.
func resolveMessage(raw string, stdin io.Reader) (string, error) {
	if raw == "-" {
		b, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("-m -: read stdin: %w", err)
		}
		return strings.TrimRight(string(b), "\n\r"), nil
	}
	if strings.HasPrefix(raw, "@") {
		path := raw[1:]
		b, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("-m @%s: read note file: %w", path, err)
		}
		return string(b), nil
	}
	return raw, nil
}

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
	cmd.AddCommand(newIdentityCmd())
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
  Planning:     add, import, edit, block, move, label
  Execution:    claim, claim-next, release, note, done, reopen, cancel, heartbeat
  Observation:  list, info, log, status, next, next all, tail

  Grammar — pick the shape from the verb category:
    Multi-operation verbs (label, block):  job <verb> <add|remove> <args>
    Single-operation verbs:                job <verb> <id> [--flags]

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
	path := job.ResolveDBPath(dbPath)
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("no job database found in %s. Run `job init` or specify a database with --db", path)
	}
	return job.OpenDB(path)
}

// requireAs resolves the writer identity for this call. Precedence:
//  1. --as flag
//  2. DB-level default identity (config.default_identity), unless strict mode
//  3. error: "identity required. Pass --as <name> ..."
//
// Lives in the CLI layer because it depends on the cobra-bound --as flag
// global; the domain layer supplies the underlying resolver via
// job.ResolveIdentity.
func requireAs(db *sql.DB) (string, error) {
	name, err := job.ResolveIdentity(db, asFlag)
	if err != nil {
		return "", err
	}
	if name == "" {
		return "", fmt.Errorf("identity required. Pass --as <name> before the verb.")
	}
	if _, err := job.EnsureUser(db, name); err != nil {
		return "", err
	}
	return name, nil
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

func countTasks(db *sql.DB) (total, done int, err error) {
	if err = db.QueryRow("SELECT COUNT(*) FROM tasks WHERE deleted_at IS NULL").Scan(&total); err != nil {
		return 0, 0, err
	}
	if err = db.QueryRow("SELECT COUNT(*) FROM tasks WHERE deleted_at IS NULL AND status = 'done'").Scan(&done); err != nil {
		return 0, 0, err
	}
	return total, done, nil
}

func collectLabels(db *sql.DB, nodes []*job.TaskNode) (map[int64][]string, error) {
	var ids []int64
	var walk func([]*job.TaskNode)
	walk = func(nodes []*job.TaskNode) {
		for _, node := range nodes {
			ids = append(ids, node.Task.ID)
			walk(node.Children)
		}
	}
	walk(nodes)
	return job.GetLabelsForTaskIDs(db, ids)
}

// AckLine is a single pre-formatted line in the done-ack output plan.
// Building a plan before rendering (instead of inlining fmt.Fprintfs) lets
// the builder apply append/skip conditions (suppress redundant lines,
// suppress Next when a claim fired) without tangling the render into
// nested branches.
type AckLine string

// doneAckOptions carries out-of-band context the plan builder needs that
// isn't part of the domain's DoneContext: whether --claim-next produced a
// Claimed line in this same call (which makes a Next line redundant).
type doneAckOptions struct {
	// suppressNext is true when a successful claim-next claim will be
	// emitted after the ack. The Claimed line already names the same
	// next-work; a separate Next line would duplicate it.
	suppressNext bool
}

func buildDoneAckLines(closed []*job.ClosedResult, alreadyDone []string, finalCtx *job.DoneContext, opts doneAckOptions) []AckLine {
	var lines []AckLine

	single := len(closed) == 1 && len(alreadyDone) == 0 && len(closed[0].CascadeClosed) == 0
	if single {
		c := closed[0]
		lines = append(lines, AckLine(fmt.Sprintf("Done: %s %q", c.ShortID, c.Title)))
	} else if len(closed) == 1 && len(closed[0].CascadeClosed) > 0 && len(alreadyDone) == 0 {
		c := closed[0]
		lines = append(lines, AckLine(fmt.Sprintf("Done: %s %q (and %d subtasks)", c.ShortID, c.Title, len(c.CascadeClosed))))
	} else if len(closed) > 0 {
		lines = append(lines, AckLine(fmt.Sprintf("Closed %d tasks:", len(closed))))
		for _, c := range closed {
			if len(c.CascadeClosed) > 0 {
				lines = append(lines, AckLine(fmt.Sprintf("- Done: %s %q (and %d subtasks)", c.ShortID, c.Title, len(c.CascadeClosed))))
			} else {
				lines = append(lines, AckLine(fmt.Sprintf("- Done: %s %q", c.ShortID, c.Title)))
			}
		}
	}
	if len(alreadyDone) > 0 {
		lines = append(lines, AckLine(fmt.Sprintf("  already done: %s", strings.Join(alreadyDone, ", "))))
	}

	// Surface auto-closed ancestors (leaf-frontier cascade) on every closed
	// result. Track the highest auto-closed ancestor across all results so we
	// can suppress a redundant "All tasks in X complete." that would just
	// restate what the last Auto-closed line already said.
	highestAutoClosed := ""
	for _, c := range closed {
		for _, anc := range c.AutoClosedAncestors {
			lines = append(lines, AckLine(fmt.Sprintf("  Auto-closed: %s %q", anc.ShortID, anc.Title)))
			// AutoClosedAncestors is ordered nearest-parent first, so the
			// last element is the highest. Overwriting on each iteration
			// naturally leaves `highestAutoClosed` pointing at that element.
			highestAutoClosed = anc.ShortID
		}
	}

	if finalCtx == nil {
		return lines
	}
	// Trailing context is only meaningful when we actually closed something;
	// an already-done-only call has nothing to say about "what's next."
	if len(closed) == 0 {
		return lines
	}

	ctx := finalCtx

	// Improvement 1: suppress "All tasks in X complete." when X equals the
	// highest auto-closed ancestor. The Auto-closed line already emitted
	// conveys the same "this whole thing just closed" signal; saying it
	// twice is noise.
	if ctx.WholeTreeComplete && ctx.WholeTreeRootID != highestAutoClosed {
		lines = append(lines, AckLine(fmt.Sprintf("  All tasks in %s complete. (%d done, 0 open)", ctx.WholeTreeRootID, ctx.WholeTreeDoneCount)))
	}

	// Improvement 3: suppress Next: when --claim-next already claimed the
	// next work in the same call. The Claimed line names the same target,
	// so a Next line would be a stale duplicate. Skip-blocked info is kept
	// even when suppressing Next — it's context on a different sibling.
	if ctx.SkippedBlocked != nil && ctx.NextSibling != nil {
		lines = append(lines, AckLine(fmt.Sprintf("  Next sibling %s is blocked on %s. Skipping to %s.",
			ctx.SkippedBlocked.ShortID, ctx.SkippedBlockedBy, ctx.NextSibling.ShortID)))
	} else if !opts.suppressNext {
		if ctx.NextSibling != nil {
			lines = append(lines, AckLine(fmt.Sprintf("  Next: %s %q", ctx.NextSibling.ShortID, ctx.NextSibling.Title)))
		} else if ctx.NextAfterParent != nil {
			// Parent auto-closed; surface the next work past it.
			lines = append(lines, AckLine(fmt.Sprintf("  Next: %s %q", ctx.NextAfterParent.ShortID, ctx.NextAfterParent.Title)))
		}
	}

	// Only show "Parent X: N of M complete" when the parent is still open.
	// An auto-closed parent has nothing meaningful to report here.
	if ctx.ParentID != "" && !ctx.ParentWasDone && !ctx.ParentAutoClosed {
		lines = append(lines, AckLine(fmt.Sprintf("  Parent %s: %d of %d complete", ctx.ParentID, ctx.ParentDoneCount, ctx.ParentTotalCount)))
	}

	return lines
}

func renderDoneAck(w io.Writer, closed []*job.ClosedResult, alreadyDone []string, finalCtx *job.DoneContext) {
	renderDoneAckWithOptions(w, closed, alreadyDone, finalCtx, doneAckOptions{})
}

func renderDoneAckWithOptions(w io.Writer, closed []*job.ClosedResult, alreadyDone []string, finalCtx *job.DoneContext, opts doneAckOptions) {
	for _, line := range buildDoneAckLines(closed, alreadyDone, finalCtx, opts) {
		fmt.Fprintln(w, string(line))
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

func renderDoneJSON(w io.Writer, closed []*job.ClosedResult, alreadyDone []string, ctx *job.DoneContext) error {
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
