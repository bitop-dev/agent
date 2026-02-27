// Package ai defines the core types for LLM interactions: messages, tools,
// streaming events, and the provider interface.
package ai

import (
	"context"
	"encoding/json"
)

// ---------------------------------------------------------------------------
// Provider interface
// ---------------------------------------------------------------------------

// Provider streams an LLM response for a given context.
// Events are sent to the returned channel; it is closed when the stream ends.
// The returned AssistantMessage is the final, complete message.
//
// Implementations must close the channel (and not panic) even when ctx is
// cancelled, so callers can always range over it safely.
type Provider interface {
	// Name returns the provider identifier, e.g. "openai", "anthropic".
	Name() string

	// Stream starts a streaming LLM call. It returns:
	//   - a channel of incremental events
	//   - a function that blocks until the stream is complete and returns the
	//     final AssistantMessage (or error)
	Stream(
		ctx context.Context,
		model string,
		llmCtx Context,
		opts StreamOptions,
	) (<-chan StreamEvent, func() (*AssistantMessage, error))
}

// ---------------------------------------------------------------------------
// Content blocks
// ---------------------------------------------------------------------------

type TextContent struct {
	Type string `json:"type"` // "text"
	Text string `json:"text"`
}

type ImageContent struct {
	Type     string `json:"type"`      // "image"
	Data     string `json:"data"`      // base64
	MIMEType string `json:"mime_type"` // e.g. "image/png"
}

type ThinkingContent struct {
	Type     string `json:"type"`     // "thinking"
	Thinking string `json:"thinking"`
}

type ToolCall struct {
	Type      string         `json:"type"`      // "tool_call"
	ID        string         `json:"id"`        // unique call ID
	Name      string         `json:"name"`      // tool name
	Arguments map[string]any `json:"arguments"` // parsed JSON args
}

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

type Role string

const (
	RoleUser       Role = "user"
	RoleAssistant  Role = "assistant"
	RoleToolResult Role = "tool_result"
)

type StopReason string

const (
	StopReasonStop    StopReason = "stop"
	StopReasonLength  StopReason = "length"
	StopReasonTool    StopReason = "tool_use"
	StopReasonError   StopReason = "error"
	StopReasonAborted StopReason = "aborted"
)

// UserMessage is a message from the user (human turn).
type UserMessage struct {
	Role      Role             `json:"role"`
	Content   []ContentBlock   `json:"content"`
	Timestamp int64            `json:"timestamp"` // unix ms
}

func (m UserMessage) GetRole() Role { return m.Role }

// AssistantMessage is a response from the LLM.
type AssistantMessage struct {
	Role         Role             `json:"role"`
	Content      []ContentBlock   `json:"content"`
	Model        string           `json:"model"`
	Provider     string           `json:"provider"`
	Usage        Usage            `json:"usage"`
	StopReason   StopReason       `json:"stop_reason"`
	ErrorMessage string           `json:"error_message,omitempty"`
	Timestamp    int64            `json:"timestamp"`
}

func (m AssistantMessage) GetRole() Role { return m.Role }

// ToolResultMessage carries the result of a tool call back to the LLM.
type ToolResultMessage struct {
	Role       Role           `json:"role"`
	ToolCallID string         `json:"tool_call_id"`
	ToolName   string         `json:"tool_name"`
	Content    []ContentBlock `json:"content"`
	Details    any            `json:"details,omitempty"`
	IsError    bool           `json:"is_error"`
	Timestamp  int64          `json:"timestamp"`
}

func (m ToolResultMessage) GetRole() Role { return m.Role }

// Message is the union type — all three message kinds implement this.
type Message interface {
	GetRole() Role
}

// ContentBlock is an interface implemented by TextContent, ImageContent,
// ThinkingContent, and ToolCall.
type ContentBlock interface {
	contentBlock()
}

func (TextContent) contentBlock()     {}
func (ImageContent) contentBlock()    {}
func (ThinkingContent) contentBlock() {}
func (ToolCall) contentBlock()        {}

// ---------------------------------------------------------------------------
// Usage / cost
// ---------------------------------------------------------------------------

type Cost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cache_read"`
	CacheWrite float64 `json:"cache_write"`
	Total      float64 `json:"total"`
}

type Usage struct {
	Input       int  `json:"input"`
	Output      int  `json:"output"`
	CacheRead   int  `json:"cache_read"`
	CacheWrite  int  `json:"cache_write"`
	TotalTokens int  `json:"total_tokens"`
	Cost        Cost `json:"cost"`
}

// ---------------------------------------------------------------------------
// Tool definition (schema handed to LLM)
// ---------------------------------------------------------------------------

// ToolDefinition describes a tool to the LLM.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"` // JSON Schema object
}

// ---------------------------------------------------------------------------
// Streaming events
// ---------------------------------------------------------------------------

// StreamEventType enumerates all events the provider can emit.
type StreamEventType string

const (
	// Lifecycle
	StreamEventStart StreamEventType = "start"
	StreamEventDone  StreamEventType = "done"
	StreamEventError StreamEventType = "error"

	// Text
	StreamEventTextStart StreamEventType = "text_start"
	StreamEventTextDelta StreamEventType = "text_delta"
	StreamEventTextEnd   StreamEventType = "text_end"

	// Thinking
	StreamEventThinkingStart StreamEventType = "thinking_start"
	StreamEventThinkingDelta StreamEventType = "thinking_delta"
	StreamEventThinkingEnd   StreamEventType = "thinking_end"

	// Tool calls
	StreamEventToolCallStart StreamEventType = "tool_call_start"
	StreamEventToolCallDelta StreamEventType = "tool_call_delta"
	StreamEventToolCallEnd   StreamEventType = "tool_call_end"
)

// StreamEvent is sent over the events channel by providers.
type StreamEvent struct {
	Type    StreamEventType
	Partial *AssistantMessage // always the latest partial snapshot
	Delta   string            // incremental text / thinking / args delta
	Error   error             // set on StreamEventError
}

// ---------------------------------------------------------------------------
// Context passed to provider
// ---------------------------------------------------------------------------

// Context holds the full conversation state for one LLM call.
type Context struct {
	SystemPrompt string
	Messages     []Message
	Tools        []ToolDefinition
}

// ---------------------------------------------------------------------------
// Thinking / reasoning
// ---------------------------------------------------------------------------

// ThinkingLevel controls how much extended reasoning a model performs.
// Not all providers or models support all levels.
//
//   - "off"     — disable thinking/reasoning entirely
//   - "minimal" — shortest possible thinking trace
//   - "low"     — light reasoning (fast, cheap)
//   - "medium"  — balanced (default for most tasks)
//   - "high"    — thorough reasoning
//   - "xhigh"   — maximum (only supported by select OpenAI models)
type ThinkingLevel string

const (
	ThinkingOff     ThinkingLevel = "off"
	ThinkingMinimal ThinkingLevel = "minimal"
	ThinkingLow     ThinkingLevel = "low"
	ThinkingMedium  ThinkingLevel = "medium"
	ThinkingHigh    ThinkingLevel = "high"
	ThinkingXHigh   ThinkingLevel = "xhigh"
)

// ThinkingBudgets holds custom token budgets per thinking level.
// Only relevant for budget-based providers (Anthropic older models, Google Gemini).
// Zero means "use the provider default for that level".
type ThinkingBudgets struct {
	Minimal int
	Low     int
	Medium  int
	High    int
}

// defaultThinkingBudgets are the token budgets used when no custom budgets
// are provided. Sized to match pi-mono's defaults.
var defaultThinkingBudgets = ThinkingBudgets{
	Minimal: 1024,
	Low:     2048,
	Medium:  8192,
	High:    16384,
}

// ThinkingBudgetFor returns the token budget for the given level,
// using the custom budgets if non-zero, otherwise the defaults.
func (b ThinkingBudgets) ThinkingBudgetFor(level ThinkingLevel) int {
	clamp := level
	if clamp == ThinkingXHigh {
		clamp = ThinkingHigh // xhigh is only meaningful for OpenAI
	}
	var custom int
	switch clamp {
	case ThinkingMinimal:
		custom = b.Minimal
	case ThinkingLow:
		custom = b.Low
	case ThinkingMedium:
		custom = b.Medium
	case ThinkingHigh:
		custom = b.High
	}
	if custom > 0 {
		return custom
	}
	switch clamp {
	case ThinkingMinimal:
		return defaultThinkingBudgets.Minimal
	case ThinkingLow:
		return defaultThinkingBudgets.Low
	case ThinkingMedium:
		return defaultThinkingBudgets.Medium
	default:
		return defaultThinkingBudgets.High
	}
}

// ---------------------------------------------------------------------------
// Stream options
// ---------------------------------------------------------------------------

// CacheRetention controls prompt caching aggressiveness for providers that
// support it (primarily Anthropic).
type CacheRetention string

const (
	CacheRetentionNone  CacheRetention = "none"
	CacheRetentionShort CacheRetention = "short" // default
	CacheRetentionLong  CacheRetention = "long"
)

type StreamOptions struct {
	Temperature    *float64
	MaxTokens      int
	APIKey         string
	ThinkingLevel  ThinkingLevel
	ThinkingBudgets ThinkingBudgets
	CacheRetention CacheRetention
}
