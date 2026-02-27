package agent

import (
	"testing"
	"time"

	"github.com/bitop-dev/agent/pkg/ai"
)

func userMsg(text string) ai.UserMessage {
	return ai.UserMessage{
		Role:      ai.RoleUser,
		Content:   []ai.ContentBlock{ai.TextContent{Type: "text", Text: text}},
		Timestamp: time.Now().UnixMilli(),
	}
}

func assistantMsg(text string, inputTokens, outputTokens int) ai.AssistantMessage {
	return ai.AssistantMessage{
		Role:       ai.RoleAssistant,
		Content:    []ai.ContentBlock{ai.TextContent{Type: "text", Text: text}},
		StopReason: ai.StopReasonStop,
		Usage: ai.Usage{
			Input:       inputTokens,
			Output:      outputTokens,
			TotalTokens: inputTokens + outputTokens,
		},
		Timestamp: time.Now().UnixMilli(),
	}
}

func TestEstimateContextTokens_NoUsage(t *testing.T) {
	// 400 chars of text ≈ 100 tokens (chars/4)
	msgs := []ai.Message{
		userMsg(string(make([]byte, 400))),
	}
	usage := EstimateContextTokens(msgs)
	if usage.Tokens < 90 || usage.Tokens > 110 {
		t.Errorf("expected ~100 tokens, got %d", usage.Tokens)
	}
	if usage.UsageTokens != 0 {
		t.Errorf("expected 0 usage tokens, got %d", usage.UsageTokens)
	}
}

func TestEstimateContextTokens_WithUsage(t *testing.T) {
	msgs := []ai.Message{
		userMsg("hello"),
		assistantMsg("world", 1000, 100), // known usage: 1100 tokens
		// trailing tool result (not yet counted in usage)
		ai.ToolResultMessage{
			Role:       ai.RoleToolResult,
			ToolCallID: "x",
			Content:    []ai.ContentBlock{ai.TextContent{Type: "text", Text: string(make([]byte, 400))}}, // ~100 tokens
		},
	}

	usage := EstimateContextTokens(msgs)

	if usage.UsageTokens != 1100 {
		t.Errorf("UsageTokens = %d, want 1100", usage.UsageTokens)
	}
	if usage.TrailingTokens < 90 || usage.TrailingTokens > 110 {
		t.Errorf("TrailingTokens = %d, want ~100", usage.TrailingTokens)
	}
	if usage.Tokens != usage.UsageTokens+usage.TrailingTokens {
		t.Errorf("Tokens (%d) != UsageTokens (%d) + TrailingTokens (%d)",
			usage.Tokens, usage.UsageTokens, usage.TrailingTokens)
	}
}

func TestEstimateContextTokens_SkipsAborted(t *testing.T) {
	aborted := ai.AssistantMessage{
		Role:       ai.RoleAssistant,
		StopReason: ai.StopReasonAborted,
		Usage:      ai.Usage{TotalTokens: 99999}, // should be ignored
	}
	msgs := []ai.Message{
		userMsg("hi"),
		aborted,
	}
	usage := EstimateContextTokens(msgs)
	// Should fall back to estimation, not use the aborted usage.
	if usage.UsageTokens != 0 {
		t.Errorf("UsageTokens should be 0 for aborted messages, got %d", usage.UsageTokens)
	}
}
