package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
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
	return currentNowFunc().Format("2006-01-02 15:04")
}
