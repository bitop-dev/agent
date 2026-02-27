// Package agent provides the high-level Agent type and event system.
package agent

import (
	"log/slog"
	"time"

	"github.com/bitop-dev/agent/pkg/ai"
	"github.com/bitop-dev/agent/pkg/ai/models"
	"github.com/bitop-dev/agent/pkg/tools"
)

// ---------------------------------------------------------------------------
// Events
// ---------------------------------------------------------------------------

// EventType identifies an agent lifecycle event.
type EventType string

const (
	// Lifecycle
	EventAgentStart EventType = "agent_start"
	EventAgentEnd   EventType = "agent_end"

	// Turn = one assistant response + any resulting tool calls/results
	EventTurnStart EventType = "turn_start"
	EventTurnEnd   EventType = "turn_end"

	// Message lifecycle
	EventMessageStart  EventType = "message_start"
	EventMessageUpdate EventType = "message_update"
	EventMessageEnd    EventType = "message_end"

	// Tool execution
	EventToolStart  EventType = "tool_start"
	EventToolUpdate EventType = "tool_update"
	EventToolEnd    EventType = "tool_end"

	// Compaction
	EventCompaction EventType = "compaction"

	// Turn limit reached — loop stopped before the LLM finished naturally.
	EventTurnLimitReached EventType = "turn_limit_reached"

	// Retry — the agent is retrying a failed LLM call.
	EventRetry EventType = "retry"

	// Tool denied — the user denied a tool call via the confirmation hook.
	EventToolDenied EventType = "tool_denied"
)

// ContextUsage carries a snapshot of estimated context token usage after a turn.
type ContextUsage struct {
	// Estimated total tokens in the current context window.
	Tokens int
	// Tokens reported by the last assistant message's usage object.
	UsageTokens int
	// Estimated tokens added after the last usage report (tool results, etc.)
	TrailingTokens int
}

// CostUsage tracks cumulative cost across turns.
type CostUsage struct {
	InputTokens  int     // cumulative input tokens
	OutputTokens int     // cumulative output tokens
	InputCost    float64 // cumulative input cost in USD
	OutputCost   float64 // cumulative output cost in USD
	TotalCost    float64 // cumulative total cost in USD
}

// CompactionEvent describes a completed context compaction.
type CompactionEvent struct {
	Summary         string
	MessagesRemoved int
	MessagesKept    int
	TokensBefore    int
	TokensAfter     int
}

// Event carries a lifecycle notification from the agent loop.
type Event struct {
	Type EventType

	// Set for message_* events
	Message ai.Message

	// Set for message_update
	StreamEvent *ai.StreamEvent

	// Set for turn_end
	ToolResults  []ai.ToolResultMessage
	ContextUsage ContextUsage // estimated context token usage after this turn
	CostUsage    CostUsage    // cumulative cost after this turn
	TurnDuration time.Duration // wall-clock time for this turn

	// Set for compaction events
	Compaction *CompactionEvent

	// Set for tool_* events
	ToolCallID string
	ToolName   string
	ToolArgs   map[string]any
	ToolResult *tools.Result
	IsError    bool

	// Set for agent_end
	NewMessages []ai.Message

	// Set for retry events
	RetryAttempt int
	RetryError   error
	RetryDelay   time.Duration

	// Set for metrics callback
	Metrics *TurnMetrics
}

// TurnMetrics captures per-turn performance data for observability.
type TurnMetrics struct {
	TurnNumber       int
	ProviderLatency  time.Duration // time from Stream() call to final message
	ToolDurations    map[string]time.Duration // tool name → execution time
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
	TotalCost        float64
	Error            string
}

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------

// State is the observable state of the agent (read-only snapshot).
type State struct {
	SystemPrompt     string
	Model            string
	Provider         string
	Messages         []ai.Message
	IsStreaming       bool
	PendingToolCalls  map[string]bool // callID → in-flight
	Error            string
	ContextTokens    int // estimated context size after the last turn
	CumulativeCost   CostUsage // cumulative cost across all turns
}

// ---------------------------------------------------------------------------
// Config
// ---------------------------------------------------------------------------

// ConfirmResult is the response from a ConfirmToolCall callback.
type ConfirmResult int

const (
	// ConfirmAllow permits the tool call to execute.
	ConfirmAllow ConfirmResult = iota
	// ConfirmDeny blocks the tool call and informs the LLM.
	ConfirmDeny
	// ConfirmAbort blocks the tool call and stops the agent loop.
	ConfirmAbort
)

// Config holds everything needed to run the agent loop for one call.
type Config struct {
	// ConvertToLLM transforms the agent message history to the slice that gets
	// sent to the LLM. Default: keep only user/assistant/toolResult messages.
	ConvertToLLM func([]ai.Message) ([]ai.Message, error)

	// TransformContext optionally prunes / enriches messages before ConvertToLLM.
	TransformContext func([]ai.Message) ([]ai.Message, error)

	// GetAPIKey returns the API key for the named provider (for dynamic/expiring keys).
	GetAPIKey func(provider string) (string, error)

	// GetSteeringMessages returns interruption messages to inject between tool calls.
	// Return nil/empty to continue normally.
	GetSteeringMessages func() ([]ai.Message, error)

	// GetFollowUpMessages returns follow-up messages after the agent would otherwise stop.
	GetFollowUpMessages func() ([]ai.Message, error)

	// ConfirmToolCall gates tool execution. Called before each tool runs.
	// Return ConfirmAllow to proceed, ConfirmDeny to skip (LLM gets "denied"
	// result), or ConfirmAbort to stop the entire loop.
	//
	// Set to nil for auto-approve (default — all tools execute immediately).
	// Set to AutoApproveAll for explicit auto-approve documentation.
	ConfirmToolCall func(name string, args map[string]any) (ConfirmResult, error)

	// StreamOptions passed to the provider.
	StreamOptions ai.StreamOptions

	// MaxTurns is the maximum number of LLM calls (turns) per Run.
	// Each turn = one assistant response + its tool calls.
	// 0 means unlimited (default). When the limit is hit the loop stops
	// and an EventTurnLimitReached event is broadcast.
	MaxTurns int

	// MaxRetries is the maximum number of retry attempts for transient LLM
	// errors (rate limits, 5xx, network errors). 0 = no retries (default).
	MaxRetries int

	// RetryBaseDelay is the initial backoff delay for retries.
	// Subsequent retries use exponential backoff: delay * 2^attempt.
	// Default (zero): 1 second.
	RetryBaseDelay time.Duration

	// MaxToolConcurrency controls how many tool calls run in parallel when the
	// LLM returns multiple tool calls in one turn.
	// 0 or 1 = sequential (default). Values > 1 enable parallel execution.
	MaxToolConcurrency int

	// ToolTimeout is the maximum duration for a single tool execution.
	// 0 = no timeout (default). Individual tools may have their own timeouts
	// that take precedence (e.g. bash timeout parameter).
	ToolTimeout time.Duration

	// MaxCostUSD is a budget cap. When cumulative cost exceeds this value the
	// loop stops cleanly. 0 = no limit (default).
	MaxCostUSD float64

	// OnMetrics is called at the end of each turn with performance data.
	// Useful for logging, dashboards, or OpenTelemetry emission.
	OnMetrics func(TurnMetrics)
}

// AutoApproveAll is a ConfirmToolCall function that allows everything.
// Use this to be explicit about auto-approval:
//
//	cfg := agent.Config{ConfirmToolCall: agent.AutoApproveAll}
func AutoApproveAll(_ string, _ map[string]any) (ConfirmResult, error) {
	return ConfirmAllow, nil
}

// DefaultMaxTurns is used by the CLI when no explicit limit is set.
// 0 = unlimited; change to a non-zero value to cap all runs.
const DefaultMaxTurns = 0

// defaultRetryBaseDelay is used when Config.RetryBaseDelay is zero.
const defaultRetryBaseDelay = 1 * time.Second

// ---------------------------------------------------------------------------
// Cost calculation
// ---------------------------------------------------------------------------

// computeTurnCost calculates the cost for a single turn's token usage.
func computeTurnCost(model string, usage ai.Usage) CostUsage {
	info := models.Lookup(model)
	if info == nil {
		return CostUsage{
			InputTokens:  usage.Input,
			OutputTokens: usage.Output,
		}
	}
	inputCost := float64(usage.Input) * info.InputCostPer1M / 1_000_000
	outputCost := float64(usage.Output) * info.OutputCostPer1M / 1_000_000
	cacheReadCost := float64(usage.CacheRead) * info.CacheReadCostPer1M / 1_000_000
	cacheWriteCost := float64(usage.CacheWrite) * info.CacheWriteCostPer1M / 1_000_000
	total := inputCost + outputCost + cacheReadCost + cacheWriteCost

	return CostUsage{
		InputTokens:  usage.Input,
		OutputTokens: usage.Output,
		InputCost:    inputCost + cacheReadCost + cacheWriteCost,
		OutputCost:   outputCost,
		TotalCost:    total,
	}
}

// ---------------------------------------------------------------------------
// Logger
// ---------------------------------------------------------------------------

// defaultLogger returns a no-op logger (discards all output).
// Embedders should pass a real *slog.Logger via Options.Logger.
func defaultLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}
