# Using the Agent as a Go Library

The agent is designed to be embedded in other Go programs. Import the packages
directly — no CLI required.

---

## Module

```bash
go get github.com/bitop-dev/agent
```

---

## Minimal Embedded Agent

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/bitop-dev/agent/pkg/agent"
    "github.com/bitop-dev/agent/pkg/ai"
    "github.com/bitop-dev/agent/pkg/ai/providers/anthropic"
    "github.com/bitop-dev/agent/pkg/tools"
    "github.com/bitop-dev/agent/pkg/tools/builtin"
)

func main() {
    provider := anthropic.New(os.Getenv("ANTHROPIC_API_KEY"))

    reg := tools.NewRegistry()
    builtin.Register(reg, builtin.PresetCoding, ".")

    a := agent.New(agent.Options{
        Provider:     provider,
        Model:        "claude-sonnet-4-5",
        Tools:        reg,
        SystemPrompt: "You are a helpful assistant.",
    })

    a.Subscribe(func(e agent.Event) {
        switch e.Type {
        case agent.EventMessageUpdate:
            if se := e.StreamEvent; se != nil && se.Type == ai.StreamEventTextDelta {
                fmt.Print(se.Delta)
            }
        case agent.EventToolStart:
            fmt.Printf("\n[tool] %s\n", e.ToolName)
        case agent.EventTurnEnd:
            fmt.Printf("\n[tokens: %d | cost: $%.4f]\n",
                e.ContextUsage.Tokens, e.CostUsage.TotalCost)
        }
    })

    if err := agent.Prompt(context.Background(), "List .go files here.", agent.Config{
        StreamOptions: ai.StreamOptions{MaxTokens: 1024},
        MaxTurns:      10,
    }); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
```

---

## Agent API

### `agent.New`

```go
func New(opts Options) *Agent
```

Creates a new Agent. `Options` fields:

```go
type Options struct {
    Provider      ai.Provider       // required
    Model         string            // required
    Tools         *tools.Registry   // nil → empty registry
    SystemPrompt  string
    Session       *session.Session  // optional: persist to JSONL file
    Compaction    CompactionConfig  // optional: auto-compact context
    StreamOptions ai.StreamOptions  // passed to every LLM call
    Logger        *slog.Logger      // nil → silent; slog.Default() for stderr
}
```

### `agent.Agent.Prompt`

```go
func (a *Agent) Prompt(ctx context.Context, text string, cfg Config) error
```

Sends a user message and runs the agent loop to completion. Appends the user
message to history, then calls `PromptMessages`.

### `agent.Agent.PromptMessages`

```go
func (a *Agent) PromptMessages(ctx context.Context, msgs []ai.Message, cfg Config) error
```

Runs the agent loop with the given initial messages. Blocks until the agent
stops (no more tool calls, context cancelled, or turn limit reached). Returns
non-nil only for unrecoverable failures.

### `agent.Agent.Continue`

```go
func (a *Agent) Continue(ctx context.Context, cfg Config) error
```

Re-runs the agent loop without adding a new user message. Useful after
calling `Steer()` or `FollowUp()`.

### `agent.Agent.Steer`

```go
func (a *Agent) Steer(m ai.Message)
func (a *Agent) SteerText(text string)
```

Injects a message into the running loop between tool calls (for steering a
live session). Calls to `SteerText` are equivalent to `Steer` with a plain
text `UserMessage`.

### `agent.Agent.FollowUp`

```go
func (a *Agent) FollowUp(m ai.Message)
func (a *Agent) FollowUpText(text string)
```

Queues a message to be added after the agent would otherwise stop. Causes the
loop to continue with the queued message.

### `agent.Agent.Subscribe`

```go
func (a *Agent) Subscribe(fn func(Event)) func()
```

Registers an event handler. Returns an unsubscribe function. Thread-safe.
Handlers are called synchronously from the agent loop goroutine.

### `agent.Agent.State`

```go
func (a *Agent) State() State
```

Returns a snapshot of the current agent state. Thread-safe.

```go
type State struct {
    SystemPrompt    string
    Model           string
    Provider        string
    Messages        []ai.Message
    IsStreaming     bool
    PendingToolCalls map[string]bool
    Error           string
    ContextTokens   int
    CumulativeCost  CostUsage
}
```

### Other Methods

| Method | Description |
|--------|-------------|
| `SetModel(m string)` | Switch model (takes effect on next turn) |
| `SetProvider(p ai.Provider)` | Switch provider |
| `SetSystemPrompt(s string)` | Update system prompt |
| `AttachSession(s, msgs)` | Attach a session and load its messages |
| `Tools() *tools.Registry` | Access the tool registry |
| `Messages() []ai.Message` | All messages in the current conversation |
| `ClearMessages()` | Reset conversation history |
| `Abort()` | Cancel any running loop |
| `IsStreaming() bool` | Whether the loop is currently running |

---

## Config

```go
type Config struct {
    // ConvertToLLM transforms history before sending to the LLM.
    // Default: keeps only user/assistant/tool_result messages.
    ConvertToLLM func([]ai.Message) ([]ai.Message, error)

    // TransformContext prunes/enriches messages before ConvertToLLM.
    TransformContext func([]ai.Message) ([]ai.Message, error)

    // GetAPIKey returns a (possibly dynamic/expiring) API key.
    GetAPIKey func(provider string) (string, error)

    // GetSteeringMessages injects messages between tool calls.
    // Return nil to continue normally.
    GetSteeringMessages func() ([]ai.Message, error)

    // GetFollowUpMessages adds messages after the loop would stop.
    // Return nil to stop.
    GetFollowUpMessages func() ([]ai.Message, error)

    // ConfirmToolCall gates tool execution before each call.
    //   nil (default) — auto-approve all tools
    //   AutoApproveAll — explicit auto-approve
    //   custom func — interactive or policy-based approval
    ConfirmToolCall func(name string, args map[string]any) (ConfirmResult, error)

    // StreamOptions are passed to the provider.
    StreamOptions ai.StreamOptions

    // MaxTurns caps turns per Prompt call (0 = unlimited).
    MaxTurns int

    // MaxRetries for transient LLM errors (rate limits, 5xx, network).
    // 0 = no retries (default).
    MaxRetries int

    // RetryBaseDelay is the initial backoff. Doubles each attempt, caps at 60s.
    // Zero defaults to 1s.
    RetryBaseDelay time.Duration

    // MaxToolConcurrency runs multiple tool calls in parallel.
    // 0 or 1 = sequential (default). > 1 = parallel with semaphore.
    MaxToolConcurrency int

    // ToolTimeout limits each tool execution. 0 = no timeout.
    ToolTimeout time.Duration

    // MaxCostUSD stops the loop when cumulative cost exceeds this. 0 = no cap.
    MaxCostUSD float64

    // OnMetrics is called after each turn with performance data.
    OnMetrics func(TurnMetrics)
}
```

### Confirmation Hooks

```go
// Allow a tool call
cfg.ConfirmToolCall = agent.AutoApproveAll

// Prompt the user
cfg.ConfirmToolCall = func(name string, args map[string]any) (agent.ConfirmResult, error) {
    fmt.Printf("Allow %s %v? [y/n/q]: ", name, args)
    var reply string
    fmt.Scanln(&reply)
    switch reply {
    case "y":
        return agent.ConfirmAllow, nil
    case "q":
        return agent.ConfirmAbort, nil
    default:
        return agent.ConfirmDeny, nil
    }
}

// Policy-based: deny all bash calls
cfg.ConfirmToolCall = func(name string, args map[string]any) (agent.ConfirmResult, error) {
    if name == "bash" {
        return agent.ConfirmDeny, nil
    }
    return agent.ConfirmAllow, nil
}
```

---

## Events

```go
type Event struct {
    Type EventType

    // message_* events
    Message     ai.Message
    StreamEvent *ai.StreamEvent  // set on message_update

    // turn_end
    ToolResults  []ai.ToolResultMessage
    ContextUsage ContextUsage
    CostUsage    CostUsage        // cumulative cost to date
    TurnDuration time.Duration    // wall-clock time for this turn

    // compaction
    Compaction *CompactionEvent

    // tool_* events
    ToolCallID string
    ToolName   string
    ToolArgs   map[string]any
    ToolResult *tools.Result
    IsError    bool

    // agent_end
    NewMessages []ai.Message

    // retry events
    RetryAttempt int
    RetryError   error
    RetryDelay   time.Duration

    // metrics callback
    Metrics *TurnMetrics
}
```

| Event Type | Description |
|-----------|-------------|
| `agent_start` | Loop started |
| `agent_end` | Loop finished; `NewMessages` has this run's messages |
| `turn_end` | One LLM call + all tool calls finished; `ContextUsage`, `CostUsage` populated |
| `message_start` | Message added |
| `message_update` | Streaming delta; `StreamEvent.Delta` has the text |
| `message_end` | Message finalised |
| `tool_start` | Tool execution starting |
| `tool_update` | Streaming progress from tool |
| `tool_end` | Tool finished; `ToolResult`, `IsError` set |
| `tool_denied` | Tool blocked by `ConfirmToolCall` returning `ConfirmDeny` |
| `compaction` | Context compaction completed |
| `turn_limit_reached` | `MaxTurns` hit; loop stopped |
| `retry` | Retrying after transient error; `RetryAttempt`, `RetryError`, `RetryDelay` set |
| `config_reloaded` | `ConfigReloader` applied a new config file |

---

## TurnMetrics

```go
type TurnMetrics struct {
    TurnNumber       int
    ProviderLatency  time.Duration
    ToolDurations    map[string]time.Duration  // tool name → execution time
    InputTokens      int
    OutputTokens     int
    CacheReadTokens  int
    CacheWriteTokens int
    TotalCost        float64
    Error            string
}
```

```go
cfg.OnMetrics = func(m agent.TurnMetrics) {
    slog.Info("turn",
        "n", m.TurnNumber,
        "latency_ms", m.ProviderLatency.Milliseconds(),
        "in", m.InputTokens,
        "out", m.OutputTokens,
        "cost_usd", m.TotalCost,
    )
}
```

---

## Providers

```go
type Provider interface {
    Name() string
    Stream(ctx context.Context, model string, llmCtx Context, opts StreamOptions) (<-chan StreamEvent, func() (*AssistantMessage, error))
}
```

| Package | Constructor |
|---------|------------|
| `pkg/ai/providers/anthropic` | `anthropic.New(apiKey)` |
| `pkg/ai/providers/openai` | `openai.New(apiKey)` (completions) |
| `pkg/ai/providers/openai` | `responses.New(apiKey)` (responses API) |
| `pkg/ai/providers/google` | `google.New(apiKey)` |
| `pkg/ai/providers/azure` | `azure.New(apiKey, baseURL, apiVersion)` |
| `pkg/ai/providers/bedrock` | `bedrock.New(region, profile)` |
| `pkg/ai/providers/proxy` | `proxy.New(serverURL, token)` |

```go
// OpenAI-compatible endpoint (OpenRouter, Ollama, etc.)
provider := openai.NewWithBaseURL(apiKey, "https://openrouter.ai/api/v1")
```

---

## Custom Provider

```go
type MyProvider struct{}

func (p *MyProvider) Name() string { return "my-provider" }

func (p *MyProvider) Stream(
    ctx context.Context,
    model string,
    llmCtx ai.Context,
    opts ai.StreamOptions,
) (<-chan ai.StreamEvent, func() (*ai.AssistantMessage, error)) {
    ch := make(chan ai.StreamEvent, 64)
    var finalMsg *ai.AssistantMessage
    var finalErr error
    done := make(chan struct{})

    go func() {
        defer close(ch)
        defer close(done)
        finalMsg, finalErr = callMyAPI(ctx, model, llmCtx, opts, ch)
    }()

    return ch, func() (*ai.AssistantMessage, error) {
        <-done
        return finalMsg, finalErr
    }
}
```

---

## Custom Tools

```go
type MyTool struct{}

func (t *MyTool) Definition() ai.ToolDefinition {
    return ai.ToolDefinition{
        Name:        "my_tool",
        Description: "What it does.",
        Parameters: tools.MustSchema(tools.SimpleSchema{
            Properties: map[string]tools.Property{
                "input": {Type: "string", Description: "Input value"},
            },
            Required: []string{"input"},
        }),
    }
}

func (t *MyTool) Execute(
    ctx context.Context,
    callID string,
    params map[string]any,
    onUpdate tools.UpdateFn,
) (tools.Result, error) {
    input, _ := params["input"].(string)
    // Emit progress (optional)
    if onUpdate != nil {
        onUpdate(tools.TextResult("Processing..."))
    }
    return tools.TextResult("Result: " + input), nil
}

reg := tools.NewRegistry()
reg.Register(&MyTool{})
```

---

## Sub-Agent Delegation

Sub-agents let you compose agents — a parent agent calls a child agent as a
tool and uses its final response.

```go
// Standalone sub-agent
sub := agent.NewSubAgent(agent.SubAgentOptions{
    Provider:     provider,
    Model:        "gpt-4o",
    SystemPrompt: "You are a code reviewer. Be concise.",
    Tools:        readonlyReg,
    MaxTurns:     10,
    OnEvent: func(e agent.Event) {
        // forward events to a logger, parent subscriber, etc.
    },
})
result, err := sub.Run(ctx, "Review this diff...")

// As a tool the parent agent can call
reviewTool := agent.NewSubAgentTool(
    "code_review",
    "Reviews a code snippet and returns structured feedback",
    agent.SubAgentOptions{
        Provider:     provider,
        Model:        "gpt-4o",
        SystemPrompt: "You are a strict code reviewer.",
        MaxTurns:     5,
    },
)
reg.Register(reviewTool)
```

`SubAgentTool` forwards streaming deltas from the child agent back to the
parent as `tool_update` events.

---

## Config Hot-Reload

Watch a YAML config file and apply mutable changes to a running agent without
a restart. Mutable fields: `model`, `max_tokens`, `temperature`,
`thinking_level`, `cache_retention`, `context_window`.

```go
reloader := agent.NewConfigReloader("agent.yaml", myAgent, slog.Default())
reloader.OnReload = func(cfg *agent.FileConfig) {
    fmt.Printf("Reloaded: model=%s max_tokens=%d\n", cfg.Model, cfg.MaxTokens)
}
reloader.Start()        // polls every 2 seconds
defer reloader.Stop()

// Or trigger manually (e.g. from a /reload REPL command):
if err := reloader.ReloadOnce(); err != nil {
    fmt.Println("reload failed:", err)
}
```

Emits `EventConfigReloaded` after each successful reload.

---

## Pluggable Bash Executor

```go
type SSHExecutor struct{ host string }

func (e *SSHExecutor) Exec(ctx context.Context, command, cwd string, onData func(string)) (int, error) {
    return runSSH(ctx, e.host, command, cwd, onData)
}

reg.Register(builtin.NewBashToolWithExecutor(".", &SSHExecutor{host: "myserver"}))
```

---

## Multimodal / Image Input

Include `ai.ImageContent` blocks in user messages or tool results:

```go
msg := ai.UserMessage{
    Role: ai.RoleUser,
    Content: []ai.ContentBlock{
        ai.TextContent{Type: "text", Text: "What's in this image?"},
        ai.ImageContent{Type: "image", MIMEType: "image/png", Data: base64Data},
    },
    Timestamp: time.Now().UnixMilli(),
}

err := a.PromptMessages(ctx, []ai.Message{msg}, cfg)
```

All providers serialize `ImageContent` automatically:

| Provider | Wire format |
|----------|-------------|
| Anthropic | `"type":"image"` with base64 source |
| OpenAI | `"type":"image_url"` with `data:` URI |
| Google | `inlineData` with base64 |

---

## Session Persistence

```go
import "github.com/bitop-dev/agent/pkg/session"

dir := filepath.Join(os.UserHomeDir(), ".config", "agent", "sessions")
sess, err := session.Create(dir, cwd)

// Restore existing conversation
msgs, err := sess.Messages()
a.AttachSession(sess, msgs)

// All subsequent messages auto-persisted.
// On exit:
sess.Close()

// Resume later
sess2, err := session.Load(dir, "a3f7c9") // prefix match
msgs2, _ := sess2.Messages()
```

---

## Compaction

```go
a := agent.New(agent.Options{
    // ...
    Compaction: agent.CompactionConfig{
        Enabled:          true,
        ContextWindow:    200000,
        ReserveTokens:    16384,
        KeepRecentTokens: 20000,
    },
})

a.Subscribe(func(e agent.Event) {
    if e.Type == agent.EventCompaction {
        c := e.Compaction
        fmt.Printf("Compacted %d → %d tokens (%d messages removed)\n",
            c.TokensBefore, c.TokensAfter, c.MessagesRemoved)
    }
})
```

---

## Proxy Server

```go
import (
    "net/http"
    "github.com/bitop-dev/agent/pkg/ai/providers/proxy"
    "github.com/bitop-dev/agent/pkg/ai/providers/anthropic"
)

upstream := anthropic.New(os.Getenv("ANTHROPIC_API_KEY"))
handler  := proxy.NewHandler(upstream, "my-secret-token")

http.Handle("/stream", handler)
http.ListenAndServe(":8080", nil)
```

Clients connect via `provider: proxy` with `base_url: http://localhost:8080`.

---

## Structured Logging

```go
import "log/slog"

// Log to stderr
a := agent.New(agent.Options{
    Logger: slog.Default(),
    // ...
})

// Log to a file with JSON format
logFile, _ := os.OpenFile("agent.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
logger := slog.New(slog.NewJSONHandler(logFile, nil))
a := agent.New(agent.Options{
    Logger: logger,
    // ...
})
```

Internal warnings (retry attempts, session write failures, tool panics) all
use the logger. Silent by default.

---

## Complete Example: Production-Grade Agent

```go
package main

import (
    "context"
    "fmt"
    "log/slog"
    "os"
    "time"

    "github.com/bitop-dev/agent/pkg/agent"
    "github.com/bitop-dev/agent/pkg/ai"
    "github.com/bitop-dev/agent/pkg/ai/providers/anthropic"
    "github.com/bitop-dev/agent/pkg/tools"
    "github.com/bitop-dev/agent/pkg/tools/builtin"
)

func main() {
    provider := anthropic.New(os.Getenv("ANTHROPIC_API_KEY"))

    reg := tools.NewRegistry()
    builtin.Register(reg, builtin.PresetCoding, ".")

    a := agent.New(agent.Options{
        Provider:     provider,
        Model:        "claude-sonnet-4-5",
        Tools:        reg,
        SystemPrompt: "You are a coding assistant.",
        Logger:       slog.Default(),
        Compaction: agent.CompactionConfig{
            Enabled:          true,
            ContextWindow:    200000,
            ReserveTokens:    16384,
            KeepRecentTokens: 20000,
        },
    })

    a.Subscribe(func(e agent.Event) {
        switch e.Type {
        case agent.EventMessageUpdate:
            if se := e.StreamEvent; se != nil && se.Type == ai.StreamEventTextDelta {
                fmt.Print(se.Delta)
            }
        case agent.EventToolStart:
            fmt.Printf("\n[%s] running...\n", e.ToolName)
        case agent.EventToolDenied:
            fmt.Printf("\n[%s] denied\n", e.ToolName)
        case agent.EventRetry:
            fmt.Printf("\n[retry %d] %v (waiting %s)\n",
                e.RetryAttempt, e.RetryError, e.RetryDelay.Round(time.Millisecond))
        case agent.EventTurnEnd:
            fmt.Printf("\n[cost: $%.4f total | %d tokens]\n",
                e.CostUsage.TotalCost, e.ContextUsage.Tokens)
        }
    })

    cfg := agent.Config{
        StreamOptions:      ai.StreamOptions{MaxTokens: 4096},
        MaxTurns:           50,
        MaxRetries:         3,
        RetryBaseDelay:     2 * time.Second,
        MaxToolConcurrency: 4,
        MaxCostUSD:         1.00, // stop if > $1 spent
        ConfirmToolCall:    agent.AutoApproveAll,
        OnMetrics: func(m agent.TurnMetrics) {
            slog.Info("turn", "n", m.TurnNumber,
                "latency_ms", m.ProviderLatency.Milliseconds(),
                "cost", m.TotalCost)
        },
    }

    if err := a.Prompt(context.Background(), os.Args[1], cfg); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
    fmt.Println()
}
```
