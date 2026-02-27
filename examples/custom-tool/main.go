// examples/custom-tool — demonstrates writing and registering custom compiled-in tools.
//
// This example creates two tools the LLM can call:
//   - "calculator" for basic arithmetic
//   - "roll_dice" for dice rolling
//
// Shows:
//   - Implementing the tools.Tool interface
//   - tools.MustSchema for parameter definitions with enums
//   - Streaming progress updates via onUpdate
//   - Returning structured results with Details
//
// Usage:
//
//	ANTHROPIC_API_KEY=sk-... go run ./examples/custom-tool
//	ANTHROPIC_API_KEY=sk-... go run ./examples/custom-tool "What is 15% of 847? Roll 3d6 too."
package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/nickcecere/agent/pkg/agent"
	"github.com/nickcecere/agent/pkg/ai"
	"github.com/nickcecere/agent/pkg/ai/providers/anthropic"
	"github.com/nickcecere/agent/pkg/tools"
)

// ---------------------------------------------------------------------------
// Calculator tool
// ---------------------------------------------------------------------------

// CalculatorTool performs basic arithmetic.
type CalculatorTool struct{}

func (t *CalculatorTool) Definition() ai.ToolDefinition {
	return ai.ToolDefinition{
		Name: "calculator",
		Description: "Perform a basic arithmetic operation. " +
			"Always use this tool for arithmetic — never compute in your head.",
		Parameters: tools.MustSchema(tools.SimpleSchema{
			Properties: map[string]tools.Property{
				"operation": {
					Type:        "string",
					Description: "The operation to perform",
					Enum:        []any{"add", "subtract", "multiply", "divide", "power", "sqrt"},
				},
				"a": {Type: "number", Description: "First operand"},
				"b": {Type: "number", Description: "Second operand (not required for sqrt)"},
			},
			Required: []string{"operation", "a"},
		}),
	}
}

func (t *CalculatorTool) Execute(
	_ context.Context, _ string,
	params map[string]any, onUpdate tools.UpdateFn,
) (tools.Result, error) {
	op, _ := params["operation"].(string)
	a, _ := params["a"].(float64)
	b, _ := params["b"].(float64)

	if onUpdate != nil {
		onUpdate(tools.TextResult(fmt.Sprintf("Computing %s(%g, %g)...", op, a, b)))
	}

	var result float64
	switch op {
	case "add":
		result = a + b
	case "subtract":
		result = a - b
	case "multiply":
		result = a * b
	case "divide":
		if b == 0 {
			return tools.ErrorResult(fmt.Errorf("division by zero")), nil
		}
		result = a / b
	case "power":
		result = math.Pow(a, b)
	case "sqrt":
		if a < 0 {
			return tools.ErrorResult(fmt.Errorf("cannot sqrt negative number")), nil
		}
		result = math.Sqrt(a)
	default:
		return tools.ErrorResult(fmt.Errorf("unknown operation: %s", op)), nil
	}

	var expr string
	if op == "sqrt" {
		expr = fmt.Sprintf("√%g = %g", a, result)
	} else {
		expr = fmt.Sprintf("%g %s %g = %g", a, opSymbol(op), b, result)
	}
	return tools.Result{
		Content: []ai.ContentBlock{ai.TextContent{Type: "text", Text: expr}},
		Details: map[string]any{"result": result, "op": op, "a": a, "b": b},
	}, nil
}

func opSymbol(op string) string {
	switch op {
	case "add":
		return "+"
	case "subtract":
		return "−"
	case "multiply":
		return "×"
	case "divide":
		return "÷"
	case "power":
		return "^"
	}
	return op
}

// ---------------------------------------------------------------------------
// DiceRoll tool
// ---------------------------------------------------------------------------

type DiceRollTool struct{}

func (t *DiceRollTool) Definition() ai.ToolDefinition {
	return ai.ToolDefinition{
		Name:        "roll_dice",
		Description: "Roll one or more dice and return the individual rolls and total.",
		Parameters: tools.MustSchema(tools.SimpleSchema{
			Properties: map[string]tools.Property{
				"count": {Type: "number", Description: "Number of dice to roll (1–20)"},
				"sides": {Type: "number", Description: "Sides per die (e.g. 4, 6, 8, 10, 12, 20, 100)"},
			},
			Required: []string{"count", "sides"},
		}),
	}
}

func (t *DiceRollTool) Execute(
	_ context.Context, _ string,
	params map[string]any, _ tools.UpdateFn,
) (tools.Result, error) {
	count := int(params["count"].(float64))
	sides := int(params["sides"].(float64))
	if count < 1 || count > 20 {
		return tools.ErrorResult(fmt.Errorf("count must be 1–20")), nil
	}
	if sides < 2 {
		return tools.ErrorResult(fmt.Errorf("sides must be ≥ 2")), nil
	}

	// Simple LCG pseudo-random (not crypto-safe, fine for dice)
	seed := time.Now().UnixNano()
	rolls := make([]int, count)
	total := 0
	for i := range rolls {
		seed = seed*6364136223846793005 + 1442695040888963407
		rolls[i] = int(uint64(seed)>>33)%sides + 1
		total += rolls[i]
	}

	return tools.TextResult(fmt.Sprintf(
		"Rolled %dd%d: %v → total %d", count, sides, rolls, total,
	)), nil
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

func main() {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "ANTHROPIC_API_KEY is not set")
		os.Exit(1)
	}

	reg := tools.NewRegistry()
	reg.Register(&CalculatorTool{})
	reg.Register(&DiceRollTool{})

	a := agent.New(agent.Options{
		Provider: anthropic.New(apiKey),
		Model:    "claude-sonnet-4-5",
		Tools:    reg,
		SystemPrompt: "You are a helpful assistant with access to a calculator " +
			"and dice roller. Always use the calculator tool for arithmetic.",
	})

	a.Subscribe(func(e agent.Event) {
		switch e.Type {
		case agent.EventMessageUpdate:
			if se := e.StreamEvent; se != nil && se.Type == ai.StreamEventTextDelta {
				fmt.Print(se.Delta)
			}
		case agent.EventToolStart:
			fmt.Printf("\n[tool] %s(%v)\n", e.ToolName, e.ToolArgs)
		}
	})

	prompt := "What is 15% of 847? Also roll 3d6 for me."
	if len(os.Args) > 1 {
		prompt = strings.Join(os.Args[1:], " ")
	}

	fmt.Printf("Prompt: %s\n\nResponse: ", prompt)
	if err := a.Prompt(context.Background(), prompt, agent.Config{
		StreamOptions: ai.StreamOptions{MaxTokens: 1024},
	}); err != nil {
		fmt.Fprintln(os.Stderr, "\nerror:", err)
		os.Exit(1)
	}
	fmt.Println()
}
