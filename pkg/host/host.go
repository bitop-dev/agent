package host

import (
	"context"

	"github.com/bitop-dev/agent/pkg/tool"
)

// AgentInfo describes a discoverable agent profile.
type AgentInfo struct {
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Description  string   `json:"description"`
	Capabilities []string `json:"capabilities,omitempty"`
	Accepts      string   `json:"accepts,omitempty"`
	Returns      string   `json:"returns,omitempty"`
	Tools        []string `json:"tools"` // tool IDs available to this agent
}

// Capabilities is the bounded API surface exposed to privileged host-runtime plugins.
// It intentionally does not expose core internals - it exposes only controlled operations.
type Capabilities interface {
	// DiscoverAgents returns metadata for all discoverable agent profiles.
	// Orchestrators use this to decide which agents to delegate to.
	DiscoverAgents(ctx context.Context) ([]AgentInfo, error)
	// SpawnSubRun creates a bounded sub-agent run with a restricted profile and tool set.
	SpawnSubRun(ctx context.Context, req SubRunRequest) (SubRunResult, error)
	// SpawnSubRunParallel runs multiple sub-agent tasks concurrently and returns all results.
	// Results are in the same order as the input requests.
	// If any sub-agent fails, its error is included in the combined error but other results
	// are still returned.
	SpawnSubRunParallel(ctx context.Context, reqs []SubRunRequest) ([]SubRunResult, []error)
}

type SubRunRequest struct {
	Task         string
	Profile      string
	MaxTurns     int
	AllowedTools []string
}

type SubRunResult struct {
	Output    string
	SessionID string
	Turns     int
}

// Tool is the interface for host-runtime tool implementations.
// Unlike HTTP or command tools, these are executed directly against host capabilities.
type Tool interface {
	tool.Tool
	// Operation returns the operation name this tool handles.
	Operation() string
}
