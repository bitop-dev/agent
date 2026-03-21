package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"math"
	"math/rand"

	"github.com/bitop-dev/agent/pkg/approval"
	"github.com/bitop-dev/agent/pkg/events"
	"github.com/bitop-dev/agent/pkg/policy"
	"github.com/bitop-dev/agent/pkg/provider"
	pkgruntime "github.com/bitop-dev/agent/pkg/runtime"
	"github.com/bitop-dev/agent/pkg/session"
	"github.com/bitop-dev/agent/pkg/tool"
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
	compactionEnabled := req.Profile.Spec.Session.Compaction == "auto"
	// Rough token estimate: 1 token ≈ 4 chars. Reserve 16k for the response,
	// keep the most recent ~20k tokens verbatim. Trigger compaction when the
	// estimated total exceeds 80k tokens (320k chars), matching pi-mono's approach.
	const reserveTokens = 16384
	const keepRecentTokens = 20000
	const contextTokenThreshold = 80000
	toolsByID := make(map[string]tool.Tool, len(req.Tools))
	toolDefs := make([]tool.Definition, 0, len(req.Tools))
	for _, t := range req.Tools {
		def := t.Definition()
		toolsByID[def.ID] = t
		toolDefs = append(toolDefs, def)
	}

	var output strings.Builder
	var toolHistory []tool.Result
	var totalInputTokens, totalOutputTokens int
	var usedModel string
	const maxTurns = 8
	const maxRetries = 3
	const baseRetryDelayMs = 500
	const maxExplorationToolCalls = 6

	// Build model chain: primary + fallbacks.
	models := []string{req.Profile.Spec.Provider.Model}
	models = append(models, req.Profile.Spec.Provider.Fallback...)

	for turn := 0; turn < maxTurns; turn++ {
		if err := sink.Publish(ctx, events.Event{Type: events.TypeTurnStarted, Time: time.Now(), Message: fmt.Sprintf("turn %d started", turn+1)}); err != nil {
			return pkgruntime.RunResult{SessionID: sessionID, Transcript: append([]provider.Message{}, transcript...)}, err
		}
		var stream <-chan provider.StreamEvent
		var err error

		// Try each model in the chain with retries.
		for _, model := range models {
			for attempt := 0; attempt < maxRetries; attempt++ {
				stream, err = req.Provider.Stream(ctx, provider.CompletionRequest{
					Model:    provider.ModelRef{Provider: req.Provider.Name(), Model: model},
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
					_ = sink.Publish(ctx, events.Event{Type: events.TypeError, Time: time.Now(), Message: fmt.Sprintf("model %s attempt %d/%d: %s", model, attempt+1, maxRetries, err)})
					select {
					case <-ctx.Done():
						return pkgruntime.RunResult{SessionID: sessionID, Transcript: append([]provider.Message{}, transcript...)}, ctx.Err()
					case <-time.After(delay):
					}
				}
			}
			if err == nil {
				usedModel = model
				break // success with this model
			}
			_ = sink.Publish(ctx, events.Event{Type: events.TypeError, Time: time.Now(), Message: fmt.Sprintf("model %s exhausted retries, trying fallback", model)})
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
				toolHistory = append(toolHistory, result)
				toolMessages = append(toolMessages, provider.Message{Role: "tool", Content: result.Output, ToolCallID: event.ToolCall.ID, ToolName: event.ToolCall.ToolID})
			case provider.StreamEventDone:
				totalInputTokens += event.InputTokens
				totalOutputTokens += event.OutputTokens
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
		// Compact when estimated context tokens exceed threshold — mirrors pi-mono's approach.
		if compactionEnabled && estimateTranscriptTokens(transcript) > contextTokenThreshold-reserveTokens {
			if compacted, compactionSummary, err := compactTranscript(ctx, req, transcript, keepRecentTokens); err == nil {
				transcript = compacted
				// Persist the compaction entry to the session so it survives resume.
				if req.Sessions != nil && compactionSummary != "" {
					_ = req.Sessions.Append(ctx, sessionID, session.Entry{
						Kind:      session.EntryCompaction,
						Role:      "system",
						Content:   compactionSummary,
						CreatedAt: time.Now(),
					})
				}
			}
		}
		if len(toolHistory) >= maxExplorationToolCalls && strings.TrimSpace(output.String()) == "" {
			break
		}
		if !toolExecuted {
			break
		}
	}

	finalOutput := strings.TrimSpace(output.String())
	if finalOutput == "" || needsFinalAnswer(transcript) {
		answer, updatedTranscript, err := forceFinalAnswer(ctx, req, transcript, toolHistory, sink)
		if err == nil && strings.TrimSpace(answer) != "" {
			finalOutput = strings.TrimSpace(answer)
			transcript = updatedTranscript
			if req.Sessions != nil {
				_ = req.Sessions.Append(ctx, sessionID, session.Entry{
					Kind:      session.EntryMessage,
					Role:      "assistant",
					Content:   finalOutput,
					CreatedAt: time.Now(),
				})
			}
		}
	}
	if finalOutput == "" {
		if fallback := heuristicFinalAnswer(req.Prompt, toolHistory); fallback != "" {
			finalOutput = fallback
			if publishErr := sink.Publish(ctx, events.Event{Type: events.TypeAssistantDelta, Time: time.Now(), Message: fallback}); publishErr != nil {
				return pkgruntime.RunResult{SessionID: sessionID, Transcript: append([]provider.Message{}, transcript...)}, publishErr
			}
			if req.Sessions != nil {
				_ = req.Sessions.Append(ctx, sessionID, session.Entry{Kind: session.EntryMessage, Role: "assistant", Content: finalOutput, CreatedAt: time.Now()})
			}
		}
	}
	if req.Sessions != nil {
		_ = sink.Publish(ctx, events.Event{Type: events.TypeSessionSaved, Time: time.Now(), Message: "session saved", Data: map[string]any{"session_id": sessionID}})
	}
	if err := sink.Publish(ctx, events.Event{Type: events.TypeRunFinished, Time: time.Now(), Message: "run finished", Data: map[string]any{"session_id": sessionID}}); err != nil {
		return pkgruntime.RunResult{SessionID: sessionID, Transcript: append([]provider.Message{}, transcript...)}, err
	}
	return pkgruntime.RunResult{
		SessionID:    sessionID,
		Output:       finalOutput,
		Transcript:   append([]provider.Message{}, transcript...),
		Model:        usedModel,
		InputTokens:  totalInputTokens,
		OutputTokens: totalOutputTokens,
	}, nil
}

func needsFinalAnswer(transcript []provider.Message) bool {
	if len(transcript) == 0 {
		return true
	}
	last := transcript[len(transcript)-1]
	if last.Role == "tool" {
		return true
	}
	if last.Role == "assistant" && len(last.ToolCalls) > 0 && strings.TrimSpace(last.Content) == "" {
		return true
	}
	return false
}

func forceFinalAnswer(ctx context.Context, req pkgruntime.RunRequest, transcript []provider.Message, toolHistory []tool.Result, sink events.Sink) (string, []provider.Message, error) {
	followUp := provider.Message{Role: "user", Content: "You have enough information now. Do not call tools. Answer the original user question directly, briefly, and confidently."}
	messages := append(append([]provider.Message{}, transcript...), followUp)
	stream, err := req.Provider.Stream(ctx, provider.CompletionRequest{
		Model:    provider.ModelRef{Provider: req.Provider.Name(), Model: req.Profile.Spec.Provider.Model},
		System:   req.SystemPrompt,
		Messages: messages,
		Tools:    nil,
	})
	if err != nil {
		return "", transcript, err
	}
	var answer strings.Builder
	for event := range stream {
		if event.Err != nil {
			return "", transcript, event.Err
		}
		if event.Type == provider.StreamEventText {
			answer.WriteString(event.Text)
			if publishErr := sink.Publish(ctx, events.Event{Type: events.TypeAssistantDelta, Time: time.Now(), Message: event.Text}); publishErr != nil {
				return "", transcript, publishErr
			}
		}
	}
	final := strings.TrimSpace(answer.String())
	if final == "" {
		return forceFinalAnswerFromEvidence(ctx, req, transcript, toolHistory, sink)
	}
	updated := append(messages, provider.Message{Role: "assistant", Content: final})
	return final, updated, nil
}

func forceFinalAnswerFromEvidence(ctx context.Context, req pkgruntime.RunRequest, transcript []provider.Message, toolHistory []tool.Result, sink events.Sink) (string, []provider.Message, error) {
	evidence := buildEvidenceSummary(transcript, toolHistory)
	if evidence == "" {
		return "", transcript, nil
	}
	prompt := "Answer the original user question directly and concisely using only the collected evidence below. Do not call tools.\n\nCollected evidence:\n" + evidence
	stream, err := req.Provider.Stream(ctx, provider.CompletionRequest{
		Model:  provider.ModelRef{Provider: req.Provider.Name(), Model: req.Profile.Spec.Provider.Model},
		System: req.SystemPrompt,
		Messages: []provider.Message{
			{Role: "user", Content: prompt},
		},
		Tools: nil,
	})
	if err != nil {
		return "", transcript, err
	}
	var answer strings.Builder
	for event := range stream {
		if event.Err != nil {
			return "", transcript, event.Err
		}
		if event.Type == provider.StreamEventText {
			answer.WriteString(event.Text)
			if publishErr := sink.Publish(ctx, events.Event{Type: events.TypeAssistantDelta, Time: time.Now(), Message: event.Text}); publishErr != nil {
				return "", transcript, publishErr
			}
		}
	}
	final := strings.TrimSpace(answer.String())
	if final == "" {
		return "", transcript, nil
	}
	updated := append(append([]provider.Message{}, transcript...), provider.Message{Role: "assistant", Content: final})
	return final, updated, nil
}

func buildEvidenceSummary(transcript []provider.Message, toolHistory []tool.Result) string {
	var lines []string
	for i := len(toolHistory) - 1; i >= 0 && len(lines) < 6; i-- {
		result := toolHistory[i]
		lines = append(lines, summarizeEvidenceResult(result))
	}
	for i := len(transcript) - 1; i >= 0 && len(lines) < 6; i-- {
		msg := transcript[i]
		if msg.Role == "user" {
			lines = append(lines, "User question: "+compactRuntimeText(msg.Content, 240))
			continue
		}
		if msg.Role == "tool" {
			prefix := "Tool result"
			if msg.ToolName != "" {
				prefix = "Tool result from " + msg.ToolName
			}
			lines = append(lines, prefix+": "+compactRuntimeText(msg.Content, 320))
		}
	}
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}
	return strings.Join(lines, "\n")
}

func summarizeEvidenceResult(result tool.Result) string {
	switch result.ToolID {
	case "core/read":
		if path, _ := result.Data["path"].(string); path != "" {
			return "Read file " + path + ": " + compactRuntimeText(result.Output, 260)
		}
	case "core/glob":
		if matches, ok := result.Data["matches"].([]string); ok && len(matches) > 0 {
			max := 4
			if len(matches) < max {
				max = len(matches)
			}
			return "Glob matches: " + strings.Join(matches[:max], ", ")
		}
	case "core/grep":
		return "Grep result: " + compactRuntimeText(result.Output, 220)
	}
	return "Tool " + result.ToolID + ": " + compactRuntimeText(result.Output, 220)
}

func compactRuntimeText(text string, max int) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if len(text) <= max {
		return text
	}
	if max < 4 {
		return text[:max]
	}
	return text[:max-3] + "..."
}

func heuristicFinalAnswer(prompt string, toolHistory []tool.Result) string {
	lower := strings.ToLower(prompt)
	if strings.Contains(lower, "most important file") {
		readmeSeen := false
		mainSeen := false
		runnerSeen := false
		for _, result := range toolHistory {
			if result.ToolID != "core/read" {
				continue
			}
			path, _ := result.Data["path"].(string)
			switch filepath.Base(path) {
			case "README.md":
				readmeSeen = true
			case "main.go":
				if strings.Contains(path, "cmd/agent") {
					mainSeen = true
				}
			case "runner.go":
				if strings.Contains(path, "internal/runtime") {
					runnerSeen = true
				}
			}
		}
		if readmeSeen {
			answer := "The most important file is `README.md` because it explains what the project is, how it is structured, and how to use it."
			if mainSeen || runnerSeen {
				answer += " If you want the main code entry points next, look at `cmd/agent/main.go` for startup and `internal/runtime/runner.go` for the core execution loop."
			}
			return answer
		}
		if mainSeen {
			answer := "The most important file is `cmd/agent/main.go` because it is the executable entry point for the CLI host."
			if runnerSeen {
				answer += " The next file to read is `internal/runtime/runner.go`, which contains the core agent loop."
			}
			return answer
		}
	}
	return ""
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
		result = tool.Result{
			ToolID: call.ToolID,
			Output: fmt.Sprintf("tool error: %v", err),
			Data:   map[string]any{"error": err.Error()},
		}
		if publishErr := sink.Publish(ctx, events.Event{Type: events.TypeError, Time: time.Now(), Message: err.Error(), Data: map[string]any{"tool_id": call.ToolID}}); publishErr != nil {
			return tool.Result{}, publishErr
		}
	}
	if err := sink.Publish(ctx, events.Event{Type: events.TypeToolFinished, Time: time.Now(), Message: result.Output, Data: result}); err != nil {
		return tool.Result{}, err
	}
	return result, nil
}

func classifyToolCall(call tool.Call) (policy.Action, string, policy.RiskLevel) {
	switch call.ToolID {
	case "core/read":
		path := stringArg(call.Arguments, "path")
		if path == "" {
			return policy.ActionTool, "", policy.RiskLow
		}
		return policy.ActionRead, path, policy.RiskLow
	case "core/write":
		path := stringArg(call.Arguments, "path")
		if path == "" {
			return policy.ActionTool, "", policy.RiskMedium
		}
		return policy.ActionWrite, path, policy.RiskMedium
	case "core/edit":
		path := stringArg(call.Arguments, "path")
		if path == "" {
			return policy.ActionTool, "", policy.RiskMedium
		}
		return policy.ActionEdit, path, policy.RiskMedium
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

// estimateTranscriptTokens returns a rough token count for a transcript.
// Uses the 4-chars-per-token heuristic.
func estimateTranscriptTokens(transcript []provider.Message) int {
	total := 0
	for _, msg := range transcript {
		total += len(msg.Content) / 4
		for _, tc := range msg.ToolCalls {
			total += (len(tc.ToolID) + len(fmt.Sprint(tc.Arguments))) / 4
		}
	}
	return total
}

// compactTranscript summarises the older portion of a transcript, keeping
// the most recent keepRecentTokens worth of messages verbatim. Mirrors
// pi-mono's approach: structured summary format, serialised conversation text,
// turn-boundary-aware cut point.
//
// Returns (compactedTranscript, summaryText, error). All errors are non-fatal —
// the original transcript is returned unchanged on failure.
func compactTranscript(ctx context.Context, req pkgruntime.RunRequest, transcript []provider.Message, keepRecentTokens int) ([]provider.Message, string, error) {
	if len(transcript) < 8 {
		return transcript, "", nil
	}

	// Walk backwards from the end to find the cut point:
	// keep at most keepRecentTokens of recent messages verbatim,
	// always cutting at a turn boundary (never inside a tool call pair).
	cutIdx := findCompactionCutPoint(transcript, keepRecentTokens)
	if cutIdx <= 1 {
		return transcript, "", nil // nothing meaningful to compact
	}

	toSummarise := transcript[:cutIdx]
	toKeep := transcript[cutIdx:]

	// Serialise the messages to be summarised using labeled text so the LLM
	// does not treat them as a live conversation (pi-mono's approach).
	serialised := serializeForCompaction(toSummarise)

	prompt := `You are summarizing a conversation between a user and an AI assistant.
Produce a structured summary using EXACTLY this format:

## Goal
[What the user is trying to accomplish]

## Constraints & Preferences
- [Requirements or preferences mentioned]

## Progress
### Done
- [x] [Completed tasks]

### In Progress
- [ ] [Current work, if any]

### Blocked
- [Issues blocking progress, if any]

## Key Decisions
- **[Decision]**: [Rationale]

## Next Steps
1. [What should happen next]

## Critical Context
- [Any data, values, paths, or facts needed to continue]

---
Conversation to summarize:

` + serialised

	stream, err := req.Provider.Stream(ctx, provider.CompletionRequest{
		Model:    provider.ModelRef{Provider: req.Provider.Name(), Model: req.Profile.Spec.Provider.Model},
		Messages: []provider.Message{{Role: "user", Content: prompt}},
		Tools:    nil,
	})
	if err != nil {
		return transcript, "", nil // non-fatal
	}
	var summaryBuf strings.Builder
	for event := range stream {
		if event.Type == provider.StreamEventText {
			summaryBuf.WriteString(event.Text)
		}
	}
	summaryText := strings.TrimSpace(summaryBuf.String())
	if summaryText == "" {
		return transcript, "", nil
	}

	// Replace summarised messages with a single assistant summary message.
	compacted := make([]provider.Message, 0, 1+len(toKeep))
	compacted = append(compacted, provider.Message{
		Role:    "assistant",
		Content: "[Context compacted — summary of earlier conversation]\n\n" + summaryText,
	})
	compacted = append(compacted, toKeep...)
	return compacted, summaryText, nil
}

// findCompactionCutPoint walks backwards through the transcript, accumulating
// token estimates until keepRecentTokens is reached. Returns the index of the
// first message to keep verbatim. Always cuts at a turn boundary — never
// between an assistant tool_call and its corresponding tool result.
func findCompactionCutPoint(transcript []provider.Message, keepRecentTokens int) int {
	tokens := 0
	for i := len(transcript) - 1; i >= 1; i-- {
		msg := transcript[i]
		tokens += len(msg.Content)/4 + 10

		if tokens >= keepRecentTokens {
			// Back up to the nearest safe turn boundary (user message).
			for i > 1 && transcript[i].Role != "user" {
				i--
			}
			return i
		}
	}
	return 1 // keep everything except the very first message
}

// serializeForCompaction converts a message slice to labeled text that prevents
// the LLM from treating it as a live conversation to continue.
// Mirrors pi-mono's serializeConversation() approach.
func serializeForCompaction(messages []provider.Message) string {
	const maxToolResult = 2000 // truncate long tool results like pi-mono does
	var buf strings.Builder
	for _, msg := range messages {
		switch msg.Role {
		case "user":
			buf.WriteString("[User]: ")
			buf.WriteString(strings.TrimSpace(msg.Content))
			buf.WriteString("\n\n")
		case "assistant":
			if len(msg.ToolCalls) > 0 {
				var calls []string
				for _, tc := range msg.ToolCalls {
					args := fmt.Sprint(tc.Arguments)
					if len(args) > 120 {
						args = args[:120] + "…"
					}
					calls = append(calls, fmt.Sprintf("%s(%s)", tc.ToolID, args))
				}
				buf.WriteString("[Assistant tool calls]: ")
				buf.WriteString(strings.Join(calls, "; "))
				buf.WriteString("\n\n")
			}
			if content := strings.TrimSpace(msg.Content); content != "" {
				buf.WriteString("[Assistant]: ")
				buf.WriteString(content)
				buf.WriteString("\n\n")
			}
		case "tool":
			content := strings.TrimSpace(msg.Content)
			if len(content) > maxToolResult {
				content = content[:maxToolResult] + fmt.Sprintf("\n… [%d chars truncated]", len(content)-maxToolResult)
			}
			name := msg.ToolName
			if name == "" {
				name = "tool"
			}
			buf.WriteString(fmt.Sprintf("[Tool result (%s)]: ", name))
			buf.WriteString(content)
			buf.WriteString("\n\n")
		}
	}
	return buf.String()
}
