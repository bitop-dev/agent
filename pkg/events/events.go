package events

import (
	"context"
	"time"
)

type Type string

const (
	TypeRunStarted      Type = "run_started"
	TypeRunFinished     Type = "run_finished"
	TypeTurnStarted     Type = "turn_started"
	TypeTurnFinished    Type = "turn_finished"
	TypeAssistantDelta  Type = "assistant_delta"
	TypeToolRequested   Type = "tool_requested"
	TypeToolStarted     Type = "tool_started"
	TypeToolFinished    Type = "tool_finished"
	TypePolicyDecision  Type = "policy_decision"
	TypeApprovalRequest Type = "approval_requested"
	TypeApprovalResult  Type = "approval_resolved"
	TypeSessionSaved    Type = "session_saved"
	TypeError           Type = "error"
)

type Event struct {
	Type    Type
	Time    time.Time
	Message string
	Data    any
}

type Sink interface {
	Publish(ctx context.Context, event Event) error
}

type SinkFunc func(ctx context.Context, event Event) error

func (f SinkFunc) Publish(ctx context.Context, event Event) error {
	return f(ctx, event)
}

type NopSink struct{}

func (NopSink) Publish(context.Context, Event) error {
	return nil
}
