package runtime

import (
	"context"

	"github.com/bitop-dev/agent/pkg/approval"
	"github.com/bitop-dev/agent/pkg/events"
	"github.com/bitop-dev/agent/pkg/policy"
	"github.com/bitop-dev/agent/pkg/profile"
	"github.com/bitop-dev/agent/pkg/provider"
	"github.com/bitop-dev/agent/pkg/session"
	"github.com/bitop-dev/agent/pkg/tool"
	"github.com/bitop-dev/agent/pkg/workspace"
)

type Runner interface {
	Run(ctx context.Context, req RunRequest) (RunResult, error)
}

type ExecutionContext struct {
	CWD        string
	SessionID  string
	ProfileRef string
	Workspace  workspace.Workspace
}

type RunRequest struct {
	Prompt        string
	SystemPrompt  string
	Profile       profile.Manifest
	Provider      provider.Provider
	Tools         []tool.Tool
	Policy        policy.Engine
	Approvals     approval.Resolver
	Sessions      session.Store
	Events        events.Sink
	Execution     ExecutionContext
	Transcript    []provider.Message
	ModelOverride string // If set, overrides profile's model (from config/CLI/env)
}

type RunResult struct {
	SessionID    string
	Output       string
	Transcript   []provider.Message
	Model        string // which model was actually used
	InputTokens  int    // total input tokens across all turns
	OutputTokens int    // total output tokens across all turns
}
