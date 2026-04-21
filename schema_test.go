package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestSchema_ValidJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := runSchema(&buf); err != nil {
		t.Fatalf("runSchema: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("schema is not valid JSON: %v\n%s", err, buf.String())
	}
	if len(buf.Bytes()) == 0 || buf.Bytes()[buf.Len()-1] != '\n' {
		t.Error("schema output should end with a newline")
	}
}

func TestSchema_DeclaresRequiredTitle(t *testing.T) {
	var buf bytes.Buffer
	if err := runSchema(&buf); err != nil {
		t.Fatalf("runSchema: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(buf.Bytes(), &schema); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema: missing top-level properties")
	}
	tasks, ok := props["tasks"].(map[string]any)
	if !ok {
		t.Fatal("schema: missing properties.tasks")
	}
	if tasks["type"] != "array" {
		t.Errorf("schema: properties.tasks.type = %v, want array", tasks["type"])
	}
	items, ok := tasks["items"].(map[string]any)
	if !ok {
		t.Fatal("schema: missing properties.tasks.items")
	}
	required, ok := items["required"].([]any)
	if !ok {
		t.Fatal("schema: missing properties.tasks.items.required")
	}
	found := false
	for _, r := range required {
		if s, _ := r.(string); s == "title" {
			found = true
		}
	}
	if !found {
		t.Errorf("schema: items.required must contain \"title\", got %v", required)
	}

	itemProps, ok := items["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema: missing items.properties")
	}
	blockedBy, ok := itemProps["blockedBy"].(map[string]any)
	if !ok {
		t.Fatal("schema: missing items.properties.blockedBy")
	}
	if blockedBy["type"] != "array" {
		t.Errorf("blockedBy.type = %v, want array", blockedBy["type"])
	}
	bItems, ok := blockedBy["items"].(map[string]any)
	if !ok {
		t.Fatal("blockedBy.items missing")
	}
	if bItems["type"] != "string" {
		t.Errorf("blockedBy.items.type = %v, want string", bItems["type"])
	}

	children, ok := itemProps["children"].(map[string]any)
	if !ok {
		t.Fatal("schema: missing items.properties.children")
	}
	if children["type"] != "array" {
		t.Errorf("children.type = %v, want array", children["type"])
	}
}
