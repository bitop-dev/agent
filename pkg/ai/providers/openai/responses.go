package openai

// ResponsesProvider implements the OpenAI Responses API
// (POST /responses), which is the current API used by OpenAI and is now
// supported by many proxy providers (OpenRouter, etc.).
//
// Key differences from the Chat Completions API:
//   - Endpoint:  POST {baseURL}/responses  (not /chat/completions)
//   - Messages:  "input" field instead of "messages"; items use "input_text"/"output_text"
//   - Tool calls: "function_call" items with separate call_id and item id
//   - Tool results: "function_call_output" items (not a "tool" role message)
//   - Thinking:  "reasoning" items with "summary" text blocks
//   - Limits:    "max_output_tokens" instead of "max_tokens"
//   - Auth:      same Authorization: Bearer header; base URL is configurable

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nickcecere/agent/pkg/ai"
	"github.com/nickcecere/agent/pkg/ai/sse"
)

// ResponsesProvider streams via the OpenAI Responses API.
// Set BaseURL to override the default (e.g. for proxies that support the
// Responses API format).
type ResponsesProvider struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewResponses creates a ResponsesProvider.
// Pass "" for baseURL to use the default OpenAI endpoint.
func NewResponses(baseURL string) *ResponsesProvider {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &ResponsesProvider{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{Timeout: 10 * time.Minute},
	}
}

func (p *ResponsesProvider) Name() string { return "openai" }

// ---------------------------------------------------------------------------
// Wire types — Responses API
// ---------------------------------------------------------------------------

// Input item — union type; only the relevant fields are populated per type.
type respInputItem struct {
	// Common
	Type string `json:"type"`
	Role string `json:"role,omitempty"`

	// role=system/user/assistant — content array
	Content json.RawMessage `json:"content,omitempty"`

	// type=function_call (assistant tool call)
	ID        string `json:"id,omitempty"`       // item id
	CallID    string `json:"call_id,omitempty"`  // call id (used in output)
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`

	// type=function_call_output (tool result)
	Output string `json:"output,omitempty"`
}

type respInputText struct {
	Type string `json:"type"` // "input_text"
	Text string `json:"text"`
}

type respInputImage struct {
	Type     string `json:"type"`     // "input_image"
	Detail   string `json:"detail"`   // "auto"
	ImageURL string `json:"image_url"`
}

type respTool struct {
	Type        string          `json:"type"`        // "function"
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
	Strict      bool            `json:"strict"`
}

type respReasoning struct {
	Effort  string `json:"effort,omitempty"`  // "low" | "medium" | "high"
	Summary string `json:"summary,omitempty"` // "auto"
}

type respRequest struct {
	Model           string          `json:"model"`
	Input           []respInputItem `json:"input"`
	Tools           []respTool      `json:"tools,omitempty"`
	MaxOutputTokens int             `json:"max_output_tokens,omitempty"`
	Temperature     *float64        `json:"temperature,omitempty"`
	Stream          bool            `json:"stream"`
	Reasoning       *respReasoning  `json:"reasoning,omitempty"`
	Include         []string        `json:"include,omitempty"`
}

// SSE event types
type respEvent struct {
	Type string `json:"type"`

	// response.output_item.added / done
	Item *respItem `json:"item,omitempty"`

	// delta events
	Delta string `json:"delta,omitempty"`

	// response.completed
	Response *respCompleted `json:"response,omitempty"`

	// error
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

type respItem struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"` // "message" | "function_call" | "reasoning"
	CallID    string          `json:"call_id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Arguments string          `json:"arguments,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	Summary   []respSummaryPart `json:"summary,omitempty"`
}

type respSummaryPart struct {
	Type string `json:"type"` // "summary_text"
	Text string `json:"text"`
}

type respCompleted struct {
	Status string `json:"status"`
	Usage  struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
		TotalTokens  int `json:"total_tokens"`
		InputTokensDetails struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"input_tokens_details"`
	} `json:"usage"`
}

// ---------------------------------------------------------------------------
// Stream
// ---------------------------------------------------------------------------

func (p *ResponsesProvider) Stream(
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

	return events, func() (*ai.AssistantMessage, error) {
		<-done
		return finalMsg, finalErr
	}
}

func (p *ResponsesProvider) stream(
	ctx context.Context,
	model string,
	llmCtx ai.Context,
	opts ai.StreamOptions,
	events chan<- ai.StreamEvent,
) (*ai.AssistantMessage, error) {
	req, err := p.buildRequest(model, llmCtx, opts)
	if err != nil {
		return nil, err
	}

	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/responses", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+opts.APIKey)

	resp, err := p.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai-responses: HTTP %d: %s", resp.StatusCode, string(b))
	}

	partial := &ai.AssistantMessage{
		Role:      ai.RoleAssistant,
		Model:     model,
		Provider:  "openai",
		Timestamp: time.Now().UnixMilli(),
	}

	// Track current item across events
	type itemState struct {
		typ        string // "message" | "function_call" | "reasoning"
		id         string // item id
		callID     string // for function_call
		name       string // for function_call
		partialArgs string
		contentIdx int // index in partial.Content for text/thinking blocks
	}
	var currentItem *itemState
	emittedStart := false
	reader := sse.NewReader(resp.Body)

	for {
		ev, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("openai-responses: sse read: %w", err)
		}
		if ev.Data == "" || ev.Data == "[DONE]" {
			continue
		}

		var e respEvent
		if err := json.Unmarshal([]byte(ev.Data), &e); err != nil {
			continue
		}

		if !emittedStart && e.Type != "response.created" {
			events <- ai.StreamEvent{Type: ai.StreamEventStart, Partial: snapshotMsg(partial)}
			emittedStart = true
		}

		switch e.Type {
		case "response.output_item.added":
			if e.Item == nil {
				continue
			}
			item := e.Item
			currentItem = &itemState{typ: item.Type, id: item.ID, callID: item.CallID, name: item.Name}
			switch item.Type {
			case "message":
				partial.Content = append(partial.Content, ai.TextContent{Type: "text", Text: ""})
				currentItem.contentIdx = len(partial.Content) - 1
				events <- ai.StreamEvent{Type: ai.StreamEventTextStart, Partial: snapshotMsg(partial)}
			case "reasoning":
				partial.Content = append(partial.Content, ai.ThinkingContent{Type: "thinking", Thinking: ""})
				currentItem.contentIdx = len(partial.Content) - 1
				events <- ai.StreamEvent{Type: ai.StreamEventThinkingStart, Partial: snapshotMsg(partial)}
			case "function_call":
				// Will be finalised on output_item.done
			}

		case "response.output_text.delta":
			if currentItem != nil && currentItem.typ == "message" {
				tb := partial.Content[currentItem.contentIdx].(ai.TextContent)
				tb.Text += e.Delta
				partial.Content[currentItem.contentIdx] = tb
				events <- ai.StreamEvent{Type: ai.StreamEventTextDelta, Partial: snapshotMsg(partial), Delta: e.Delta}
			}

		case "response.reasoning_summary_text.delta":
			if currentItem != nil && currentItem.typ == "reasoning" {
				tb := partial.Content[currentItem.contentIdx].(ai.ThinkingContent)
				tb.Thinking += e.Delta
				partial.Content[currentItem.contentIdx] = tb
				events <- ai.StreamEvent{Type: ai.StreamEventThinkingDelta, Partial: snapshotMsg(partial), Delta: e.Delta}
			}

		case "response.reasoning_summary_part.done":
			// Add separator between summary parts
			if currentItem != nil && currentItem.typ == "reasoning" {
				tb := partial.Content[currentItem.contentIdx].(ai.ThinkingContent)
				tb.Thinking += "\n\n"
				partial.Content[currentItem.contentIdx] = tb
			}

		case "response.function_call_arguments.delta":
			if currentItem != nil && currentItem.typ == "function_call" {
				currentItem.partialArgs += e.Delta
				events <- ai.StreamEvent{Type: ai.StreamEventToolCallDelta, Partial: snapshotMsg(partial), Delta: e.Delta}
			}

		case "response.function_call_arguments.done":
			if currentItem != nil && currentItem.typ == "function_call" {
				currentItem.partialArgs = e.Delta
			}

		case "response.output_item.done":
			if e.Item == nil || currentItem == nil {
				continue
			}
			item := e.Item
			switch item.Type {
			case "message":
				events <- ai.StreamEvent{Type: ai.StreamEventTextEnd, Partial: snapshotMsg(partial)}
			case "reasoning":
				events <- ai.StreamEvent{Type: ai.StreamEventThinkingEnd, Partial: snapshotMsg(partial)}
			case "function_call":
				// Parse accumulated args
				var args map[string]any
				argsStr := currentItem.partialArgs
				if argsStr == "" {
					argsStr = item.Arguments
				}
				_ = json.Unmarshal([]byte(argsStr), &args)

				// Compound ID: call_id|item_id  (mirrors pi convention)
				compoundID := item.CallID + "|" + item.ID
				if item.CallID == "" {
					compoundID = item.ID
				}

				tc := ai.ToolCall{
					Type:      "tool_call",
					ID:        compoundID,
					Name:      item.Name,
					Arguments: args,
				}
				partial.Content = append(partial.Content, tc)
				events <- ai.StreamEvent{Type: ai.StreamEventToolCallStart, Partial: snapshotMsg(partial), Delta: tc.Name}
				events <- ai.StreamEvent{Type: ai.StreamEventToolCallEnd, Partial: snapshotMsg(partial)}
			}
			currentItem = nil

		case "response.completed":
			if e.Response != nil {
				r := e.Response
				cached := r.Usage.InputTokensDetails.CachedTokens
				partial.Usage.Input = r.Usage.InputTokens - cached
				partial.Usage.Output = r.Usage.OutputTokens
				partial.Usage.CacheRead = cached
				partial.Usage.TotalTokens = r.Usage.TotalTokens
				partial.StopReason = mapResponseStatus(r.Status)
			}

		case "error":
			return nil, fmt.Errorf("openai-responses: API error %d: %s", e.Code, e.Message)
		}
	}

	if partial.StopReason == "" {
		partial.StopReason = ai.StopReasonStop
	}
	for _, c := range partial.Content {
		if _, ok := c.(ai.ToolCall); ok {
			partial.StopReason = ai.StopReasonTool
			break
		}
	}

	events <- ai.StreamEvent{Type: ai.StreamEventDone, Partial: snapshotMsg(partial)}
	return partial, nil
}

// ---------------------------------------------------------------------------
// Request building
// ---------------------------------------------------------------------------

func (p *ResponsesProvider) buildRequest(model string, llmCtx ai.Context, opts ai.StreamOptions) (respRequest, error) {
	req := respRequest{
		Model:           model,
		Stream:          true,
		MaxOutputTokens: opts.MaxTokens,
		Temperature:     opts.Temperature,
	}

	// Reasoning / thinking
	if level := opts.ThinkingLevel; level != "" && level != ai.ThinkingOff {
		effort := mapResponsesToEffort(level)
		req.Reasoning = &respReasoning{Effort: effort, Summary: "auto"}
		req.Include = []string{"reasoning.encrypted_content"}
	}

	// System message
	if llmCtx.SystemPrompt != "" {
		content, _ := json.Marshal(llmCtx.SystemPrompt)
		req.Input = append(req.Input, respInputItem{Type: "message", Role: "system", Content: content})
	}

	for _, m := range llmCtx.Messages {
		items, err := convertResponsesMessage(m)
		if err != nil {
			return respRequest{}, err
		}
		req.Input = append(req.Input, items...)
	}

	for _, t := range llmCtx.Tools {
		req.Tools = append(req.Tools, respTool{
			Type:        "function",
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
			Strict:      false,
		})
	}

	return req, nil
}

func mapResponsesToEffort(level ai.ThinkingLevel) string {
	switch level {
	case ai.ThinkingMinimal, ai.ThinkingLow:
		return "low"
	case ai.ThinkingMedium:
		return "medium"
	default: // high, xhigh
		return "high"
	}
}

func convertResponsesMessage(m ai.Message) ([]respInputItem, error) {
	switch msg := m.(type) {
	case ai.UserMessage:
		type part struct {
			Type     string `json:"type"`
			Text     string `json:"text,omitempty"`
			Detail   string `json:"detail,omitempty"`
			ImageURL string `json:"image_url,omitempty"`
		}
		var parts []part
		for _, c := range msg.Content {
			switch blk := c.(type) {
			case ai.TextContent:
				parts = append(parts, part{Type: "input_text", Text: blk.Text})
			case ai.ImageContent:
				parts = append(parts, part{
					Type:     "input_image",
					Detail:   "auto",
					ImageURL: fmt.Sprintf("data:%s;base64,%s", blk.MIMEType, blk.Data),
				})
			}
		}
		content, _ := json.Marshal(parts)
		return []respInputItem{{Type: "message", Role: "user", Content: content}}, nil

	case ai.AssistantMessage:
		var items []respInputItem
		var textParts []map[string]string
		for _, c := range msg.Content {
			switch blk := c.(type) {
			case ai.TextContent:
				if strings.TrimSpace(blk.Text) != "" {
					textParts = append(textParts, map[string]string{"type": "output_text", "text": blk.Text})
				}
			case ai.ToolCall:
				// Flush any accumulated text first
				if len(textParts) > 0 {
					content, _ := json.Marshal(textParts)
					items = append(items, respInputItem{Type: "message", Role: "assistant", Content: content})
					textParts = nil
				}
				// Split compound ID: call_id|item_id
				callID, itemID := splitCompoundID(blk.ID)
				argsJSON, _ := json.Marshal(blk.Arguments)
				items = append(items, respInputItem{
					Type:      "function_call",
					ID:        itemID,
					CallID:    callID,
					Name:      blk.Name,
					Arguments: string(argsJSON),
				})
			}
		}
		if len(textParts) > 0 {
			content, _ := json.Marshal(textParts)
			items = append(items, respInputItem{Type: "message", Role: "assistant", Content: content})
		}
		return items, nil

	case ai.ToolResultMessage:
		var sb strings.Builder
		for _, c := range msg.Content {
			if tc, ok := c.(ai.TextContent); ok {
				sb.WriteString(tc.Text)
			}
		}
		// Extract call_id from compound ID
		callID, _ := splitCompoundID(msg.ToolCallID)
		return []respInputItem{{
			Type:   "function_call_output",
			CallID: callID,
			Output: sb.String(),
		}}, nil
	}

	return nil, fmt.Errorf("openai-responses: unsupported message type: %T", m)
}

// splitCompoundID splits "call_id|item_id" → (call_id, item_id).
// If there is no "|", the whole string is returned as call_id.
func splitCompoundID(id string) (callID, itemID string) {
	if idx := strings.Index(id, "|"); idx != -1 {
		return id[:idx], id[idx+1:]
	}
	return id, id
}

func mapResponseStatus(status string) ai.StopReason {
	switch status {
	case "completed":
		return ai.StopReasonStop
	case "incomplete":
		return ai.StopReasonLength
	default:
		return ai.StopReasonStop
	}
}
