package job

import (
	"fmt"
	"io"
)

// schemaJSON describes the `job import` grammar as a JSON Schema (Draft 2020-12).
// Hand-authored so the documented semantics (required title, string-arrays, the
// "reserved; not yet persisted" note on labels) don't drift with Go-struct quirks.
const schemaJSON = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://github.com/bensyverson/jobs/schema/import.json",
  "title": "job import grammar",
  "description": "Root document for ` + "`" + `job import` + "`" + `. Parses the first fenced YAML block in a Markdown file whose top-level key is ` + "`" + `tasks` + "`" + `.",
  "type": "object",
  "required": ["tasks"],
  "properties": {
    "tasks": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["title"],
        "properties": {
          "title": {
            "type": "string",
            "description": "Required. Human-readable task title."
          },
          "desc": {
            "type": "string",
            "description": "Optional free-text description. YAML block scalars ('|', '>') are supported."
          },
          "labels": {
            "type": "array",
            "items": { "type": "string" },
            "description": "Free-form labels. Persisted; queryable via ` + "`" + `list --label` + "`" + `."
          },
          "ref": {
            "type": "string",
            "description": "Optional author-chosen handle, unique across the entire import. Used by blockedBy entries elsewhere in this document. Refs are not persisted on task rows."
          },
          "blockedBy": {
            "type": "array",
            "items": { "type": "string" },
            "description": "Optional list. Each entry must resolve in order: (1) a ref defined elsewhere in this import; (2) a verbatim task title elsewhere in this import (must be unambiguous); (3) a pre-existing short ID in the database."
          },
          "children": {
            "type": "array",
            "items": { "$ref": "#/properties/tasks/items" },
            "description": "Optional sub-tasks, recursive. Same grammar as the root task entries."
          }
        },
        "additionalProperties": false
      }
    }
  }
}`

func RunSchema(w io.Writer) error {
	if _, err := fmt.Fprintln(w, schemaJSON); err != nil {
		return err
	}
	return nil
}
