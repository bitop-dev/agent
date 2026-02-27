package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bitop-dev/agent/pkg/ai"
)

// stubTool is a minimal Tool for testing ValidateAndCoerce.
type stubTool struct {
	name   string
	schema string // raw JSON Schema
}

func (s stubTool) Definition() ai.ToolDefinition {
	return ai.ToolDefinition{
		Name:        s.name,
		Description: "test tool",
		Parameters:  json.RawMessage(s.schema),
	}
}

func (s stubTool) Execute(_ context.Context, _ string, _ map[string]any, _ UpdateFn) (Result, error) {
	return TextResult("ok"), nil
}

func TestValidateAndCoerce_Valid(t *testing.T) {
	tool := stubTool{name: "t", schema: `{
		"type":"object",
		"properties":{"name":{"type":"string"},"count":{"type":"integer"}},
		"required":["name","count"]
	}`}

	args, err := ValidateAndCoerce(tool, map[string]any{"name": "foo", "count": float64(3)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if args["name"] != "foo" {
		t.Errorf("name = %v, want foo", args["name"])
	}
}

func TestValidateAndCoerce_CoerceStringToNumber(t *testing.T) {
	tool := stubTool{name: "t", schema: `{
		"type":"object",
		"properties":{"offset":{"type":"integer"}},
		"required":["offset"]
	}`}

	// LLM sent "5" (a string) — should be coerced to integer.
	args, err := ValidateAndCoerce(tool, map[string]any{"offset": "5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	switch v := args["offset"].(type) {
	case int64:
		if v != 5 {
			t.Errorf("offset = %v, want 5", v)
		}
	case float64:
		if v != 5 {
			t.Errorf("offset = %v, want 5", v)
		}
	default:
		t.Errorf("offset type = %T, want numeric; value = %v", args["offset"], args["offset"])
	}
}

func TestValidateAndCoerce_CoerceNumberToString(t *testing.T) {
	tool := stubTool{name: "t", schema: `{
		"type":"object",
		"properties":{"path":{"type":"string"}},
		"required":["path"]
	}`}

	// LLM sent 42 for a string field — should be coerced.
	args, err := ValidateAndCoerce(tool, map[string]any{"path": float64(42)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if args["path"] != "42" {
		t.Errorf("path = %v, want \"42\"", args["path"])
	}
}

func TestValidateAndCoerce_MissingRequired(t *testing.T) {
	tool := stubTool{name: "t", schema: `{
		"type":"object",
		"properties":{"name":{"type":"string"}},
		"required":["name"]
	}`}

	_, err := ValidateAndCoerce(tool, map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing required field")
	}
}

func TestValidateAndCoerce_EmptySchema(t *testing.T) {
	tool := stubTool{name: "t", schema: ""}
	args, err := ValidateAndCoerce(tool, map[string]any{"x": 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if args["x"] != 1 {
		t.Errorf("args[x] = %v, want 1", args["x"])
	}
}
