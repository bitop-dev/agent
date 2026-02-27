package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bitop-dev/agent/pkg/ai"
	"github.com/bitop-dev/agent/pkg/tools"
)

// SubAgentOptions configures a sub-agent for delegation.
type SubAgentOptions struct {
	// Provider and Model for the sub-agent's LLM calls.
	// If nil, inherited from the parent.
	Provider ai.Provider
	Model    string

	// SystemPrompt is the sub-agent's role/instructions.
	SystemPrompt string

	// Tools available to the sub-agent. Nil = empty (no tools).
	Tools *tools.Registry

	// StreamOptions for the sub-agent's LLM calls.
	StreamOptions ai.StreamOptions

	// MaxTurns caps the sub-agent's loop (0 = unlimited).
	MaxTurns int

	// OnEvent optionally receives events from the sub-agent.
	// Useful for logging or forwarding to the parent's listeners.
	OnEvent func(Event)
}

// SubAgent wraps an Agent for use as a delegated worker.
// It runs a prompt to completion and returns the final text response.
type SubAgent struct {
	agent *Agent
	opts  SubAgentOptions
}

// NewSubAgent creates a sub-agent. Call Run() to execute a task.
//
// Example:
//
//	sub := agent.NewSubAgent(agent.SubAgentOptions{
//	    Provider:     parentProvider,
//	    Model:        "gpt-4o",
//	    SystemPrompt: "You are a code reviewer. Be concise.",
//	    Tools:        readonlyTools,
//	    MaxTurns:     10,
//	})
//	result, err := sub.Run(ctx, "Review this diff: ...")
func NewSubAgent(opts SubAgentOptions) *SubAgent {
	reg := opts.Tools
	if reg == nil {
		reg = tools.NewRegistry()
	}
	a := New(Options{
		Provider:      opts.Provider,
		Model:         opts.Model,
		SystemPrompt:  opts.SystemPrompt,
		Tools:         reg,
		StreamOptions: opts.StreamOptions,
	})
	if opts.OnEvent != nil {
		a.Subscribe(opts.OnEvent)
	}
	return &SubAgent{agent: a, opts: opts}
}

// Run sends a prompt to the sub-agent and returns the final assistant text.
// It blocks until the sub-agent loop completes or ctx is cancelled.
func (s *SubAgent) Run(ctx context.Context, prompt string) (string, error) {
	cfg := Config{
		StreamOptions: s.opts.StreamOptions,
		MaxTurns:      s.opts.MaxTurns,
	}

	if err := s.agent.Prompt(ctx, prompt, cfg); err != nil {
		return "", fmt.Errorf("subagent: %w", err)
	}

	return s.LastResponse(), nil
}

// RunMessages sends pre-built messages and returns the final assistant text.
func (s *SubAgent) RunMessages(ctx context.Context, msgs []ai.Message) (string, error) {
	cfg := Config{
		StreamOptions: s.opts.StreamOptions,
		MaxTurns:      s.opts.MaxTurns,
	}

	if err := s.agent.PromptMessages(ctx, msgs, cfg); err != nil {
		return "", fmt.Errorf("subagent: %w", err)
	}

	return s.LastResponse(), nil
}

// LastResponse extracts the text from the last assistant message.
func (s *SubAgent) LastResponse() string {
	msgs := s.agent.Messages()
	for i := len(msgs) - 1; i >= 0; i-- {
		if am, ok := msgs[i].(ai.AssistantMessage); ok {
			return extractText(am)
		}
	}
	return ""
}

// Agent returns the underlying Agent for advanced use cases.
func (s *SubAgent) Agent() *Agent {
	return s.agent
}

// extractText concatenates all TextContent blocks from an assistant message.
func extractText(msg ai.AssistantMessage) string {
	var parts []string
	for _, b := range msg.Content {
		if tc, ok := b.(ai.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return strings.Join(parts, "")
}

// ---------------------------------------------------------------------------
// SubAgentTool — wraps a SubAgent as a Tool for the parent agent.
// ---------------------------------------------------------------------------

// SubAgentTool is a Tool that delegates to a sub-agent. When the parent LLM
// calls this tool, it runs the sub-agent with the given prompt and returns
// the sub-agent's final response as the tool result.
type SubAgentTool struct {
	name        string
	description string
	subOpts     SubAgentOptions
}

// NewSubAgentTool creates a tool that delegates to a sub-agent.
//
// Example:
//
//	reviewTool := agent.NewSubAgentTool("code_review",
//	    "Reviews code and returns feedback",
//	    agent.SubAgentOptions{
//	        Provider:     provider,
//	        Model:        "gpt-4o",
//	        SystemPrompt: "You are a code reviewer.",
//	        MaxTurns:     5,
//	    },
//	)
//	registry.Register(reviewTool)
func NewSubAgentTool(name, description string, opts SubAgentOptions) *SubAgentTool {
	return &SubAgentTool{
		name:        name,
		description: description,
		subOpts:     opts,
	}
}

func (t *SubAgentTool) Definition() ai.ToolDefinition {
	return ai.ToolDefinition{
		Name:        t.name,
		Description: t.description,
		Parameters: tools.MustSchema(tools.SimpleSchema{
			Properties: map[string]tools.Property{
				"prompt": {Type: "string", Description: "The task or question for the sub-agent"},
			},
			Required: []string{"prompt"},
		}),
	}
}

func (t *SubAgentTool) Execute(ctx context.Context, _ string, params map[string]any, onUpdate tools.UpdateFn) (tools.Result, error) {
	prompt, _ := params["prompt"].(string)
	if prompt == "" {
		return tools.ErrorResult(fmt.Errorf("prompt is required")), nil
	}

	// Wire progress updates from the sub-agent back to the parent.
	opts := t.subOpts
	if onUpdate != nil {
		opts.OnEvent = func(e Event) {
			if e.Type == EventMessageUpdate && e.StreamEvent != nil {
				onUpdate(tools.Result{
					Content: []ai.ContentBlock{ai.TextContent{
						Type: "text",
						Text: e.StreamEvent.Delta,
					}},
				})
			}
		}
	}

	sub := NewSubAgent(opts)
	result, err := sub.Run(ctx, prompt)
	if err != nil {
		return tools.ErrorResult(err), nil
	}

	if result == "" {
		result = "(sub-agent produced no response)"
	}

	return tools.Result{
		Content: []ai.ContentBlock{ai.TextContent{Type: "text", Text: result}},
		Details: map[string]any{
			"sub_agent_model":    opts.Model,
			"sub_agent_turns":    len(sub.Agent().Messages()),
			"sub_agent_finished": time.Now().UnixMilli(),
		},
	}, nil
}
