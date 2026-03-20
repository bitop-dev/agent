package provider

import (
	"context"

	"github.com/bitop-dev/agent/pkg/tool"
)

type ModelRef struct {
	Provider string
	Model    string
}

type Message struct {
	Role       string
	Content    string
	ToolCallID string
	ToolName   string
	ToolCalls  []tool.Call
}

type StreamEventType string

const (
	StreamEventText     StreamEventType = "text"
	StreamEventToolCall StreamEventType = "tool_call"
	StreamEventDone     StreamEventType = "done"
)

type StreamEvent struct {
	Type     StreamEventType
	Text     string
	ToolCall tool.Call
	Err      error
}

type CompletionRequest struct {
	Model    ModelRef
	System   string
	Messages []Message
	Tools    []tool.Definition
}

type Provider interface {
	Name() string
	Stream(ctx context.Context, req CompletionRequest) (<-chan StreamEvent, error)
}

type Registry interface {
	Register(provider Provider) error
	Get(name string) (Provider, bool)
	List() []string
}

type CredentialSource interface {
	Token(ctx context.Context, provider string) (string, error)
}
