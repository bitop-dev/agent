package session

import (
	"testing"
	"time"

	"github.com/nickcecere/agent/pkg/ai"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func makeUserMsg(text string) ai.UserMessage {
	return ai.UserMessage{
		Role:      ai.RoleUser,
		Content:   []ai.ContentBlock{ai.TextContent{Type: "text", Text: text}},
		Timestamp: time.Now().UnixMilli(),
	}
}

func makeAssistantMsg(text string) ai.AssistantMessage {
	return ai.AssistantMessage{
		Role:       ai.RoleAssistant,
		Content:    []ai.ContentBlock{ai.TextContent{Type: "text", Text: text}},
		Model:      "test-model",
		Provider:   "test",
		StopReason: ai.StopReasonStop,
		Usage:      ai.Usage{Input: 10, Output: 20, TotalTokens: 30},
		Timestamp:  time.Now().UnixMilli(),
	}
}

func makeToolResultMsg(name, result string) ai.ToolResultMessage {
	return ai.ToolResultMessage{
		Role:       ai.RoleToolResult,
		ToolCallID: "call-1",
		ToolName:   name,
		Content:    []ai.ContentBlock{ai.TextContent{Type: "text", Text: result}},
		Timestamp:  time.Now().UnixMilli(),
	}
}

// ---------------------------------------------------------------------------
// Message serialisation round-trip
// ---------------------------------------------------------------------------

func TestMarshalUnmarshalUserMessage(t *testing.T) {
	orig := makeUserMsg("hello world")
	data, err := MarshalMessage(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := UnmarshalMessage("user", data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	um, ok := got.(ai.UserMessage)
	if !ok {
		t.Fatalf("got type %T, want UserMessage", got)
	}
	if len(um.Content) != 1 {
		t.Fatalf("content len %d, want 1", len(um.Content))
	}
	tc, ok := um.Content[0].(ai.TextContent)
	if !ok {
		t.Fatalf("content[0] is %T, want TextContent", um.Content[0])
	}
	if tc.Text != "hello world" {
		t.Errorf("text = %q, want %q", tc.Text, "hello world")
	}
}

func TestMarshalUnmarshalAssistantMessage(t *testing.T) {
	orig := makeAssistantMsg("I will help")
	data, err := MarshalMessage(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := UnmarshalMessage("assistant", data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	am, ok := got.(ai.AssistantMessage)
	if !ok {
		t.Fatalf("got type %T, want AssistantMessage", got)
	}
	if am.Model != "test-model" {
		t.Errorf("model = %q", am.Model)
	}
	if am.Usage.TotalTokens != 30 {
		t.Errorf("usage total = %d, want 30", am.Usage.TotalTokens)
	}
}

func TestMarshalUnmarshalToolResultMessage(t *testing.T) {
	orig := makeToolResultMsg("bash", "exit 0")
	data, err := MarshalMessage(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := UnmarshalMessage("tool_result", data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	tr, ok := got.(ai.ToolResultMessage)
	if !ok {
		t.Fatalf("got type %T, want ToolResultMessage", got)
	}
	if tr.ToolName != "bash" {
		t.Errorf("tool_name = %q", tr.ToolName)
	}
}

func TestMarshalUnmarshalAllContentTypes(t *testing.T) {
	msg := ai.AssistantMessage{
		Role: ai.RoleAssistant,
		Content: []ai.ContentBlock{
			ai.TextContent{Type: "text", Text: "here is my plan"},
			ai.ThinkingContent{Type: "thinking", Thinking: "<ponder>"},
			ai.ToolCall{Type: "tool_call", ID: "x1", Name: "bash", Arguments: map[string]any{"cmd": "ls"}},
		},
		StopReason: ai.StopReasonStop,
		Timestamp:  time.Now().UnixMilli(),
	}

	data, err := MarshalMessage(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := UnmarshalMessage("assistant", data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	am := got.(ai.AssistantMessage)
	if len(am.Content) != 3 {
		t.Fatalf("content len %d, want 3", len(am.Content))
	}
	if _, ok := am.Content[0].(ai.TextContent); !ok {
		t.Error("content[0] should be TextContent")
	}
	if _, ok := am.Content[1].(ai.ThinkingContent); !ok {
		t.Error("content[1] should be ThinkingContent")
	}
	tc, ok := am.Content[2].(ai.ToolCall)
	if !ok {
		t.Error("content[2] should be ToolCall")
	} else if tc.Name != "bash" {
		t.Errorf("tool name = %q", tc.Name)
	}
}

// ---------------------------------------------------------------------------
// Session create/load/messages
// ---------------------------------------------------------------------------

func TestCreateAndLoadSession(t *testing.T) {
	dir := t.TempDir()

	// Create new session.
	sess, err := Create(dir, "/test/cwd")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sess.ID() == "" {
		t.Error("session ID should not be empty")
	}

	// Append some messages.
	_, err = sess.AppendMessage(makeUserMsg("hello"))
	if err != nil {
		t.Fatalf("AppendMessage user: %v", err)
	}
	_, err = sess.AppendMessage(makeAssistantMsg("hi there"))
	if err != nil {
		t.Fatalf("AppendMessage assistant: %v", err)
	}
	_, err = sess.AppendMessage(makeToolResultMsg("bash", "ok"))
	if err != nil {
		t.Fatalf("AppendMessage tool_result: %v", err)
	}
	sess.Close()

	// Load session.
	sess2, err := Load(dir, sess.ID()[:8])
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer sess2.Close()

	msgs, err := sess2.Messages()
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("got %d messages, want 3", len(msgs))
	}
	if msgs[0].GetRole() != ai.RoleUser {
		t.Errorf("msgs[0] role = %v", msgs[0].GetRole())
	}
	if msgs[1].GetRole() != ai.RoleAssistant {
		t.Errorf("msgs[1] role = %v", msgs[1].GetRole())
	}
	if msgs[2].GetRole() != ai.RoleToolResult {
		t.Errorf("msgs[2] role = %v", msgs[2].GetRole())
	}
}

// ---------------------------------------------------------------------------
// ParseMessages with compaction
// ---------------------------------------------------------------------------

func TestParseMessagesWithCompaction(t *testing.T) {
	dir := t.TempDir()

	sess, err := Create(dir, "/cwd")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Write 4 messages: u1 a1 u2 a2
	id1, _ := sess.AppendMessage(makeUserMsg("first question"))
	_, _ = sess.AppendMessage(makeAssistantMsg("first answer"))
	firstKeptID, _ := sess.AppendMessage(makeUserMsg("second question"))
	_, _ = sess.AppendMessage(makeAssistantMsg("second answer"))

	_ = id1 // suppress unused warning

	// Simulate compaction: summarise messages 0-1, keep messages 2-3.
	err = sess.AppendCompaction("Summary of early conversation.", firstKeptID, 500)
	if err != nil {
		t.Fatalf("AppendCompaction: %v", err)
	}

	// Append more messages after compaction.
	_, _ = sess.AppendMessage(makeUserMsg("third question"))
	_, _ = sess.AppendMessage(makeAssistantMsg("third answer"))
	sess.Close()

	// Load and parse.
	sess2, err := Load(dir, sess.ID()[:8])
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer sess2.Close()

	msgs, err := sess2.Messages()
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}

	// Expected: [summary user msg, u2, a2, u3, a3] = 5 messages
	// (u1 and a1 were compacted away)
	if len(msgs) != 5 {
		t.Fatalf("got %d messages, want 5:\n%v", len(msgs), roles(msgs))
	}

	// First message should be the summary.
	um, ok := msgs[0].(ai.UserMessage)
	if !ok {
		t.Fatalf("msgs[0] should be UserMessage, got %T", msgs[0])
	}
	tc := um.Content[0].(ai.TextContent)
	if !contains(tc.Text, "compacted") {
		t.Errorf("summary msg should contain 'compacted', got: %q", tc.Text[:80])
	}
	if !contains(tc.Text, "Summary of early conversation.") {
		t.Errorf("summary msg should contain the summary text")
	}
}

func roles(msgs []ai.Message) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		out[i] = string(m.GetRole())
	}
	return out
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
