package proxy

import (
	"testing"
)

func TestAntigravityFlattenSchemaUnionsAnyOf(t *testing.T) {
	root := map[string]any{
		"type": "object",
		"anyOf": []any{
			map[string]any{"type": "null"},
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"q": map[string]any{"type": "string"},
				},
			},
		},
	}
	flattened, ok := antigravityFlattenSchemaUnions(root).(map[string]any)
	if !ok {
		t.Fatal("expected object schema")
	}
	if _, ok := flattened["anyOf"]; ok {
		t.Fatalf("anyOf must be flattened: %#v", flattened)
	}
	props, ok := flattened["properties"].(map[string]any)
	if !ok || props["q"] == nil {
		t.Fatalf("expected merged properties: %#v", flattened)
	}
	if flattened["nullable"] != true {
		t.Fatalf("expected nullable true: %#v", flattened)
	}
}

func TestAntigravityApplyGeminiDeclarationSchemaUsesParametersJsonSchema(t *testing.T) {
	decl := map[string]any{
		"name": "lookup",
		"parametersJsonSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"q": map[string]any{"type": "string"},
			},
		},
	}
	antigravityApplyGeminiDeclarationSchema(decl)
	if _, ok := decl["parameters"]; ok {
		t.Fatal("parameters must be removed")
	}
	schema, ok := decl["parametersJsonSchema"].(map[string]any)
	if !ok || schema["type"] != "object" {
		t.Fatalf("schema = %#v", decl)
	}
}
