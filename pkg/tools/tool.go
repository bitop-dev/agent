// Package tools defines the Tool interface, registry, and the external
// subprocess plugin protocol.
package tools

import (
	"context"
	"encoding/json"

	"github.com/bitop-dev/agent/pkg/ai"
)

// ---------------------------------------------------------------------------
// Tool interface
// ---------------------------------------------------------------------------

// Result is the output of a tool execution.
type Result struct {
	// Content is sent back to the LLM (text or images).
	Content []ai.ContentBlock
	// Details is arbitrary structured data for UIs/logging (not sent to LLM).
	Details any
}

// UpdateFn is an optional callback for streaming partial results to a UI.
type UpdateFn func(partial Result)

// Tool is the interface every tool must implement.
// Register it with the Registry; the agent loop calls Execute automatically.
type Tool interface {
	// Definition returns the schema handed to the LLM.
	Definition() ai.ToolDefinition
	// Execute runs the tool. ctx carries the agent's cancel signal.
	// onUpdate may be nil; implementations must guard before calling it.
	Execute(ctx context.Context, callID string, params map[string]any, onUpdate UpdateFn) (Result, error)
}

// ---------------------------------------------------------------------------
// Convenience constructors for Result content
// ---------------------------------------------------------------------------

func TextResult(text string) Result {
	return Result{Content: []ai.ContentBlock{ai.TextContent{Type: "text", Text: text}}}
}

func ErrorResult(err error) Result {
	return TextResult("error: " + err.Error())
}

// ---------------------------------------------------------------------------
// SimpleSchema is a helper for building JSON Schema objects inline.
// ---------------------------------------------------------------------------

type SimpleSchema struct {
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required,omitempty"`
}

type Property struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Enum        []any  `json:"enum,omitempty"`
}

// MustSchema returns a JSON Schema for the given SimpleSchema.
func MustSchema(s SimpleSchema) json.RawMessage {
	s2 := map[string]any{
		"type":       "object",
		"properties": s.Properties,
	}
	if len(s.Required) > 0 {
		s2["required"] = s.Required
	}
	b, err := json.Marshal(s2)
	if err != nil {
		panic("tools.MustSchema: " + err.Error())
	}
	return b
}
