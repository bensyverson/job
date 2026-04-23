package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	job "github.com/bensyverson/job/internal/job"
	"github.com/spf13/cobra"
)

// notePreviewMax is the maximum rune count of the preview shown on the
// note ack line. Long enough to give the user confidence the right body
// landed; short enough to keep the ack on one line in a normal terminal.
const notePreviewMax = 60

// notePreview builds the body preview shown on a successful `note` ack.
// Returns the raw rune count of the stored body and a single-line
// preview clamped to notePreviewMax runes. The preview collapses
// newlines and tabs to spaces so the ack stays one line; on overflow it
// snaps to the last space in the back third of the window before
// appending an ellipsis, so words don't get chopped mid-token.
func notePreview(body string) (count int, preview string) {
	count = utf8.RuneCountInString(body)

	collapsed := strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\r', '\t':
			return ' '
		default:
			return r
		}
	}, body)

	runes := []rune(collapsed)
	if len(runes) <= notePreviewMax {
		return count, strings.TrimRight(string(runes), " ")
	}

	cut := notePreviewMax
	for i := cut - 1; i >= notePreviewMax*2/3; i-- {
		if runes[i] == ' ' {
			cut = i
			break
		}
	}
	return count, strings.TrimRight(string(runes[:cut]), " ") + "…"
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

			if err := job.RunNote(db, shortID, text, resultRaw, actor); err != nil {
				return err
			}
			count, preview := notePreview(text)
			fmt.Fprintf(cmd.OutOrStdout(), "Noted: %s · %d chars · %q\n", shortID, count, preview)
			return nil
		},
	}
	cmd.Flags().StringVarP(&message, "message", "m", "", "note text to append")
	cmd.Flags().StringVar(&resultStr, "result", "", "structured JSON result recorded on the noted event")
	return cmd
}
