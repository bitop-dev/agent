// Package agent provides the high-level Agent type and event system.
package agent

import (
	"github.com/nickcecere/agent/pkg/ai"
	"github.com/nickcecere/agent/pkg/tools"
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
}

// ---------------------------------------------------------------------------
// Config
// ---------------------------------------------------------------------------

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

	// StreamOptions passed to the provider.
	StreamOptions ai.StreamOptions

	// MaxTurns is the maximum number of LLM calls (turns) per Run.
	// Each turn = one assistant response + its tool calls.
	// 0 means unlimited (default). When the limit is hit the loop stops
	// and an EventTurnLimitReached event is broadcast.
	MaxTurns int
}

// DefaultMaxTurns is used by the CLI when no explicit limit is set.
// 0 = unlimited; change to a non-zero value to cap all runs.
const DefaultMaxTurns = 0
