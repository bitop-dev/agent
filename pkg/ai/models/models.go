// Package models provides a registry of well-known LLM model metadata.
//
// Usage:
//
//	info := models.Lookup("claude-sonnet-4-5-20251219")
//	if info != nil {
//	    fmt.Println(info.ContextWindow)  // 200000
//	}
package models

import "strings"

// ---------------------------------------------------------------------------
// ModelInfo
// ---------------------------------------------------------------------------

// ModelInfo holds static metadata for a known model.
type ModelInfo struct {
	// ID is the canonical model identifier string.
	ID string

	// Provider is the canonical provider name ("anthropic", "openai", "google", …).
	Provider string

	// DisplayName is a short human-readable name.
	DisplayName string

	// ContextWindow is the maximum number of input tokens (prompt + history).
	ContextWindow int

	// MaxOutputTokens is the maximum tokens the model will generate in one response.
	MaxOutputTokens int

	// SupportsVision is true when the model accepts image inputs.
	SupportsVision bool

	// SupportsThinking is true when the model has an extended-reasoning mode.
	SupportsThinking bool

	// InputCostPer1M is the cost in USD per 1 million input tokens.
	InputCostPer1M float64

	// OutputCostPer1M is the cost in USD per 1 million output tokens.
	OutputCostPer1M float64

	// CacheReadCostPer1M is the cost in USD per 1 million cache-read tokens.
	CacheReadCostPer1M float64

	// CacheWriteCostPer1M is the cost in USD per 1 million cache-write tokens.
	CacheWriteCostPer1M float64
}

// ---------------------------------------------------------------------------
// Registry
// ---------------------------------------------------------------------------

// registry holds all known models, indexed by their canonical ID.
var registry = buildRegistry()

// Lookup returns the ModelInfo for id (exact match first, then prefix/suffix
// match). Returns nil if the model is unknown.
func Lookup(id string) *ModelInfo {
	if m, ok := registry[id]; ok {
		return m
	}
	// Fuzzy: check if any registered key is a prefix of id or vice-versa.
	// This handles versioned IDs like "claude-sonnet-4-5-20251219" matching
	// a key registered as "claude-sonnet-4-5".
	id = strings.ToLower(id)
	for k, m := range registry {
		kl := strings.ToLower(k)
		if strings.HasPrefix(id, kl) || strings.HasPrefix(kl, id) {
			return m
		}
	}
	return nil
}

// ContextWindowFor returns the context window for id, or 0 if unknown.
func ContextWindowFor(id string) int {
	if m := Lookup(id); m != nil {
		return m.ContextWindow
	}
	return 0
}

// MaxOutputFor returns the max output tokens for id, or 0 if unknown.
func MaxOutputFor(id string) int {
	if m := Lookup(id); m != nil {
		return m.MaxOutputTokens
	}
	return 0
}

// All returns every registered ModelInfo, unsorted.
func All() []*ModelInfo {
	out := make([]*ModelInfo, 0, len(registry))
	for _, m := range registry {
		out = append(out, m)
	}
	return out
}

// ---------------------------------------------------------------------------
// Registry builder
// ---------------------------------------------------------------------------

func reg(m ModelInfo) *ModelInfo { return &m }

func buildRegistry() map[string]*ModelInfo {
	ms := []*ModelInfo{
		// ── Anthropic ──────────────────────────────────────────────────────
		reg(ModelInfo{
			ID: "claude-opus-4-5", Provider: "anthropic", DisplayName: "Claude Opus 4.5",
			ContextWindow: 200000, MaxOutputTokens: 32000,
			SupportsVision: true, SupportsThinking: true,
			InputCostPer1M: 15, OutputCostPer1M: 75,
			CacheReadCostPer1M: 1.5, CacheWriteCostPer1M: 18.75,
		}),
		reg(ModelInfo{
			ID: "claude-opus-4", Provider: "anthropic", DisplayName: "Claude Opus 4",
			ContextWindow: 200000, MaxOutputTokens: 32000,
			SupportsVision: true, SupportsThinking: true,
			InputCostPer1M: 15, OutputCostPer1M: 75,
			CacheReadCostPer1M: 1.5, CacheWriteCostPer1M: 18.75,
		}),
		reg(ModelInfo{
			ID: "claude-sonnet-4-5", Provider: "anthropic", DisplayName: "Claude Sonnet 4.5",
			ContextWindow: 200000, MaxOutputTokens: 64000,
			SupportsVision: true, SupportsThinking: true,
			InputCostPer1M: 3, OutputCostPer1M: 15,
			CacheReadCostPer1M: 0.3, CacheWriteCostPer1M: 3.75,
		}),
		reg(ModelInfo{
			ID: "claude-3-7-sonnet-20250219", Provider: "anthropic", DisplayName: "Claude 3.7 Sonnet",
			ContextWindow: 200000, MaxOutputTokens: 64000,
			SupportsVision: true, SupportsThinking: true,
			InputCostPer1M: 3, OutputCostPer1M: 15,
			CacheReadCostPer1M: 0.3, CacheWriteCostPer1M: 3.75,
		}),
		reg(ModelInfo{
			ID: "claude-3-5-sonnet-20241022", Provider: "anthropic", DisplayName: "Claude 3.5 Sonnet",
			ContextWindow: 200000, MaxOutputTokens: 8192,
			SupportsVision: true, SupportsThinking: false,
			InputCostPer1M: 3, OutputCostPer1M: 15,
			CacheReadCostPer1M: 0.3, CacheWriteCostPer1M: 3.75,
		}),
		reg(ModelInfo{
			ID: "claude-3-5-haiku-20241022", Provider: "anthropic", DisplayName: "Claude 3.5 Haiku",
			ContextWindow: 200000, MaxOutputTokens: 8192,
			SupportsVision: true, SupportsThinking: false,
			InputCostPer1M: 0.8, OutputCostPer1M: 4,
			CacheReadCostPer1M: 0.08, CacheWriteCostPer1M: 1,
		}),
		reg(ModelInfo{
			ID: "claude-haiku-4-5", Provider: "anthropic", DisplayName: "Claude Haiku 4.5",
			ContextWindow: 200000, MaxOutputTokens: 16000,
			SupportsVision: true, SupportsThinking: false,
			InputCostPer1M: 0.8, OutputCostPer1M: 4,
			CacheReadCostPer1M: 0.08, CacheWriteCostPer1M: 1,
		}),
		reg(ModelInfo{
			ID: "claude-3-opus-20240229", Provider: "anthropic", DisplayName: "Claude 3 Opus",
			ContextWindow: 200000, MaxOutputTokens: 4096,
			SupportsVision: true, SupportsThinking: false,
			InputCostPer1M: 15, OutputCostPer1M: 75,
			CacheReadCostPer1M: 1.5, CacheWriteCostPer1M: 18.75,
		}),

		// ── OpenAI ────────────────────────────────────────────────────────
		reg(ModelInfo{
			ID: "gpt-4o", Provider: "openai", DisplayName: "GPT-4o",
			ContextWindow: 128000, MaxOutputTokens: 16384,
			SupportsVision: true, SupportsThinking: false,
			InputCostPer1M: 2.5, OutputCostPer1M: 10,
			CacheReadCostPer1M: 1.25,
		}),
		reg(ModelInfo{
			ID: "gpt-4o-mini", Provider: "openai", DisplayName: "GPT-4o Mini",
			ContextWindow: 128000, MaxOutputTokens: 16384,
			SupportsVision: true, SupportsThinking: false,
			InputCostPer1M: 0.15, OutputCostPer1M: 0.6,
			CacheReadCostPer1M: 0.075,
		}),
		reg(ModelInfo{
			ID: "o1", Provider: "openai", DisplayName: "o1",
			ContextWindow: 200000, MaxOutputTokens: 100000,
			SupportsVision: true, SupportsThinking: true,
			InputCostPer1M: 15, OutputCostPer1M: 60,
			CacheReadCostPer1M: 7.5,
		}),
		reg(ModelInfo{
			ID: "o1-mini", Provider: "openai", DisplayName: "o1-mini",
			ContextWindow: 128000, MaxOutputTokens: 65536,
			SupportsVision: false, SupportsThinking: true,
			InputCostPer1M: 1.1, OutputCostPer1M: 4.4,
			CacheReadCostPer1M: 0.55,
		}),
		reg(ModelInfo{
			ID: "o3", Provider: "openai", DisplayName: "o3",
			ContextWindow: 200000, MaxOutputTokens: 100000,
			SupportsVision: true, SupportsThinking: true,
			InputCostPer1M: 10, OutputCostPer1M: 40,
			CacheReadCostPer1M: 2.5,
		}),
		reg(ModelInfo{
			ID: "o3-mini", Provider: "openai", DisplayName: "o3-mini",
			ContextWindow: 200000, MaxOutputTokens: 100000,
			SupportsVision: false, SupportsThinking: true,
			InputCostPer1M: 1.1, OutputCostPer1M: 4.4,
			CacheReadCostPer1M: 0.55,
		}),
		reg(ModelInfo{
			ID: "o4-mini", Provider: "openai", DisplayName: "o4-mini",
			ContextWindow: 200000, MaxOutputTokens: 100000,
			SupportsVision: true, SupportsThinking: true,
			InputCostPer1M: 1.1, OutputCostPer1M: 4.4,
			CacheReadCostPer1M: 0.275,
		}),
		reg(ModelInfo{
			ID: "gpt-4-turbo", Provider: "openai", DisplayName: "GPT-4 Turbo",
			ContextWindow: 128000, MaxOutputTokens: 4096,
			SupportsVision: true, SupportsThinking: false,
			InputCostPer1M: 10, OutputCostPer1M: 30,
		}),

		// ── Google Gemini ─────────────────────────────────────────────────
		reg(ModelInfo{
			ID: "gemini-2.5-pro", Provider: "google", DisplayName: "Gemini 2.5 Pro",
			ContextWindow: 1048576, MaxOutputTokens: 65536,
			SupportsVision: true, SupportsThinking: true,
			InputCostPer1M: 1.25, OutputCostPer1M: 10,
		}),
		reg(ModelInfo{
			ID: "gemini-2.5-flash", Provider: "google", DisplayName: "Gemini 2.5 Flash",
			ContextWindow: 1048576, MaxOutputTokens: 65536,
			SupportsVision: true, SupportsThinking: true,
			InputCostPer1M: 0.15, OutputCostPer1M: 0.6,
		}),
		reg(ModelInfo{
			ID: "gemini-2.0-flash", Provider: "google", DisplayName: "Gemini 2.0 Flash",
			ContextWindow: 1048576, MaxOutputTokens: 8192,
			SupportsVision: true, SupportsThinking: false,
			InputCostPer1M: 0.1, OutputCostPer1M: 0.4,
		}),
		reg(ModelInfo{
			ID: "gemini-2.0-flash-thinking", Provider: "google", DisplayName: "Gemini 2.0 Flash Thinking",
			ContextWindow: 1048576, MaxOutputTokens: 65536,
			SupportsVision: true, SupportsThinking: true,
			InputCostPer1M: 0.1, OutputCostPer1M: 0.4,
		}),
		reg(ModelInfo{
			ID: "gemini-1.5-pro", Provider: "google", DisplayName: "Gemini 1.5 Pro",
			ContextWindow: 2097152, MaxOutputTokens: 8192,
			SupportsVision: true, SupportsThinking: false,
			InputCostPer1M: 1.25, OutputCostPer1M: 5,
		}),
		reg(ModelInfo{
			ID: "gemini-1.5-flash", Provider: "google", DisplayName: "Gemini 1.5 Flash",
			ContextWindow: 1048576, MaxOutputTokens: 8192,
			SupportsVision: true, SupportsThinking: false,
			InputCostPer1M: 0.075, OutputCostPer1M: 0.3,
		}),

		// ── Groq (OpenAI-compatible, fast inference) ──────────────────────
		reg(ModelInfo{
			ID: "llama-3.3-70b-versatile", Provider: "groq", DisplayName: "Llama 3.3 70B",
			ContextWindow: 128000, MaxOutputTokens: 32768,
			SupportsVision: false, SupportsThinking: false,
			InputCostPer1M: 0.59, OutputCostPer1M: 0.79,
		}),
		reg(ModelInfo{
			ID: "llama-3.1-8b-instant", Provider: "groq", DisplayName: "Llama 3.1 8B",
			ContextWindow: 128000, MaxOutputTokens: 8000,
			SupportsVision: false, SupportsThinking: false,
			InputCostPer1M: 0.05, OutputCostPer1M: 0.08,
		}),
		reg(ModelInfo{
			ID: "deepseek-r1-distill-llama-70b", Provider: "groq", DisplayName: "DeepSeek R1 Distill 70B",
			ContextWindow: 128000, MaxOutputTokens: 16000,
			SupportsVision: false, SupportsThinking: true,
			InputCostPer1M: 0.75, OutputCostPer1M: 0.99,
		}),

		// ── xAI Grok ─────────────────────────────────────────────────────
		reg(ModelInfo{
			ID: "grok-3", Provider: "xai", DisplayName: "Grok 3",
			ContextWindow: 131072, MaxOutputTokens: 131072,
			SupportsVision: false, SupportsThinking: false,
			InputCostPer1M: 3, OutputCostPer1M: 15,
		}),
		reg(ModelInfo{
			ID: "grok-3-mini", Provider: "xai", DisplayName: "Grok 3 Mini",
			ContextWindow: 131072, MaxOutputTokens: 131072,
			SupportsVision: false, SupportsThinking: true,
			InputCostPer1M: 0.3, OutputCostPer1M: 0.5,
		}),

		// ── Mistral ───────────────────────────────────────────────────────
		reg(ModelInfo{
			ID: "mistral-large-latest", Provider: "mistral", DisplayName: "Mistral Large",
			ContextWindow: 131072, MaxOutputTokens: 4096,
			SupportsVision: true, SupportsThinking: false,
			InputCostPer1M: 2, OutputCostPer1M: 6,
		}),
		reg(ModelInfo{
			ID: "mistral-small-latest", Provider: "mistral", DisplayName: "Mistral Small",
			ContextWindow: 32768, MaxOutputTokens: 4096,
			SupportsVision: true, SupportsThinking: false,
			InputCostPer1M: 0.1, OutputCostPer1M: 0.3,
		}),

		// ── Bedrock (Claude on AWS) ───────────────────────────────────────
		reg(ModelInfo{
			ID: "us.anthropic.claude-3-7-sonnet-20250219-v1:0", Provider: "bedrock", DisplayName: "Claude 3.7 Sonnet (Bedrock)",
			ContextWindow: 200000, MaxOutputTokens: 64000,
			SupportsVision: true, SupportsThinking: true,
		}),
		reg(ModelInfo{
			ID: "us.anthropic.claude-3-5-sonnet-20241022-v2:0", Provider: "bedrock", DisplayName: "Claude 3.5 Sonnet (Bedrock)",
			ContextWindow: 200000, MaxOutputTokens: 8192,
			SupportsVision: true, SupportsThinking: false,
		}),
	}

	out := make(map[string]*ModelInfo, len(ms))
	for _, m := range ms {
		out[m.ID] = m
	}
	return out
}
