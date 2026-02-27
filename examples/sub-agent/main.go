// examples/sub-agent — demonstrates sub-agent delegation.
//
// Shows two patterns:
//  1. Standalone SubAgent: run a child agent directly and get its response.
//  2. SubAgentTool: wrap a child agent as a tool the parent LLM can call.
//
// The parent agent is a "research coordinator" that delegates deep-dive
// analysis tasks to a specialist sub-agent.
//
// Usage:
//
//	ANTHROPIC_API_KEY=sk-... go run ./examples/sub-agent
//	ANTHROPIC_API_KEY=sk-... go run ./examples/sub-agent "Analyse pkg/agent/loop.go"
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/nickcecere/agent/pkg/agent"
	"github.com/nickcecere/agent/pkg/ai"
	"github.com/nickcecere/agent/pkg/ai/providers/anthropic"
	"github.com/nickcecere/agent/pkg/tools"
	"github.com/nickcecere/agent/pkg/tools/builtin"
)

func main() {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "ANTHROPIC_API_KEY is not set")
		os.Exit(1)
	}

	provider := anthropic.New(apiKey)

	// ── Pattern 1: Standalone SubAgent ────────────────────────────────────
	//
	// Create a specialist "code analyser" sub-agent with read-only tools.
	// Run it directly and print its response.

	fmt.Println("=== Pattern 1: Standalone SubAgent ===")
	fmt.Println()

	readonlyReg := tools.NewRegistry()
	builtin.Register(readonlyReg, builtin.PresetReadOnly, ".")

	analyser := agent.NewSubAgent(agent.SubAgentOptions{
		Provider: provider,
		Model:    "claude-sonnet-4-5",
		SystemPrompt: "You are a Go code analyser. Examine code and provide " +
			"concise, structured analysis covering: purpose, key abstractions, " +
			"notable patterns, and potential issues. Be brief.",
		Tools:    readonlyReg,
		MaxTurns: 5,
		OnEvent: func(e agent.Event) {
			// Forward tool activity to stdout for visibility.
			if e.Type == agent.EventToolStart {
				fmt.Printf("  [sub-agent tool] %s\n", e.ToolName)
			}
		},
	})

	target := "pkg/agent/loop.go"
	if len(os.Args) > 1 {
		target = strings.Join(os.Args[1:], " ")
	}

	fmt.Printf("Analysing: %s\n\n", target)
	result, err := analyser.Run(context.Background(),
		fmt.Sprintf("Analyse %s and give me a concise summary.", target))
	if err != nil {
		fmt.Fprintln(os.Stderr, "sub-agent error:", err)
		os.Exit(1)
	}
	fmt.Println(result)

	// ── Pattern 2: SubAgentTool ────────────────────────────────────────────
	//
	// Wrap the analyser as a tool the parent agent can call.
	// The parent LLM decides when to delegate analysis tasks.

	fmt.Println("\n=== Pattern 2: SubAgentTool ===")
	fmt.Println()

	// Create the sub-agent tool.
	analyseTool := agent.NewSubAgentTool(
		"analyse_code",
		"Deeply analyses a Go source file or package and returns structured findings. "+
			"Use this for thorough code review or when you need to understand complex code.",
		agent.SubAgentOptions{
			Provider: provider,
			Model:    "claude-sonnet-4-5",
			SystemPrompt: "You are a Go code analyser. Examine code and provide " +
				"concise, structured analysis: purpose, key abstractions, patterns, issues.",
			Tools:    readonlyReg,
			MaxTurns: 8,
		},
	)

	// Parent agent has the sub-agent tool plus basic read.
	parentReg := tools.NewRegistry()
	builtin.Register(parentReg, builtin.PresetReadOnly, ".")
	parentReg.Register(analyseTool)

	parent := agent.New(agent.Options{
		Provider: provider,
		Model:    "claude-sonnet-4-5",
		Tools:    parentReg,
		SystemPrompt: "You are a research coordinator. When you need deep code " +
			"analysis, use the analyse_code tool to delegate to a specialist. " +
			"Synthesise findings into actionable recommendations.",
	})

	parent.Subscribe(func(e agent.Event) {
		switch e.Type {
		case agent.EventMessageUpdate:
			if se := e.StreamEvent; se != nil && se.Type == ai.StreamEventTextDelta {
				fmt.Print(se.Delta)
			}
		case agent.EventToolStart:
			switch e.ToolName {
			case "analyse_code":
				prompt, _ := e.ToolArgs["prompt"].(string)
				fmt.Printf("\n[delegating to sub-agent] %s\n", prompt)
			default:
				fmt.Printf("\n[tool] %s\n", e.ToolName)
			}
		case agent.EventToolUpdate:
			// Sub-agent streams deltas through as tool updates.
			if r := e.ToolResult; r != nil {
				for _, b := range r.Content {
					if tc, ok := b.(ai.TextContent); ok && tc.Text != "" {
						fmt.Print("  ", tc.Text)
					}
				}
			}
		}
	})

	fmt.Printf("Coordinator prompt: Summarise the agent package architecture\n\n")
	if err := parent.Prompt(context.Background(),
		"Give me a high-level summary of how the pkg/agent package is structured. "+
			"Use the analyse_code tool on the key files.",
		agent.Config{
			StreamOptions: ai.StreamOptions{MaxTokens: 2048},
			MaxTurns:      10,
		},
	); err != nil {
		fmt.Fprintln(os.Stderr, "parent error:", err)
		os.Exit(1)
	}
	fmt.Println()
}
