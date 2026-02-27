// Package openai implements the ai.Provider interface for the OpenAI
// chat-completions API (streaming).  It is also compatible with any
// OpenAI-compatible endpoint (Groq, OpenRouter, Azure, …) by setting BaseURL.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/bitop-dev/agent/pkg/ai"
	"github.com/bitop-dev/agent/pkg/ai/sse"
)

const defaultBaseURL = "https://api.openai.com/v1"

// Provider is the OpenAI streaming provider.
type Provider struct {
	BaseURL    string
	HTTPClient *http.Client
}

// New creates a Provider. Pass "" for baseURL to use the default OpenAI endpoint.
func New(baseURL string) *Provider {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Provider{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{Timeout: 10 * time.Minute},
	}
}

func (p *Provider) Name() string { return "openai" }

// ---------------------------------------------------------------------------
// Wire types (OpenAI request/response)
// ---------------------------------------------------------------------------

type wireMessage struct {
	Role       string      `json:"role"`
	Content    interface{} `json:"content"` // string | []wirePart
	ToolCallID string      `json:"tool_call_id,omitempty"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	Name       string      `json:"name,omitempty"`
}

type wirePart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL *struct {
		URL string `json:"url"`
	} `json:"image_url,omitempty"`
}

type wireTool struct {
	Type     string          `json:"type"` // "function"
	Function wireToolFunc    `json:"function"`
}

type wireToolFunc struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type wireToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"` // "function"
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"` // JSON string
	} `json:"function"`
}

type wireRequest struct {
	Model    string        `json:"model"`
	Messages []wireMessage `json:"messages"`
	Tools    []wireTool    `json:"tools,omitempty"`
	Stream   bool          `json:"stream"`
	MaxTokens   int        `json:"max_tokens,omitempty"`
	Temperature *float64   `json:"temperature,omitempty"`
	StreamOptions *struct {
		IncludeUsage bool `json:"include_usage"`
	} `json:"stream_options,omitempty"`
}

// SSE chunk types
type chunkDelta struct {
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	ToolCalls []wireToolCall `json:"tool_calls"`
}

type chunkChoice struct {
	Index        int        `json:"index"`
	Delta        chunkDelta `json:"delta"`
	FinishReason string     `json:"finish_reason"`
}

type chunkUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	PromptTokensDetails *struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details,omitempty"`
}

type streamChunk struct {
	ID      string        `json:"id"`
	Choices []chunkChoice `json:"choices"`
	Usage   *chunkUsage   `json:"usage"`
}

// ---------------------------------------------------------------------------
// Stream implementation
// ---------------------------------------------------------------------------

func (p *Provider) Stream(
	ctx context.Context,
	model string,
	llmCtx ai.Context,
	opts ai.StreamOptions,
) (<-chan ai.StreamEvent, func() (*ai.AssistantMessage, error)) {
	events := make(chan ai.StreamEvent, 64)
	var finalMsg *ai.AssistantMessage
	var finalErr error

	done := make(chan struct{})

	go func() {
		defer close(events)
		defer close(done)
		finalMsg, finalErr = p.stream(ctx, model, llmCtx, opts, events)
	}()

	wait := func() (*ai.AssistantMessage, error) {
		<-done
		return finalMsg, finalErr
	}

	return events, wait
}

func (p *Provider) stream(
	ctx context.Context,
	model string,
	llmCtx ai.Context,
	opts ai.StreamOptions,
	events chan<- ai.StreamEvent,
) (*ai.AssistantMessage, error) {
	// Build request body
	req, err := p.buildRequest(model, llmCtx, opts)
	if err != nil {
		return nil, err
	}

	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+opts.APIKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := p.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai: HTTP %d: %s", resp.StatusCode, string(b))
	}

	// Build partial message progressively
	partial := &ai.AssistantMessage{
		Role:      ai.RoleAssistant,
		Model:     model,
		Provider:  "openai",
		Timestamp: time.Now().UnixMilli(),
	}

	// State for accumulating tool call arguments across deltas
	type tcState struct {
		id    string
		name  string
		args  string
		index int
	}
	tcMap := map[int]*tcState{}

	emitStart := false
	reader := sse.NewReader(resp.Body)

	sendPartial := func(evType ai.StreamEventType, delta string) {
		events <- ai.StreamEvent{Type: evType, Partial: snapshotMsg(partial), Delta: delta}
	}

	for {
		ev, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("openai: sse read: %w", err)
		}
		if ev.Data == "[DONE]" {
			break
		}
		if ev.Data == "" {
			continue
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(ev.Data), &chunk); err != nil {
			continue
		}

		if !emitStart {
			events <- ai.StreamEvent{Type: ai.StreamEventStart, Partial: snapshotMsg(partial)}
			emitStart = true
		}

		if len(chunk.Choices) == 0 {
			// usage-only chunk
			if chunk.Usage != nil {
				partial.Usage.Input = chunk.Usage.PromptTokens
				partial.Usage.Output = chunk.Usage.CompletionTokens
				partial.Usage.TotalTokens = chunk.Usage.TotalTokens
				if chunk.Usage.PromptTokensDetails != nil {
					partial.Usage.CacheRead = chunk.Usage.PromptTokensDetails.CachedTokens
				}
			}
			continue
		}

		choice := chunk.Choices[0]
		delta := choice.Delta

		// Text delta
		if delta.Content != "" {
			idx := findOrAppendText(partial)
			tb := partial.Content[idx].(ai.TextContent)
			tb.Text += delta.Content
			partial.Content[idx] = tb
			sendPartial(ai.StreamEventTextDelta, delta.Content)
		}

		// Tool call deltas
		for _, tc := range delta.ToolCalls {
			idx := tc.Index
			st, exists := tcMap[idx]
			if !exists {
				st = &tcState{index: idx}
				tcMap[idx] = st
			}
			if tc.ID != "" {
				st.id = tc.ID
			}
			if tc.Function.Name != "" {
				st.name = tc.Function.Name
				events <- ai.StreamEvent{Type: ai.StreamEventToolCallStart, Partial: snapshotMsg(partial), Delta: tc.Function.Name}
			}
			if tc.Function.Arguments != "" {
				st.args += tc.Function.Arguments
				events <- ai.StreamEvent{Type: ai.StreamEventToolCallDelta, Partial: snapshotMsg(partial), Delta: tc.Function.Arguments}
			}
		}

		// Finish reason
		if choice.FinishReason != "" {
			partial.StopReason = mapStopReason(choice.FinishReason)
		}
	}

	// Finalise tool calls
	for _, st := range tcMap {
		var args map[string]any
		_ = json.Unmarshal([]byte(st.args), &args)
		partial.Content = append(partial.Content, ai.ToolCall{
			Type:      "tool_call",
			ID:        st.id,
			Name:      st.name,
			Arguments: args,
		})
		events <- ai.StreamEvent{Type: ai.StreamEventToolCallEnd, Partial: snapshotMsg(partial)}
	}

	if partial.StopReason == "" {
		partial.StopReason = ai.StopReasonStop
	}

	events <- ai.StreamEvent{Type: ai.StreamEventDone, Partial: snapshotMsg(partial)}
	return partial, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (p *Provider) buildRequest(model string, llmCtx ai.Context, opts ai.StreamOptions) (wireRequest, error) {
	req := wireRequest{
		Model:       model,
		Stream:      true,
		MaxTokens:   opts.MaxTokens,
		Temperature: opts.Temperature,
		StreamOptions: &struct {
			IncludeUsage bool `json:"include_usage"`
		}{IncludeUsage: true},
	}

	// System message
	if llmCtx.SystemPrompt != "" {
		req.Messages = append(req.Messages, wireMessage{Role: "system", Content: llmCtx.SystemPrompt})
	}

	// Conversation messages
	for _, m := range llmCtx.Messages {
		wm, err := convertMessage(m)
		if err != nil {
			return wireRequest{}, err
		}
		req.Messages = append(req.Messages, wm)
	}

	// Tools
	for _, t := range llmCtx.Tools {
		req.Tools = append(req.Tools, wireTool{
			Type: "function",
			Function: wireToolFunc{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}

	return req, nil
}

func convertMessage(m ai.Message) (wireMessage, error) {
	switch msg := m.(type) {
	case ai.UserMessage:
		wm := wireMessage{Role: "user"}
		parts := make([]wirePart, 0, len(msg.Content))
		for _, c := range msg.Content {
			switch blk := c.(type) {
			case ai.TextContent:
				parts = append(parts, wirePart{Type: "text", Text: blk.Text})
			case ai.ImageContent:
				url := fmt.Sprintf("data:%s;base64,%s", blk.MIMEType, blk.Data)
				parts = append(parts, wirePart{Type: "image_url", ImageURL: &struct{ URL string `json:"url"` }{URL: url}})
			}
		}
		if len(parts) == 1 && parts[0].Type == "text" {
			wm.Content = parts[0].Text
		} else {
			wm.Content = parts
		}
		return wm, nil

	case ai.AssistantMessage:
		wm := wireMessage{Role: "assistant"}
		var textParts []wirePart
		for _, c := range msg.Content {
			switch blk := c.(type) {
			case ai.TextContent:
				textParts = append(textParts, wirePart{Type: "text", Text: blk.Text})
			case ai.ToolCall:
				argsJSON, _ := json.Marshal(blk.Arguments)
				wm.ToolCalls = append(wm.ToolCalls, wireToolCall{
					ID:   blk.ID,
					Type: "function",
					Function: struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					}{Name: blk.Name, Arguments: string(argsJSON)},
				})
			}
		}
		if len(textParts) == 1 {
			wm.Content = textParts[0].Text
		} else if len(textParts) > 1 {
			wm.Content = textParts
		}
		return wm, nil

	case ai.ToolResultMessage:
		var content string
		for _, c := range msg.Content {
			if tc, ok := c.(ai.TextContent); ok {
				content += tc.Text
			}
		}
		return wireMessage{
			Role:       "tool",
			ToolCallID: msg.ToolCallID,
			Content:    content,
		}, nil
	}

	return wireMessage{}, fmt.Errorf("openai: unsupported message role: %T", m)
}

func findOrAppendText(msg *ai.AssistantMessage) int {
	for i, c := range msg.Content {
		if _, ok := c.(ai.TextContent); ok {
			return i
		}
	}
	msg.Content = append(msg.Content, ai.TextContent{Type: "text", Text: ""})
	return len(msg.Content) - 1
}

func snapshotMsg(msg *ai.AssistantMessage) *ai.AssistantMessage {
	cp := *msg
	cp.Content = make([]ai.ContentBlock, len(msg.Content))
	copy(cp.Content, msg.Content)
	return &cp
}

func mapStopReason(s string) ai.StopReason {
	switch s {
	case "stop":
		return ai.StopReasonStop
	case "length":
		return ai.StopReasonLength
	case "tool_calls":
		return ai.StopReasonTool
	default:
		return ai.StopReason(s)
	}
}


