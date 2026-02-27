package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nickcecere/agent/pkg/ai"
)

// mockProvider is an ai.Provider that returns a canned response.
type mockProvider struct {
	events []ai.StreamEvent
	result *ai.AssistantMessage
}

func (m *mockProvider) Name() string { return "mock" }

func (m *mockProvider) Stream(ctx context.Context, model string, llmCtx ai.Context, opts ai.StreamOptions) (<-chan ai.StreamEvent, func() (*ai.AssistantMessage, error)) {
	ch := make(chan ai.StreamEvent, len(m.events))
	for _, ev := range m.events {
		ch <- ev
	}
	close(ch)
	return ch, func() (*ai.AssistantMessage, error) {
		return m.result, nil
	}
}

func textMsg(text string) *ai.AssistantMessage {
	return &ai.AssistantMessage{
		Role:       ai.RoleAssistant,
		Content:    []ai.ContentBlock{ai.TextContent{Type: "text", Text: text}},
		Model:      "test-model",
		Provider:   "mock",
		StopReason: ai.StopReasonStop,
		Usage:      ai.Usage{Input: 10, Output: 5, TotalTokens: 15},
		Timestamp:  time.Now().UnixMilli(),
	}
}

// ---------------------------------------------------------------------------
// Round-trip: client → handler → mock provider → client
// ---------------------------------------------------------------------------

func testServer(t *testing.T, prov ai.Provider, token string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(NewHandler(prov, token))
}

func TestProxyRoundTrip_TextResponse(t *testing.T) {
	mock := &mockProvider{
		events: []ai.StreamEvent{
			{Type: ai.StreamEventTextDelta, Delta: "Hello"},
			{Type: ai.StreamEventTextDelta, Delta: ", world!"},
		},
		result: textMsg("Hello, world!"),
	}

	srv := testServer(t, mock, "")
	defer srv.Close()

	client := New(srv.URL, "")
	ch, wait := client.Stream(context.Background(), "test-model", ai.Context{
		SystemPrompt: "You are helpful.",
		Messages: []ai.Message{
			ai.UserMessage{
				Role:    ai.RoleUser,
				Content: []ai.ContentBlock{ai.TextContent{Type: "text", Text: "hi"}},
			},
		},
	}, ai.StreamOptions{MaxTokens: 512})

	// Drain events.
	var deltas []string
	for ev := range ch {
		if ev.Type == ai.StreamEventTextDelta {
			deltas = append(deltas, ev.Delta)
		}
	}

	result, err := wait()
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if result.StopReason != ai.StopReasonStop {
		t.Errorf("stop_reason = %q, want stop", result.StopReason)
	}
	if result.Usage.TotalTokens != 15 {
		t.Errorf("total_tokens = %d, want 15", result.Usage.TotalTokens)
	}

	combined := ""
	for _, d := range deltas {
		combined += d
	}
	if combined != "Hello, world!" {
		t.Errorf("streamed text = %q, want %q", combined, "Hello, world!")
	}
}

func TestProxyRoundTrip_ToolCall(t *testing.T) {
	partialWithTool := &ai.AssistantMessage{
		Role: ai.RoleAssistant,
		Content: []ai.ContentBlock{
			ai.ToolCall{Type: "tool_call", ID: "c1", Name: "bash", Arguments: map[string]any{}},
		},
	}
	result := &ai.AssistantMessage{
		Role: ai.RoleAssistant,
		Content: []ai.ContentBlock{
			ai.ToolCall{Type: "tool_call", ID: "c1", Name: "bash", Arguments: map[string]any{"command": "ls"}},
		},
		StopReason: ai.StopReasonTool,
		Timestamp:  time.Now().UnixMilli(),
	}
	mock := &mockProvider{
		events: []ai.StreamEvent{
			{Type: ai.StreamEventToolCallStart, Partial: partialWithTool, Delta: "bash"},
			{Type: ai.StreamEventToolCallDelta, Partial: partialWithTool, Delta: `{"command":"ls"}`},
			{Type: ai.StreamEventToolCallEnd, Partial: partialWithTool},
		},
		result: result,
	}

	srv := testServer(t, mock, "")
	defer srv.Close()

	client := New(srv.URL, "")
	ch, wait := client.Stream(context.Background(), "test-model", ai.Context{}, ai.StreamOptions{})

	// Drain events.
	for range ch {
	}

	got, err := wait()
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if got.StopReason != ai.StopReasonTool {
		t.Errorf("stop_reason = %q, want tool_use", got.StopReason)
	}

	// Find the reconstructed tool call.
	var found bool
	for _, b := range got.Content {
		if tc, ok := b.(ai.ToolCall); ok && tc.Name == "bash" {
			found = true
			if tc.Arguments["command"] != "ls" {
				t.Errorf("tool args = %v", tc.Arguments)
			}
		}
	}
	if !found {
		t.Error("response should contain a bash tool call")
	}
}

func TestProxyAuth_RequiresToken(t *testing.T) {
	mock := &mockProvider{result: textMsg("hi")}
	srv := testServer(t, mock, "secret-token")
	defer srv.Close()

	// Without token → 401.
	resp, _ := http.Post(srv.URL+"/stream", "application/json", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}

	// With wrong token → 401.
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/stream", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	req.Header.Set("Content-Type", "application/json")
	resp2, _ := http.DefaultClient.Do(req)
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong token, got %d", resp2.StatusCode)
	}
}

func TestProxyMessageEncoding_AllRoles(t *testing.T) {
	var receivedMsgs []ai.Message
	capturer := &capturingProvider{onStream: func(msgs []ai.Message) {
		receivedMsgs = msgs
	}}

	srv := testServer(t, capturer, "")
	defer srv.Close()

	client := New(srv.URL, "")
	_, wait := client.Stream(context.Background(), "test-model", ai.Context{
		Messages: []ai.Message{
			ai.UserMessage{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextContent{Type: "text", Text: "user msg"}}},
			ai.AssistantMessage{Role: ai.RoleAssistant, Content: []ai.ContentBlock{ai.TextContent{Type: "text", Text: "assistant msg"}}, StopReason: ai.StopReasonStop},
			ai.ToolResultMessage{Role: ai.RoleToolResult, ToolName: "bash", Content: []ai.ContentBlock{ai.TextContent{Type: "text", Text: "result"}}},
		},
	}, ai.StreamOptions{})

	wait()

	if len(receivedMsgs) != 3 {
		t.Fatalf("server received %d messages, want 3", len(receivedMsgs))
	}
	if receivedMsgs[0].GetRole() != ai.RoleUser {
		t.Errorf("msg[0] role = %v", receivedMsgs[0].GetRole())
	}
	if receivedMsgs[1].GetRole() != ai.RoleAssistant {
		t.Errorf("msg[1] role = %v", receivedMsgs[1].GetRole())
	}
	if receivedMsgs[2].GetRole() != ai.RoleToolResult {
		t.Errorf("msg[2] role = %v", receivedMsgs[2].GetRole())
	}
}

// capturingProvider captures the messages it receives and returns an empty response.
type capturingProvider struct {
	onStream func([]ai.Message)
}

func (c *capturingProvider) Name() string { return "capturing" }

func (c *capturingProvider) Stream(_ context.Context, _ string, llmCtx ai.Context, _ ai.StreamOptions) (<-chan ai.StreamEvent, func() (*ai.AssistantMessage, error)) {
	if c.onStream != nil {
		c.onStream(llmCtx.Messages)
	}
	ch := make(chan ai.StreamEvent)
	close(ch)
	return ch, func() (*ai.AssistantMessage, error) {
		return &ai.AssistantMessage{
			Role: ai.RoleAssistant, StopReason: ai.StopReasonStop,
			Timestamp: time.Now().UnixMilli(),
		}, nil
	}
}
