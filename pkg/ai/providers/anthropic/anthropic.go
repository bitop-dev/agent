// Package anthropic implements ai.Provider for the Anthropic Messages API
// (streaming via SSE).
package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/bitop-dev/agent/pkg/ai"
	"github.com/bitop-dev/agent/pkg/ai/sse"
)

const defaultBaseURL = "https://api.anthropic.com/v1"
const anthropicVersion = "2023-06-01"

// Provider is the Anthropic streaming provider.
type Provider struct {
	BaseURL    string
	HTTPClient *http.Client
}

// New creates a Provider.
func New(baseURL string) *Provider {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Provider{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{Timeout: 10 * time.Minute},
	}
}

func (p *Provider) Name() string { return "anthropic" }

// ---------------------------------------------------------------------------
// Wire types
// ---------------------------------------------------------------------------

type wireContent struct {
	Type  string `json:"type"`
	Text  string `json:"text,omitempty"`
	// Tool use (assistant)
	ID    string         `json:"id,omitempty"`
	Name  string         `json:"name,omitempty"`
	Input map[string]any `json:"input,omitempty"`
	// Tool result (user)
	ToolUseID string        `json:"tool_use_id,omitempty"`
	Content   []wireContent `json:"content,omitempty"`
	IsError   bool          `json:"is_error,omitempty"`
	// Image
	Source *wireImageSource `json:"source,omitempty"`
	// Prompt caching
	CacheControl *wireCacheCtrl `json:"cache_control,omitempty"`
}

type wireImageSource struct {
	Type      string `json:"type"`       // "base64"
	MediaType string `json:"media_type"` // "image/png"
	Data      string `json:"data"`
}

type wireMessage struct {
	Role    string        `json:"role"`
	Content []wireContent `json:"content"`
}

type wireTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type wireThinking struct {
	Type         string `json:"type"`                    // "enabled" or "adaptive"
	BudgetTokens int    `json:"budget_tokens,omitempty"` // for budget-based thinking
	Effort       string `json:"effort,omitempty"`        // for adaptive thinking
}

type wireSystemBlock struct {
	Type        string          `json:"type"`                   // "text"
	Text        string          `json:"text"`
	CacheControl *wireCacheCtrl `json:"cache_control,omitempty"`
}

type wireCacheCtrl struct {
	Type string `json:"type"` // "ephemeral"
}

type wireRequest struct {
	Model       string        `json:"model"`
	MaxTokens   int           `json:"max_tokens"`
	System      any           `json:"system,omitempty"` // string or []wireSystemBlock
	Messages    []wireMessage `json:"messages"`
	Tools       []wireTool    `json:"tools,omitempty"`
	Stream      bool          `json:"stream"`
	Temperature *float64      `json:"temperature,omitempty"`
	Thinking    *wireThinking `json:"thinking,omitempty"`
}

// SSE event payloads
type evContentBlockStart struct {
	Index        int         `json:"index"`
	ContentBlock wireContent `json:"content_block"`
}

type evContentBlockDelta struct {
	Index int `json:"index"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		PartialJSON string `json:"partial_json"`
	} `json:"delta"`
}

type evMessageDelta struct {
	Delta struct {
		StopReason string `json:"stop_reason"`
	} `json:"delta"`
	Usage struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type evMessageStart struct {
	Message struct {
		Usage struct {
			InputTokens               int `json:"input_tokens"`
			OutputTokens              int `json:"output_tokens"`
			CacheReadInputTokens      int `json:"cache_read_input_tokens"`
			CacheCreationInputTokens  int `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// ---------------------------------------------------------------------------
// Thinking helpers
// ---------------------------------------------------------------------------

// adaptiveModels support Anthropic's adaptive thinking (effort-based rather than budget-based).
func supportsAdaptiveThinking(modelID string) bool {
	return contains(modelID, "opus-4-6") || contains(modelID, "opus-4.6") ||
		contains(modelID, "sonnet-4-6") || contains(modelID, "sonnet-4.6")
}

func contains(s, substr string) bool { return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr)) }
func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// buildThinking constructs the thinking wire object (or nil if thinking is off).
// Also adjusts maxTokens when budget-based thinking is used.
func buildThinking(modelID string, opts ai.StreamOptions, maxTokens int) *wireThinking {
	level := opts.ThinkingLevel
	if level == "" || level == ai.ThinkingOff {
		return nil
	}

	if supportsAdaptiveThinking(modelID) {
		effort := mapEffort(level, modelID)
		return &wireThinking{Type: "adaptive", Effort: effort}
	}

	budget := opts.ThinkingBudgets.ThinkingBudgetFor(level)
	return &wireThinking{Type: "enabled", BudgetTokens: budget}
}

func mapEffort(level ai.ThinkingLevel, modelID string) string {
	switch level {
	case ai.ThinkingMinimal, ai.ThinkingLow:
		return "low"
	case ai.ThinkingMedium:
		return "medium"
	case ai.ThinkingHigh:
		return "high"
	case ai.ThinkingXHigh:
		if containsStr(modelID, "opus-4-6") || containsStr(modelID, "opus-4.6") {
			return "max"
		}
		return "high"
	default:
		return "high"
	}
}

// ---------------------------------------------------------------------------
// Stream
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

	return events, func() (*ai.AssistantMessage, error) {
		<-done
		return finalMsg, finalErr
	}
}

func (p *Provider) stream(
	ctx context.Context,
	model string,
	llmCtx ai.Context,
	opts ai.StreamOptions,
	events chan<- ai.StreamEvent,
) (*ai.AssistantMessage, error) {
	maxTokens := opts.MaxTokens
	if maxTokens == 0 {
		maxTokens = 8192
	}

	req := wireRequest{
		Model:       model,
		MaxTokens:   maxTokens,
		Stream:      true,
		Temperature: opts.Temperature,
	}

	// System prompt — wrap in cache block when caching is enabled.
	caching := opts.CacheRetention != ai.CacheRetentionNone
	if llmCtx.SystemPrompt != "" {
		if caching {
			req.System = []wireSystemBlock{{
				Type:         "text",
				Text:         llmCtx.SystemPrompt,
				CacheControl: &wireCacheCtrl{Type: "ephemeral"},
			}}
		} else {
			req.System = llmCtx.SystemPrompt
		}
	}

	// Thinking / extended reasoning
	thinking := buildThinking(model, opts, maxTokens)
	if thinking != nil {
		req.Thinking = thinking
		// Thinking requires temperature ≤ 1 and disallows temperature=0;
		// if the caller set an explicit temperature, leave it; otherwise clear it.
		// (Anthropic rejects temperature outside [1,1] for budget thinking.)
	}

	for _, m := range llmCtx.Messages {
		wm, err := convertMessage(m)
		if err != nil {
			return nil, err
		}
		req.Messages = append(req.Messages, wm)
	}

	// Add cache breakpoint on the last user message (promotes stable prefix caching).
	if caching && len(req.Messages) > 0 {
		last := &req.Messages[len(req.Messages)-1]
		if last.Role == "user" && len(last.Content) > 0 {
			// Tag the last content block with cache_control.
			last.Content[len(last.Content)-1].CacheControl = &wireCacheCtrl{Type: "ephemeral"}
		}
	}

	for _, t := range llmCtx.Tools {
		req.Tools = append(req.Tools, wireTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Parameters,
		})
	}

	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", opts.APIKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)
	httpReq.Header.Set("Accept", "text/event-stream")
	if thinking != nil {
		httpReq.Header.Set("anthropic-beta", "interleaved-thinking-2025-05-14")
	}

	resp, err := p.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("anthropic: HTTP %d: %s", resp.StatusCode, string(b))
	}

	partial := &ai.AssistantMessage{
		Role:      ai.RoleAssistant,
		Model:     model,
		Provider:  "anthropic",
		Timestamp: time.Now().UnixMilli(),
	}

	// Track content blocks by index
	type blockState struct {
		kind    string // "text" | "tool_use"
		id      string
		name    string
		args    string
		textIdx int // index in partial.Content
	}
	blocks := map[int]*blockState{}

	emittedStart := false
	reader := sse.NewReader(resp.Body)

	send := func(t ai.StreamEventType, delta string) {
		events <- ai.StreamEvent{Type: t, Partial: snapshotMsg(partial), Delta: delta}
	}

	for {
		ev, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("anthropic: sse read: %w", err)
		}
		if ev.Data == "" {
			continue
		}

		switch ev.Type {
		case "message_start":
			var ms evMessageStart
			if json.Unmarshal([]byte(ev.Data), &ms) == nil {
				partial.Usage.Input = ms.Message.Usage.InputTokens
				partial.Usage.CacheRead = ms.Message.Usage.CacheReadInputTokens
				partial.Usage.CacheWrite = ms.Message.Usage.CacheCreationInputTokens
			}
			events <- ai.StreamEvent{Type: ai.StreamEventStart, Partial: snapshotMsg(partial)}
			emittedStart = true

		case "content_block_start":
			var cbs evContentBlockStart
			if json.Unmarshal([]byte(ev.Data), &cbs) != nil {
				continue
			}
			bs := &blockState{kind: cbs.ContentBlock.Type}
			blocks[cbs.Index] = bs
			switch cbs.ContentBlock.Type {
			case "text":
				partial.Content = append(partial.Content, ai.TextContent{Type: "text", Text: ""})
				bs.textIdx = len(partial.Content) - 1
				send(ai.StreamEventTextStart, "")
			case "tool_use":
				bs.id = cbs.ContentBlock.ID
				if bs.id == "" {
					bs.id = "call_" + uuid.New().String()[:8]
				}
				bs.name = cbs.ContentBlock.Name
				send(ai.StreamEventToolCallStart, bs.name)
			}

		case "content_block_delta":
			var cbd evContentBlockDelta
			if json.Unmarshal([]byte(ev.Data), &cbd) != nil {
				continue
			}
			bs := blocks[cbd.Index]
			if bs == nil {
				continue
			}
			switch cbd.Delta.Type {
			case "text_delta":
				tb := partial.Content[bs.textIdx].(ai.TextContent)
				tb.Text += cbd.Delta.Text
				partial.Content[bs.textIdx] = tb
				send(ai.StreamEventTextDelta, cbd.Delta.Text)
			case "input_json_delta":
				bs.args += cbd.Delta.PartialJSON
				send(ai.StreamEventToolCallDelta, cbd.Delta.PartialJSON)
			}

		case "content_block_stop":
			// finalise block
			// (text block is already done; finalize tool_use)
			var idx struct{ Index int `json:"index"` }
			if json.Unmarshal([]byte(ev.Data), &idx) != nil {
				break
			}
			bs := blocks[idx.Index]
			if bs == nil {
				break
			}
			switch bs.kind {
			case "text":
				send(ai.StreamEventTextEnd, "")
			case "tool_use":
				var args map[string]any
				_ = json.Unmarshal([]byte(bs.args), &args)
				partial.Content = append(partial.Content, ai.ToolCall{
					Type:      "tool_call",
					ID:        bs.id,
					Name:      bs.name,
					Arguments: args,
				})
				send(ai.StreamEventToolCallEnd, "")
			}

		case "message_delta":
			var md evMessageDelta
			if json.Unmarshal([]byte(ev.Data), &md) == nil {
				partial.StopReason = mapStopReason(md.Delta.StopReason)
				partial.Usage.Output = md.Usage.OutputTokens
				partial.Usage.TotalTokens = partial.Usage.Input + partial.Usage.Output +
					partial.Usage.CacheRead + partial.Usage.CacheWrite
			}

		case "message_stop":
			if !emittedStart {
				events <- ai.StreamEvent{Type: ai.StreamEventStart, Partial: snapshotMsg(partial)}
			}
			events <- ai.StreamEvent{Type: ai.StreamEventDone, Partial: snapshotMsg(partial)}
		}
	}

	if partial.StopReason == "" {
		partial.StopReason = ai.StopReasonStop
	}

	return partial, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func convertMessage(m ai.Message) (wireMessage, error) {
	switch msg := m.(type) {
	case ai.UserMessage:
		var content []wireContent
		for _, c := range msg.Content {
			switch blk := c.(type) {
			case ai.TextContent:
				content = append(content, wireContent{Type: "text", Text: blk.Text})
			case ai.ImageContent:
				content = append(content, wireContent{
					Type:   "image",
					Source: &wireImageSource{Type: "base64", MediaType: blk.MIMEType, Data: blk.Data},
				})
			}
		}
		return wireMessage{Role: "user", Content: content}, nil

	case ai.AssistantMessage:
		var content []wireContent
		for _, c := range msg.Content {
			switch blk := c.(type) {
			case ai.TextContent:
				content = append(content, wireContent{Type: "text", Text: blk.Text})
			case ai.ToolCall:
				content = append(content, wireContent{
					Type:  "tool_use",
					ID:    blk.ID,
					Name:  blk.Name,
					Input: blk.Arguments,
				})
			}
		}
		return wireMessage{Role: "assistant", Content: content}, nil

	case ai.ToolResultMessage:
		var inner []wireContent
		for _, c := range msg.Content {
			if tc, ok := c.(ai.TextContent); ok {
				inner = append(inner, wireContent{Type: "text", Text: tc.Text})
			}
		}
		return wireMessage{
			Role: "user",
			Content: []wireContent{{
				Type:      "tool_result",
				ToolUseID: msg.ToolCallID,
				Content:   inner,
				IsError:   msg.IsError,
			}},
		}, nil
	}

	return wireMessage{}, fmt.Errorf("anthropic: unsupported message type: %T", m)
}

func snapshotMsg(msg *ai.AssistantMessage) *ai.AssistantMessage {
	cp := *msg
	cp.Content = make([]ai.ContentBlock, len(msg.Content))
	copy(cp.Content, msg.Content)
	return &cp
}

func mapStopReason(s string) ai.StopReason {
	switch s {
	case "end_turn":
		return ai.StopReasonStop
	case "max_tokens":
		return ai.StopReasonLength
	case "tool_use":
		return ai.StopReasonTool
	default:
		return ai.StopReason(s)
	}
}
