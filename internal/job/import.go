package job

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

type ImportedTask struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Parent    string   `json:"parent"`
	BlockedBy []string `json:"blocked_by,omitempty"`
}

type ImportResult struct {
	DryRun bool           `json:"dry_run"`
	Tasks  []ImportedTask `json:"tasks"`
}

// parsedTask is the intermediate tree after YAML decode + validation.
type parsedTask struct {
	Title     string
	Desc      string
	Labels    []string
	Ref       string
	BlockedBy []string
	Children  []*parsedTask

	// pathLabel is the YAML path like "tasks[1].children[0]"; filled during validation.
	pathLabel string
	// index in the flat pre-order DFS walk; filled during validation.
	flatIndex int
}

// Raw YAML shape: we decode into a loose structure first so we can enforce
// additionalProperties=false and emit precise path-based errors.
type rawTask struct {
	Title     string     `yaml:"title"`
	Desc      string     `yaml:"desc"`
	Labels    []string   `yaml:"labels"`
	Ref       string     `yaml:"ref"`
	BlockedBy []string   `yaml:"blockedBy"`
	Children  []*rawTask `yaml:"children"`

	// Set when a title key was present in the YAML at all (even empty).
	titlePresent bool
}

// Custom unmarshal to track whether title was explicitly provided.
func (r *rawTask) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("expected mapping, got %v", n.Kind)
	}
	for i := 0; i < len(n.Content); i += 2 {
		k := n.Content[i]
		v := n.Content[i+1]
		switch k.Value {
		case "title":
			r.titlePresent = true
			if err := v.Decode(&r.Title); err != nil {
				return err
			}
		case "desc":
			if err := v.Decode(&r.Desc); err != nil {
				return err
			}
		case "labels":
			if err := v.Decode(&r.Labels); err != nil {
				return err
			}
		case "ref":
			if err := v.Decode(&r.Ref); err != nil {
				return err
			}
		case "blockedBy":
			if err := v.Decode(&r.BlockedBy); err != nil {
				return err
			}
		case "children":
			if err := v.Decode(&r.Children); err != nil {
				return err
			}
		default:
			// Unknown key: ignore silently. JSON Schema declares additionalProperties=false,
			// but Phase 2 is lenient here — makes forward-compat (labels, future keys) painless.
		}
	}
	return nil
}

type rawRoot struct {
	Tasks []*rawTask `yaml:"tasks"`
}

var fenceOpenRe = regexp.MustCompile(`^(` + "```" + `|~~~)([a-zA-Z0-9_+-]*)\s*$`)

// extractTasksYAML scans raw Markdown text for fenced code blocks and returns the
// body of the first block whose YAML decode yields a top-level map with a `tasks` key.
// If no block matches and at least one candidate fence produced a YAML parse error,
// that error is returned so callers can surface it instead of a generic message.
func extractTasksYAML(content string) (string, bool, error) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	scanner.Buffer(make([]byte, 64*1024), 1<<20)

	var (
		inFence bool
		fence   string
		curBody strings.Builder
		curLang string
		lastErr error

		tryBlock = func(lang, body string) (string, bool) {
			// Only consider yaml/yml/unlabeled fences as candidates.
			if lang != "" && lang != "yaml" && lang != "yml" {
				return "", false
			}
			var probe map[string]any
			if err := yaml.Unmarshal([]byte(body), &probe); err != nil {
				lastErr = err
				return "", false
			}
			if _, ok := probe["tasks"]; !ok {
				return "", false
			}
			return body, true
		}
	)

	for scanner.Scan() {
		line := scanner.Text()
		if !inFence {
			if m := fenceOpenRe.FindStringSubmatch(line); m != nil {
				inFence = true
				fence = m[1]
				curLang = m[2]
				curBody.Reset()
			}
			continue
		}
		// Closing fence must match opener (``` or ~~~).
		trimmed := strings.TrimRight(line, " \t")
		if trimmed == fence {
			body := curBody.String()
			if got, ok := tryBlock(curLang, body); ok {
				return got, true, nil
			}
			inFence = false
			fence = ""
			curLang = ""
			curBody.Reset()
			continue
		}
		curBody.WriteString(line)
		curBody.WriteByte('\n')
	}
	return "", false, lastErr
}

func RunImport(db *sql.DB, filePath, parentShortID string, dryRun bool, actor string) (*ImportResult, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	yamlBody, ok, parseErr := extractTasksYAML(string(data))
	if !ok {
		if parseErr != nil {
			return nil, fmt.Errorf("YAML parse error in %s: %w", filePath, parseErr)
		}
		return nil, fmt.Errorf("no YAML `tasks:` block found in %s", filePath)
	}

	var raw rawRoot
	if err := yaml.Unmarshal([]byte(yamlBody), &raw); err != nil {
		return nil, fmt.Errorf("YAML parse error: %s", err.Error())
	}

	// Phase A: convert raw → parsed tree with YAML-path labels, and validate.
	tree, flat, err := buildParsedTree(raw.Tasks)
	if err != nil {
		return nil, err
	}
	if err := validateRefs(flat); err != nil {
		return nil, err
	}

	// Validate --parent target (before any writes).
	var parentTask *Task
	if parentShortID != "" {
		parentTask, err = GetTaskByShortID(db, parentShortID)
		if err != nil {
			return nil, err
		}
		if parentTask == nil {
			return nil, fmt.Errorf("parent task %q not found", parentShortID)
		}
	}

	// Build title index for blockedBy resolution (local to the import).
	titleCounts := make(map[string]int)
	for _, p := range flat {
		titleCounts[p.Title]++
	}
	refIndex := make(map[string]*parsedTask)
	for _, p := range flat {
		if p.Ref != "" {
			refIndex[p.Ref] = p
		}
	}
	titleIndex := make(map[string]*parsedTask)
	for _, p := range flat {
		if titleCounts[p.Title] == 1 {
			titleIndex[p.Title] = p
		}
	}

	// Pre-resolve blockedBy — some resolutions hit existing DB rows.
	// Resolution targets: refIndex, titleIndex, or existing DB short ID.
	// The plan: resolve to either a *parsedTask (local) or a *Task (existing DB row).
	type resolved struct {
		local  *parsedTask
		dbTask *Task
	}
	blockedByResolved := make(map[*parsedTask][]resolved)
	for _, p := range flat {
		if len(p.BlockedBy) == 0 {
			continue
		}
		list := make([]resolved, 0, len(p.BlockedBy))
		for i, entry := range p.BlockedBy {
			if t, ok := refIndex[entry]; ok {
				list = append(list, resolved{local: t})
				continue
			}
			if cnt := titleCounts[entry]; cnt >= 2 {
				return nil, fmt.Errorf(
					"%s: blockedBy[%d] %q matches multiple tasks; use a ref or a short ID to disambiguate",
					p.pathLabel, i, entry,
				)
			}
			if t, ok := titleIndex[entry]; ok {
				list = append(list, resolved{local: t})
				continue
			}
			// Try existing DB short ID.
			existing, err := GetTaskByShortID(db, entry)
			if err != nil {
				return nil, err
			}
			if existing != nil {
				list = append(list, resolved{dbTask: existing})
				continue
			}
			return nil, fmt.Errorf(
				"%s: blockedBy[%d] %q does not match any ref, imported task title, or existing task ID",
				p.pathLabel, i, entry,
			)
		}
		blockedByResolved[p] = list
	}

	// Build echo order. If dry-run, emit placeholders.
	result := &ImportResult{DryRun: dryRun}
	if dryRun {
		for i, p := range flat {
			parent := ""
			if p != nil && isRoot(flat, tree, p) && parentTask != nil {
				parent = parentTask.ShortID
			} else if pp := findParsedParent(tree, p); pp != nil {
				parent = fmt.Sprintf("<new-%d>", pp.flatIndex+1)
			}
			var blockedBy []string
			for _, r := range blockedByResolved[p] {
				if r.local != nil {
					blockedBy = append(blockedBy, fmt.Sprintf("<new-%d>", r.local.flatIndex+1))
				} else if r.dbTask != nil {
					blockedBy = append(blockedBy, r.dbTask.ShortID)
				}
			}
			result.Tasks = append(result.Tasks, ImportedTask{
				ID:        fmt.Sprintf("<new-%d>", i+1),
				Title:     p.Title,
				Parent:    parent,
				BlockedBy: blockedBy,
			})
		}
		return result, nil
	}

	// Phase B: single transaction.
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	shortIDByParsed := make(map[*parsedTask]string)
	dbIDByParsed := make(map[*parsedTask]int64)

	// Insert in pre-order DFS so parents exist before children. Each node
	// receives an explicit sort_order so findNextSibling's strict-greater
	// comparison can distinguish imported siblings.
	var insert func(node *parsedTask, parentDBID *int64, parentShort string, sortOrder int64) error
	insert = func(node *parsedTask, parentDBID *int64, parentShort string, sortOrder int64) error {
		sid, err := generateShortID(tx)
		if err != nil {
			return err
		}
		now := CurrentNowFunc().Unix()
		var id int64
		err = tx.QueryRow(`
			INSERT INTO tasks (short_id, parent_id, title, description, status, sort_order, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'available', ?, ?, ?)
			RETURNING id
		`, sid, parentDBID, node.Title, node.Desc, sortOrder, now, now).Scan(&id)
		if err != nil {
			return err
		}
		shortIDByParsed[node] = sid
		dbIDByParsed[node] = id

		if err := recordEvent(tx, id, "created", actor, map[string]any{
			"parent_id":   parentShort,
			"title":       node.Title,
			"description": node.Desc,
			"sort_order":  sortOrder,
		}); err != nil {
			return err
		}

		if len(node.Labels) > 0 {
			added, _, err := insertLabels(tx, id, node.Labels)
			if err != nil {
				return err
			}
			if len(added) > 0 {
				if err := recordEvent(tx, id, "labeled", actor, map[string]any{
					"names":    added,
					"existing": []string{},
				}); err != nil {
					return err
				}
			}
		}

		result.Tasks = append(result.Tasks, ImportedTask{
			ID:     sid,
			Title:  node.Title,
			Parent: parentShort,
		})

		// Children of this just-inserted node have no pre-existing
		// siblings in the DB, so they start at sort_order 0.
		for i, child := range node.Children {
			cid := id
			if err := insert(child, &cid, sid, int64(i)); err != nil {
				return err
			}
		}
		return nil
	}

	var rootParentDBID *int64
	rootParentShort := ""
	if parentTask != nil {
		pid := parentTask.ID
		rootParentDBID = &pid
		rootParentShort = parentTask.ShortID
	}

	// Offset the import's roots by any pre-existing siblings so we don't
	// collide with existing tasks under the target parent (or at DB root
	// when --parent is omitted).
	rootSortOffset, err := nextSortOrderForParent(tx, rootParentDBID)
	if err != nil {
		return nil, err
	}
	for i, root := range tree {
		if err := insert(root, rootParentDBID, rootParentShort, rootSortOffset+int64(i)); err != nil {
			return nil, err
		}
	}

	// Resolve blockedBy after all inserts (forward references).
	for parsed, list := range blockedByResolved {
		blockedDBID := dbIDByParsed[parsed]
		for _, r := range list {
			var blockerDBID int64
			if r.local != nil {
				blockerDBID = dbIDByParsed[r.local]
			} else {
				blockerDBID = r.dbTask.ID
			}
			if _, err := tx.Exec(
				"INSERT OR IGNORE INTO blocks (blocker_id, blocked_id) VALUES (?, ?)",
				blockerDBID, blockedDBID,
			); err != nil {
				return nil, err
			}
			var blockerShort, blockedShort string
			blockedShort = shortIDByParsed[parsed]
			if r.local != nil {
				blockerShort = shortIDByParsed[r.local]
			} else {
				blockerShort = r.dbTask.ShortID
			}
			if err := recordEvent(tx, blockedDBID, "blocked", actor, map[string]any{
				"blocked_id": blockedShort,
				"blocker_id": blockerShort,
			}); err != nil {
				return nil, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

// nextSortOrderForParent returns one past the current max sort_order
// among live children of parentDBID (or root-level tasks when nil).
// Used by RunImport to avoid collisions with pre-existing siblings.
func nextSortOrderForParent(tx dbtx, parentDBID *int64) (int64, error) {
	var maxSort sql.NullInt64
	var err error
	if parentDBID == nil {
		err = tx.QueryRow(
			"SELECT MAX(sort_order) FROM tasks WHERE parent_id IS NULL AND deleted_at IS NULL",
		).Scan(&maxSort)
	} else {
		err = tx.QueryRow(
			"SELECT MAX(sort_order) FROM tasks WHERE parent_id = ? AND deleted_at IS NULL",
			*parentDBID,
		).Scan(&maxSort)
	}
	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}
	if !maxSort.Valid {
		return 0, nil
	}
	return maxSort.Int64 + 1, nil
}

// buildParsedTree converts the raw YAML tree into parsed tasks, assigning YAML-path
// labels (e.g. `tasks[0].children[1]`) and a flat pre-order index. Rejects rows
// missing the title key.
func buildParsedTree(rawList []*rawTask) ([]*parsedTask, []*parsedTask, error) {
	var flat []*parsedTask
	var walk func(list []*rawTask, parentPath string) ([]*parsedTask, error)
	walk = func(list []*rawTask, parentPath string) ([]*parsedTask, error) {
		var out []*parsedTask
		for i, r := range list {
			var path string
			if parentPath == "" {
				path = fmt.Sprintf("tasks[%d]", i)
			} else {
				path = fmt.Sprintf("%s.children[%d]", parentPath, i)
			}
			if !r.titlePresent || strings.TrimSpace(r.Title) == "" {
				return nil, fmt.Errorf("%s: title is required", path)
			}
			labels, lerr := validateImportLabels(path, r.Labels)
			if lerr != nil {
				return nil, lerr
			}
			p := &parsedTask{
				Title:     r.Title,
				Desc:      r.Desc,
				Labels:    labels,
				Ref:       r.Ref,
				BlockedBy: r.BlockedBy,
				pathLabel: path,
				flatIndex: len(flat),
			}
			flat = append(flat, p)
			children, err := walk(r.Children, path)
			if err != nil {
				return nil, err
			}
			p.Children = children
			out = append(out, p)
		}
		return out, nil
	}
	tree, err := walk(rawList, "")
	if err != nil {
		return nil, nil, err
	}
	return tree, flat, nil
}

// validateImportLabels normalizes a per-task labels list using the same rules
// the CLI applies (trim whitespace, reject empty, reject commas), and dedupes
// preserving first-seen order. Errors include the YAML path so users can
// locate the offending entry.
func validateImportLabels(path string, raw []string) ([]string, error) {
	seen := make(map[string]bool)
	out := make([]string, 0, len(raw))
	for i, r := range raw {
		name, err := validateLabelName(r)
		if err != nil {
			return nil, fmt.Errorf("%s: labels[%d]: %s", path, i, err.Error())
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out, nil
}

// validateRefs ensures refs are unique across the import.
func validateRefs(flat []*parsedTask) error {
	seen := make(map[string]*parsedTask)
	for _, p := range flat {
		if p.Ref == "" {
			continue
		}
		if prior, ok := seen[p.Ref]; ok {
			return fmt.Errorf("%s: ref %q is already used at %s", p.pathLabel, p.Ref, prior.pathLabel)
		}
		seen[p.Ref] = p
	}
	return nil
}

func isRoot(flat []*parsedTask, tree []*parsedTask, p *parsedTask) bool {
	return slices.Contains(tree, p)
}

func findParsedParent(tree []*parsedTask, target *parsedTask) *parsedTask {
	var walk func(node *parsedTask) *parsedTask
	walk = func(node *parsedTask) *parsedTask {
		for _, c := range node.Children {
			if c == target {
				return node
			}
			if found := walk(c); found != nil {
				return found
			}
		}
		return nil
	}
	for _, root := range tree {
		if root == target {
			return nil
		}
		if found := walk(root); found != nil {
			return found
		}
	}
	return nil
}
