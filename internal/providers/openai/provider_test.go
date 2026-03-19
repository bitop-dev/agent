package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ncecere/agent/pkg/provider"
	"github.com/ncecere/agent/pkg/tool"
)

func TestProviderChatModeToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message": map[string]any{
					"tool_calls": []any{map[string]any{
						"function": map[string]any{
							"name":      "core/read",
							"arguments": `{"path":"AGENTS.md"}`,
						},
					}},
				},
			}},
		})
	}))
	defer server.Close()

	p := Provider{BaseURL: server.URL, APIKey: "test-key", APIMode: apiModeChat, HTTPClient: server.Client()}
	stream, err := p.Stream(context.Background(), provider.CompletionRequest{
		Model:    provider.ModelRef{Model: "gpt-4.1"},
		Messages: []provider.Message{{Role: "user", Content: "read AGENTS.md"}},
		Tools:    []tool.Definition{{ID: "core/read"}},
	})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	var gotTool bool
	for event := range stream {
		if event.Err != nil {
			t.Fatalf("event error: %v", event.Err)
		}
		if event.Type == provider.StreamEventToolCall {
			gotTool = true
			if event.ToolCall.ToolID != "core/read" {
				t.Fatalf("unexpected tool id: %s", event.ToolCall.ToolID)
			}
		}
	}
	if !gotTool {
		t.Fatal("expected tool call event")
	}
}

func TestProviderChatModeEncodesToolContinuation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		messages, ok := body["messages"].([]any)
		if !ok || len(messages) < 2 {
			t.Fatalf("expected at least 2 messages, got %#v", body["messages"])
		}
		assistant, ok := messages[0].(map[string]any)
		if !ok {
			t.Fatalf("assistant message missing")
		}
		toolCalls, ok := assistant["tool_calls"].([]any)
		if !ok || len(toolCalls) != 1 {
			t.Fatalf("expected one tool call, got %#v", assistant["tool_calls"])
		}
		toolMessage, ok := messages[1].(map[string]any)
		if !ok {
			t.Fatalf("tool message missing")
		}
		if toolMessage["role"] != "tool" {
			t.Fatalf("unexpected tool role: %#v", toolMessage["role"])
		}
		if toolMessage["tool_call_id"] != "call_123" {
			t.Fatalf("unexpected tool_call_id: %#v", toolMessage["tool_call_id"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message": map[string]any{
					"content": "continued after tool",
				},
			}},
		})
	}))
	defer server.Close()

	p := Provider{BaseURL: server.URL, APIKey: "test-key", APIMode: apiModeChat, HTTPClient: server.Client()}
	stream, err := p.Stream(context.Background(), provider.CompletionRequest{
		Model: provider.ModelRef{Model: "gpt-4.1"},
		Messages: []provider.Message{
			{Role: "assistant", ToolCalls: []tool.Call{{ID: "call_123", ToolID: "core/read", Arguments: map[string]any{"path": "AGENTS.md"}}}},
			{Role: "tool", ToolCallID: "call_123", ToolName: "core/read", Content: "file contents"},
		},
	})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	var gotText string
	for event := range stream {
		if event.Err != nil {
			t.Fatalf("event error: %v", event.Err)
		}
		if event.Type == provider.StreamEventText {
			gotText = strings.TrimSpace(event.Text)
		}
	}
	if gotText != "continued after tool" {
		t.Fatalf("unexpected text: %q", gotText)
	}
}

func TestProviderChatModeSSEStreaming(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["stream"] != true {
			t.Fatalf("expected stream=true, got %#v", body["stream"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"Hello"}}]}`)
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":", world"}}]}`)
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_sse_1","function":{"name":"core/read","arguments":""}}]}}]}`)
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":\"AGENTS.md\"}"}}]}}]}`)
		fmt.Fprintln(w, `data: [DONE]`)
	}))
	defer server.Close()

	p := Provider{BaseURL: server.URL, APIKey: "test-key", APIMode: apiModeChat, HTTPClient: server.Client()}
	stream, err := p.Stream(context.Background(), provider.CompletionRequest{
		Model:    provider.ModelRef{Model: "gpt-4.1"},
		Messages: []provider.Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	var textParts []string
	var gotToolCall bool
	for event := range stream {
		if event.Err != nil {
			t.Fatalf("event error: %v", event.Err)
		}
		switch event.Type {
		case provider.StreamEventText:
			textParts = append(textParts, event.Text)
		case provider.StreamEventToolCall:
			gotToolCall = true
			if event.ToolCall.ToolID != "core/read" {
				t.Fatalf("unexpected tool id: %s", event.ToolCall.ToolID)
			}
			if event.ToolCall.ID != "call_sse_1" {
				t.Fatalf("unexpected tool call id: %s", event.ToolCall.ID)
			}
		}
	}
	if joined := strings.Join(textParts, ""); joined != "Hello, world" {
		t.Fatalf("unexpected text: %q", joined)
	}
	if !gotToolCall {
		t.Fatal("expected SSE tool call event")
	}
}

func TestProviderResponsesModeText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"output_text": "hello from responses",
		})
	}))
	defer server.Close()

	p := Provider{BaseURL: server.URL, APIKey: "test-key", APIMode: apiModeResponses, HTTPClient: server.Client()}
	stream, err := p.Stream(context.Background(), provider.CompletionRequest{
		Model:    provider.ModelRef{Model: "gpt-4.1"},
		Messages: []provider.Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	var gotText string
	for event := range stream {
		if event.Err != nil {
			t.Fatalf("event error: %v", event.Err)
		}
		if event.Type == provider.StreamEventText {
			gotText = event.Text
		}
	}
	if gotText != "hello from responses" {
		t.Fatalf("unexpected text: %q", gotText)
	}
}
