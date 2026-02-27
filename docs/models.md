# Model Registry

The agent ships a built-in registry of well-known models from every supported
provider. The registry is used to:

1. Auto-populate the `context_window` for compaction (no manual config needed)
2. Switch models at runtime with `/model <id>`
3. Display model metadata in the REPL

---

## Switching Models

In the REPL:

```
/model gpt-4o
/model claude-opus-4-5
/model gemini-2.5-pro
```

The new model is used for all subsequent turns in the session. The provider
must already be configured in `agent.yaml`.

---

## Registered Models

### Anthropic

| ID | Context | Vision | Thinking |
|----|---------|--------|---------|
| `claude-opus-4-5` | 200k | ✓ | ✓ |
| `claude-sonnet-4-5` | 200k | ✓ | ✓ |
| `claude-haiku-4-5` | 200k | ✓ | ✗ |
| `claude-3-7-sonnet-20250219` | 200k | ✓ | ✓ |
| `claude-3-5-sonnet-20241022` | 200k | ✓ | ✓ |
| `claude-3-5-haiku-20241022` | 200k | ✓ | ✗ |
| `claude-3-opus-20240229` | 200k | ✓ | ✗ |

### OpenAI

| ID | Context | Thinking |
|----|---------|---------|
| `gpt-4o` | 128k | ✗ |
| `gpt-4o-mini` | 128k | ✗ |
| `o1` | 200k | ✓ |
| `o1-mini` | 128k | ✓ |
| `o3` | 200k | ✓ |
| `o3-mini` | 200k | ✓ |
| `o4-mini` | 200k | ✓ |

### Google

| ID | Context | Vision | Thinking |
|----|---------|--------|---------|
| `gemini-2.5-pro` | 1M | ✓ | ✓ |
| `gemini-2.5-flash` | 1M | ✓ | ✓ |
| `gemini-2.0-flash` | 1M | ✓ | ✗ |
| `gemini-1.5-pro` | 2M | ✓ | ✗ |
| `gemini-1.5-flash` | 1M | ✓ | ✗ |
| `gemini-1.0-pro` | 32k | ✗ | ✗ |

### Groq

| ID | Context |
|----|---------|
| `llama-3.3-70b-versatile` | 128k |
| `llama-3.1-8b-instant` | 128k |
| `mixtral-8x7b-32768` | 32k |
| `gemma2-9b-it` | 8k |

### xAI

| ID | Context | Vision |
|----|---------|--------|
| `grok-3` | 131k | ✓ |
| `grok-3-mini` | 131k | ✗ |
| `grok-2` | 131k | ✓ |

### Mistral

| ID | Context |
|----|---------|
| `mistral-large-latest` | 128k |
| `mistral-small-latest` | 32k |
| `codestral-latest` | 32k |
| `mixtral-8x22b-instruct` | 64k |

### Amazon Bedrock

| ID | Context |
|----|---------|
| `us.anthropic.claude-opus-4-20250514-v1:0` | 200k |
| `us.anthropic.claude-sonnet-4-20250514-v1:0` | 200k |
| `us.anthropic.claude-3-7-sonnet-20250219-v1:0` | 200k |
| `us.meta.llama3-3-70b-instruct-v1:0` | 128k |

---

## Fuzzy Matching

The registry uses prefix matching, so versioned model IDs work automatically:

```
claude-sonnet-4-5-20251219  →  matches  claude-sonnet-4-5
gpt-4o-2024-11-01           →  matches  gpt-4o
```

This means you can use any versioned ID in your config and the registry will
find the right metadata.

---

## Using the Registry in Go

```go
import "github.com/nickcecere/agent/pkg/ai/models"

// Lookup by ID (fuzzy)
info := models.Lookup("claude-sonnet-4-5-20251219")
if info != nil {
    fmt.Println(info.ContextWindow)   // 200000
    fmt.Println(info.SupportsThinking) // true
    fmt.Println(info.InputCostPer1M)  // 3.0 (USD/M tokens)
}

// Get context window directly
window := models.ContextWindowFor("gpt-4o") // 128000

// List all known models
for _, m := range models.All() {
    fmt.Printf("%s (%s): %d context\n", m.ID, m.Provider, m.ContextWindow)
}
```

### ModelInfo fields

```go
type ModelInfo struct {
    ID                  string
    Provider            string
    DisplayName         string
    ContextWindow       int
    MaxOutputTokens     int
    SupportsVision      bool
    SupportsThinking    bool
    InputCostPer1M      float64   // USD per 1M input tokens
    OutputCostPer1M     float64
    CacheReadCostPer1M  float64
    CacheWriteCostPer1M float64
}
```

---

## Unknown Models

If a model is not in the registry (e.g. a newly released version or a custom
deployment), the agent still works — it just can't auto-populate
`context_window`. Set it manually in your config:

```yaml
model: my-custom-model-v99
context_window: 100000     # manual override

compaction:
  enabled: true
  context_window: 100000   # or set here; same resolution order
```
