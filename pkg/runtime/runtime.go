package runtime

import (
	"context"

	"github.com/ncecere/agent/pkg/approval"
	"github.com/ncecere/agent/pkg/events"
	"github.com/ncecere/agent/pkg/policy"
	"github.com/ncecere/agent/pkg/profile"
	"github.com/ncecere/agent/pkg/provider"
	"github.com/ncecere/agent/pkg/session"
	"github.com/ncecere/agent/pkg/tool"
	"github.com/ncecere/agent/pkg/workspace"
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
	Prompt       string
	SystemPrompt string
	Profile      profile.Manifest
	Provider     provider.Provider
	Tools        []tool.Tool
	Policy       policy.Engine
	Approvals    approval.Resolver
	Sessions     session.Store
	Events       events.Sink
	Execution    ExecutionContext
	Transcript   []provider.Message
}

type RunResult struct {
	SessionID  string
	Output     string
	Transcript []provider.Message
}
