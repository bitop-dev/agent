package agent_test

import (
	"context"
	"testing"
	"time"

	"github.com/bitop-dev/agent/pkg/agent"
	"github.com/bitop-dev/agent/pkg/ai"
	"github.com/bitop-dev/agent/pkg/tools"
)

func TestSubAgent_Run(t *testing.T) {
	prov := &staticProvider{msg: textMsg("sub-agent response")}

	sub := agent.NewSubAgent(agent.SubAgentOptions{
		Provider:     prov,
		Model:        "test",
		SystemPrompt: "You are a helper.",
		MaxTurns:     5,
	})

	result, err := sub.Run(context.Background(), "do something")
	if err != nil {
		t.Fatal(err)
	}
	if result != "sub-agent response" {
		t.Errorf("result = %q, want %q", result, "sub-agent response")
	}
}

func TestSubAgent_LastResponse(t *testing.T) {
	prov := &staticProvider{msg: textMsg("final answer")}
	sub := agent.NewSubAgent(agent.SubAgentOptions{
		Provider: prov,
		Model:    "test",
	})

	sub.Run(context.Background(), "question")
	if sub.LastResponse() != "final answer" {
		t.Errorf("LastResponse = %q", sub.LastResponse())
	}
}

func TestSubAgent_OnEvent(t *testing.T) {
	prov := &staticProvider{msg: textMsg("done")}

	var events []agent.EventType
	sub := agent.NewSubAgent(agent.SubAgentOptions{
		Provider: prov,
		Model:    "test",
		OnEvent: func(e agent.Event) {
			events = append(events, e.Type)
		},
	})

	sub.Run(context.Background(), "go")
	if len(events) == 0 {
		t.Error("OnEvent should have received events")
	}
}

func TestSubAgent_Agent(t *testing.T) {
	prov := &staticProvider{msg: textMsg("done")}
	sub := agent.NewSubAgent(agent.SubAgentOptions{
		Provider: prov,
		Model:    "test",
	})

	if sub.Agent() == nil {
		t.Error("Agent() should not be nil")
	}
}

func TestSubAgentTool_Execute(t *testing.T) {
	prov := &staticProvider{msg: textMsg("reviewed: looks good")}

	tool := agent.NewSubAgentTool("review", "Reviews code", agent.SubAgentOptions{
		Provider:     prov,
		Model:        "test",
		SystemPrompt: "You are a reviewer.",
		MaxTurns:     3,
	})

	if tool.Definition().Name != "review" {
		t.Errorf("name = %q", tool.Definition().Name)
	}

	result, err := tool.Execute(context.Background(), "c1", map[string]any{
		"prompt": "Review this code",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	text := ""
	for _, b := range result.Content {
		if tc, ok := b.(ai.TextContent); ok {
			text += tc.Text
		}
	}
	if text != "reviewed: looks good" {
		t.Errorf("result = %q", text)
	}
}

func TestSubAgentTool_MissingPrompt(t *testing.T) {
	tool := agent.NewSubAgentTool("test", "test", agent.SubAgentOptions{
		Provider: &staticProvider{msg: textMsg("done")},
		Model:    "test",
	})

	result, _ := tool.Execute(context.Background(), "c1", map[string]any{}, nil)
	text := ""
	for _, b := range result.Content {
		if tc, ok := b.(ai.TextContent); ok {
			text += tc.Text
		}
	}
	if text == "" {
		t.Error("expected error message for missing prompt")
	}
}

func TestSubAgent_WithTools(t *testing.T) {
	// Sub-agent gets a tool call, executes it, then returns final text.
	prov := &callCountProvider{msgs: []*ai.AssistantMessage{
		{
			Role: ai.RoleAssistant,
			Content: []ai.ContentBlock{
				ai.ToolCall{Type: "tool_call", ID: "c1", Name: "echo", Arguments: map[string]any{"text": "sub"}},
			},
			StopReason: ai.StopReasonTool,
			Timestamp:  time.Now().UnixMilli(),
		},
		textMsg("done with tools"),
	}}

	reg := tools.NewRegistry()
	reg.Register(&echoToolImpl{})

	sub := agent.NewSubAgent(agent.SubAgentOptions{
		Provider: prov,
		Model:    "test",
		Tools:    reg,
		MaxTurns: 5,
	})

	result, err := sub.Run(context.Background(), "use echo")
	if err != nil {
		t.Fatal(err)
	}
	if result != "done with tools" {
		t.Errorf("result = %q", result)
	}
}
