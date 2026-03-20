package mock

import (
	"context"
	"fmt"
	"strings"

	"github.com/bitop-dev/agent/pkg/provider"
	"github.com/bitop-dev/agent/pkg/tool"
)

type Provider struct{}

func (Provider) Name() string {
	return "mock"
}

func (Provider) Stream(_ context.Context, req provider.CompletionRequest) (<-chan provider.StreamEvent, error) {
	ch := make(chan provider.StreamEvent, 2)
	go func() {
		defer close(ch)
		last := lastMessage(req.Messages)
		if last.Role == "tool" {
			ch <- provider.StreamEvent{
				Type: provider.StreamEventText,
				Text: fmt.Sprintf("Tool result received:\n%s", last.Content),
			}
			ch <- provider.StreamEvent{Type: provider.StreamEventDone}
			return
		}

		prompt := strings.TrimSpace(last.Content)
		switch {
		case strings.HasPrefix(prompt, "read "):
			ch <- provider.StreamEvent{Type: provider.StreamEventToolCall, ToolCall: tool.Call{ToolID: "core/read", Arguments: map[string]any{"path": strings.TrimSpace(strings.TrimPrefix(prompt, "read "))}}}
		case strings.HasPrefix(prompt, "write "):
			path, content := splitTwo(strings.TrimSpace(strings.TrimPrefix(prompt, "write ")))
			ch <- provider.StreamEvent{Type: provider.StreamEventToolCall, ToolCall: tool.Call{ToolID: "core/write", Arguments: map[string]any{"path": path, "content": content}}}
		case strings.HasPrefix(prompt, "edit "):
			path, old, newVal := splitThree(strings.TrimSpace(strings.TrimPrefix(prompt, "edit ")))
			ch <- provider.StreamEvent{Type: provider.StreamEventToolCall, ToolCall: tool.Call{ToolID: "core/edit", Arguments: map[string]any{"path": path, "old": old, "new": newVal}}}
		case strings.HasPrefix(prompt, "bash "):
			ch <- provider.StreamEvent{Type: provider.StreamEventToolCall, ToolCall: tool.Call{ToolID: "core/bash", Arguments: map[string]any{"command": strings.TrimSpace(strings.TrimPrefix(prompt, "bash "))}}}
		case strings.HasPrefix(prompt, "search "):
			ch <- provider.StreamEvent{Type: provider.StreamEventToolCall, ToolCall: tool.Call{ToolID: "web/search", Arguments: map[string]any{"query": strings.TrimSpace(strings.TrimPrefix(prompt, "search ")), "topK": 5}}}
		case strings.HasPrefix(prompt, "fetch "):
			ch <- provider.StreamEvent{Type: provider.StreamEventToolCall, ToolCall: tool.Call{ToolID: "web/fetch", Arguments: map[string]any{"url": strings.TrimSpace(strings.TrimPrefix(prompt, "fetch "))}}}
		case strings.HasPrefix(prompt, "draft email "):
			to, subject, body := parseEmailPrompt(strings.TrimSpace(strings.TrimPrefix(prompt, "draft email ")))
			ch <- provider.StreamEvent{Type: provider.StreamEventToolCall, ToolCall: tool.Call{ToolID: "email/draft", Arguments: map[string]any{"to": to, "subject": subject, "body": body}}}
		case strings.HasPrefix(prompt, "send email "):
			to, subject, body := parseEmailPrompt(strings.TrimSpace(strings.TrimPrefix(prompt, "send email ")))
			ch <- provider.StreamEvent{Type: provider.StreamEventToolCall, ToolCall: tool.Call{ToolID: "email/send", Arguments: map[string]any{"to": to, "subject": subject, "body": body}}}
		default:
			ch <- provider.StreamEvent{Type: provider.StreamEventText, Text: fmt.Sprintf("mock provider response: %s", prompt)}
		}
		ch <- provider.StreamEvent{Type: provider.StreamEventDone}
	}()
	return ch, nil
}

func lastMessage(messages []provider.Message) provider.Message {
	if len(messages) == 0 {
		return provider.Message{}
	}
	return messages[len(messages)-1]
}

func splitTwo(input string) (string, string) {
	parts := strings.SplitN(input, ":::", 2)
	if len(parts) < 2 {
		return strings.TrimSpace(input), ""
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
}

func splitThree(input string) (string, string, string) {
	parts := strings.SplitN(input, ":::", 3)
	if len(parts) < 3 {
		first, second := splitTwo(input)
		return first, second, ""
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), strings.TrimSpace(parts[2])
}

func parseEmailPrompt(input string) (string, string, string) {
	parts := strings.SplitN(input, ":::", 3)
	if len(parts) == 3 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), strings.TrimSpace(parts[2])
	}
	return "test@example.com", "Test", strings.TrimSpace(input)
}
