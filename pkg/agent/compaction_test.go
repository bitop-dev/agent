package agent

import (
	"testing"
	"time"

	"github.com/bitop-dev/agent/pkg/ai"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func cmpUserMsg(text string) ai.UserMessage {
	return ai.UserMessage{
		Role:      ai.RoleUser,
		Content:   []ai.ContentBlock{ai.TextContent{Type: "text", Text: text}},
		Timestamp: time.Now().UnixMilli(),
	}
}

func cmpAssistantMsg(text string, tokens int) ai.AssistantMessage {
	return ai.AssistantMessage{
		Role:       ai.RoleAssistant,
		Content:    []ai.ContentBlock{ai.TextContent{Type: "text", Text: text}},
		StopReason: ai.StopReasonStop,
		Usage:      ai.Usage{Input: tokens, Output: 50, TotalTokens: tokens + 50},
		Timestamp:  time.Now().UnixMilli(),
	}
}

func cmpToolCallMsg(toolName string) ai.AssistantMessage {
	return ai.AssistantMessage{
		Role: ai.RoleAssistant,
		Content: []ai.ContentBlock{
			ai.ToolCall{Type: "tool_call", ID: "c1", Name: toolName, Arguments: map[string]any{}},
		},
		StopReason: ai.StopReasonTool,
		Timestamp:  time.Now().UnixMilli(),
	}
}

func cmpToolResult(name, result string) ai.ToolResultMessage {
	return ai.ToolResultMessage{
		Role:       ai.RoleToolResult,
		ToolCallID: "c1",
		ToolName:   name,
		Content:    []ai.ContentBlock{ai.TextContent{Type: "text", Text: result}},
		Timestamp:  time.Now().UnixMilli(),
	}
}

// ---------------------------------------------------------------------------
// ShouldCompact
// ---------------------------------------------------------------------------

func TestShouldCompact_Disabled(t *testing.T) {
	cfg := CompactionConfig{Enabled: false, ContextWindow: 100000}
	if ShouldCompact(90000, cfg) {
		t.Error("should not compact when Enabled=false")
	}
}

func TestShouldCompact_NoWindow(t *testing.T) {
	cfg := CompactionConfig{Enabled: true, ContextWindow: 0}
	if ShouldCompact(90000, cfg) {
		t.Error("should not compact when ContextWindow=0")
	}
}

func TestShouldCompact_BelowThreshold(t *testing.T) {
	cfg := CompactionConfig{Enabled: true, ContextWindow: 100000, ReserveTokens: 16384}
	// threshold = 100000 - 16384 = 83616
	if ShouldCompact(80000, cfg) {
		t.Error("should not compact when below threshold")
	}
}

func TestShouldCompact_AboveThreshold(t *testing.T) {
	cfg := CompactionConfig{Enabled: true, ContextWindow: 100000, ReserveTokens: 16384}
	// threshold = 83616; 85000 > 83616 → compact
	if !ShouldCompact(85000, cfg) {
		t.Error("should compact when above threshold")
	}
}

// ---------------------------------------------------------------------------
// FindCutPoint
// ---------------------------------------------------------------------------

func TestFindCutPoint_TooShort(t *testing.T) {
	msgs := []ai.Message{cmpUserMsg("hi"), cmpAssistantMsg("hello", 100)}
	if FindCutPoint(msgs, 500) != -1 {
		t.Error("too-short conversation should return -1")
	}
}

func TestFindCutPoint_FindsUserBoundary(t *testing.T) {
	// Build a conversation with enough tokens that the cut falls before u2.
	// u1=small, a1=large, u2=small, a2=small, u3=small, a3=small
	large := make([]byte, 4000) // ~1000 tokens
	for i := range large {
		large[i] = 'x'
	}

	msgs := []ai.Message{
		cmpUserMsg("u1"),
		cmpAssistantMsg(string(large), 2000), // large: ~1000 char tokens
		cmpUserMsg("u2"),
		cmpAssistantMsg("a2", 50),
		cmpUserMsg("u3"),
		cmpAssistantMsg("a3", 50),
	}

	// Keep recent 500 tokens; the large a1 should push the cut before it
	// so we end up starting from u2 or later.
	cut := FindCutPoint(msgs, 500)
	if cut <= 0 {
		t.Errorf("expected a valid cut point, got %d", cut)
	}
	_, isUser := msgs[cut].(ai.UserMessage)
	if !isUser {
		t.Errorf("cut point (index %d) should be at a UserMessage, got %T", cut, msgs[cut])
	}
}

func TestFindCutPoint_CutsAtUserMessage(t *testing.T) {
	msgs := []ai.Message{
		cmpUserMsg("first"),
		cmpAssistantMsg("response 1", 100),
		cmpUserMsg("second"),
		cmpAssistantMsg("response 2", 100),
		cmpUserMsg("third"),
		cmpAssistantMsg("response 3", 100),
		cmpUserMsg("fourth"),
		cmpAssistantMsg("response 4", 100),
	}

	cut := FindCutPoint(msgs, 5) // keep very few tokens → cut early
	if cut <= 0 {
		t.Fatalf("expected valid cut, got %d", cut)
	}
	if _, ok := msgs[cut].(ai.UserMessage); !ok {
		t.Errorf("cut at %d is %T, want UserMessage", cut, msgs[cut])
	}
}

// ---------------------------------------------------------------------------
// serializeConversation
// ---------------------------------------------------------------------------

func TestSerializeConversation(t *testing.T) {
	msgs := []ai.Message{
		cmpUserMsg("What time is it?"),
		cmpAssistantMsg("It is noon.", 20),
	}
	text := serializeConversation(msgs)
	if text == "" {
		t.Error("serialized conversation should not be empty")
	}
	if !containsStr(text, "[USER]") {
		t.Error("should contain [USER] marker")
	}
	if !containsStr(text, "[ASSISTANT]") {
		t.Error("should contain [ASSISTANT] marker")
	}
	if !containsStr(text, "What time is it?") {
		t.Error("should contain user message text")
	}
}

func TestSerializeConversation_ToolCallsAndResults(t *testing.T) {
	msgs := []ai.Message{
		cmpUserMsg("Run ls"),
		cmpToolCallMsg("bash"),
		cmpToolResult("bash", "file1.go\nfile2.go"),
	}
	text := serializeConversation(msgs)
	if !containsStr(text, "[TOOL CALL: bash]") {
		t.Error("should contain tool call marker")
	}
	if !containsStr(text, "[TOOL RESULT: bash]") {
		t.Error("should contain tool result marker")
	}
}

func containsStr(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}())
}
