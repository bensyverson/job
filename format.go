package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

type taskNodeJSON struct {
	ID             string         `json:"id"`
	Title          string         `json:"title"`
	Status         string         `json:"status"`
	Description    string         `json:"description"`
	ClaimedBy      *string        `json:"claimed_by,omitempty"`
	ClaimExpiresAt *int64         `json:"claim_expires_at,omitempty"`
	Children       []taskNodeJSON `json:"children"`
}

func formatTaskNodesJSON(nodes []*TaskNode) ([]byte, error) {
	var result []taskNodeJSON
	for _, node := range nodes {
		result = append(result, taskNodeToJSON(node))
	}
	return json.MarshalIndent(result, "", "  ")
}

func taskNodeToJSON(node *TaskNode) taskNodeJSON {
	var children []taskNodeJSON
	for _, child := range node.Children {
		children = append(children, taskNodeToJSON(child))
	}
	return taskNodeJSON{
		ID:             node.Task.ShortID,
		Title:          node.Task.Title,
		Status:         node.Task.Status,
		Description:    node.Task.Description,
		ClaimedBy:      node.Task.ClaimedBy,
		ClaimExpiresAt: node.Task.ClaimExpiresAt,
		Children:       children,
	}
}

func renderMarkdownList(w io.Writer, nodes []*TaskNode, blockers map[string][]string, depth int) {
	indent := strings.Repeat("  ", depth)
	for _, node := range nodes {
		fmt.Fprintf(w, "%s- %s  %s", indent, node.Task.ShortID, node.Task.Title)
		if node.Task.Status == "done" {
			if node.Task.CompletionNote != nil && *node.Task.CompletionNote != "" {
				fmt.Fprintf(w, "  [done, %s]", *node.Task.CompletionNote)
			} else {
				fmt.Fprintf(w, "  [done]")
			}
		} else if node.Task.Status == "claimed" {
			claimedBy := ""
			if node.Task.ClaimedBy != nil {
				claimedBy = " by " + *node.Task.ClaimedBy
			}
			if node.Task.ClaimExpiresAt != nil {
				remaining := *node.Task.ClaimExpiresAt - nowUnix()
				if remaining > 0 {
					fmt.Fprintf(w, "  [claimed%s, %s left]", claimedBy, formatDuration(remaining))
				} else {
					fmt.Fprintf(w, "  [claimed%s]", claimedBy)
				}
			} else {
				fmt.Fprintf(w, "  [claimed%s]", claimedBy)
			}
		} else if blks, ok := blockers[node.Task.ShortID]; ok && len(blks) > 0 {
			fmt.Fprintf(w, "  [blocked by %s]", strings.Join(blks, ", "))
		}
		fmt.Fprintln(w)
		renderMarkdownList(w, node.Children, blockers, depth+1)
	}
}

func formatDuration(seconds int64) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	if seconds < 3600 {
		return fmt.Sprintf("%dm", seconds/60)
	}
	if seconds < 86400 {
		return fmt.Sprintf("%dh", seconds/3600)
	}
	return fmt.Sprintf("%dd", seconds/86400)
}

func nowUnix() int64 {
	return currentNowFunc().Unix()
}

func renderInfoMarkdown(w io.Writer, info *TaskInfo) {
	fmt.Fprintf(w, "ID:           %s\n", info.Task.ShortID)
	fmt.Fprintf(w, "Title:        %s\n", info.Task.Title)
	if info.Task.Description != "" {
		fmt.Fprintf(w, "Description:  %s\n", info.Task.Description)
	}
	fmt.Fprintf(w, "Status:       %s\n", info.Task.Status)
	if info.Task.Status == "claimed" {
		fmt.Fprintf(w, "Claim:        %s\n", formatClaimExpires(info.Task.ClaimedBy, info.Task.ClaimExpiresAt))
	}
	if info.Parent != nil {
		fmt.Fprintf(w, "Parent:       %s (%s)\n", info.Parent.ShortID, info.Parent.Title)
	} else {
		fmt.Fprintf(w, "Parent:       (root)\n")
	}
	if len(info.Children) > 0 {
		done := 0
		for _, c := range info.Children {
			if c.Status == "done" {
				done++
			}
		}
		fmt.Fprintf(w, "Children:     %d", len(info.Children))
		if done > 0 {
			fmt.Fprintf(w, " (%d done", done)
			remaining := len(info.Children) - done
			if remaining > 0 {
				fmt.Fprintf(w, ", %d open", remaining)
			}
			fmt.Fprintf(w, ")")
		}
		fmt.Fprintln(w)
	}
	if len(info.Blockers) > 0 {
		var ids []string
		for _, b := range info.Blockers {
			ids = append(ids, b.ShortID)
		}
		fmt.Fprintf(w, "Blocking:     %s\n", strings.Join(ids, ", "))
	}
	fmt.Fprintf(w, "Created:      %s\n", formatTimestamp(info.Task.CreatedAt))
}

func renderInfoJSON(w io.Writer, info *TaskInfo) {
	type infoJSON struct {
		ID          string   `json:"id"`
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Status      string   `json:"status"`
		Parent      *string  `json:"parent,omitempty"`
		Children    int      `json:"children"`
		Blockers    []string `json:"blockers,omitempty"`
		CreatedAt   int64    `json:"created_at"`
	}

	var parentID *string
	if info.Parent != nil {
		parentID = &info.Parent.ShortID
	}
	var blockers []string
	for _, b := range info.Blockers {
		blockers = append(blockers, b.ShortID)
	}

	obj := infoJSON{
		ID:          info.Task.ShortID,
		Title:       info.Task.Title,
		Description: info.Task.Description,
		Status:      info.Task.Status,
		Parent:      parentID,
		Children:    len(info.Children),
		Blockers:    blockers,
		CreatedAt:   info.Task.CreatedAt,
	}
	b, _ := json.MarshalIndent(obj, "", "  ")
	w.Write(b)
}

func formatTimestamp(unix int64) string {
	return time.Unix(unix, 0).Format("2006-01-02 15:04")
}

func renderTaskJSON(w io.Writer, task *Task) {
	obj := taskNodeJSON{
		ID:             task.ShortID,
		Title:          task.Title,
		Status:         task.Status,
		Description:    task.Description,
		ClaimedBy:      task.ClaimedBy,
		ClaimExpiresAt: task.ClaimExpiresAt,
	}
	b, _ := json.MarshalIndent(obj, "", "  ")
	w.Write(b)
}

func renderNextText(w io.Writer, task *Task) {
	fmt.Fprintf(w, "%s  %s\n", task.ShortID, task.Title)
	if task.Description != "" {
		fmt.Fprintf(w, "\n  %s\n", task.Description)
	}
}

func renderEventLogMarkdown(w io.Writer, events []EventEntry) {
	for _, e := range events {
		ts := formatTimestamp(e.CreatedAt)
		desc := formatEventDescription(e.EventType, e.Detail)
		fmt.Fprintf(w, "[%s] %s %s\n", ts, e.ShortID, desc)
	}
}

func formatEventDescription(eventType, detailJSON string) string {
	var detail map[string]any
	if detailJSON != "" {
		json.Unmarshal([]byte(detailJSON), &detail)
	}

	switch eventType {
	case "created":
		title := ""
		if detail != nil {
			if t, ok := detail["title"].(string); ok {
				title = t
			}
		}
		return fmt.Sprintf("created: %q", title)
	case "claimed":
		by := ""
		dur := "1h"
		if detail != nil {
			if b, ok := detail["by"].(string); ok && b != "" {
				by = " by " + b
			}
			if d, ok := detail["duration"].(string); ok && d != "" {
				dur = d
			}
		}
		return fmt.Sprintf("claimed%s (%s)", by, dur)
	case "released":
		wasBy := ""
		if detail != nil {
			if w, ok := detail["was_claimed_by"].(string); ok && w != "" {
				wasBy = " (was claimed by " + w + ")"
			}
		}
		return fmt.Sprintf("released%s", wasBy)
	case "claim_expired":
		wasBy := ""
		if detail != nil {
			if w, ok := detail["was_claimed_by"].(string); ok && w != "" {
				wasBy = " (was claimed by " + w + ")"
			}
		}
		return fmt.Sprintf("claim expired%s", wasBy)
	case "done":
		parts := []string{"done"}
		force := false
		if detail != nil {
			if f, ok := detail["force"].(bool); ok && f {
				force = true
			}
			if note, ok := detail["note"].(string); ok && note != "" {
				parts = append(parts, "note: "+note)
			}
			if children, ok := detail["force_closed_children"].([]any); ok && len(children) > 0 {
				parts = append(parts, fmt.Sprintf("and %d subtasks", len(children)))
			}
		}
		if force {
			parts[0] = "done --force"
		}
		return strings.Join(parts, " (") + strings.Repeat(")", len(parts)-1)
	case "reopened":
		if detail != nil {
			if children, ok := detail["reopened_children"].([]any); ok && len(children) > 0 {
				return fmt.Sprintf("reopened (and %d subtasks)", len(children))
			}
		}
		return "reopened"
	case "noted":
		text := ""
		if detail != nil {
			if t, ok := detail["text"].(string); ok {
				text = t
			}
		}
		return fmt.Sprintf("noted: %q", text)
	case "edited":
		oldTitle := ""
		newTitle := ""
		if detail != nil {
			if o, ok := detail["old_title"].(string); ok {
				oldTitle = o
			}
			if n, ok := detail["new_title"].(string); ok {
				newTitle = n
			}
		}
		return fmt.Sprintf("edited: %q -> %q", oldTitle, newTitle)
	case "blocked":
		blockerID := ""
		if detail != nil {
			if b, ok := detail["blocker_id"].(string); ok {
				blockerID = b
			}
		}
		return fmt.Sprintf("blocked by %s", blockerID)
	case "unblocked":
		blockerID := ""
		reason := ""
		if detail != nil {
			if b, ok := detail["blocker_id"].(string); ok {
				blockerID = b
			}
			if r, ok := detail["reason"].(string); ok {
				reason = " (" + r + ")"
			}
		}
		return fmt.Sprintf("unblocked from %s%s", blockerID, reason)
	case "moved":
		direction := ""
		relativeTo := ""
		if detail != nil {
			if d, ok := detail["direction"].(string); ok {
				direction = d
			}
			if r, ok := detail["relative_to"].(string); ok {
				relativeTo = r
			}
		}
		return fmt.Sprintf("moved %s %s", direction, relativeTo)
	case "removed":
		parts := []string{"removed"}
		if detail != nil {
			if children, ok := detail["children_removed"].([]any); ok && len(children) > 0 {
				parts = append(parts, fmt.Sprintf("and %d subtasks", len(children)))
			}
		}
		return strings.Join(parts, " (") + strings.Repeat(")", len(parts)-1)
	default:
		return eventType
	}
}

type eventJSON struct {
	ID        int64  `json:"id"`
	TaskID    int64  `json:"task_id"`
	ShortID   string `json:"short_id"`
	EventType string `json:"event_type"`
	Detail    any    `json:"detail"`
	CreatedAt int64  `json:"created_at"`
}

func formatEventLogJSON(events []EventEntry) ([]byte, error) {
	var result []eventJSON
	for _, e := range events {
		var detail any
		if e.Detail != "" {
			var parsed any
			if err := json.Unmarshal([]byte(e.Detail), &parsed); err == nil {
				detail = parsed
			}
		}
		result = append(result, eventJSON{
			ID:        e.ID,
			TaskID:    e.TaskID,
			ShortID:   e.ShortID,
			EventType: e.EventType,
			Detail:    detail,
			CreatedAt: e.CreatedAt,
		})
	}
	return json.MarshalIndent(result, "", "  ")
}
