package main

import (
	"encoding/json"
	"fmt"
	job "github.com/bensyverson/jobs/internal/job"
	"github.com/spf13/cobra"
	"strings"
)

func newDoneCmd() *cobra.Command {
	var cascade bool
	var note string
	var resultStr string
	var format string
	var claimNext bool
	var setCriterion []string
	var quiet bool
	cmd := &cobra.Command{
		Use:   "done <id> [<id>...]",
		Short: "Mark one or more tasks as done",
		Long: `Mark one or more tasks as done, atomically. Use --cascade to close a task and all open descendants in one call. Use -m to record a completion note, and --result for structured JSON output. Idempotent: already-done tasks are reported, not re-recorded.

Tip: pass --claim-next to atomically close this task and claim the next available leaf, collapsing the close-then-advance flow into one call. The ack ends with the same briefing that 'job claim' / 'job show' produces, so you don't need a follow-up 'show' on the new claim.`,
		Args: cobra.MinimumNArgs(1),
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

			// Catch the `done <id> "prose"` footgun early with a specific
			// suggestion, before job.RunDone emits a generic "task not found".
			for i, a := range args {
				if !looksLikeShortID(a) {
					return fmt.Errorf("done: %q does not look like a task ID (5-char alphanumeric). Did you mean `-m %q`? (positional arg #%d)",
						a, a, i+1)
				}
			}

			if cmd.Flags().Changed("message") {
				resolved, rerr := resolveMessage(note, cmd.InOrStdin())
				if rerr != nil {
					return rerr
				}
				note = resolved
			}

			var resultRaw json.RawMessage
			if resultStr != "" {
				if !json.Valid([]byte(resultStr)) {
					return fmt.Errorf("--result: invalid JSON: %s", resultStr)
				}
				resultRaw = json.RawMessage(resultStr)
			}

			// Apply --criterion updates before close so the soft pending check
			// reflects the operator's just-recorded state changes.
			for _, kv := range setCriterion {
				label, state, perr := parseCriterionAssignment(kv)
				if perr != nil {
					return perr
				}
				// Apply to the first arg by default. When closing multiple ids,
				// require an explicit "id:label=state" form to disambiguate.
				targetID := args[0]
				labelOnly := label
				if colon := strings.Index(label, ":"); colon > 0 && len(args) > 1 {
					targetID = label[:colon]
					labelOnly = label[colon+1:]
				}
				if _, err := job.RunSetCriterion(db, targetID, labelOnly, state, actor); err != nil {
					return err
				}
			}

			// Soft pending warning: count pending criteria across all closed
			// targets and surface a single line so the operator notices but
			// the close still proceeds.
			pendingByID := map[string]int{}
			for _, id := range args {
				task, _ := job.GetTaskByShortID(db, id)
				if task == nil {
					continue
				}
				n, _ := job.CountPendingCriteria(db, task.ID)
				if n > 0 {
					pendingByID[id] = n
				}
			}

			closed, alreadyDone, err := job.RunDone(db, args, cascade, note, resultRaw, actor)
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
			// job.ComputeDoneContext can distinguish "already-done parent" from
			// "just-auto-closed parent".
			autoClosedSet := make(map[string]bool)
			for _, c := range closed {
				for _, anc := range c.AutoClosedAncestors {
					autoClosedSet[anc.ShortID] = true
				}
			}

			var ctx *job.DoneContext
			if lastCtxID != "" {
				c, cerr := job.ComputeDoneContext(db, lastCtxID, autoClosedSet)
				if cerr != nil {
					return cerr
				}
				ctx = c
			}

			// --claim-next: attempt to claim the next leaf. Done is irreversible,
			// claim is opportunistic — if the leaf got taken between done and
			// claim, we report a status line instead of erroring.
			var claimed *job.Task
			var claimRaceTaken string
			if claimNext && len(closed) > 0 {
				t, cerr := job.RunClaimNextFiltered(db, "", "", actor, false, false)
				if cerr == nil {
					claimed = t
				} else {
					// Distinguish "no work available" from "someone grabbed it".
					// job.RunClaimNext wraps job.RunNext, which emits "No available tasks."
					// on empty. Other errors (race on claim, task not found) we
					// surface as a race status line with whatever detail we have.
					msg := cerr.Error()
					if !strings.Contains(msg, "No available tasks") {
						claimRaceTaken = msg
					}
				}
			}

			if format == "json" {
				return renderDoneJSON(cmd.OutOrStdout(), closed, alreadyDone, ctx)
			}

			// Idempotent single-ID already-done: preserve Phase 3 wording.
			if len(closed) == 0 && len(alreadyDone) == 1 && len(args) == 1 {
				fmt.Fprintf(cmd.OutOrStdout(), "Already done: %s\n", alreadyDone[0])
				return nil
			}

			// Suppress the ack's Next line when --claim-next successfully
			// claimed the next leaf: the Claimed line below already names
			// the target. Race-lost claims still surface Next as a useful
			// fallback ("you didn't get anything, try this instead").
			renderDoneAckWithOptions(cmd.OutOrStdout(), closed, alreadyDone, ctx, doneAckOptions{
				suppressNext: claimed != nil,
				actor:        actor,
			})

			if claimed != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Claimed: %s %q (expires in %s) as=%s\n",
					claimed.ShortID, claimed.Title, job.FormatDuration(job.DefaultClaimTTLSeconds), actor)
				if !quiet {
					renderClaimBriefing(cmd.OutOrStdout(), db, claimed.ShortID)
				}
			} else if claimRaceTaken != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Next leaf unavailable: %s\n", claimRaceTaken)
			}
			for id, n := range pendingByID {
				fmt.Fprintf(cmd.OutOrStdout(), "  Note: %s closed with %d pending criteria.\n", id, n)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&cascade, "cascade", false, "close the target and all open descendants")
	cmd.Flags().StringVarP(&note, "message", "m", "", "record a completion note")
	cmd.Flags().StringVar(&resultStr, "result", "", "structured JSON result recorded on the done event")
	cmd.Flags().StringVar(&format, "format", "md", "output format (md|json)")
	cmd.Flags().BoolVar(&claimNext, "claim-next", false, "after closing, atomically claim the next available leaf")
	cmd.Flags().StringArrayVar(&setCriterion, "criterion", nil, "update an acceptance criterion before close, format \"label=passed\" (repeatable; for multi-id closes use \"id:label=state\")")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "suppress the trailing show briefing on a follow-on --claim-next claim")
	return cmd
}
