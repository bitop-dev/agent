// Package proxy provides an ai.Provider that forwards requests to a central
// proxy server, and an http.Handler that runs that server.
//
// # Architecture
//
// The proxy decouples API key management from agent processes. A single server
// holds the API keys and routes to any backend provider. Agent processes (on
// laptops, CI, etc.) authenticate with a shared bearer token and stream LLM
// responses back through the proxy.
//
//	┌─────────────┐   POST /stream      ┌──────────────┐   provider.Stream()   ┌──────────┐
//	│ agent (cli) │ ──────────────────► │ proxy server │ ─────────────────────► │ Anthropic│
//	│ proxy.New() │ ◄── SSE events ──── │ NewHandler() │ ◄─── stream events ─── │ OpenAI.. │
//	└─────────────┘                     └──────────────┘                        └──────────┘
//
// # Wire format
//
// Request (POST /stream, Content-Type: application/json):
//
//	{ "model": "...", "context": { "system_prompt": "...", "messages": [...] },
//	  "options": { "max_tokens": 4096, "thinking_level": "medium", ... } }
//
// Response (SSE, Content-Type: text/event-stream):
//
//	data: {"type":"text_delta","delta":"Hello"}
//	data: {"type":"thinking_delta","delta":"..."}
//	data: {"type":"tool_call_start","id":"c1","name":"bash"}
//	data: {"type":"tool_call_args_delta","id":"c1","delta":"{\"cmd\":"}
//	data: {"type":"message_end","stop_reason":"stop","usage":{...}}
//	data: [DONE]
//
// # Usage — server
//
//	provider, _ := anthropic.New("")
//	handler := proxy.NewHandler(provider, "my-bearer-token")
//	http.ListenAndServe(":8080", handler)
//
// # Usage — client (agent.yaml)
//
//	provider: proxy
//	base_url: https://my-proxy.example.com
//	api_key:  my-bearer-token
//	model:    claude-opus-4-5
package proxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bitop-dev/agent/pkg/ai"
)

// ---------------------------------------------------------------------------
// Wire types
// ---------------------------------------------------------------------------

type wireRequest struct {
	Model   string       `json:"model"`
	Context wireContext  `json:"context"`
	Options wireOptions  `json:"options"`
}

type wireContext struct {
	SystemPrompt string         `json:"system_prompt"`
	Messages     []wireMessage  `json:"messages"`
}

type wireOptions struct {
	MaxTokens      int      `json:"max_tokens,omitempty"`
	Temperature    *float64 `json:"temperature,omitempty"`
	ThinkingLevel  string   `json:"thinking_level,omitempty"`
	CacheRetention string   `json:"cache_retention,omitempty"`
}

type wireMessage struct {
	Role       string             `json:"role"`
	Content    []wireContentBlock `json:"content"`
	Model      string             `json:"model,omitempty"`
	Provider   string             `json:"provider,omitempty"`
	Usage      *ai.Usage          `json:"usage,omitempty"`
	StopReason string             `json:"stop_reason,omitempty"`
	ErrorMsg   string             `json:"error_message,omitempty"`
	ToolCallID string             `json:"tool_call_id,omitempty"`
	ToolName   string             `json:"tool_name,omitempty"`
	IsError    bool               `json:"is_error,omitempty"`
	Timestamp  int64              `json:"timestamp"`
}

type wireContentBlock struct {
	Type      string         `json:"type"`
	Text      string         `json:"text,omitempty"`
	Thinking  string         `json:"thinking,omitempty"`
	Data      string         `json:"data,omitempty"`
	MIMEType  string         `json:"mime_type,omitempty"`
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

type wireEvent struct {
	Type         string    `json:"type"`
	Delta        string    `json:"delta,omitempty"`
	ID           string    `json:"id,omitempty"`
	Name         string    `json:"name,omitempty"`
	StopReason   string    `json:"stop_reason,omitempty"`
	ErrorMessage string    `json:"error_message,omitempty"`
	Usage        *ai.Usage `json:"usage,omitempty"`
	Model        string    `json:"model,omitempty"`
	Provider     string    `json:"provider,omitempty"`
}

// ---------------------------------------------------------------------------
// Message conversion helpers
// ---------------------------------------------------------------------------

func encodeMessage(m ai.Message) wireMessage {
	switch msg := m.(type) {
	case ai.UserMessage:
		return wireMessage{Role: "user", Content: encodeBlocks(msg.Content), Timestamp: msg.Timestamp}
	case ai.AssistantMessage:
		u := msg.Usage
		return wireMessage{
			Role: "assistant", Content: encodeBlocks(msg.Content),
			Model: msg.Model, Provider: string(msg.Provider),
			Usage: &u, StopReason: string(msg.StopReason),
			ErrorMsg: msg.ErrorMessage, Timestamp: msg.Timestamp,
		}
	case ai.ToolResultMessage:
		return wireMessage{
			Role: "tool_result", Content: encodeBlocks(msg.Content),
			ToolCallID: msg.ToolCallID, ToolName: msg.ToolName,
			IsError: msg.IsError, Timestamp: msg.Timestamp,
		}
	}
	return wireMessage{}
}

func encodeBlocks(blocks []ai.ContentBlock) []wireContentBlock {
	out := make([]wireContentBlock, 0, len(blocks))
	for _, b := range blocks {
		switch c := b.(type) {
		case ai.TextContent:
			out = append(out, wireContentBlock{Type: "text", Text: c.Text})
		case ai.ThinkingContent:
			out = append(out, wireContentBlock{Type: "thinking", Thinking: c.Thinking})
		case ai.ImageContent:
			out = append(out, wireContentBlock{Type: "image", Data: c.Data, MIMEType: c.MIMEType})
		case ai.ToolCall:
			out = append(out, wireContentBlock{Type: "tool_call", ID: c.ID, Name: c.Name, Arguments: c.Arguments})
		}
	}
	return out
}

func decodeMessage(w wireMessage) ai.Message {
	blocks := decodeBlocks(w.Content)
	ts := w.Timestamp
	if ts == 0 {
		ts = time.Now().UnixMilli()
	}
	switch w.Role {
	case "user":
		return ai.UserMessage{Role: ai.RoleUser, Content: blocks, Timestamp: ts}
	case "assistant":
		var u ai.Usage
		if w.Usage != nil {
			u = *w.Usage
		}
		return ai.AssistantMessage{
			Role: ai.RoleAssistant, Content: blocks,
			Model: w.Model, Provider: w.Provider,
			Usage: u, StopReason: ai.StopReason(w.StopReason),
			ErrorMessage: w.ErrorMsg, Timestamp: ts,
		}
	case "tool_result":
		return ai.ToolResultMessage{
			Role: ai.RoleToolResult, Content: blocks,
			ToolCallID: w.ToolCallID, ToolName: w.ToolName,
			IsError: w.IsError, Timestamp: ts,
		}
	}
	return nil
}

func decodeBlocks(ws []wireContentBlock) []ai.ContentBlock {
	out := make([]ai.ContentBlock, 0, len(ws))
	for _, b := range ws {
		switch b.Type {
		case "text":
			out = append(out, ai.TextContent{Type: "text", Text: b.Text})
		case "thinking":
			out = append(out, ai.ThinkingContent{Type: "thinking", Thinking: b.Thinking})
		case "image":
			out = append(out, ai.ImageContent{Type: "image", Data: b.Data, MIMEType: b.MIMEType})
		case "tool_call":
			out = append(out, ai.ToolCall{Type: "tool_call", ID: b.ID, Name: b.Name, Arguments: b.Arguments})
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Client — ai.Provider implementation
// ---------------------------------------------------------------------------

// Client is an ai.Provider that forwards requests to a proxy server.
type Client struct {
	serverURL string
	token     string
	http      *http.Client
}

// New returns a proxy Client.
// serverURL is the base URL of the proxy server (e.g. "https://proxy.example.com").
// token is the bearer token for authentication.
func New(serverURL, token string) *Client {
	return &Client{
		serverURL: strings.TrimRight(serverURL, "/"),
		token:     token,
		http:      &http.Client{Timeout: 10 * time.Minute},
	}
}

func (c *Client) Name() string { return "proxy" }

func (c *Client) Stream(
	ctx context.Context,
	model string,
	llmCtx ai.Context,
	opts ai.StreamOptions,
) (<-chan ai.StreamEvent, func() (*ai.AssistantMessage, error)) {
	msgs := make([]wireMessage, len(llmCtx.Messages))
	for i, m := range llmCtx.Messages {
		msgs[i] = encodeMessage(m)
	}

	req := wireRequest{
		Model: model,
		Context: wireContext{
			SystemPrompt: llmCtx.SystemPrompt,
			Messages:     msgs,
		},
		Options: wireOptions{
			MaxTokens:      opts.MaxTokens,
			Temperature:    opts.Temperature,
			ThinkingLevel:  string(opts.ThinkingLevel),
			CacheRetention: string(opts.CacheRetention),
		},
	}

	ch := make(chan ai.StreamEvent, 64)

	var finalMsg *ai.AssistantMessage
	var finalErr error
	done := make(chan struct{})

	// Start the HTTP request in a goroutine so the channel can be drained
	// concurrently, matching the contract every other provider uses.
	go func() {
		defer close(ch)
		defer close(done)

		body, err := json.Marshal(req)
		if err != nil {
			finalMsg = errorMsg(fmt.Errorf("proxy: marshal request: %w", err))
			return
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.serverURL+"/stream", bytes.NewReader(body))
		if err != nil {
			finalMsg = errorMsg(fmt.Errorf("proxy: build request: %w", err))
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "text/event-stream")
		if c.token != "" {
			httpReq.Header.Set("Authorization", "Bearer "+c.token)
		}

		resp, err := c.http.Do(httpReq)
		if err != nil {
			finalMsg = errorMsg(fmt.Errorf("proxy: http: %w", err))
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			finalMsg = errorMsg(fmt.Errorf("proxy: server %d: %s", resp.StatusCode, b))
			return
		}

		finalMsg, finalErr = parseProxySSE(ctx, resp.Body, ch)
	}()

	return ch, func() (*ai.AssistantMessage, error) {
		<-done
		return finalMsg, finalErr
	}
}

func parseProxySSE(ctx context.Context, r io.Reader, ch chan<- ai.StreamEvent) (*ai.AssistantMessage, error) {
	msg := &ai.AssistantMessage{
		Role:      ai.RoleAssistant,
		Timestamp: time.Now().UnixMilli(),
	}

	// Track tool calls being assembled.
	type partialCall struct {
		id   string
		name string
		args strings.Builder
	}
	calls := map[string]*partialCall{}
	var callOrder []string

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		if ctx.Err() != nil {
			msg.StopReason = ai.StopReasonAborted
			return msg, nil
		}
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var ev wireEvent
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			continue
		}

		switch ev.Type {
		case "text_delta":
			ch <- ai.StreamEvent{Type: ai.StreamEventTextDelta, Delta: ev.Delta}
			// Accumulate into content
			found := false
			for i := range msg.Content {
				if tc, ok := msg.Content[i].(ai.TextContent); ok {
					msg.Content[i] = ai.TextContent{Type: "text", Text: tc.Text + ev.Delta}
					found = true
					break
				}
			}
			if !found {
				msg.Content = append(msg.Content, ai.TextContent{Type: "text", Text: ev.Delta})
			}

		case "thinking_delta":
			ch <- ai.StreamEvent{Type: ai.StreamEventThinkingDelta, Delta: ev.Delta}
			found := false
			for i := range msg.Content {
				if tc, ok := msg.Content[i].(ai.ThinkingContent); ok {
					msg.Content[i] = ai.ThinkingContent{Type: "thinking", Thinking: tc.Thinking + ev.Delta}
					found = true
					break
				}
			}
			if !found {
				msg.Content = append(msg.Content, ai.ThinkingContent{Type: "thinking", Thinking: ev.Delta})
			}

		case "tool_call_start":
			calls[ev.ID] = &partialCall{id: ev.ID, name: ev.Name}
			callOrder = append(callOrder, ev.ID)

		case "tool_call_args_delta":
			if pc, ok := calls[ev.ID]; ok {
				pc.args.WriteString(ev.Delta)
			}

		case "message_end":
			if ev.StopReason != "" {
				msg.StopReason = ai.StopReason(ev.StopReason)
			}
			if ev.ErrorMessage != "" {
				msg.ErrorMessage = ev.ErrorMessage
			}
			if ev.Usage != nil {
				msg.Usage = *ev.Usage
			}
			if ev.Model != "" {
				msg.Model = ev.Model
			}
			if ev.Provider != "" {
				msg.Provider = ev.Provider
			}
			// Finalise tool calls.
			for _, id := range callOrder {
				pc := calls[id]
				var args map[string]any
				json.Unmarshal([]byte(pc.args.String()), &args)
				msg.Content = append(msg.Content, ai.ToolCall{
					Type: "tool_call", ID: pc.id, Name: pc.name, Arguments: args,
				})
			}
		}
	}

	if msg.StopReason == "" {
		msg.StopReason = ai.StopReasonStop
	}
	return msg, scanner.Err()
}

// latestToolCallIDName extracts the ID and Name from the last ToolCall in partial.
func latestToolCallIDName(partial *ai.AssistantMessage) (id, name string) {
	if partial == nil {
		return "", ""
	}
	for i := len(partial.Content) - 1; i >= 0; i-- {
		if tc, ok := partial.Content[i].(ai.ToolCall); ok {
			return tc.ID, tc.Name
		}
	}
	return "", ""
}

func errorMsg(err error) *ai.AssistantMessage {
	return &ai.AssistantMessage{
		Role:         ai.RoleAssistant,
		StopReason:   ai.StopReasonError,
		ErrorMessage: err.Error(),
		Timestamp:    time.Now().UnixMilli(),
	}
}

// ---------------------------------------------------------------------------
// Server — http.Handler that wraps any ai.Provider
// ---------------------------------------------------------------------------

// Handler is an http.Handler that proxies LLM requests to a backend provider.
// It serves the proxy SSE wire format described in the package doc.
type Handler struct {
	provider ai.Provider
	token    string // if non-empty, require Authorization: Bearer <token>
}

// NewHandler returns an http.Handler that wraps provider.
// If authToken is non-empty, all requests must include
// "Authorization: Bearer <authToken>".
func NewHandler(provider ai.Provider, authToken string) *Handler {
	return &Handler{provider: provider, token: authToken}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Auth check.
	if h.token != "" {
		bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if bearer != h.token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	if r.URL.Path != "/stream" || r.Method != http.MethodPost {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	var req wireRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Decode messages.
	msgs := make([]ai.Message, 0, len(req.Context.Messages))
	for _, wm := range req.Context.Messages {
		if m := decodeMessage(wm); m != nil {
			msgs = append(msgs, m)
		}
	}

	llmCtx := ai.Context{
		SystemPrompt: req.Context.SystemPrompt,
		Messages:     msgs,
	}

	opts := ai.StreamOptions{
		MaxTokens:      req.Options.MaxTokens,
		Temperature:    req.Options.Temperature,
		ThinkingLevel:  ai.ThinkingLevel(req.Options.ThinkingLevel),
		CacheRetention: ai.CacheRetention(req.Options.CacheRetention),
	}

	// Set up SSE.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, canFlush := w.(http.Flusher)

	writeEvent := func(ev wireEvent) {
		data, _ := json.Marshal(ev)
		fmt.Fprintf(w, "data: %s\n\n", data)
		if canFlush {
			flusher.Flush()
		}
	}

	eventCh, wait := h.provider.Stream(r.Context(), req.Model, llmCtx, opts)

	// Pipe events. Tool call ID/name come from the Partial snapshot.
	for ev := range eventCh {
		switch ev.Type {
		case ai.StreamEventTextDelta:
			writeEvent(wireEvent{Type: "text_delta", Delta: ev.Delta})
		case ai.StreamEventThinkingDelta:
			writeEvent(wireEvent{Type: "thinking_delta", Delta: ev.Delta})
		case ai.StreamEventToolCallStart:
			// The name is in the Delta; the partial has the current tool call.
			id, name := latestToolCallIDName(ev.Partial)
			writeEvent(wireEvent{Type: "tool_call_start", ID: id, Name: name})
		case ai.StreamEventToolCallDelta:
			id, _ := latestToolCallIDName(ev.Partial)
			writeEvent(wireEvent{Type: "tool_call_args_delta", ID: id, Delta: ev.Delta})
		}
	}

	finalMsg, err := wait()
	if err != nil || finalMsg == nil {
		writeEvent(wireEvent{Type: "message_end", StopReason: "error", ErrorMessage: fmt.Sprintf("%v", err)})
	} else {
		u := finalMsg.Usage
		writeEvent(wireEvent{
			Type:         "message_end",
			StopReason:   string(finalMsg.StopReason),
			ErrorMessage: finalMsg.ErrorMessage,
			Usage:        &u,
			Model:        finalMsg.Model,
			Provider:     string(finalMsg.Provider),
		})
	}

	fmt.Fprintf(w, "data: [DONE]\n\n")
	if canFlush {
		flusher.Flush()
	}
}
