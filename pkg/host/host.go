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

// PipelineStep defines one step in an agent pipeline.
type PipelineStep struct {
	Agent    string         `json:"agent"`              // profile name
	Task     string         `json:"task"`               // task template — {{var}} is expanded
	Context  map[string]any `json:"context,omitempty"`  // structured context — values can use {{var}}
	As       string         `json:"as,omitempty"`       // variable name to store this step's output
	MaxTurns int            `json:"maxTurns,omitempty"` // 0 = default
	Parallel []PipelineStep `json:"parallel,omitempty"` // run these steps concurrently instead of using agent
}

// PipelineResult contains the outcome of a pipeline execution.
type PipelineResult struct {
	Steps   []PipelineStepResult `json:"steps"`
	Outputs map[string]string    `json:"outputs"` // named outputs from all steps
}

// PipelineStepResult is the result of one pipeline step.
type PipelineStepResult struct {
	Agent  string `json:"agent"`
	As     string `json:"as,omitempty"`
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
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
	SpawnSubRunParallel(ctx context.Context, reqs []SubRunRequest) ([]SubRunResult, []error)
	// RunPipeline executes a sequence of agent steps where outputs flow between steps
	// via template variables. Steps with a Parallel field run concurrently.
	RunPipeline(ctx context.Context, steps []PipelineStep) (PipelineResult, error)
}

type SubRunRequest struct {
	Task         string
	Profile      string
	MaxTurns     int
	AllowedTools []string
	Context      map[string]any // structured context passed from parent to child
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
