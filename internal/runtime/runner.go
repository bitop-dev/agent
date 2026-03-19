package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"math"
	"math/rand"

	"github.com/ncecere/agent/pkg/approval"
	"github.com/ncecere/agent/pkg/events"
	"github.com/ncecere/agent/pkg/policy"
	"github.com/ncecere/agent/pkg/provider"
	pkgruntime "github.com/ncecere/agent/pkg/runtime"
	"github.com/ncecere/agent/pkg/session"
	"github.com/ncecere/agent/pkg/tool"
)

type Runner struct{}

func (Runner) Run(ctx context.Context, req pkgruntime.RunRequest) (pkgruntime.RunResult, error) {
	sink := req.Events
	if sink == nil {
		sink = events.NopSink{}
	}
	now := time.Now()
	sessionID := req.Execution.SessionID
	createSession := sessionID == ""
	if sessionID == "" {
		sessionID = session.NewID(now)
	}
	if err := sink.Publish(ctx, events.Event{Type: events.TypeRunStarted, Time: now, Message: "run started", Data: map[string]any{"session_id": sessionID}}); err != nil {
		return pkgruntime.RunResult{SessionID: sessionID}, err
	}
	if req.Provider == nil {
		err := errors.New("runtime scaffold: provider is not configured")
		_ = sink.Publish(ctx, events.Event{Type: events.TypeError, Time: time.Now(), Message: err.Error()})
		return pkgruntime.RunResult{SessionID: sessionID, Transcript: append([]provider.Message{}, req.Transcript...)}, err
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return pkgruntime.RunResult{SessionID: sessionID, Transcript: append([]provider.Message{}, req.Transcript...)}, errors.New("prompt is required")
	}

	if req.Sessions != nil && createSession {
		_, err := req.Sessions.Create(ctx, session.Metadata{
			ID:        sessionID,
			Profile:   req.Profile.Metadata.Name,
			CWD:       req.Execution.CWD,
			CreatedAt: now,
			UpdatedAt: now,
		})
		if err != nil {
			return pkgruntime.RunResult{SessionID: sessionID, Transcript: append([]provider.Message{}, req.Transcript...)}, err
		}
	}
	if req.Sessions != nil {
		_ = req.Sessions.Append(ctx, sessionID, session.Entry{Kind: session.EntryMessage, Role: "user", Content: req.Prompt, CreatedAt: now})
	}

	transcript := append([]provider.Message{}, req.Transcript...)
	transcript = append(transcript, provider.Message{Role: "user", Content: req.Prompt})
	toolsByID := make(map[string]tool.Tool, len(req.Tools))
	toolDefs := make([]tool.Definition, 0, len(req.Tools))
	for _, t := range req.Tools {
		def := t.Definition()
		toolsByID[def.ID] = t
		toolDefs = append(toolDefs, def)
	}

	var output strings.Builder
	const maxTurns = 8
	const maxRetries = 3
	const baseRetryDelayMs = 500
	for turn := 0; turn < maxTurns; turn++ {
		if err := sink.Publish(ctx, events.Event{Type: events.TypeTurnStarted, Time: time.Now(), Message: fmt.Sprintf("turn %d started", turn+1)}); err != nil {
			return pkgruntime.RunResult{SessionID: sessionID, Transcript: append([]provider.Message{}, transcript...)}, err
		}
		var stream <-chan provider.StreamEvent
		var err error
		for attempt := 0; attempt < maxRetries; attempt++ {
			stream, err = req.Provider.Stream(ctx, provider.CompletionRequest{
				Model:    provider.ModelRef{Provider: req.Provider.Name(), Model: req.Profile.Spec.Provider.Model},
				System:   req.SystemPrompt,
				Messages: transcript,
				Tools:    toolDefs,
			})
			if err == nil {
				break
			}
			if attempt < maxRetries-1 {
				jitter := time.Duration(rand.Intn(200)) * time.Millisecond
				delay := time.Duration(math.Pow(2, float64(attempt)))*time.Duration(baseRetryDelayMs)*time.Millisecond + jitter
				_ = sink.Publish(ctx, events.Event{Type: events.TypeError, Time: time.Now(), Message: fmt.Sprintf("provider error (attempt %d/%d): %s", attempt+1, maxRetries, err)})
				select {
				case <-ctx.Done():
					return pkgruntime.RunResult{SessionID: sessionID, Transcript: append([]provider.Message{}, transcript...)}, ctx.Err()
				case <-time.After(delay):
				}
			}
		}
		if err != nil {
			return pkgruntime.RunResult{SessionID: sessionID, Transcript: append([]provider.Message{}, transcript...)}, err
		}

		toolExecuted := false
		var assistantText strings.Builder
		var assistantToolCalls []tool.Call
		var toolMessages []provider.Message
		for event := range stream {
			if event.Err != nil {
				return pkgruntime.RunResult{SessionID: sessionID, Transcript: append([]provider.Message{}, transcript...)}, event.Err
			}
			switch event.Type {
			case provider.StreamEventText:
				output.WriteString(event.Text)
				assistantText.WriteString(event.Text)
				if err := sink.Publish(ctx, events.Event{Type: events.TypeAssistantDelta, Time: time.Now(), Message: event.Text}); err != nil {
					return pkgruntime.RunResult{SessionID: sessionID, Transcript: append([]provider.Message{}, transcript...)}, err
				}
			case provider.StreamEventToolCall:
				toolExecuted = true
				assistantToolCalls = append(assistantToolCalls, event.ToolCall)
				result, err := executeTool(ctx, req, sink, toolsByID, event.ToolCall)
				if err != nil {
					return pkgruntime.RunResult{SessionID: sessionID, Transcript: append([]provider.Message{}, transcript...)}, err
				}
				toolMessages = append(toolMessages, provider.Message{Role: "tool", Content: result.Output, ToolCallID: event.ToolCall.ID, ToolName: event.ToolCall.ToolID})
			}
		}
		assistantMessage := provider.Message{Role: "assistant", Content: assistantText.String(), ToolCalls: assistantToolCalls}
		if assistantMessage.Content != "" || len(assistantMessage.ToolCalls) > 0 {
			transcript = append(transcript, assistantMessage)
			if req.Sessions != nil {
				_ = req.Sessions.Append(ctx, sessionID, session.Entry{
					Kind:      session.EntryMessage,
					Role:      "assistant",
					Content:   assistantMessage.Content,
					Metadata:  encodeSessionMetadata(session.MessageMetadata{ToolCalls: assistantMessage.ToolCalls}),
					CreatedAt: time.Now(),
				})
			}
		}
		if req.Sessions != nil {
			for _, message := range toolMessages {
				_ = req.Sessions.Append(ctx, sessionID, session.Entry{
					Kind:      session.EntryMessage,
					Role:      "tool",
					Content:   message.Content,
					Metadata:  encodeSessionMetadata(session.MessageMetadata{ToolCallID: message.ToolCallID, ToolName: message.ToolName}),
					CreatedAt: time.Now(),
				})
			}
		}
		transcript = append(transcript, toolMessages...)
		if err := sink.Publish(ctx, events.Event{Type: events.TypeTurnFinished, Time: time.Now(), Message: fmt.Sprintf("turn %d finished", turn+1)}); err != nil {
			return pkgruntime.RunResult{SessionID: sessionID, Transcript: append([]provider.Message{}, transcript...)}, err
		}
		if !toolExecuted {
			break
		}
	}

	finalOutput := strings.TrimSpace(output.String())
	if req.Sessions != nil {
		_ = sink.Publish(ctx, events.Event{Type: events.TypeSessionSaved, Time: time.Now(), Message: "session saved", Data: map[string]any{"session_id": sessionID}})
	}
	if err := sink.Publish(ctx, events.Event{Type: events.TypeRunFinished, Time: time.Now(), Message: "run finished", Data: map[string]any{"session_id": sessionID}}); err != nil {
		return pkgruntime.RunResult{SessionID: sessionID, Transcript: append([]provider.Message{}, transcript...)}, err
	}
	return pkgruntime.RunResult{SessionID: sessionID, Output: finalOutput, Transcript: append([]provider.Message{}, transcript...)}, nil
}

func encodeSessionMetadata(meta session.MessageMetadata) string {
	if meta.ToolCallID == "" && meta.ToolName == "" && len(meta.ToolCalls) == 0 {
		return ""
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return ""
	}
	return string(data)
}

func executeTool(ctx context.Context, req pkgruntime.RunRequest, sink events.Sink, tools map[string]tool.Tool, call tool.Call) (tool.Result, error) {
	if err := sink.Publish(ctx, events.Event{Type: events.TypeToolRequested, Time: time.Now(), Message: call.ToolID}); err != nil {
		return tool.Result{}, err
	}
	toolImpl, ok := tools[call.ToolID]
	if !ok {
		return tool.Result{}, fmt.Errorf("tool %q is not enabled", call.ToolID)
	}
	action, path, risk := classifyToolCall(call)
	if req.Policy != nil {
		decision, err := req.Policy.Check(ctx, policy.CheckRequest{Action: action, ToolID: call.ToolID, Path: path, Risk: risk})
		if err != nil {
			return tool.Result{}, err
		}
		if err := sink.Publish(ctx, events.Event{Type: events.TypePolicyDecision, Time: time.Now(), Message: decision.Reason, Data: decision}); err != nil {
			return tool.Result{}, err
		}
		if decision.Kind == policy.DecisionDeny {
			return tool.Result{}, fmt.Errorf("policy denied %s: %s", call.ToolID, decision.Reason)
		}
		if decision.Kind == policy.DecisionRequireApproval {
			if req.Approvals == nil {
				return tool.Result{}, fmt.Errorf("approval required for %s but no resolver configured", call.ToolID)
			}
			if err := sink.Publish(ctx, events.Event{Type: events.TypeApprovalRequest, Time: time.Now(), Message: decision.Reason, Data: call}); err != nil {
				return tool.Result{}, err
			}
			approvalDecision, err := req.Approvals.Resolve(ctx, approval.Request{Action: string(action), ToolID: call.ToolID, Reason: decision.Reason, Risk: string(decision.Risk)})
			if err != nil {
				return tool.Result{}, err
			}
			if err := sink.Publish(ctx, events.Event{Type: events.TypeApprovalResult, Time: time.Now(), Message: approvalDecision.Reason, Data: approvalDecision}); err != nil {
				return tool.Result{}, err
			}
			if !approvalDecision.Approved {
				return tool.Result{}, fmt.Errorf("approval denied for %s", call.ToolID)
			}
		}
	}
	if err := sink.Publish(ctx, events.Event{Type: events.TypeToolStarted, Time: time.Now(), Message: call.ToolID}); err != nil {
		return tool.Result{}, err
	}
	result, err := toolImpl.Run(ctx, call)
	if err != nil {
		return tool.Result{}, err
	}
	if err := sink.Publish(ctx, events.Event{Type: events.TypeToolFinished, Time: time.Now(), Message: result.Output, Data: result}); err != nil {
		return tool.Result{}, err
	}
	return result, nil
}

func classifyToolCall(call tool.Call) (policy.Action, string, policy.RiskLevel) {
	switch call.ToolID {
	case "core/read":
		return policy.ActionRead, stringArg(call.Arguments, "path"), policy.RiskLow
	case "core/write":
		return policy.ActionWrite, stringArg(call.Arguments, "path"), policy.RiskMedium
	case "core/edit":
		return policy.ActionEdit, stringArg(call.Arguments, "path"), policy.RiskMedium
	case "core/bash":
		return policy.ActionShell, "", policy.RiskHigh
	default:
		return policy.ActionTool, "", policy.RiskMedium
	}
}

func stringArg(args map[string]any, key string) string {
	v, ok := args[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}
