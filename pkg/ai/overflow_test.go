package ai

import "testing"

func TestIsContextOverflow_ErrorPatterns(t *testing.T) {
	cases := []struct {
		name     string
		errMsg   string
		want     bool
	}{
		{"anthropic", "prompt is too long: 213462 tokens > 200000 maximum", true},
		{"bedrock", "input is too long for requested model", true},
		{"openai-completions", "This request's messages exceed the model's context window.", true},
		{"openai-responses", "exceeds the context window of this model", true},
		{"google", "The input token count (1196265) exceeds the maximum number of tokens allowed (1048575)", true},
		{"xai", "This model's maximum prompt length is 131072 but the request contains 537812 tokens", true},
		{"groq", "Please reduce the length of the messages or completion", true},
		{"openrouter", "This endpoint's maximum context length is 8192 tokens. However, you requested 9000 tokens", true},
		{"github-copilot", "prompt token count of 30000 exceeds the limit of 28000", true},
		{"llama.cpp", "the request exceeds the available context size, try increasing it", true},
		{"lm-studio", "tokens to keep from the initial prompt is greater than the context length", true},
		{"minimax", "invalid params, context window exceeds limit", true},
		{"kimi", "Your request exceeded model token limit: 8192 (requested: 9000)", true},
		{"cerebras-413", "413 status code (no body)", true},
		{"cerebras-400", "400 (no body)", true},
		{"rate-limit-not-overflow", "429 Too Many Requests", false},
		{"generic-error", "internal server error", false},
		{"empty", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg := &AssistantMessage{
				StopReason:   StopReasonError,
				ErrorMessage: tc.errMsg,
			}
			if got := IsContextOverflow(msg, 0); got != tc.want {
				t.Errorf("IsContextOverflow(%q) = %v, want %v", tc.errMsg, got, tc.want)
			}
		})
	}
}

func TestIsContextOverflow_SilentOverflow(t *testing.T) {
	msg := &AssistantMessage{
		StopReason: StopReasonStop,
		Usage: Usage{
			Input:     50000,
			CacheRead: 5000, // total input = 55000
		},
	}

	// Below limit — not overflow.
	if IsContextOverflow(msg, 100000) {
		t.Error("expected false for input (55000) < contextWindow (100000)")
	}

	// Above limit — overflow.
	if !IsContextOverflow(msg, 40000) {
		t.Error("expected true for input (55000) > contextWindow (40000)")
	}

	// Zero contextWindow — check disabled.
	if IsContextOverflow(msg, 0) {
		t.Error("expected false when contextWindow=0 (check disabled)")
	}
}

func TestIsContextOverflow_NilMsg(t *testing.T) {
	if IsContextOverflow(nil, 0) {
		t.Error("expected false for nil message")
	}
}
