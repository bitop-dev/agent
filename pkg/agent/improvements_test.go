package agent_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nickcecere/agent/pkg/agent"
	"github.com/nickcecere/agent/pkg/ai"
	"github.com/nickcecere/agent/pkg/tools"
)

// ── Panic recovery (#2) ──────────────────────────────────────────────────

type panicTool struct{}

func (p *panicTool) Definition() ai.ToolDefinition {
	return ai.ToolDefinition{
		Name:        "panic_tool",
		Description: "always panics",
		Parameters:  tools.MustSchema(tools.SimpleSchema{}),
	}
}
func (p *panicTool) Execute(_ context.Context, _ string, _ map[string]any, _ tools.UpdateFn) (tools.Result, error) {
	panic("deliberate panic in tool")
}

func TestPanicRecovery(t *testing.T) {
	// Provider returns a tool call to the panic tool, then stops.
	prov := &callCountProvider{msgs: []*ai.AssistantMessage{
		toolCallMsg("c1", "panic_tool", nil),
		textMsg("recovered"),
	}}
	reg := tools.NewRegistry()
	reg.Register(&panicTool{})
	a := agent.New(agent.Options{Provider: prov, Model: "test", Tools: reg})

	var toolErrors int
	a.Subscribe(func(e agent.Event) {
		if e.Type == agent.EventToolEnd && e.IsError {
			toolErrors++
		}
	})

	err := a.Prompt(context.Background(), "go", agent.Config{})
	if err != nil {
		t.Fatalf("Prompt should not return error on tool panic: %v", err)
	}
	if toolErrors != 1 {
		t.Errorf("expected 1 tool error from panic, got %d", toolErrors)
	}
}

// ── Confirmation hooks (#3) ──────────────────────────────────────────────

func TestConfirmToolCall_Deny(t *testing.T) {
	prov := &callCountProvider{msgs: []*ai.AssistantMessage{
		toolCallMsg("c1", "echo", map[string]any{"text": "hello"}),
		textMsg("done"),
	}}
	a := newAgent(prov)

	var denied int
	a.Subscribe(func(e agent.Event) {
		if e.Type == agent.EventToolDenied {
			denied++
		}
	})

	cfg := agent.Config{
		ConfirmToolCall: func(name string, args map[string]any) (agent.ConfirmResult, error) {
			return agent.ConfirmDeny, nil
		},
	}

	err := a.Prompt(context.Background(), "go", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if denied != 1 {
		t.Errorf("denied = %d, want 1", denied)
	}
}

func TestConfirmToolCall_Abort(t *testing.T) {
	prov := &callCountProvider{msgs: []*ai.AssistantMessage{
		toolCallMsg("c1", "echo", map[string]any{"text": "hello"}),
		textMsg("done"),
	}}
	a := newAgent(prov)

	cfg := agent.Config{
		ConfirmToolCall: func(name string, args map[string]any) (agent.ConfirmResult, error) {
			return agent.ConfirmAbort, nil
		},
	}

	err := a.Prompt(context.Background(), "go", cfg)
	if err == nil {
		t.Fatal("expected error on ConfirmAbort")
	}
}

func TestConfirmToolCall_AutoApprove(t *testing.T) {
	prov := &callCountProvider{msgs: []*ai.AssistantMessage{
		toolCallMsg("c1", "echo", map[string]any{"text": "hello"}),
		textMsg("done"),
	}}
	a := newAgent(prov)

	var toolEnds int
	a.Subscribe(func(e agent.Event) {
		if e.Type == agent.EventToolEnd {
			toolEnds++
		}
	})

	cfg := agent.Config{
		ConfirmToolCall: agent.AutoApproveAll,
	}

	err := a.Prompt(context.Background(), "go", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if toolEnds != 1 {
		t.Errorf("toolEnds = %d, want 1", toolEnds)
	}
}

func TestConfirmToolCall_Nil_AutoApproves(t *testing.T) {
	// nil ConfirmToolCall should auto-approve (backward compatible).
	prov := &callCountProvider{msgs: []*ai.AssistantMessage{
		toolCallMsg("c1", "echo", map[string]any{"text": "hello"}),
		textMsg("done"),
	}}
	a := newAgent(prov)

	var toolEnds int
	a.Subscribe(func(e agent.Event) {
		if e.Type == agent.EventToolEnd {
			toolEnds++
		}
	})

	err := a.Prompt(context.Background(), "go", agent.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if toolEnds != 1 {
		t.Errorf("toolEnds = %d, want 1 (nil should auto-approve)", toolEnds)
	}
}

// ── Retry with backoff (#1) ──────────────────────────────────────────────

type failNTimesProvider struct {
	mu       sync.Mutex
	failures int
	calls    int
	finalMsg *ai.AssistantMessage
}

func (p *failNTimesProvider) Name() string { return "failN" }
func (p *failNTimesProvider) Stream(_ context.Context, _ string, _ ai.Context, _ ai.StreamOptions) (<-chan ai.StreamEvent, func() (*ai.AssistantMessage, error)) {
	p.mu.Lock()
	p.calls++
	n := p.calls
	p.mu.Unlock()

	ch := make(chan ai.StreamEvent)
	close(ch)

	if n <= p.failures {
		errMsg := &ai.AssistantMessage{
			Role:         ai.RoleAssistant,
			StopReason:   ai.StopReasonError,
			ErrorMessage: "503 service unavailable",
			Timestamp:    time.Now().UnixMilli(),
		}
		return ch, func() (*ai.AssistantMessage, error) { return errMsg, nil }
	}

	return ch, func() (*ai.AssistantMessage, error) { return p.finalMsg, nil }
}

func TestRetry_RecoversFromTransient(t *testing.T) {
	prov := &failNTimesProvider{
		failures: 2,
		finalMsg: textMsg("success"),
	}
	a := agent.New(agent.Options{Provider: prov, Model: "test", Tools: tools.NewRegistry()})

	var retries int
	a.Subscribe(func(e agent.Event) {
		if e.Type == agent.EventRetry {
			retries++
		}
	})

	cfg := agent.Config{
		MaxRetries:     3,
		RetryBaseDelay: 10 * time.Millisecond,
	}

	err := a.Prompt(context.Background(), "go", cfg)
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if retries != 2 {
		t.Errorf("retries = %d, want 2", retries)
	}
}

func TestRetry_ExhaustsRetries(t *testing.T) {
	prov := &failNTimesProvider{
		failures: 10, // more failures than retries
		finalMsg: textMsg("never reached"),
	}
	a := agent.New(agent.Options{Provider: prov, Model: "test", Tools: tools.NewRegistry()})

	cfg := agent.Config{
		MaxRetries:     2,
		RetryBaseDelay: 10 * time.Millisecond,
	}

	// The agent should still not return an error — it records the error turn.
	err := a.Prompt(context.Background(), "go", cfg)
	if err != nil {
		t.Fatalf("expected nil error (error turn recorded), got: %v", err)
	}
}

func TestRetry_ZeroRetries_NoRetry(t *testing.T) {
	prov := &failNTimesProvider{
		failures: 1,
		finalMsg: textMsg("never reached"),
	}
	a := agent.New(agent.Options{Provider: prov, Model: "test", Tools: tools.NewRegistry()})

	var retries int
	a.Subscribe(func(e agent.Event) {
		if e.Type == agent.EventRetry {
			retries++
		}
	})

	cfg := agent.Config{MaxRetries: 0}
	a.Prompt(context.Background(), "go", cfg)
	if retries != 0 {
		t.Errorf("retries = %d, want 0 with MaxRetries=0", retries)
	}
}

// ── Parallel tool execution (#4) ─────────────────────────────────────────

type slowTool struct {
	name     string
	delay    time.Duration
	started  atomic.Int32
}

func (s *slowTool) Definition() ai.ToolDefinition {
	return ai.ToolDefinition{
		Name:        s.name,
		Description: "slow tool",
		Parameters:  tools.MustSchema(tools.SimpleSchema{}),
	}
}
func (s *slowTool) Execute(ctx context.Context, _ string, _ map[string]any, _ tools.UpdateFn) (tools.Result, error) {
	s.started.Add(1)
	select {
	case <-time.After(s.delay):
	case <-ctx.Done():
	}
	return tools.TextResult("done"), nil
}

func TestParallelToolExecution(t *testing.T) {
	slow1 := &slowTool{name: "slow1", delay: 100 * time.Millisecond}
	slow2 := &slowTool{name: "slow2", delay: 100 * time.Millisecond}

	prov := &callCountProvider{msgs: []*ai.AssistantMessage{
		{
			Role: ai.RoleAssistant,
			Content: []ai.ContentBlock{
				ai.ToolCall{Type: "tool_call", ID: "c1", Name: "slow1", Arguments: nil},
				ai.ToolCall{Type: "tool_call", ID: "c2", Name: "slow2", Arguments: nil},
			},
			StopReason: ai.StopReasonTool,
			Timestamp:  time.Now().UnixMilli(),
		},
		textMsg("done"),
	}}

	reg := tools.NewRegistry()
	reg.Register(slow1)
	reg.Register(slow2)
	a := agent.New(agent.Options{Provider: prov, Model: "test", Tools: reg})

	start := time.Now()
	cfg := agent.Config{MaxToolConcurrency: 2}
	err := a.Prompt(context.Background(), "go", cfg)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatal(err)
	}
	// With concurrency=2, both tools run simultaneously.
	// Sequential would be ~200ms; parallel should be ~100ms.
	if elapsed > 180*time.Millisecond {
		t.Errorf("parallel execution took %s (expected ~100ms)", elapsed)
	}
}

// ── Tool timeout (#9) ────────────────────────────────────────────────────

func TestToolTimeout(t *testing.T) {
	slow := &slowTool{name: "slow", delay: 5 * time.Second}

	prov := &callCountProvider{msgs: []*ai.AssistantMessage{
		toolCallMsg("c1", "slow", nil),
		textMsg("done"),
	}}

	reg := tools.NewRegistry()
	reg.Register(slow)
	a := agent.New(agent.Options{Provider: prov, Model: "test", Tools: reg})

	start := time.Now()
	cfg := agent.Config{ToolTimeout: 50 * time.Millisecond}
	err := a.Prompt(context.Background(), "go", cfg)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatal(err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("tool timeout not respected: took %s", elapsed)
	}
}

// ── Cost tracking (#6) ──────────────────────────────────────────────────

func TestCostTracking(t *testing.T) {
	// Use a known model ID with pricing.
	msg := &ai.AssistantMessage{
		Role:       ai.RoleAssistant,
		Content:    []ai.ContentBlock{ai.TextContent{Type: "text", Text: "hi"}},
		StopReason: ai.StopReasonStop,
		Usage:      ai.Usage{Input: 100, Output: 50, TotalTokens: 150},
		Timestamp:  time.Now().UnixMilli(),
	}
	prov := &staticProvider{msg: msg}
	a := agent.New(agent.Options{Provider: prov, Model: "gpt-4o", Tools: tools.NewRegistry()})

	var turnCost agent.CostUsage
	a.Subscribe(func(e agent.Event) {
		if e.Type == agent.EventTurnEnd {
			turnCost = e.CostUsage
		}
	})

	a.Prompt(context.Background(), "hi", agent.Config{})

	if turnCost.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", turnCost.InputTokens)
	}
	if turnCost.OutputTokens != 50 {
		t.Errorf("OutputTokens = %d, want 50", turnCost.OutputTokens)
	}
	if turnCost.TotalCost <= 0 {
		t.Error("TotalCost should be > 0 for gpt-4o")
	}

	// Check cumulative in State
	state := a.State()
	if state.CumulativeCost.TotalCost <= 0 {
		t.Error("State.CumulativeCost should be > 0 after one turn")
	}
}

// ── Metrics callback (#10) ──────────────────────────────────────────────

func TestMetricsCallback(t *testing.T) {
	prov := &staticProvider{msg: textMsg("done")}
	a := agent.New(agent.Options{Provider: prov, Model: "test", Tools: tools.NewRegistry()})

	var got *agent.TurnMetrics
	cfg := agent.Config{
		OnMetrics: func(m agent.TurnMetrics) {
			got = &m
		},
	}

	a.Prompt(context.Background(), "hi", cfg)

	if got == nil {
		t.Fatal("OnMetrics was not called")
	}
	if got.TurnNumber != 1 {
		t.Errorf("TurnNumber = %d, want 1", got.TurnNumber)
	}
	if got.ProviderLatency <= 0 {
		t.Error("ProviderLatency should be > 0")
	}
}

// ── TurnDuration in EventTurnEnd ─────────────────────────────────────────

func TestTurnDuration(t *testing.T) {
	prov := &staticProvider{msg: textMsg("done")}
	a := agent.New(agent.Options{Provider: prov, Model: "test", Tools: tools.NewRegistry()})

	var dur time.Duration
	a.Subscribe(func(e agent.Event) {
		if e.Type == agent.EventTurnEnd {
			dur = e.TurnDuration
		}
	})

	a.Prompt(context.Background(), "hi", agent.Config{})
	if dur <= 0 {
		t.Error("TurnDuration should be > 0")
	}
}

// ── Budget cap (MaxCostUSD) ─────────────────────────────────────────────

func TestMaxCostUSD(t *testing.T) {
	// Use gpt-4o (known pricing: $2.5/1M input, $10/1M output).
	// 1M input tokens = $2.50, so with 1M input + 500k output the cost is:
	// $2.50 + $5.00 = $7.50 per turn — easily exceeds a $0.01 budget.
	msg := &ai.AssistantMessage{
		Role: ai.RoleAssistant,
		Content: []ai.ContentBlock{
			ai.ToolCall{Type: "tool_call", ID: "c1", Name: "echo", Arguments: map[string]any{"text": "loop"}},
		},
		StopReason: ai.StopReasonTool,
		Usage:      ai.Usage{Input: 1000000, Output: 500000, TotalTokens: 1500000},
		Timestamp:  time.Now().UnixMilli(),
	}
	prov := &staticProvider{msg: msg}

	reg := tools.NewRegistry()
	reg.Register(&echoToolImpl{})
	a := agent.New(agent.Options{Provider: prov, Model: "gpt-4o", Tools: reg})

	var limitReached bool
	a.Subscribe(func(e agent.Event) {
		if e.Type == agent.EventTurnLimitReached {
			limitReached = true
		}
	})

	cfg := agent.Config{
		MaxCostUSD: 0.01, // very low budget
	}
	err := a.Prompt(context.Background(), "go", cfg)
	if err != nil {
		t.Fatal(err)
	}
	// After the first turn, cumulative cost exceeds $0.01.
	// The budget guard at the top of the next iteration stops the loop.
	if !limitReached {
		t.Error("expected EventTurnLimitReached from budget cap")
	}
}

// Missing import for sync
var _ = fmt.Sprint // keep fmt import
