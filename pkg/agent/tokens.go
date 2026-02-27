// Package agent — context token estimation.
//
// Mirrors pi-mono packages/coding-agent/src/core/compaction/compaction.ts
// (estimateContextTokens, estimateTokens).
package agent

import (
	"encoding/json"

	"github.com/bitop-dev/agent/pkg/ai"
)

// EstimateContextTokens estimates the total token count of the given message
// history using a two-part strategy:
//
//  1. Find the last AssistantMessage that has a non-zero Usage.TotalTokens.
//     That gives us the exact count up to that point.
//  2. For any messages appended after that (tool results, steering, new user
//     message) estimate chars/4 tokens each.
//
// This mirrors pi-mono's estimateContextTokens() and is used to decide when
// to trigger context compaction.
func EstimateContextTokens(msgs []ai.Message) ContextUsage {
	// Find the last assistant message with known usage.
	lastUsageIdx := -1
	var lastUsage ai.Usage
	for i := len(msgs) - 1; i >= 0; i-- {
		if am, ok := msgs[i].(ai.AssistantMessage); ok {
			if am.StopReason != ai.StopReasonError && am.StopReason != ai.StopReasonAborted &&
				(am.Usage.TotalTokens > 0 || am.Usage.Input > 0) {
				lastUsageIdx = i
				lastUsage = am.Usage
				break
			}
		}
	}

	if lastUsageIdx == -1 {
		// No usage data yet — estimate everything.
		total := 0
		for _, m := range msgs {
			total += estimateTokens(m)
		}
		return ContextUsage{Tokens: total, TrailingTokens: total}
	}

	usageTokens := lastUsage.TotalTokens
	if usageTokens == 0 {
		usageTokens = lastUsage.Input + lastUsage.Output + lastUsage.CacheRead + lastUsage.CacheWrite
	}

	trailing := 0
	for _, m := range msgs[lastUsageIdx+1:] {
		trailing += estimateTokens(m)
	}

	return ContextUsage{
		Tokens:         usageTokens + trailing,
		UsageTokens:    usageTokens,
		TrailingTokens: trailing,
	}
}

// estimateTokens estimates the token count of a single message using chars/4.
// This is intentionally conservative (overestimates tokens).
func estimateTokens(m ai.Message) int {
	chars := 0
	switch msg := m.(type) {
	case ai.UserMessage:
		for _, b := range msg.Content {
			switch blk := b.(type) {
			case ai.TextContent:
				chars += len(blk.Text)
			case ai.ImageContent:
				chars += 4 * 1200 // ~1200 tokens per image
			}
		}
	case ai.AssistantMessage:
		for _, b := range msg.Content {
			switch blk := b.(type) {
			case ai.TextContent:
				chars += len(blk.Text)
			case ai.ThinkingContent:
				chars += len(blk.Thinking)
			case ai.ToolCall:
				chars += len(blk.Name)
				if j, err := json.Marshal(blk.Arguments); err == nil {
					chars += len(j)
				}
			}
		}
	case ai.ToolResultMessage:
		for _, b := range msg.Content {
			switch blk := b.(type) {
			case ai.TextContent:
				chars += len(blk.Text)
			case ai.ImageContent:
				chars += 4 * 1200
			}
		}
	}
	if chars == 0 {
		return 0
	}
	t := chars / 4
	if t == 0 {
		t = 1
	}
	return t
}
