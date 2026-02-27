// examples/confirmation demonstrates tool confirmation hooks.
//
// Shows three confirmation patterns:
//
//  1. Interactive: asks the user before each tool call (default mode)
//  2. Policy-based: allow/deny based on tool name ("policy" mode)
//  3. AutoApproveAll: explicit unattended/autonomous mode ("auto" mode)
//
// Usage:
//
//	ANTHROPIC_API_KEY=sk-... go run ./examples/confirmation
//	ANTHROPIC_API_KEY=sk-... go run ./examples/confirmation policy
//	ANTHROPIC_API_KEY=sk-... go run ./examples/confirmation auto
package main

import (
	"bufio"
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

	mode := "interactive"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}

	provider := anthropic.New(apiKey)

	reg := tools.NewRegistry()
	builtin.Register(reg, builtin.PresetCoding, ".")

	a := agent.New(agent.Options{
		Provider:     provider,
		Model:        "claude-sonnet-4-5",
		Tools:        reg,
		SystemPrompt: "You are a coding assistant. Use tools to help with tasks.",
	})

	a.Subscribe(func(e agent.Event) {
		switch e.Type {
		case agent.EventMessageUpdate:
			if se := e.StreamEvent; se != nil && se.Type == ai.StreamEventTextDelta {
				fmt.Print(se.Delta)
			}
		case agent.EventToolDenied:
			fmt.Printf("\n[denied] %s call blocked by confirmation hook\n", e.ToolName)
		case agent.EventTurnEnd:
			fmt.Println()
		}
	})

	cfg := agent.Config{
		StreamOptions: ai.StreamOptions{MaxTokens: 1024},
		MaxTurns:      5,
	}

	switch mode {
	case "interactive":
		fmt.Println("=== Interactive confirmation ===")
		fmt.Println("You will be asked before each tool call.")
		fmt.Println()
		cfg.ConfirmToolCall = interactiveConfirm()

	case "policy":
		fmt.Println("=== Policy-based confirmation ===")
		fmt.Println("Rules: allow read/grep/find/ls, deny write operations.")
		fmt.Println()
		cfg.ConfirmToolCall = policyConfirm()

	case "auto":
		fmt.Println("=== Auto-approve (unattended) ===")
		fmt.Println("All tool calls approved automatically.")
		fmt.Println()
		cfg.ConfirmToolCall = agent.AutoApproveAll

	default:
		fmt.Fprintf(os.Stderr, "Unknown mode %q. Use: interactive | policy | auto\n", mode)
		os.Exit(1)
	}

	prompt := "List the .go files in pkg/agent/ and show me the first 10 lines of loop.go"
	fmt.Printf("Prompt: %s\n\nResponse: ", prompt)

	if err := a.Prompt(context.Background(), prompt, cfg); err != nil {
		fmt.Fprintln(os.Stderr, "\nerror:", err)
		os.Exit(1)
	}
	fmt.Println()
}

// interactiveConfirm returns a ConfirmToolCall that asks the user via stdin.
func interactiveConfirm() func(string, map[string]any) (agent.ConfirmResult, error) {
	scanner := bufio.NewScanner(os.Stdin)
	return func(name string, args map[string]any) (agent.ConfirmResult, error) {
		var argParts []string
		for k, v := range args {
			s := fmt.Sprintf("%v", v)
			if len(s) > 60 {
				s = s[:57] + "..."
			}
			argParts = append(argParts, fmt.Sprintf("%s=%q", k, s))
		}
		fmt.Printf("\n[confirm] %s(%s)\n", name, strings.Join(argParts, ", "))
		fmt.Print("Allow? [y]es / [n]o / [q]uit: ")

		if !scanner.Scan() {
			return agent.ConfirmAbort, nil
		}
		switch strings.TrimSpace(strings.ToLower(scanner.Text())) {
		case "y", "yes", "":
			return agent.ConfirmAllow, nil
		case "q", "quit":
			return agent.ConfirmAbort, nil
		default:
			return agent.ConfirmDeny, nil
		}
	}
}

// policyConfirm returns a ConfirmToolCall based on a simple allow-list.
// Allows read-only tools; denies anything that modifies the filesystem.
func policyConfirm() func(string, map[string]any) (agent.ConfirmResult, error) {
	readOnly := map[string]bool{
		"read": true, "grep": true, "find": true, "ls": true,
	}
	return func(name string, args map[string]any) (agent.ConfirmResult, error) {
		if readOnly[name] {
			fmt.Printf("\n[policy] allowed: %s\n", name)
			return agent.ConfirmAllow, nil
		}
		if name == "bash" {
			cmd, _ := args["command"].(string)
			if isReadOnlyBash(cmd) {
				fmt.Printf("\n[policy] allowed bash: %q\n", cmd)
				return agent.ConfirmAllow, nil
			}
			fmt.Printf("\n[policy] denied bash: %q\n", cmd)
			return agent.ConfirmDeny, nil
		}
		fmt.Printf("\n[policy] denied: %s\n", name)
		return agent.ConfirmDeny, nil
	}
}

// isReadOnlyBash is a simple heuristic; not exhaustive or security-grade.
func isReadOnlyBash(cmd string) bool {
	dangerous := []string{">", ">>", "rm ", "mv ", "cp ", "mkdir", "touch", "chmod"}
	lower := strings.ToLower(cmd)
	for _, d := range dangerous {
		if strings.Contains(lower, d) {
			return false
		}
	}
	return true
}
