package host

import (
	"context"

	"github.com/ncecere/agent/pkg/tool"
)

// Capabilities is the bounded API surface exposed to privileged host-runtime plugins.
// It intentionally does not expose core internals - it exposes only controlled operations.
type Capabilities interface {
	// SpawnSubRun creates a bounded sub-agent run with a restricted profile and tool set.
	SpawnSubRun(ctx context.Context, req SubRunRequest) (SubRunResult, error)
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
