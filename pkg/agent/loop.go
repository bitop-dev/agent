package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nickcecere/agent/pkg/ai"
	"github.com/nickcecere/agent/pkg/tools"
)

// runLoop is the core agentic loop. It:
//  1. Sends the current conversation to the LLM (streaming) with retry on transient errors.
//  2. Executes any tool calls (with confirmation, timeout, and parallel support).
//  3. Checks for steering messages (user interruption) after each tool.
//  4. Tracks cost per turn and cumulatively.
//  5. Repeats until no tool calls and no follow-up messages.
func (a *Agent) runLoop(
	ctx context.Context,
	newMsgs []ai.Message, // nil = continue from existing context
	cfg Config,
) error {
	emit := func(e Event) {
		a.broadcast(e)
	}

	emit(Event{Type: EventAgentStart})
	defer func() {
		emit(Event{Type: EventAgentEnd, NewMessages: a.collectNew()})
	}()

	// Add new messages to conversation history
	if len(newMsgs) > 0 {
		for _, m := range newMsgs {
			a.appendMsg(m)
			emit(Event{Type: EventMessageStart, Message: m})
			emit(Event{Type: EventMessageEnd, Message: m})
		}
	}

	var pendingMessages []ai.Message

	turnCount := 0
	for {
		hasToolCalls := true
		var steeringAfterTools []ai.Message

		for hasToolCalls || len(pendingMessages) > 0 {
			// ── Max-turn guard ──────────────────────────────────────────
			if cfg.MaxTurns > 0 && turnCount >= cfg.MaxTurns {
				emit(Event{Type: EventTurnLimitReached})
				return nil
			}

			// ── Budget guard ────────────────────────────────────────────
			if cfg.MaxCostUSD > 0 {
				a.mu.RLock()
				cost := a.cumulativeCost.TotalCost
				a.mu.RUnlock()
				if cost >= cfg.MaxCostUSD {
					a.logger.Warn("budget limit reached", "cost", cost, "limit", cfg.MaxCostUSD)
					emit(Event{Type: EventTurnLimitReached})
					return nil
				}
			}

			turnCount++
			turnStart := time.Now()

			// Inject steering / follow-up messages
			for _, m := range pendingMessages {
				a.appendMsg(m)
				emit(Event{Type: EventMessageStart, Message: m})
				emit(Event{Type: EventMessageEnd, Message: m})
			}
			pendingMessages = nil

			// Compact context if needed (before next LLM call).
			if err := a.maybeCompact(ctx); err != nil {
				a.logger.Warn("compaction failed", "error", err)
			}

			// Stream assistant response (with retry)
			providerStart := time.Now()
			assistantMsg, err := a.streamResponseWithRetry(ctx, cfg, emit)
			providerLatency := time.Since(providerStart)
			if err != nil {
				return err
			}
			a.appendMsg(assistantMsg)

			if assistantMsg.StopReason == ai.StopReasonError ||
				assistantMsg.StopReason == ai.StopReasonAborted {
				emit(Event{Type: EventTurnEnd, Message: assistantMsg})
				return nil
			}

			// Collect tool calls
			var toolCalls []ai.ToolCall
			for _, c := range assistantMsg.Content {
				if tc, ok := c.(ai.ToolCall); ok {
					toolCalls = append(toolCalls, tc)
				}
			}
			hasToolCalls = len(toolCalls) > 0

			var toolResults []ai.ToolResultMessage
			var toolDurations map[string]time.Duration
			if hasToolCalls {
				var results []ai.ToolResultMessage
				var steering []ai.Message
				var durations map[string]time.Duration
				var execErr error

				results, steering, durations, execErr = a.executeToolCalls(ctx, toolCalls, cfg, emit)
				if execErr != nil {
					return execErr
				}
				toolResults = results
				toolDurations = durations
				steeringAfterTools = steering
				for _, r := range toolResults {
					a.appendMsg(r)
				}
			}

			// ── Cost tracking ───────────────────────────────────────────
			turnCost := computeTurnCost(a.model, assistantMsg.Usage)
			a.mu.Lock()
			a.cumulativeCost.InputTokens += turnCost.InputTokens
			a.cumulativeCost.OutputTokens += turnCost.OutputTokens
			a.cumulativeCost.InputCost += turnCost.InputCost
			a.cumulativeCost.OutputCost += turnCost.OutputCost
			a.cumulativeCost.TotalCost += turnCost.TotalCost
			cumCost := a.cumulativeCost
			a.mu.Unlock()

			usage := EstimateContextTokens(a.snapshotMessages())
			turnDur := time.Since(turnStart)

			emit(Event{
				Type:         EventTurnEnd,
				Message:      assistantMsg,
				ToolResults:  toolResults,
				ContextUsage: usage,
				CostUsage:    cumCost,
				TurnDuration: turnDur,
			})

			// ── Metrics callback ────────────────────────────────────────
			if cfg.OnMetrics != nil {
				cfg.OnMetrics(TurnMetrics{
					TurnNumber:       turnCount,
					ProviderLatency:  providerLatency,
					ToolDurations:    toolDurations,
					InputTokens:      assistantMsg.Usage.Input,
					OutputTokens:     assistantMsg.Usage.Output,
					CacheReadTokens:  assistantMsg.Usage.CacheRead,
					CacheWriteTokens: assistantMsg.Usage.CacheWrite,
					TotalCost:        turnCost.TotalCost,
				})
			}

			if len(steeringAfterTools) > 0 {
				pendingMessages = steeringAfterTools
				steeringAfterTools = nil
			} else if cfg.GetSteeringMessages != nil {
				msgs, _ := cfg.GetSteeringMessages()
				pendingMessages = msgs
			}
		}

		// Would stop here — check for follow-up messages
		if cfg.GetFollowUpMessages != nil {
			followUp, _ := cfg.GetFollowUpMessages()
			if len(followUp) > 0 {
				pendingMessages = followUp
				continue
			}
		}
		break
	}

	return nil
}

// ---------------------------------------------------------------------------
// Retry logic
// ---------------------------------------------------------------------------

// isTransientError returns true if the error is likely transient and retryable.
func isTransientError(msg *ai.AssistantMessage, err error) bool {
	if err != nil {
		s := err.Error()
		for _, pattern := range []string{
			"429", "rate limit", "too many requests",
			"500", "502", "503", "504",
			"internal server error", "bad gateway", "service unavailable",
			"connection reset", "connection refused", "EOF",
			"timeout", "timed out",
		} {
			if strings.Contains(strings.ToLower(s), pattern) {
				return true
			}
		}
	}
	if msg != nil && msg.StopReason == ai.StopReasonError {
		s := msg.ErrorMessage
		for _, pattern := range []string{
			"429", "rate limit", "too many requests",
			"500", "502", "503", "504",
			"overloaded", "capacity",
		} {
			if strings.Contains(strings.ToLower(s), pattern) {
				return true
			}
		}
	}
	return false
}

// streamResponseWithRetry calls streamResponse with exponential backoff retry.
func (a *Agent) streamResponseWithRetry(
	ctx context.Context,
	cfg Config,
	emit func(Event),
) (*ai.AssistantMessage, error) {
	maxRetries := cfg.MaxRetries
	baseDelay := cfg.RetryBaseDelay
	if baseDelay == 0 {
		baseDelay = defaultRetryBaseDelay
	}

	for attempt := 0; ; attempt++ {
		msg, err := a.streamResponse(ctx, cfg, emit)

		// Success or non-retryable
		if err == nil && (msg.StopReason != ai.StopReasonError || !isTransientError(msg, nil)) {
			return msg, nil
		}
		if err != nil && !isTransientError(nil, err) {
			return msg, err
		}

		// Check if we've exhausted retries
		if attempt >= maxRetries {
			return msg, err
		}

		// Backoff
		delay := baseDelay * (1 << attempt)
		if delay > 60*time.Second {
			delay = 60 * time.Second
		}

		a.logger.Warn("retrying LLM call",
			"attempt", attempt+1,
			"max_retries", maxRetries,
			"delay", delay,
			"error", fmt.Sprintf("%v", err),
		)

		emit(Event{
			Type:         EventRetry,
			RetryAttempt: attempt + 1,
			RetryError:   err,
			RetryDelay:   delay,
		})

		select {
		case <-ctx.Done():
			return msg, ctx.Err()
		case <-time.After(delay):
		}
	}
}

// streamResponse calls the provider and fans stream events to listeners.
func (a *Agent) streamResponse(
	ctx context.Context,
	cfg Config,
	emit func(Event),
) (*ai.AssistantMessage, error) {
	// Snapshot history
	history := a.snapshotMessages()

	// Apply transform
	if cfg.TransformContext != nil {
		var err error
		history, err = cfg.TransformContext(history)
		if err != nil {
			return nil, fmt.Errorf("transform context: %w", err)
		}
	}

	// Convert to LLM messages
	llmMsgs := history
	if cfg.ConvertToLLM != nil {
		var err error
		llmMsgs, err = cfg.ConvertToLLM(history)
		if err != nil {
			return nil, fmt.Errorf("convert to llm: %w", err)
		}
	} else {
		llmMsgs = defaultConvertToLLM(history)
	}

	// Build tool definitions from registry
	var toolDefs []ai.ToolDefinition
	for _, t := range a.tools.All() {
		toolDefs = append(toolDefs, t.Definition())
	}

	llmCtx := ai.Context{
		SystemPrompt: a.systemPrompt,
		Messages:     llmMsgs,
		Tools:        toolDefs,
	}

	// Resolve API key
	opts := cfg.StreamOptions
	if cfg.GetAPIKey != nil {
		key, err := cfg.GetAPIKey(a.provider.Name())
		if err == nil && key != "" {
			opts.APIKey = key
		}
	}

	events, wait := a.provider.Stream(ctx, a.model, llmCtx, opts)

	// Build partial message
	partial := &ai.AssistantMessage{
		Role:      ai.RoleAssistant,
		Model:     a.model,
		Provider:  a.provider.Name(),
		Timestamp: time.Now().UnixMilli(),
	}

	emit(Event{Type: EventMessageStart, Message: partial})

	for ev := range events {
		switch ev.Type {
		case ai.StreamEventStart:
			partial = ev.Partial
		case ai.StreamEventTextDelta,
			ai.StreamEventThinkingDelta,
			ai.StreamEventToolCallStart,
			ai.StreamEventToolCallDelta,
			ai.StreamEventToolCallEnd:
			partial = ev.Partial
			emit(Event{Type: EventMessageUpdate, Message: partial, StreamEvent: &ev})
		case ai.StreamEventDone:
			partial = ev.Partial
		case ai.StreamEventError:
			// surface error as error message
			partial.StopReason = ai.StopReasonError
			if ev.Error != nil {
				partial.ErrorMessage = ev.Error.Error()
			}
		}
	}

	finalMsg, err := wait()
	if err != nil {
		partial.StopReason = ai.StopReasonError
		partial.ErrorMessage = err.Error()
		emit(Event{Type: EventMessageEnd, Message: partial})
		return partial, nil // non-fatal: agent records error turn
	}

	emit(Event{Type: EventMessageEnd, Message: finalMsg})
	return finalMsg, nil
}

// ---------------------------------------------------------------------------
// Tool execution
// ---------------------------------------------------------------------------

// executeToolCalls runs tool calls, checking for steering after each.
// Supports parallel execution when cfg.MaxToolConcurrency > 1.
func (a *Agent) executeToolCalls(
	ctx context.Context,
	toolCalls []ai.ToolCall,
	cfg Config,
	emit func(Event),
) ([]ai.ToolResultMessage, []ai.Message, map[string]time.Duration, error) {
	concurrency := cfg.MaxToolConcurrency
	if concurrency <= 1 {
		results, steering, durations, err := a.executeToolCallsSequential(ctx, toolCalls, cfg, emit)
		return results, steering, durations, err
	}
	return a.executeToolCallsParallel(ctx, toolCalls, cfg, emit, concurrency)
}

// executeToolCallsSequential runs tool calls one at a time.
func (a *Agent) executeToolCallsSequential(
	ctx context.Context,
	toolCalls []ai.ToolCall,
	cfg Config,
	emit func(Event),
) ([]ai.ToolResultMessage, []ai.Message, map[string]time.Duration, error) {
	var results []ai.ToolResultMessage
	var steeringMessages []ai.Message
	durations := make(map[string]time.Duration, len(toolCalls))

	for i, tc := range toolCalls {
		// ── Confirmation hook ───────────────────────────────────────
		if cfg.ConfirmToolCall != nil {
			decision, err := cfg.ConfirmToolCall(tc.Name, tc.Arguments)
			if err != nil {
				return results, nil, durations, fmt.Errorf("confirm tool call: %w", err)
			}
			if decision == ConfirmAbort {
				emit(Event{Type: EventToolDenied, ToolCallID: tc.ID, ToolName: tc.Name, ToolArgs: tc.Arguments})
				return results, nil, durations, fmt.Errorf("tool call aborted by user")
			}
			if decision == ConfirmDeny {
				emit(Event{Type: EventToolDenied, ToolCallID: tc.ID, ToolName: tc.Name, ToolArgs: tc.Arguments})
				denied := ai.ToolResultMessage{
					Role:       ai.RoleToolResult,
					ToolCallID: tc.ID,
					ToolName:   tc.Name,
					Content:    []ai.ContentBlock{ai.TextContent{Type: "text", Text: "Tool call denied by user."}},
					IsError:    true,
					Timestamp:  time.Now().UnixMilli(),
				}
				results = append(results, denied)
				continue
			}
		}

		emit(Event{
			Type:       EventToolStart,
			ToolCallID: tc.ID,
			ToolName:   tc.Name,
			ToolArgs:   tc.Arguments,
		})

		toolStart := time.Now()
		result, isError := a.executeSingleTool(ctx, tc, cfg, emit)
		durations[tc.Name] = time.Since(toolStart)

		emit(Event{
			Type:       EventToolEnd,
			ToolCallID: tc.ID,
			ToolName:   tc.Name,
			ToolArgs:   tc.Arguments,
			ToolResult: &result,
			IsError:    isError,
		})

		contentBlocks := append([]ai.ContentBlock(nil), result.Content...)

		toolResult := ai.ToolResultMessage{
			Role:       ai.RoleToolResult,
			ToolCallID: tc.ID,
			ToolName:   tc.Name,
			Content:    contentBlocks,
			Details:    result.Details,
			IsError:    isError,
			Timestamp:  time.Now().UnixMilli(),
		}
		results = append(results, toolResult)

		emit(Event{Type: EventMessageStart, Message: toolResult})
		emit(Event{Type: EventMessageEnd, Message: toolResult})

		// Check steering after each tool (skip remaining if user interrupted)
		if cfg.GetSteeringMessages != nil {
			steering, _ := cfg.GetSteeringMessages()
			if len(steering) > 0 {
				steeringMessages = steering
				// Skip remaining tool calls with placeholder results
				for _, skipped := range toolCalls[i+1:] {
					skippedResult := ai.ToolResultMessage{
						Role:       ai.RoleToolResult,
						ToolCallID: skipped.ID,
						ToolName:   skipped.Name,
						Content:    []ai.ContentBlock{ai.TextContent{Type: "text", Text: "Skipped due to user interrupt."}},
						IsError:    true,
						Timestamp:  time.Now().UnixMilli(),
					}
					results = append(results, skippedResult)
				}
				break
			}
		}
	}

	return results, steeringMessages, durations, nil
}

// executeToolCallsParallel runs tool calls concurrently with a concurrency cap.
func (a *Agent) executeToolCallsParallel(
	ctx context.Context,
	toolCalls []ai.ToolCall,
	cfg Config,
	emit func(Event),
	concurrency int,
) ([]ai.ToolResultMessage, []ai.Message, map[string]time.Duration, error) {
	type toolOutput struct {
		result   tools.Result
		isError  bool
		duration time.Duration
		denied   bool
	}

	outputs := make([]toolOutput, len(toolCalls))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i, tc := range toolCalls {
		// ── Confirmation hook (serial, before dispatch) ─────────────
		if cfg.ConfirmToolCall != nil {
			decision, err := cfg.ConfirmToolCall(tc.Name, tc.Arguments)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("confirm tool call: %w", err)
			}
			if decision == ConfirmAbort {
				emit(Event{Type: EventToolDenied, ToolCallID: tc.ID, ToolName: tc.Name, ToolArgs: tc.Arguments})
				return nil, nil, nil, fmt.Errorf("tool call aborted by user")
			}
			if decision == ConfirmDeny {
				emit(Event{Type: EventToolDenied, ToolCallID: tc.ID, ToolName: tc.Name, ToolArgs: tc.Arguments})
				outputs[i] = toolOutput{
					result:  tools.Result{Content: []ai.ContentBlock{ai.TextContent{Type: "text", Text: "Tool call denied by user."}}},
					isError: true,
					denied:  true,
				}
				continue
			}
		}

		wg.Add(1)
		go func(idx int, tc ai.ToolCall) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			emit(Event{
				Type:       EventToolStart,
				ToolCallID: tc.ID,
				ToolName:   tc.Name,
				ToolArgs:   tc.Arguments,
			})

			start := time.Now()
			result, isError := a.executeSingleTool(ctx, tc, cfg, emit)
			dur := time.Since(start)

			outputs[idx] = toolOutput{result: result, isError: isError, duration: dur}

			emit(Event{
				Type:       EventToolEnd,
				ToolCallID: tc.ID,
				ToolName:   tc.Name,
				ToolArgs:   tc.Arguments,
				ToolResult: &result,
				IsError:    isError,
			})
		}(i, tc)
	}
	wg.Wait()

	// Build results in original order.
	var results []ai.ToolResultMessage
	durations := make(map[string]time.Duration, len(toolCalls))
	for i, tc := range toolCalls {
		out := outputs[i]
		durations[tc.Name] = out.duration
		contentBlocks := append([]ai.ContentBlock(nil), out.result.Content...)
		toolResult := ai.ToolResultMessage{
			Role:       ai.RoleToolResult,
			ToolCallID: tc.ID,
			ToolName:   tc.Name,
			Content:    contentBlocks,
			Details:    out.result.Details,
			IsError:    out.isError,
			Timestamp:  time.Now().UnixMilli(),
		}
		results = append(results, toolResult)

		emit(Event{Type: EventMessageStart, Message: toolResult})
		emit(Event{Type: EventMessageEnd, Message: toolResult})
	}

	return results, nil, durations, nil
}

// executeSingleTool looks up and runs one tool call with panic recovery and timeout.
func (a *Agent) executeSingleTool(
	ctx context.Context,
	tc ai.ToolCall,
	cfg Config,
	emit func(Event),
) (result tools.Result, isError bool) {
	// ── Panic recovery ──────────────────────────────────────────────────
	defer func() {
		if r := recover(); r != nil {
			a.logger.Error("tool panicked", "tool", tc.Name, "panic", r)
			result = tools.ErrorResult(fmt.Errorf("tool %q panicked: %v", tc.Name, r))
			isError = true
		}
	}()

	tool := a.tools.Get(tc.Name)
	if tool == nil {
		return tools.ErrorResult(fmt.Errorf("tool %q not found", tc.Name)), true
	}

	// Validate and coerce params against the tool's JSON Schema.
	params, err := tools.ValidateAndCoerce(tool, tc.Arguments)
	if err != nil {
		return tools.ErrorResult(err), true
	}

	onUpdate := func(partial tools.Result) {
		emit(Event{
			Type:       EventToolUpdate,
			ToolCallID: tc.ID,
			ToolName:   tc.Name,
			ToolArgs:   tc.Arguments,
			ToolResult: &partial,
		})
	}

	// ── Tool timeout ────────────────────────────────────────────────────
	execCtx := ctx
	if cfg.ToolTimeout > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, cfg.ToolTimeout)
		defer cancel()
	}

	res, err := tool.Execute(execCtx, tc.ID, params, onUpdate)
	if err != nil {
		return tools.ErrorResult(err), true
	}
	return res, false
}

// defaultConvertToLLM filters to the three message types LLMs understand.
func defaultConvertToLLM(msgs []ai.Message) []ai.Message {
	out := make([]ai.Message, 0, len(msgs))
	for _, m := range msgs {
		switch m.GetRole() {
		case ai.RoleUser, ai.RoleAssistant, ai.RoleToolResult:
			out = append(out, m)
		}
	}
	return out
}
