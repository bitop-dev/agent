// Package anthropic implements the Anthropic Messages API provider.
// This is a native implementation — no OpenAI-compatible proxy needed.
package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bitop-dev/agent/pkg/provider"
	"github.com/bitop-dev/agent/pkg/tool"
)

type Provider struct {
	APIKey     string
	BaseURL    string // default: https://api.anthropic.com
	HTTPClient *http.Client
}

func (p Provider) Name() string { return "anthropic" }

func (p Provider) Stream(ctx context.Context, req provider.CompletionRequest) (<-chan provider.StreamEvent, error) {
	if strings.TrimSpace(p.APIKey) == "" {
		return nil, fmt.Errorf("anthropic provider: API key is required")
	}
	baseURL := p.BaseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	ch := make(chan provider.StreamEvent, 8)
	go func() {
		defer close(ch)
		if err := p.runMessages(ctx, baseURL, req, ch); err != nil {
			ch <- provider.StreamEvent{Err: err}
			return
		}
		ch <- provider.StreamEvent{Type: provider.StreamEventDone}
	}()
	return ch, nil
}

func (p Provider) runMessages(ctx context.Context, baseURL string, req provider.CompletionRequest, ch chan<- provider.StreamEvent) error {
	body := map[string]any{
		"model":      req.Model.Model,
		"max_tokens": 4096,
		"messages":   toAnthropicMessages(req.Messages),
	}
	if strings.TrimSpace(req.System) != "" {
		body["system"] = req.System
	}
	if len(req.Tools) > 0 {
		body["tools"] = toAnthropicTools(req.Tools)
	}

	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	client := p.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(baseURL, "/")+"/v1/messages", bytes.NewReader(data))
	if err != nil {
		return err
	}
	httpReq.Header.Set("x-api-key", p.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("content-type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("anthropic API: %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
	}

	var result struct {
		Content []struct {
			Type  string `json:"type"`
			Text  string `json:"text"`
			ID    string `json:"id"`
			Name  string `json:"name"`
			Input any    `json:"input"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Usage      struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("anthropic decode: %w", err)
	}

	for _, block := range result.Content {
		switch block.Type {
		case "text":
			if strings.TrimSpace(block.Text) != "" {
				ch <- provider.StreamEvent{Type: provider.StreamEventText, Text: block.Text}
			}
		case "tool_use":
			args := make(map[string]any)
			if input, ok := block.Input.(map[string]any); ok {
				args = input
			}
			ch <- provider.StreamEvent{
				Type: provider.StreamEventToolCall,
				ToolCall: tool.Call{
					ID:        block.ID,
					ToolID:    block.Name,
					Arguments: args,
				},
			}
		}
	}

	// Report usage.
	if result.Usage.InputTokens > 0 || result.Usage.OutputTokens > 0 {
		ch <- provider.StreamEvent{
			Type:         provider.StreamEventDone,
			InputTokens:  result.Usage.InputTokens,
			OutputTokens: result.Usage.OutputTokens,
		}
	}

	return nil
}

func toAnthropicMessages(messages []provider.Message) []map[string]any {
	var out []map[string]any
	for _, msg := range messages {
		switch msg.Role {
		case "user":
			out = append(out, map[string]any{"role": "user", "content": msg.Content})
		case "assistant":
			content := []map[string]any{}
			if msg.Content != "" {
				content = append(content, map[string]any{"type": "text", "text": msg.Content})
			}
			for _, tc := range msg.ToolCalls {
				content = append(content, map[string]any{
					"type": "tool_use", "id": tc.ID, "name": tc.ToolID, "input": tc.Arguments,
				})
			}
			out = append(out, map[string]any{"role": "assistant", "content": content})
		case "tool":
			out = append(out, map[string]any{
				"role": "user",
				"content": []map[string]any{{
					"type":        "tool_result",
					"tool_use_id": msg.ToolCallID,
					"content":     msg.Content,
				}},
			})
		}
	}
	return out
}

func toAnthropicTools(defs []tool.Definition) []map[string]any {
	var out []map[string]any
	for _, def := range defs {
		schema := def.Schema
		if len(schema) == 0 {
			schema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, map[string]any{
			"name":         sanitizeName(def.ID),
			"description":  def.Description,
			"input_schema": schema,
		})
	}
	return out
}

func sanitizeName(id string) string {
	var b strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}
