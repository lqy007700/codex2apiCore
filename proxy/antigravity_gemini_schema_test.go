package proxy

import (
	"testing"
)

func TestAntigravityNormalizeMalformedToolSchemaWrapsBareProperties(t *testing.T) {
	root := map[string]any{
		"task_id": map[string]any{
			"type":        "string",
			"description": "Task identifier",
		},
	}
	got := antigravityNormalizeMalformedToolSchema(root)
	props, ok := got["properties"].(map[string]any)
	if !ok || props["task_id"] == nil {
		t.Fatalf("bare property must be wrapped: %#v", got)
	}
	if got["type"] != "object" {
		t.Fatalf("type = %#v", got["type"])
	}
}

func TestAntigravityNormalizeMalformedToolSchemaPromotesBooleanRequired(t *testing.T) {
	root := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"q": map[string]any{
				"type":     "string",
				"required": true,
			},
		},
	}
	got := antigravityNormalizeMalformedToolSchema(root)
	required := antigravityStringSlice(got["required"])
	if len(required) != 1 || required[0] != "q" {
		t.Fatalf("required = %#v", got["required"])
	}
	q, _ := got["properties"].(map[string]any)["q"].(map[string]any)
	if _, ok := q["required"]; ok {
		t.Fatalf("boolean required must be promoted: %#v", q)
	}
}

func TestAntigravityFinalizeToolSchemaDropsEnumAndAddsArrayItems(t *testing.T) {
	got := antigravityFinalizeToolSchema(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"tags": map[string]any{"type": "array"},
			"mode": map[string]any{"type": "string", "enum": []any{"a", "b"}},
		},
	})
	tags, _ := got["properties"].(map[string]any)["tags"].(map[string]any)
	if tags["items"] == nil {
		t.Fatalf("array items missing: %#v", tags)
	}
	mode, _ := got["properties"].(map[string]any)["mode"].(map[string]any)
	if _, ok := mode["enum"]; ok {
		t.Fatalf("enum must be dropped: %#v", mode)
	}
	if mode["description"] == "" {
		t.Fatal("enum hint missing from description")
	}
}
