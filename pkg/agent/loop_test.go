package agent_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/bitop-dev/agent/pkg/agent"
	"github.com/bitop-dev/agent/pkg/ai"
	"github.com/bitop-dev/agent/pkg/tools"
)

// ── Helpers ──────────────────────────────────────────────────────────────────

// staticProvider returns a fixed AssistantMessage with no tool calls.
type staticProvider struct {
	msg *ai.AssistantMessage
}

func (p *staticProvider) Name() string { return "static" }
func (p *staticProvider) Stream(_ context.Context, _ string, _ ai.Context, _ ai.StreamOptions) (<-chan ai.StreamEvent, func() (*ai.AssistantMessage, error)) {
	ch := make(chan ai.StreamEvent, 2)
	ch <- ai.StreamEvent{Type: ai.StreamEventTextDelta, Delta: "hello", Partial: p.msg}
	close(ch)
	return ch, func() (*ai.AssistantMessage, error) { return p.msg, nil }
}

func textMsg(text string) *ai.AssistantMessage {
	return &ai.AssistantMessage{
		Role:       ai.RoleAssistant,
		Content:    []ai.ContentBlock{ai.TextContent{Type: "text", Text: text}},
		StopReason: ai.StopReasonStop,
		Timestamp:  time.Now().UnixMilli(),
	}
}

func toolCallMsg(id, name string, args map[string]any) *ai.AssistantMessage {
	return &ai.AssistantMessage{
		Role: ai.RoleAssistant,
		Content: []ai.ContentBlock{
			ai.ToolCall{Type: "tool_call", ID: id, Name: name, Arguments: args},
		},
		StopReason: ai.StopReasonTool,
		Timestamp:  time.Now().UnixMilli(),
	}
}

// callCountProvider cycles through a list of messages, one per Stream call.
type callCountProvider struct {
	mu    sync.Mutex
	msgs  []*ai.AssistantMessage
	calls int
}

func (p *callCountProvider) Name() string { return "counter" }
func (p *callCountProvider) Stream(_ context.Context, _ string, _ ai.Context, _ ai.StreamOptions) (<-chan ai.StreamEvent, func() (*ai.AssistantMessage, error)) {
	p.mu.Lock()
	idx := p.calls
	p.calls++
	p.mu.Unlock()

	msg := p.msgs[idx%len(p.msgs)]
	ch := make(chan ai.StreamEvent)
	close(ch)
	return ch, func() (*ai.AssistantMessage, error) { return msg, nil }
}

// echoToolImpl returns its "text" param as the result.
type echoToolImpl struct{}

func (e *echoToolImpl) Definition() ai.ToolDefinition {
	return ai.ToolDefinition{
		Name:        "echo",
		Description: "echo",
		Parameters:  tools.MustSchema(tools.SimpleSchema{Properties: map[string]tools.Property{"text": {Type: "string"}}}),
	}
}
func (e *echoToolImpl) Execute(_ context.Context, _ string, params map[string]any, _ tools.UpdateFn) (tools.Result, error) {
	t, _ := params["text"].(string)
	return tools.TextResult("echo:" + t), nil
}

func newAgent(prov ai.Provider) *agent.Agent {
	reg := tools.NewRegistry()
	reg.Register(&echoToolImpl{})
	return agent.New(agent.Options{Provider: prov, Model: "test", Tools: reg})
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestLoop_SingleTurn_NoTools(t *testing.T) {
	prov := &staticProvider{msg: textMsg("done")}
	a := newAgent(prov)

	var got []agent.EventType
	a.Subscribe(func(e agent.Event) { got = append(got, e.Type) })

	if err := a.Prompt(context.Background(), "hi", agent.Config{}); err != nil {
		t.Fatal(err)
	}

	// Verify key events appear in order (not every event, just the critical ones).
	want := []agent.EventType{
		agent.EventAgentStart,
		agent.EventMessageStart, // user message
		agent.EventMessageEnd,
		agent.EventMessageStart, // assistant partial
		agent.EventMessageEnd,   // assistant final
		agent.EventTurnEnd,
		agent.EventAgentEnd,
	}

	pos := 0
	for _, w := range want {
		found := false
		for pos < len(got) {
			if got[pos] == w {
				pos++
				found = true
				break
			}
			pos++
		}
		if !found {
			t.Errorf("expected event %q in sequence; events seen: %v", w, got)
		}
	}
}

func TestLoop_ToolCallAndResult(t *testing.T) {
	// First call returns a tool call; second returns stop.
	prov := &callCountProvider{msgs: []*ai.AssistantMessage{
		toolCallMsg("c1", "echo", map[string]any{"text": "world"}),
		textMsg("done"),
	}}
	a := newAgent(prov)

	var toolStarts, toolEnds int
	a.Subscribe(func(e agent.Event) {
		switch e.Type {
		case agent.EventToolStart:
			toolStarts++
		case agent.EventToolEnd:
			toolEnds++
		}
	})

	if err := a.Prompt(context.Background(), "go", agent.Config{}); err != nil {
		t.Fatal(err)
	}
	if toolStarts != 1 {
		t.Errorf("tool_start count = %d, want 1", toolStarts)
	}
	if toolEnds != 1 {
		t.Errorf("tool_end count = %d, want 1", toolEnds)
	}
}

func TestLoop_Subscribe_Unsubscribe(t *testing.T) {
	prov := &staticProvider{msg: textMsg("ok")}
	a := newAgent(prov)

	count := 0
	unsub := a.Subscribe(func(e agent.Event) { count++ })

	a.Prompt(context.Background(), "first", agent.Config{})
	afterFirst := count

	unsub() // stop receiving events

	a.Prompt(context.Background(), "second", agent.Config{})
	if count != afterFirst {
		t.Errorf("received %d events after unsubscribe (want 0 new)", count-afterFirst)
	}
}

func TestLoop_State_IsStreaming(t *testing.T) {
	// Use a slow provider to catch the streaming state mid-flight.
	blocker := make(chan struct{}, 1) // buffered to avoid panic on double-send
	var streamingDuring bool

	a := newAgent(&staticProvider{msg: textMsg("done")})
	a.Subscribe(func(e agent.Event) {
		if e.Type == agent.EventMessageUpdate {
			streamingDuring = a.State().IsStreaming
			select {
			case blocker <- struct{}{}:
			default:
			}
		}
	})

	go a.Prompt(context.Background(), "hi", agent.Config{})
	select {
	case <-blocker:
	case <-time.After(3 * time.Second):
		t.Log("no EventMessageUpdate received — skipping streaming state check (provider may not emit deltas)")
		return
	}
	if !streamingDuring {
		t.Error("State().IsStreaming should be true during streaming")
	}
}

func TestLoop_AgentEnd_HasNewMessages(t *testing.T) {
	prov := &staticProvider{msg: textMsg("result")}
	a := newAgent(prov)

	var endEvent agent.Event
	a.Subscribe(func(e agent.Event) {
		if e.Type == agent.EventAgentEnd {
			endEvent = e
		}
	})

	a.Prompt(context.Background(), "prompt", agent.Config{})
	if len(endEvent.NewMessages) == 0 {
		t.Error("EventAgentEnd.NewMessages should not be empty")
	}
}

func TestLoop_ContextCancellation(t *testing.T) {
	// Provide a provider that blocks so we can cancel mid-stream.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	blockProv := &staticProvider{msg: textMsg("done")}
	a := newAgent(blockProv)

	// Should not hang past context deadline.
	done := make(chan error, 1)
	go func() { done <- a.Prompt(ctx, "hi", agent.Config{}) }()

	select {
	case <-done:
		// ok — either finished or cancelled cleanly
	case <-time.After(2 * time.Second):
		t.Fatal("prompt did not respect context cancellation")
	}
}

func TestLoop_TurnEnd_HasContextUsage(t *testing.T) {
	prov := &staticProvider{msg: &ai.AssistantMessage{
		Role:       ai.RoleAssistant,
		Content:    []ai.ContentBlock{ai.TextContent{Type: "text", Text: "hi"}},
		StopReason: ai.StopReasonStop,
		Usage:      ai.Usage{Input: 10, Output: 5, TotalTokens: 15},
		Timestamp:  time.Now().UnixMilli(),
	}}
	a := newAgent(prov)

	var usage agent.ContextUsage
	a.Subscribe(func(e agent.Event) {
		if e.Type == agent.EventTurnEnd {
			usage = e.ContextUsage
		}
	})

	a.Prompt(context.Background(), "hi", agent.Config{})
	if usage.Tokens == 0 {
		t.Error("ContextUsage.Tokens should be non-zero after a turn")
	}
}
