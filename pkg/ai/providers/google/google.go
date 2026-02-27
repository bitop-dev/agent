// Package google implements ai.Provider for the Google Gemini API
// (generateContent / streamGenerateContent via REST/SSE).
// No Google SDK dependency — pure HTTP + SSE.
package google

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/bitop-dev/agent/pkg/ai"
	"github.com/bitop-dev/agent/pkg/ai/sse"
)

const defaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"

// Provider is the Google Gemini streaming provider.
type Provider struct {
	BaseURL    string
	HTTPClient *http.Client
}

func New(baseURL string) *Provider {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Provider{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{Timeout: 10 * time.Minute},
	}
}

func (p *Provider) Name() string { return "google" }

// ---------------------------------------------------------------------------
// Wire types — Gemini REST API
// ---------------------------------------------------------------------------

type wirePart struct {
	Text             string         `json:"text,omitempty"`
	Thought          bool           `json:"thought,omitempty"`
	InlineData       *wireInline    `json:"inlineData,omitempty"`
	FunctionCall     *wireFuncCall  `json:"functionCall,omitempty"`
	FunctionResponse *wireFuncResp  `json:"functionResponse,omitempty"`
}

type wireInline struct {
	MIMEType string `json:"mimeType"`
	Data     string `json:"data"`
}

type wireFuncCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

type wireFuncResp struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type wireContent struct {
	Role  string     `json:"role"`
	Parts []wirePart `json:"parts"`
}

type wireFuncDecl struct {
	Name                 string          `json:"name"`
	Description          string          `json:"description"`
	ParametersJsonSchema json.RawMessage `json:"parametersJsonSchema,omitempty"`
}

type wireTool struct {
	FunctionDeclarations []wireFuncDecl `json:"functionDeclarations"`
}

type wireGenConfig struct {
	Temperature     *float64         `json:"temperature,omitempty"`
	MaxOutputTokens int              `json:"maxOutputTokens,omitempty"`
	ThinkingConfig  *wireThinkConfig `json:"thinkingConfig,omitempty"`
}

type wireThinkConfig struct {
	IncludeThoughts bool `json:"includeThoughts"`
	ThinkingBudget  int  `json:"thinkingBudget,omitempty"` // -1 = dynamic, 0 = off
}

type wireSystemInstruction struct {
	Parts []wirePart `json:"parts"`
}

type wireRequest struct {
	SystemInstruction *wireSystemInstruction `json:"systemInstruction,omitempty"`
	Contents          []wireContent          `json:"contents"`
	Tools             []wireTool             `json:"tools,omitempty"`
	GenerationConfig  wireGenConfig          `json:"generationConfig,omitempty"`
}

// SSE response chunk
type wireChunk struct {
	Candidates []struct {
		Content      wireContent `json:"content"`
		FinishReason string      `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount       int `json:"promptTokenCount"`
		CandidatesTokenCount   int `json:"candidatesTokenCount"`
		ThoughtsTokenCount     int `json:"thoughtsTokenCount"`
		TotalTokenCount        int `json:"totalTokenCount"`
		CachedContentTokenCount int `json:"cachedContentTokenCount"`
	} `json:"usageMetadata"`
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
	req, err := p.buildRequest(llmCtx, opts)
	if err != nil {
		return nil, err
	}

	body, _ := json.Marshal(req)

	// Gemini SSE endpoint: /models/{model}:streamGenerateContent?alt=sse&key={apiKey}
	url := fmt.Sprintf("%s/models/%s:streamGenerateContent?alt=sse&key=%s", p.BaseURL, model, opts.APIKey)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("google: HTTP %d: %s", resp.StatusCode, string(b))
	}

	partial := &ai.AssistantMessage{
		Role:      ai.RoleAssistant,
		Model:     model,
		Provider:  "google",
		Timestamp: time.Now().UnixMilli(),
	}

	var currentBlock *googleBlockState

	emittedStart := false
	toolCallCounter := 0
	reader := sse.NewReader(resp.Body)

	for {
		ev, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("google: sse read: %w", err)
		}
		if ev.Data == "" || ev.Data == "[DONE]" {
			continue
		}

		var chunk wireChunk
		if err := json.Unmarshal([]byte(ev.Data), &chunk); err != nil {
			continue
		}

		if !emittedStart {
			events <- ai.StreamEvent{Type: ai.StreamEventStart, Partial: snapshotMsg(partial)}
			emittedStart = true
		}

		// Usage
		if chunk.UsageMetadata.TotalTokenCount > 0 {
			partial.Usage.Input = chunk.UsageMetadata.PromptTokenCount
			partial.Usage.Output = chunk.UsageMetadata.CandidatesTokenCount + chunk.UsageMetadata.ThoughtsTokenCount
			partial.Usage.CacheRead = chunk.UsageMetadata.CachedContentTokenCount
			partial.Usage.TotalTokens = chunk.UsageMetadata.TotalTokenCount
		}

		if len(chunk.Candidates) == 0 {
			continue
		}

		cand := chunk.Candidates[0]
		if cand.FinishReason != "" {
			partial.StopReason = mapStopReason(cand.FinishReason)
		}

		for _, part := range cand.Content.Parts {
			if part.FunctionCall != nil {
				// Close any open text/thinking block
				if currentBlock != nil {
					closeBlock(currentBlock, partial, events)
					currentBlock = nil
				}
				toolCallCounter++
				id := part.FunctionCall.Name + fmt.Sprintf("_%d", toolCallCounter) + "_" + uuid.New().String()[:4]
				tc := ai.ToolCall{
					Type:      "tool_call",
					ID:        id,
					Name:      part.FunctionCall.Name,
					Arguments: part.FunctionCall.Args,
				}
				partial.Content = append(partial.Content, tc)
				events <- ai.StreamEvent{Type: ai.StreamEventToolCallStart, Partial: snapshotMsg(partial), Delta: tc.Name}
				events <- ai.StreamEvent{Type: ai.StreamEventToolCallDelta, Partial: snapshotMsg(partial), Delta: jsonStr(tc.Arguments)}
				events <- ai.StreamEvent{Type: ai.StreamEventToolCallEnd, Partial: snapshotMsg(partial)}
				continue
			}

			if part.Text != "" {
				isThinking := part.Thought

				// Switch block type if needed
				if currentBlock == nil || (isThinking && currentBlock.kind != "thinking") || (!isThinking && currentBlock.kind != "text") {
					if currentBlock != nil {
						closeBlock(currentBlock, partial, events)
					}
					if isThinking {
						partial.Content = append(partial.Content, ai.ThinkingContent{Type: "thinking", Thinking: ""})
						idx := len(partial.Content) - 1
						currentBlock = &googleBlockState{"thinking", idx}
						events <- ai.StreamEvent{Type: ai.StreamEventThinkingStart, Partial: snapshotMsg(partial)}
					} else {
						partial.Content = append(partial.Content, ai.TextContent{Type: "text", Text: ""})
						idx := len(partial.Content) - 1
						currentBlock = &googleBlockState{"text", idx}
						events <- ai.StreamEvent{Type: ai.StreamEventTextStart, Partial: snapshotMsg(partial)}
					}
				}

				if isThinking {
					tb := partial.Content[currentBlock.idx].(ai.ThinkingContent)
					tb.Thinking += part.Text
					partial.Content[currentBlock.idx] = tb
					events <- ai.StreamEvent{Type: ai.StreamEventThinkingDelta, Partial: snapshotMsg(partial), Delta: part.Text}
				} else {
					tb := partial.Content[currentBlock.idx].(ai.TextContent)
					tb.Text += part.Text
					partial.Content[currentBlock.idx] = tb
					events <- ai.StreamEvent{Type: ai.StreamEventTextDelta, Partial: snapshotMsg(partial), Delta: part.Text}
				}
			}
		}
	}

	if currentBlock != nil {
		closeBlock(currentBlock, partial, events)
	}

	if partial.StopReason == "" {
		partial.StopReason = ai.StopReasonStop
	}
	if len(partial.Content) > 0 {
		for _, c := range partial.Content {
			if _, ok := c.(ai.ToolCall); ok {
				partial.StopReason = ai.StopReasonTool
				break
			}
		}
	}

	events <- ai.StreamEvent{Type: ai.StreamEventDone, Partial: snapshotMsg(partial)}
	return partial, nil
}

type googleBlockState struct {
	kind string // "text" | "thinking"
	idx  int
}

func closeBlock(bs *googleBlockState, partial *ai.AssistantMessage, events chan<- ai.StreamEvent) {
	if bs.kind == "thinking" {
		events <- ai.StreamEvent{Type: ai.StreamEventThinkingEnd, Partial: snapshotMsg(partial)}
	} else {
		events <- ai.StreamEvent{Type: ai.StreamEventTextEnd, Partial: snapshotMsg(partial)}
	}
}

// ---------------------------------------------------------------------------
// Request building
// ---------------------------------------------------------------------------

func (p *Provider) buildRequest(llmCtx ai.Context, opts ai.StreamOptions) (wireRequest, error) {
	req := wireRequest{}

	if llmCtx.SystemPrompt != "" {
		req.SystemInstruction = &wireSystemInstruction{
			Parts: []wirePart{{Text: llmCtx.SystemPrompt}},
		}
	}

	if opts.Temperature != nil {
		req.GenerationConfig.Temperature = opts.Temperature
	}
	if opts.MaxTokens > 0 {
		req.GenerationConfig.MaxOutputTokens = opts.MaxTokens
	}

	// Extended thinking / reasoning
	if level := opts.ThinkingLevel; level != "" && level != ai.ThinkingOff {
		budget := googleBudget(level, opts.ThinkingBudgets)
		req.GenerationConfig.ThinkingConfig = &wireThinkConfig{
			IncludeThoughts: true,
			ThinkingBudget:  budget,
		}
	}

	for _, m := range llmCtx.Messages {
		wc, err := convertMessage(m)
		if err != nil {
			return wireRequest{}, err
		}
		if wc != nil {
			req.Contents = append(req.Contents, *wc)
		}
	}

	for _, t := range llmCtx.Tools {
		req.Tools = append(req.Tools, wireTool{
			FunctionDeclarations: []wireFuncDecl{{
				Name:                 t.Name,
				Description:          t.Description,
				ParametersJsonSchema: t.Parameters,
			}},
		})
	}

	return req, nil
}

// googleBudget maps a ThinkingLevel to a Gemini thinkingBudget token count.
// -1 means "dynamic" (model decides). These defaults mirror pi-mono's values.
func googleBudget(level ai.ThinkingLevel, custom ai.ThinkingBudgets) int {
	if b := custom.ThinkingBudgetFor(level); b > 0 {
		return b
	}
	switch level {
	case ai.ThinkingMinimal:
		return 128
	case ai.ThinkingLow:
		return 2048
	case ai.ThinkingMedium:
		return 8192
	default: // high, xhigh
		return 24576
	}
}

func convertMessage(m ai.Message) (*wireContent, error) {
	switch msg := m.(type) {
	case ai.UserMessage:
		var parts []wirePart
		for _, c := range msg.Content {
			switch blk := c.(type) {
			case ai.TextContent:
				parts = append(parts, wirePart{Text: blk.Text})
			case ai.ImageContent:
				parts = append(parts, wirePart{InlineData: &wireInline{MIMEType: blk.MIMEType, Data: blk.Data}})
			}
		}
		return &wireContent{Role: "user", Parts: parts}, nil

	case ai.AssistantMessage:
		var parts []wirePart
		for _, c := range msg.Content {
			switch blk := c.(type) {
			case ai.TextContent:
				if strings.TrimSpace(blk.Text) != "" {
					parts = append(parts, wirePart{Text: blk.Text})
				}
			case ai.ThinkingContent:
				if strings.TrimSpace(blk.Thinking) != "" {
					parts = append(parts, wirePart{Text: blk.Thinking, Thought: true})
				}
			case ai.ToolCall:
				parts = append(parts, wirePart{FunctionCall: &wireFuncCall{Name: blk.Name, Args: blk.Arguments}})
			}
		}
		if len(parts) == 0 {
			return nil, nil
		}
		return &wireContent{Role: "model", Parts: parts}, nil

	case ai.ToolResultMessage:
		var text strings.Builder
		for _, c := range msg.Content {
			if tc, ok := c.(ai.TextContent); ok {
				text.WriteString(tc.Text)
			}
		}
		resp := map[string]any{"output": text.String()}
		if msg.IsError {
			resp = map[string]any{"error": text.String()}
		}
		part := wirePart{FunctionResponse: &wireFuncResp{Name: msg.ToolName, Response: resp}}
		return &wireContent{Role: "user", Parts: []wirePart{part}}, nil
	}

	return nil, fmt.Errorf("google: unsupported message type: %T", m)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func snapshotMsg(msg *ai.AssistantMessage) *ai.AssistantMessage {
	cp := *msg
	cp.Content = make([]ai.ContentBlock, len(msg.Content))
	copy(cp.Content, msg.Content)
	return &cp
}

func mapStopReason(r string) ai.StopReason {
	switch r {
	case "STOP":
		return ai.StopReasonStop
	case "MAX_TOKENS":
		return ai.StopReasonLength
	case "TOOL_CODE", "FUNCTION_CALL":
		return ai.StopReasonTool
	default:
		return ai.StopReasonStop
	}
}

func jsonStr(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
