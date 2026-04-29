package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	job "github.com/bensyverson/jobs/internal/job"
	"github.com/spf13/cobra"
)

func newNoteCmd() *cobra.Command {
	var message string
	var resultStr string
	cmd := &cobra.Command{
		Use:   "note <id> [text]",
		Short: "Append a note to a task's description",
		Long:  "Append text to a task's description, prefixed with a timestamp. Pass the body positionally, via -m, or read from stdin with `-`. Use --result to attach a structured JSON blob to the event without touching the description.",
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
			positionalForm := len(args) == 2 && !stdinForm

			hasMessage := cmd.Flags().Changed("message")
			provided := 0
			if hasMessage {
				provided++
			}
			if stdinForm {
				provided++
			}
			if positionalForm {
				provided++
			}
			if provided == 0 {
				return fmt.Errorf("note requires text — pass it positionally, via -m \"<text>\", or via stdin (-)")
			}
			if provided > 1 {
				return fmt.Errorf("note: positional text, -m, and stdin form are mutually exclusive")
			}

			text := message
			if stdinForm {
				b, rerr := io.ReadAll(cmd.InOrStdin())
				if rerr != nil {
					return rerr
				}
				text = strings.TrimRight(string(b), "\n\r")
			} else if positionalForm {
				text = args[1]
			} else if hasMessage {
				resolved, rerr := resolveMessage(message, cmd.InOrStdin())
				if rerr != nil {
					return rerr
				}
				text = resolved
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

			task, err := job.GetTaskByShortID(db, shortID)
			if err != nil {
				return err
			}
			if task == nil {
				return fmt.Errorf("task %q not found", shortID)
			}
			title := task.Title
			if err := job.RunNote(db, shortID, text, resultRaw, actor); err != nil {
				return err
			}
			count, preview := job.NotePreview(text)
			fmt.Fprintf(cmd.OutOrStdout(), "Noted: %s %q\n  note: %d chars · %q\n", shortID, title, count, preview)
			return nil
		},
	}
	cmd.Flags().StringVarP(&message, "message", "m", "", "note text to append")
	cmd.Flags().StringVar(&resultStr, "result", "", "structured JSON result recorded on the noted event")
	return cmd
}
