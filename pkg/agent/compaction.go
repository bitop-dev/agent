// Package agent — context compaction.
//
// When the estimated context size exceeds (ContextWindow - ReserveTokens),
// compaction summarises the older portion of the conversation with the LLM and
// replaces it with a structured summary message, keeping the most recent
// KeepRecentTokens of conversation intact.
//
// The summarisation prompt produces a structured Markdown document mirroring
// pi-mono's format (Goal / Progress / Key Decisions / Next Steps / Critical Context).
// Subsequent compactions extend the previous summary incrementally.
package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bitop-dev/agent/pkg/ai"
)

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

// CompactionConfig controls when and how compaction runs.
type CompactionConfig struct {
	// Enabled turns auto-compaction on or off. Default: false.
	Enabled bool

	// ContextWindow is the model's maximum context size in tokens.
	// Required for auto-compaction (compaction triggers when the estimated
	// token count exceeds ContextWindow - ReserveTokens).
	ContextWindow int

	// ReserveTokens is the minimum free-token buffer to maintain.
	// Compaction triggers when usage > ContextWindow - ReserveTokens.
	// Default: 16384.
	ReserveTokens int

	// KeepRecentTokens is how many tokens of recent history to preserve
	// after compaction. Default: 20000.
	KeepRecentTokens int
}

func (c CompactionConfig) reserveTokens() int {
	if c.ReserveTokens > 0 {
		return c.ReserveTokens
	}
	return 16384
}

func (c CompactionConfig) keepRecentTokens() int {
	if c.KeepRecentTokens > 0 {
		return c.KeepRecentTokens
	}
	return 20000
}

// ShouldCompact reports whether compaction should be triggered given the
// current estimated token count and the compaction configuration.
func ShouldCompact(contextTokens int, cfg CompactionConfig) bool {
	if !cfg.Enabled || cfg.ContextWindow <= 0 {
		return false
	}
	return contextTokens > cfg.ContextWindow-cfg.reserveTokens()
}

// ---------------------------------------------------------------------------
// Cut-point detection
// ---------------------------------------------------------------------------

// FindCutPoint returns the index of the first message to keep after compaction,
// targeting the most recent keepRecentTokens of conversation.
//
// Rules:
//   - Never cut between an AssistantMessage (with tool calls) and its
//     immediately following ToolResultMessages.
//   - Only cut at UserMessage boundaries (the kept portion always starts
//     with a user message).
//   - At minimum, keep the last user+assistant exchange.
//
// Returns -1 if compaction cannot sensibly cut anywhere (conversation too short).
func FindCutPoint(msgs []ai.Message, keepRecentTokens int) int {
	if len(msgs) < 4 { // need at least 2 exchanges to compact
		return -1
	}

	// Walk backward from the end, accumulating token estimates.
	accumulated := 0
	cutIdx := -1

	for i := len(msgs) - 1; i >= 0; i-- {
		accumulated += estimateTokens(msgs[i])
		if accumulated >= keepRecentTokens {
			// Find the next user message at or after i.
			for j := i; j < len(msgs); j++ {
				if _, ok := msgs[j].(ai.UserMessage); ok {
					// Make sure this isn't the very first message (must leave something to summarise).
					if j > 0 {
						cutIdx = j
					}
					break
				}
			}
			break
		}
	}

	return cutIdx
}

// ---------------------------------------------------------------------------
// Summary generation
// ---------------------------------------------------------------------------

const summarisationSystemPrompt = `You are an expert at summarising technical conversations.
Create concise, structured summaries that allow another AI to continue the work seamlessly.
Focus on facts, decisions, and current state — not the conversational flow.`

const summarisationPrompt = `The messages above are a conversation to summarise. Create a structured context checkpoint that another LLM will use to continue the work.

Use this EXACT format:

## Goal
[What is the user trying to accomplish? Can be multiple items.]

## Constraints & Preferences
- [Any constraints, preferences, or requirements mentioned by the user]
- [Or "(none)" if none were mentioned]

## Progress
### Done
- [x] [Completed tasks/changes]

### In Progress
- [ ] [Current work]

### Blocked
- [Issues preventing progress, or "(none)"]

## Key Decisions
- **[Decision]**: [Brief rationale]

## Next Steps
1. [Ordered list of what should happen next]

## Critical Context
- [Exact file paths, function names, error messages, data needed to continue]
- [Or "(none)" if not applicable]

Keep each section concise. Preserve exact identifiers, file paths, and error messages.`

const updateSummarisationPrompt = `The messages above are NEW conversation messages to incorporate into the existing summary provided in <previous-summary> tags.

Update the existing structured summary with new information:
- PRESERVE all existing information unless it is now incorrect
- ADD new progress, decisions, and context from the new messages
- UPDATE Progress: move In Progress items to Done when completed
- UPDATE Next Steps based on what was accomplished

<previous-summary>
%s
</previous-summary>

Use the same EXACT format as the previous summary (Goal / Constraints / Progress / Key Decisions / Next Steps / Critical Context).
Keep each section concise. Preserve exact identifiers, file paths, and error messages.`

// GenerateSummary calls the provider to summarise messages into a structured
// Markdown document. If prevSummary is non-empty, it is incorporated
// incrementally (only the new messages need to be described).
func GenerateSummary(
	ctx context.Context,
	provider ai.Provider,
	model string,
	opts ai.StreamOptions,
	msgs []ai.Message,
	prevSummary string,
) (string, error) {
	// Serialize the conversation to plain text for the LLM to read.
	conversationText := serializeConversation(msgs)

	var promptText string
	if prevSummary != "" {
		promptText = fmt.Sprintf("<conversation>\n%s\n</conversation>\n\n%s",
			conversationText,
			fmt.Sprintf(updateSummarisationPrompt, prevSummary),
		)
	} else {
		promptText = fmt.Sprintf("<conversation>\n%s\n</conversation>\n\n%s",
			conversationText,
			summarisationPrompt,
		)
	}

	llmCtx := ai.Context{
		SystemPrompt: summarisationSystemPrompt,
		Messages: []ai.Message{
			ai.UserMessage{
				Role:      ai.RoleUser,
				Content:   []ai.ContentBlock{ai.TextContent{Type: "text", Text: promptText}},
				Timestamp: time.Now().UnixMilli(),
			},
		},
	}

	// Use the same model/provider but higher token budget for the summary.
	summaryOpts := opts
	summaryOpts.MaxTokens = 4096
	summaryOpts.ThinkingLevel = "" // no thinking needed for summarisation

	_, wait := provider.Stream(ctx, model, llmCtx, summaryOpts)
	result, err := wait()
	if err != nil {
		return "", fmt.Errorf("compaction: summarisation failed: %w", err)
	}
	if result.StopReason == ai.StopReasonError {
		return "", fmt.Errorf("compaction: summarisation error: %s", result.ErrorMessage)
	}

	var sb strings.Builder
	for _, b := range result.Content {
		if tc, ok := b.(ai.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String(), nil
}

// GenerateBranchSummary summarises the messages that were discarded when a
// session was forked at a given point, so the child session has context about
// what was explored in the parent branch.
func GenerateBranchSummary(
	ctx context.Context,
	provider ai.Provider,
	model string,
	opts ai.StreamOptions,
	discardedMsgs []ai.Message,
) (string, error) {
	if len(discardedMsgs) == 0 {
		return "", nil
	}

	text := serializeConversation(discardedMsgs)
	prompt := fmt.Sprintf(
		"<discarded-branch>\n%s\n</discarded-branch>\n\n"+
			"The conversation above is a branch that was forked away from. "+
			"Write a one-paragraph summary (max 200 words) of what was tried in that branch, "+
			"what worked, what didn't, and why the branch was abandoned. "+
			"This will be shown as context in the new branch.",
		text,
	)

	llmCtx := ai.Context{
		SystemPrompt: "You summarise discarded conversation branches concisely.",
		Messages: []ai.Message{
			ai.UserMessage{
				Role:      ai.RoleUser,
				Content:   []ai.ContentBlock{ai.TextContent{Type: "text", Text: prompt}},
				Timestamp: 0,
			},
		},
	}

	summaryOpts := opts
	summaryOpts.MaxTokens = 512
	summaryOpts.ThinkingLevel = ""

	_, wait := provider.Stream(ctx, model, llmCtx, summaryOpts)
	result, err := wait()
	if err != nil {
		return "", fmt.Errorf("branch summary: %w", err)
	}

	var sb strings.Builder
	for _, b := range result.Content {
		if tc, ok := b.(ai.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String(), nil
}

// serializeConversation converts a message slice to a human-readable text
// block for feeding to the summarisation LLM.
func serializeConversation(msgs []ai.Message) string {
	var sb strings.Builder
	for _, m := range msgs {
		switch msg := m.(type) {
		case ai.UserMessage:
			sb.WriteString("[USER]\n")
			for _, b := range msg.Content {
				if tc, ok := b.(ai.TextContent); ok {
					sb.WriteString(tc.Text)
					sb.WriteByte('\n')
				}
			}
			sb.WriteByte('\n')
		case ai.AssistantMessage:
			sb.WriteString("[ASSISTANT]\n")
			for _, b := range msg.Content {
				switch bc := b.(type) {
				case ai.TextContent:
					sb.WriteString(bc.Text)
					sb.WriteByte('\n')
				case ai.ThinkingContent:
					sb.WriteString("<thinking>\n")
					sb.WriteString(bc.Thinking)
					sb.WriteString("\n</thinking>\n")
				case ai.ToolCall:
					fmt.Fprintf(&sb, "[TOOL CALL: %s]\n", bc.Name)
				}
			}
			sb.WriteByte('\n')
		case ai.ToolResultMessage:
			fmt.Fprintf(&sb, "[TOOL RESULT: %s]\n", msg.ToolName)
			for _, b := range msg.Content {
				if tc, ok := b.(ai.TextContent); ok {
					// Truncate very long tool outputs in the summary input.
					text := tc.Text
					if len(text) > 2000 {
						text = text[:1997] + "..."
					}
					sb.WriteString(text)
					sb.WriteByte('\n')
				}
			}
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// ---------------------------------------------------------------------------
// Full compaction pipeline (called by the agent loop)
// ---------------------------------------------------------------------------

// compactionResult holds the output of a compaction run.
type compactionResult struct {
	// newMessages is the full message slice after compaction:
	// [summary user msg, ...kept messages...]
	newMessages []ai.Message

	// summarisedMessages are the messages that were replaced.
	summarisedMessages []ai.Message

	// keptMessages are the messages that were preserved.
	keptMessages []ai.Message

	// summary is the generated summary text.
	summary string

	// tokensBefore is the estimated token count before compaction.
	tokensBefore int
}

// runCompaction performs the full compaction pipeline on msgs.
// It finds the cut point, generates a summary, and returns the updated message list.
func runCompaction(
	ctx context.Context,
	provider ai.Provider,
	model string,
	opts ai.StreamOptions,
	msgs []ai.Message,
	cfg CompactionConfig,
	prevSummary string,
) (*compactionResult, error) {
	usage := EstimateContextTokens(msgs)

	cutIdx := FindCutPoint(msgs, cfg.keepRecentTokens())
	if cutIdx <= 0 {
		return nil, nil // nothing to compact
	}

	toSummarise := msgs[:cutIdx]
	toKeep := msgs[cutIdx:]

	summary, err := GenerateSummary(ctx, provider, model, opts, toSummarise, prevSummary)
	if err != nil {
		return nil, err
	}

	summaryText := fmt.Sprintf(
		"The conversation history before this point was compacted into the following summary:\n\n<summary>\n%s\n</summary>",
		summary,
	)
	summaryMsg := ai.UserMessage{
		Role:      ai.RoleUser,
		Content:   []ai.ContentBlock{ai.TextContent{Type: "text", Text: summaryText}},
		Timestamp: time.Now().UnixMilli(),
	}

	newMessages := make([]ai.Message, 0, 1+len(toKeep))
	newMessages = append(newMessages, summaryMsg)
	newMessages = append(newMessages, toKeep...)

	return &compactionResult{
		newMessages:        newMessages,
		summarisedMessages: toSummarise,
		keptMessages:       toKeep,
		summary:            summary,
		tokensBefore:       usage.Tokens,
	}, nil
}
