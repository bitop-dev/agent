package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/nickcecere/agent/pkg/ai"
	"github.com/nickcecere/agent/pkg/tools"
)

// runLoop is the core agentic loop. It:
//  1. Sends the current conversation to the LLM (streaming).
//  2. Executes any tool calls returned by the LLM.
//  3. Checks for steering messages (user interruption) after each tool.
//  4. Repeats until no tool calls and no follow-up messages.
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
			turnCount++

			// Inject steering / follow-up messages
			for _, m := range pendingMessages {
				a.appendMsg(m)
				emit(Event{Type: EventMessageStart, Message: m})
				emit(Event{Type: EventMessageEnd, Message: m})
			}
			pendingMessages = nil

			// Compact context if needed (before next LLM call).
			if err := a.maybeCompact(ctx); err != nil {
				// Non-fatal: continue without compaction.
				_ = err
			}

			// Stream assistant response
			assistantMsg, err := a.streamResponse(ctx, cfg, emit)
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
			if hasToolCalls {
				results, steering, err := a.executeToolCalls(ctx, toolCalls, cfg, emit)
				if err != nil {
					return err
				}
				toolResults = results
				steeringAfterTools = steering
				for _, r := range toolResults {
					a.appendMsg(r)
				}
			}

			usage := EstimateContextTokens(a.snapshotMessages())
			emit(Event{Type: EventTurnEnd, Message: assistantMsg, ToolResults: toolResults, ContextUsage: usage})

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

// executeToolCalls runs tool calls in sequence, checking for steering after each.
func (a *Agent) executeToolCalls(
	ctx context.Context,
	toolCalls []ai.ToolCall,
	cfg Config,
	emit func(Event),
) ([]ai.ToolResultMessage, []ai.Message, error) {
	var results []ai.ToolResultMessage
	var steeringMessages []ai.Message

	for i, tc := range toolCalls {
		emit(Event{
			Type:       EventToolStart,
			ToolCallID: tc.ID,
			ToolName:   tc.Name,
			ToolArgs:   tc.Arguments,
		})

		result, isError := a.executeSingleTool(ctx, tc, emit)

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

	return results, steeringMessages, nil
}

// executeSingleTool looks up and runs one tool call.
func (a *Agent) executeSingleTool(
	ctx context.Context,
	tc ai.ToolCall,
	emit func(Event),
) (tools.Result, bool) {
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

	result, err := tool.Execute(ctx, tc.ID, params, onUpdate)
	if err != nil {
		return tools.ErrorResult(err), true
	}
	return result, false
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


