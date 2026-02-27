// examples/basic — minimal embedded agent.
//
// Demonstrates:
//   - Creating a provider from environment variables
//   - Registering built-in tools
//   - Subscribing to events for live output
//   - Running a single prompt and a multi-turn conversation
//
// Usage:
//
//	ANTHROPIC_API_KEY=sk-... go run ./examples/basic
//	ANTHROPIC_API_KEY=sk-... go run ./examples/basic "List the .go files here"
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/bitop-dev/agent/pkg/agent"
	"github.com/bitop-dev/agent/pkg/ai"
	"github.com/bitop-dev/agent/pkg/ai/providers/anthropic"
	"github.com/bitop-dev/agent/pkg/tools"
	"github.com/bitop-dev/agent/pkg/tools/builtin"
)

func main() {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "ANTHROPIC_API_KEY is not set")
		os.Exit(1)
	}

	// ── Provider ─────────────────────────────────────────────────────────
	provider := anthropic.New(apiKey)

	// ── Tool registry ─────────────────────────────────────────────────────
	reg := tools.NewRegistry()
	builtin.Register(reg, builtin.PresetCoding, ".")

	// ── Agent ─────────────────────────────────────────────────────────────
	a := agent.New(agent.Options{
		Provider:     provider,
		Model:        "claude-sonnet-4-5",
		Tools:        reg,
		SystemPrompt: "You are a helpful assistant. Be concise.",
	})

	// ── Event handler ─────────────────────────────────────────────────────
	// Print streaming text to stdout, announce tool calls, and report usage.
	a.Subscribe(func(e agent.Event) {
		switch e.Type {
		case agent.EventMessageUpdate:
			if se := e.StreamEvent; se != nil && se.Type == ai.StreamEventTextDelta {
				fmt.Print(se.Delta)
			}
		case agent.EventToolStart:
			fmt.Printf("\n[tool] calling %s...\n", e.ToolName)
		case agent.EventToolEnd:
			status := "ok"
			if e.IsError {
				status = "error"
			}
			fmt.Printf("[tool] %s → %s\n", e.ToolName, status)
		case agent.EventTurnEnd:
			fmt.Printf("\n[context: ~%d tokens]\n", e.ContextUsage.Tokens)
		case agent.EventCompaction:
			if c := e.Compaction; c != nil {
				fmt.Printf("\n[compaction] %d → %d tokens\n", c.TokensBefore, c.TokensAfter)
			}
		}
	})

	// ── Helper: send a user message ───────────────────────────────────────
	send := func(text string) {
		fmt.Printf("\n\033[1;32mUser:\033[0m %s\n\033[1;34mAssistant:\033[0m ", text)
		if err := a.Prompt(context.Background(), text, agent.Config{
			StreamOptions: ai.StreamOptions{MaxTokens: 2048},
		}); err != nil {
			fmt.Fprintln(os.Stderr, "\nerror:", err)
		}
		fmt.Println()
	}

	// ── One-shot mode ─────────────────────────────────────────────────────
	if len(os.Args) > 1 {
		send(strings.Join(os.Args[1:], " "))
		return
	}

	// ── Interactive REPL ──────────────────────────────────────────────────
	fmt.Println("Basic agent REPL. Type 'quit' to exit.")
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("\n> ")
		if !scanner.Scan() {
			break
		}
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		if text == "quit" || text == "exit" {
			break
		}
		send(text)
	}
}
