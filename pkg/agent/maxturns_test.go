package agent_test

import (
	"context"
	"testing"
	"time"

	"github.com/nickcecere/agent/pkg/agent"
	"github.com/nickcecere/agent/pkg/ai"
	"github.com/nickcecere/agent/pkg/tools"
)

// infiniteLoopProvider always returns a tool call so the loop would run forever.
type infiniteLoopProvider struct{ calls int }

func (p *infiniteLoopProvider) Name() string { return "infinite" }
func (p *infiniteLoopProvider) Stream(_ context.Context, _ string, _ ai.Context, _ ai.StreamOptions) (<-chan ai.StreamEvent, func() (*ai.AssistantMessage, error)) {
	p.calls++
	ch := make(chan ai.StreamEvent)
	close(ch)
	return ch, func() (*ai.AssistantMessage, error) {
		return &ai.AssistantMessage{
			Role: ai.RoleAssistant,
			Content: []ai.ContentBlock{
				ai.ToolCall{Type: "tool_call", ID: "c1", Name: "echo", Arguments: map[string]any{"text": "loop"}},
			},
			StopReason: ai.StopReasonTool,
			Timestamp:  time.Now().UnixMilli(),
		}, nil
	}
}

type echoTool struct{}
func (e *echoTool) Definition() ai.ToolDefinition {
	return ai.ToolDefinition{Name: "echo", Description: "echo", Parameters: tools.MustSchema(tools.SimpleSchema{
		Properties: map[string]tools.Property{"text": {Type: "string"}},
	})}
}
func (e *echoTool) Execute(_ context.Context, _ string, params map[string]any, _ tools.UpdateFn) (tools.Result, error) {
	return tools.TextResult(params["text"].(string)), nil
}

func TestMaxTurns(t *testing.T) {
	prov := &infiniteLoopProvider{}
	reg := tools.NewRegistry()
	reg.Register(&echoTool{})

	a := agent.New(agent.Options{
		Provider: prov,
		Model:    "test",
		Tools:    reg,
	})

	limitHit := false
	a.Subscribe(func(e agent.Event) {
		if e.Type == agent.EventTurnLimitReached {
			limitHit = true
		}
	})

	err := a.Prompt(context.Background(), "go", agent.Config{MaxTurns: 3})
	if err != nil {
		t.Fatal(err)
	}
	if !limitHit {
		t.Error("expected EventTurnLimitReached to fire")
	}
	if prov.calls != 3 {
		t.Errorf("provider called %d times, want 3", prov.calls)
	}
}
