package policy

import "context"

type Action string

const (
	ActionRead   Action = "read"
	ActionWrite  Action = "write"
	ActionEdit   Action = "edit"
	ActionTool   Action = "tool"
	ActionShell  Action = "shell"
	ActionNet    Action = "network"
	ActionPlugin Action = "plugin"
)

type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

type DecisionKind string

const (
	DecisionAllow           DecisionKind = "allow"
	DecisionDeny            DecisionKind = "deny"
	DecisionRequireApproval DecisionKind = "require_approval"
)

type CheckRequest struct {
	Action  Action
	ToolID  string
	Path    string
	Command []string
	Risk    RiskLevel
}

type Decision struct {
	Kind   DecisionKind
	Reason string
	Risk   RiskLevel
}

type Engine interface {
	Check(ctx context.Context, req CheckRequest) (Decision, error)
}
