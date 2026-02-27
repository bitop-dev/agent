// examples/web-agent — a research agent with web search and web fetch tools.
//
// No API key needed for search — DuckDuckGo Lite requires no authentication.
//
// Usage:
//
//	OPENAI_API_KEY=sk-... go run ./examples/web-agent "Latest Go release notes"
//	OPENAI_API_KEY=sk-... go run ./examples/web-agent   # interactive REPL
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/bitop-dev/agent/pkg/agent"
	"github.com/bitop-dev/agent/pkg/ai"
	"github.com/bitop-dev/agent/pkg/ai/providers/openai"
	"github.com/bitop-dev/agent/pkg/tools"
	"github.com/bitop-dev/agent/pkg/tools/builtin"
)

const systemPrompt = `You are a research assistant with access to web search and web fetch tools.

When answering questions:
1. Search the web for up-to-date information
2. Fetch the most relevant pages for details
3. Synthesize findings into a clear, accurate answer
4. Always cite your sources (include URLs)

Be thorough but concise. If information is uncertain or conflicting, say so.`

func main() {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "OPENAI_API_KEY is not set")
		os.Exit(1)
	}

	reg := tools.NewRegistry()
	builtin.Register(reg, builtin.PresetWeb, ".")

	a := agent.New(agent.Options{
		Provider:     openai.New(apiKey),
		Model:        "gpt-4o",
		Tools:        reg,
		SystemPrompt: systemPrompt,
	})

	a.Subscribe(func(e agent.Event) {
		switch e.Type {
		case agent.EventMessageUpdate:
			if se := e.StreamEvent; se != nil && se.Type == ai.StreamEventTextDelta {
				fmt.Print(se.Delta)
			}
		case agent.EventToolStart:
			switch e.ToolName {
			case "web_search":
				q, _ := e.ToolArgs["query"].(string)
				fmt.Printf("\n🔍 Searching: %q\n", q)
			case "web_fetch":
				u, _ := e.ToolArgs["url"].(string)
				fmt.Printf("\n🌐 Fetching: %s\n", u)
			}
		}
	})

	send := func(text string) {
		fmt.Printf("\n\033[1;32mQuestion:\033[0m %s\n\n\033[1;34mAnswer:\033[0m ", text)
		if err := a.Prompt(context.Background(), text, agent.Config{
			StreamOptions: ai.StreamOptions{MaxTokens: 2048},
		}); err != nil {
			fmt.Fprintln(os.Stderr, "\nerror:", err)
		}
		fmt.Println()
	}

	// One-shot mode
	if len(os.Args) > 1 {
		send(strings.Join(os.Args[1:], " "))
		return
	}

	// Interactive REPL
	fmt.Println("Web research agent. Ask anything. Type 'quit' to exit.")
	fmt.Println()
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
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
