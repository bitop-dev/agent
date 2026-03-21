package openai

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

	"github.com/bitop-dev/agent/pkg/provider"
	"github.com/bitop-dev/agent/pkg/tool"
)

const (
	apiModeChat      = "chat"
	apiModeResponses = "responses"
)

type Provider struct {
	BaseURL    string
	APIKey     string
	APIMode    string
	HTTPClient *http.Client
}

func (p Provider) Name() string {
	return "openai"
}

func (p Provider) Stream(ctx context.Context, req provider.CompletionRequest) (<-chan provider.StreamEvent, error) {
	if strings.TrimSpace(p.BaseURL) == "" {
		return nil, fmt.Errorf("openai provider base URL is required")
	}
	if strings.TrimSpace(p.APIKey) == "" {
		return nil, fmt.Errorf("openai provider API key is required")
	}
	mode := normalizeMode(p.APIMode)
	ch := make(chan provider.StreamEvent, 8)
	go func() {
		defer close(ch)
		var err error
		switch mode {
		case apiModeResponses:
			err = p.runResponses(ctx, req, ch)
		default:
			err = p.runChat(ctx, req, ch)
		}
		if err != nil {
			ch <- provider.StreamEvent{Err: err}
			return
		}
	}()
	return ch, nil
}

func (p Provider) runChat(ctx context.Context, req provider.CompletionRequest, ch chan<- provider.StreamEvent) error {
	nameMap := buildToolNameMap(req.Tools)
	tools := toChatTools(req.Tools)
	toolChoice := ""
	if len(tools) > 0 {
		toolChoice = "auto"
	}
	// Request usage reporting in the response.
	body := chatRequest{
		Model:         req.Model.Model,
		Messages:      toChatMessages(req),
		Tools:         tools,
		ToolChoice:    toolChoice,
		Stream:        true,
		StreamOptions: &streamOptions{IncludeUsage: true},
	}
	if strings.TrimSpace(req.System) != "" {
		body.Messages = append([]chatMessage{{Role: "system", Content: req.System}}, body.Messages...)
	}
	return p.streamChat(ctx, body, nameMap, ch)
}

// streamChat returns (inputTokens, outputTokens, error).
func (p Provider) streamChat(ctx context.Context, body chatRequest, nameMap map[string]string, ch chan<- provider.StreamEvent) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	httpClient := p.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 120 * time.Second}
	}
	url := strings.TrimRight(p.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		rawBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("openai provider request failed: %s: %s", resp.Status, strings.TrimSpace(string(rawBody)))
	}
	// Collect accumulated tool call state keyed by index.
	type toolCallAccum struct {
		id        string
		name      string
		arguments strings.Builder
	}
	toolCalls := make(map[int]*toolCallAccum)
	// Peek at the content type to decide SSE vs plain JSON fallback.
	contentType := resp.Header.Get("Content-Type")
	isSSE := strings.Contains(contentType, "text/event-stream")
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %w", err)
	}
	if !isSSE {
		// Proxy returned a plain JSON response; fall back to non-streaming parse.
		var fallback chatResponse
		if err := json.Unmarshal(bodyBytes, &fallback); err != nil {
			return fmt.Errorf("parse fallback chat response: %w", err)
		}
		if len(fallback.Choices) == 0 {
			return fmt.Errorf("openai chat response returned no choices")
		}
		message := fallback.Choices[0].Message
		for _, call := range message.ToolCalls {
			args, parseErr := parseArguments(call.Function.Arguments)
			if parseErr != nil {
				return fmt.Errorf("parse tool call %s arguments: %w", call.Function.Name, parseErr)
			}
			ch <- provider.StreamEvent{Type: provider.StreamEventToolCall, ToolCall: tool.Call{ID: call.ID, ToolID: restoreToolID(call.Function.Name, nameMap), Arguments: args}}
		}
		if strings.TrimSpace(message.Content) != "" {
			ch <- provider.StreamEvent{Type: provider.StreamEventText, Text: message.Content}
		}
		// Report usage from non-streaming response.
		if fallback.Usage.TotalTokens > 0 {
			ch <- provider.StreamEvent{
				Type:         provider.StreamEventDone,
				InputTokens:  fallback.Usage.PromptTokens,
				OutputTokens: fallback.Usage.CompletionTokens,
			}
		}
		return nil
	}
	scanner := bufio.NewScanner(strings.NewReader(string(bodyBytes)))
	scanner.Buffer(make([]byte, 64*1024), 64*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}
		var chunk chatStreamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		// Extract usage from final chunk (stream_options.include_usage)
		if chunk.Usage != nil {
			ch <- provider.StreamEvent{
				Type:         provider.StreamEventDone,
				InputTokens:  chunk.Usage.PromptTokens,
				OutputTokens: chunk.Usage.CompletionTokens,
			}
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta
		if delta.Content != "" {
			ch <- provider.StreamEvent{Type: provider.StreamEventText, Text: delta.Content}
		}
		for _, tc := range delta.ToolCalls {
			accum, ok := toolCalls[tc.Index]
			if !ok {
				accum = &toolCallAccum{}
				toolCalls[tc.Index] = accum
			}
			if tc.ID != "" {
				accum.id = tc.ID
			}
			if tc.Function.Name != "" {
				accum.name = tc.Function.Name
			}
			accum.arguments.WriteString(tc.Function.Arguments)
		}
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return fmt.Errorf("reading stream: %w", scanErr)
	}
	for i := 0; i < len(toolCalls); i++ {
		accum, ok := toolCalls[i]
		if !ok {
			continue
		}
		args, err := parseArguments(accum.arguments.String())
		if err != nil {
			return fmt.Errorf("parse tool call %s arguments: %w", accum.name, err)
		}
		ch <- provider.StreamEvent{Type: provider.StreamEventToolCall, ToolCall: tool.Call{ID: accum.id, ToolID: restoreToolID(accum.name, nameMap), Arguments: args}}
	}
	return nil
}

func (p Provider) runResponses(ctx context.Context, req provider.CompletionRequest, ch chan<- provider.StreamEvent) error {
	nameMap := buildToolNameMap(req.Tools)
	body := responsesRequest{
		Model:        req.Model.Model,
		Instructions: req.System,
		Input:        toResponsesInput(req.Messages),
		Tools:        toResponsesTools(req.Tools),
		ToolChoice:   "auto",
	}
	responseBody, err := p.postJSON(ctx, "/responses", body)
	if err != nil {
		return err
	}
	var resp responsesResponse
	if err := json.Unmarshal(responseBody, &resp); err != nil {
		return err
	}
	var textParts []string
	for _, item := range resp.Output {
		switch item.Type {
		case "function_call":
			args, err := parseArguments(item.Arguments)
			if err != nil {
				return fmt.Errorf("parse tool call %s arguments: %w", item.Name, err)
			}
			ch <- provider.StreamEvent{Type: provider.StreamEventToolCall, ToolCall: tool.Call{ID: item.CallID, ToolID: restoreToolID(item.Name, nameMap), Arguments: args}}
		case "message":
			for _, content := range item.Content {
				if strings.TrimSpace(content.Text) != "" {
					textParts = append(textParts, content.Text)
				}
			}
		}
	}
	if strings.TrimSpace(resp.OutputText) != "" {
		textParts = append(textParts, resp.OutputText)
	}
	if joined := strings.TrimSpace(strings.Join(textParts, "\n")); joined != "" {
		ch <- provider.StreamEvent{Type: provider.StreamEventText, Text: joined}
	}
	// Emit usage from responses API.
	if resp.Usage != nil {
		ch <- provider.StreamEvent{
			Type:         provider.StreamEventDone,
			InputTokens:  resp.Usage.InputTokens,
			OutputTokens: resp.Usage.OutputTokens,
		}
	}
	return nil
}

func (p Provider) postJSON(ctx context.Context, endpoint string, body any) ([]byte, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpClient := p.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	url := strings.TrimRight(p.BaseURL, "/") + endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openai provider request failed: %s: %s", resp.Status, strings.TrimSpace(string(responseBody)))
	}
	return responseBody, nil
}

func normalizeMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case apiModeResponses:
		return apiModeResponses
	default:
		return apiModeChat
	}
}

// sanitizeToolName replaces characters not accepted by strict API backends
// (Bedrock enforces [a-zA-Z0-9_-]+, Azure enforces [a-zA-Z0-9_.-]+) with
// underscores. This lets tool IDs like "py/word-count" or "email/draft" pass
// validation on every backend without changing the internal tool registry.
func sanitizeToolName(id string) string {
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

// buildToolNameMap returns a mapping of sanitized-name → original-tool-ID built
// from the supplied tool definitions. Used to reverse-map tool call names that
// come back from the API to the original IDs the runtime understands.
func buildToolNameMap(defs []tool.Definition) map[string]string {
	m := make(map[string]string, len(defs))
	for _, def := range defs {
		m[sanitizeToolName(def.ID)] = def.ID
	}
	return m
}

// restoreToolID looks up the original tool ID for a sanitized name returned by
// the API. Falls back to the sanitized name if no mapping is found.
func restoreToolID(sanitized string, nameMap map[string]string) string {
	if original, ok := nameMap[sanitized]; ok {
		return original
	}
	return sanitized
}

func parseArguments(input string) (map[string]any, error) {
	if strings.TrimSpace(input) == "" {
		return map[string]any{}, nil
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(input), &args); err != nil {
		return nil, err
	}
	return args, nil
}

func toChatMessages(req provider.CompletionRequest) []chatMessage {
	messages := make([]chatMessage, 0, len(req.Messages))
	for _, message := range req.Messages {
		chatMsg := chatMessage{Role: mapRole(message.Role), Content: message.Content}
		if len(message.ToolCalls) > 0 {
			chatMsg.Role = "assistant"
			chatMsg.ToolCalls = make([]chatToolCall, 0, len(message.ToolCalls))
			for _, call := range message.ToolCalls {
				chatMsg.ToolCalls = append(chatMsg.ToolCalls, chatToolCall{
					ID:   call.ID,
					Type: "function",
					Function: chatToolCallFunction{
						Name:      sanitizeToolName(call.ToolID),
						Arguments: mustJSON(call.Arguments),
					},
				})
			}
		}
		if message.Role == "tool" {
			chatMsg.Role = "tool"
			chatMsg.ToolCallID = message.ToolCallID
		}
		messages = append(messages, chatMsg)
	}
	return messages
}

func toResponsesInput(messages []provider.Message) []responsesInputItem {
	items := make([]responsesInputItem, 0, len(messages))
	for _, message := range messages {
		if message.Role == "tool" {
			items = append(items, responsesInputItem{
				Type:   "function_call_output",
				CallID: message.ToolCallID,
				Output: message.Content,
			})
			continue
		}
		items = append(items, responsesInputItem{
			Role: mapRole(message.Role),
			Content: []responsesInputContent{{
				Type: "input_text",
				Text: message.Content,
			}},
		})
	}
	return items
}

func toChatTools(defs []tool.Definition) []chatTool {
	tools := make([]chatTool, 0, len(defs))
	for _, def := range defs {
		tools = append(tools, chatTool{
			Type: "function",
			Function: chatFunction{
				Name:        sanitizeToolName(def.ID),
				Description: def.Description,
				Parameters:  schemaOrObject(def.Schema),
			},
		})
	}
	return tools
}

func toResponsesTools(defs []tool.Definition) []responsesTool {
	tools := make([]responsesTool, 0, len(defs))
	for _, def := range defs {
		tools = append(tools, responsesTool{
			Type:        "function",
			Name:        sanitizeToolName(def.ID),
			Description: def.Description,
			Parameters:  schemaOrObject(def.Schema),
		})
	}
	return tools
}

func schemaOrObject(schema map[string]any) map[string]any {
	if len(schema) == 0 {
		return map[string]any{"type": "object", "properties": map[string]any{}}
	}
	return schema
}

func mapRole(role string) string {
	switch role {
	default:
		return role
	}
}

func mustJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}

type chatRequest struct {
	Model         string         `json:"model"`
	Messages      []chatMessage  `json:"messages"`
	Tools         []chatTool     `json:"tools,omitempty"`
	ToolChoice    string         `json:"tool_choice,omitempty"`
	Stream        bool           `json:"stream,omitempty"`
	StreamOptions *streamOptions `json:"stream_options,omitempty"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type chatStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
}

type chatMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type chatToolCall struct {
	ID       string               `json:"id,omitempty"`
	Type     string               `json:"type"`
	Function chatToolCallFunction `json:"function"`
}

type chatToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type chatTool struct {
	Type     string       `json:"type"`
	Function chatFunction `json:"function"`
}

type chatFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type responsesRequest struct {
	Model        string               `json:"model"`
	Instructions string               `json:"instructions,omitempty"`
	Input        []responsesInputItem `json:"input"`
	Tools        []responsesTool      `json:"tools,omitempty"`
	ToolChoice   string               `json:"tool_choice,omitempty"`
}

type responsesInputItem struct {
	Type    string                  `json:"type,omitempty"`
	Role    string                  `json:"role,omitempty"`
	Content []responsesInputContent `json:"content,omitempty"`
	CallID  string                  `json:"call_id,omitempty"`
	Output  string                  `json:"output,omitempty"`
}

type responsesInputContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type responsesTool struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type responsesResponse struct {
	OutputText string                `json:"output_text"`
	Output     []responsesOutputItem `json:"output"`
	Usage      *struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage,omitempty"`
}

type responsesOutputItem struct {
	Type      string                   `json:"type"`
	CallID    string                   `json:"call_id"`
	Name      string                   `json:"name"`
	Arguments string                   `json:"arguments"`
	Content   []responsesOutputContent `json:"content"`
}

type responsesOutputContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
