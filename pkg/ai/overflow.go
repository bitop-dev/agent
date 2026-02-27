// Package ai — context overflow detection.
//
// IsContextOverflow checks whether an assistant message represents a context-window
// overflow. Three detection strategies are used in order:
//
//  1. Error message pattern matching — covers all known provider error formats.
//  2. HTTP status code matching — for providers that return 400/413 with no body.
//  3. Silent overflow — usage.Input exceeds the known context window
//     (for providers like z.ai that accept over-long requests silently).
//
// # Limitations
//
// Strategy 1 relies on string matching against error messages. If a provider
// changes its error format, detection may fail until the pattern list is updated.
// Strategy 3 requires the caller to pass the correct contextWindow value.
package ai

import "regexp"

// overflowPatterns matches error messages returned by every known provider when
// the input exceeds the model's context window.
var overflowPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)prompt is too long`),                      // Anthropic
	regexp.MustCompile(`(?i)input is too long for requested model`),   // Amazon Bedrock
	regexp.MustCompile(`(?i)exceed.*context window`),                  // OpenAI (Completions & Responses)
	regexp.MustCompile(`(?i)input token count.*exceeds the maximum`),  // Google Gemini
	regexp.MustCompile(`(?i)maximum prompt length is \d+`),            // xAI (Grok)
	regexp.MustCompile(`(?i)reduce the length of the messages`),       // Groq
	regexp.MustCompile(`(?i)maximum context length is \d+ tokens`),    // OpenRouter (all backends)
	regexp.MustCompile(`(?i)exceeds the limit of \d+`),                // GitHub Copilot
	regexp.MustCompile(`(?i)exceeds the available context size`),      // llama.cpp
	regexp.MustCompile(`(?i)greater than the context length`),         // LM Studio
	regexp.MustCompile(`(?i)context window exceeds limit`),            // MiniMax
	regexp.MustCompile(`(?i)exceeded model token limit`),              // Kimi For Coding
	regexp.MustCompile(`(?i)context[_ ]length[_ ]exceeded`),           // Generic fallback
	regexp.MustCompile(`(?i)too many tokens`),                         // Generic fallback
	regexp.MustCompile(`(?i)token limit exceeded`),                    // Generic fallback
}

// statusOverflowPattern matches Cerebras and Mistral which return a 400/413 with
// no body for context overflow (distinct from 429 rate limiting).
var statusOverflowPattern = regexp.MustCompile(`(?i)^4(00|13)\s*(status code)?\s*\(no body\)`)

// IsContextOverflow reports whether msg represents a context-window overflow.
//
// Two cases are handled:
//  1. Error-based overflow — most providers return StopReasonError with a
//     recognisable error message. All known patterns are checked.
//  2. Silent overflow — some providers (e.g. z.ai) accept the request and
//     return successfully, but usage.Input exceeds the window. Pass contextWindow > 0
//     to enable this check.
//
// Pass contextWindow = 0 to skip the silent-overflow check.
func IsContextOverflow(msg *AssistantMessage, contextWindow int) bool {
	if msg == nil {
		return false
	}

	// Case 1: error message pattern match.
	if msg.StopReason == StopReasonError && msg.ErrorMessage != "" {
		for _, re := range overflowPatterns {
			if re.MatchString(msg.ErrorMessage) {
				return true
			}
		}
		// Cerebras / Mistral: 400 or 413 with no body.
		if statusOverflowPattern.MatchString(msg.ErrorMessage) {
			return true
		}
	}

	// Case 2: silent overflow — successful response but input > context window.
	if contextWindow > 0 && msg.StopReason == StopReasonStop {
		inputTokens := msg.Usage.Input + msg.Usage.CacheRead
		if inputTokens > contextWindow {
			return true
		}
	}

	return false
}
