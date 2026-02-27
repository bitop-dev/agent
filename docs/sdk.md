# Using the Agent as a Go Library

The agent is designed to be embedded in other Go programs. You don't have to
use the CLI at all — just import the packages directly.

---

## Module

```bash
go get github.com/nickcecere/agent
```

---

## Minimal Embedded Agent

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/nickcecere/agent/pkg/agent"
    "github.com/nickcecere/agent/pkg/ai"
    "github.com/nickcecere/agent/pkg/ai/providers/anthropic"
    "github.com/nickcecere/agent/pkg/tools"
    "github.com/nickcecere/agent/pkg/tools/builtin"
)

func main() {
    // 1. Create a provider
    provider := anthropic.New(os.Getenv("ANTHROPIC_API_KEY"))

    // 2. Create a tool registry
    reg := tools.NewRegistry()
    builtin.Register(reg, builtin.PresetCoding, ".")

    // 3. Create the agent
    a := agent.New(provider, "claude-sonnet-4-5", reg, "You are a helpful assistant.")

    // 4. Subscribe to events
    a.Subscribe(func(e agent.Event) {
        switch e.Type {
        case agent.EventMessageUpdate:
            if se := e.StreamEvent; se != nil && se.Type == ai.StreamEventTextDelta {
                fmt.Print(se.Delta)
            }
        case agent.EventToolStart:
            fmt.Printf("\n[tool] %s\n", e.ToolName)
        case agent.EventTurnEnd:
            fmt.Printf("\n[tokens] ~%d in context\n", e.ContextUsage.Tokens)
        }
    })

    // 5. Run
    ctx := context.Background()
    userMsg := ai.UserMessage{
        Role:    ai.RoleUser,
        Content: []ai.ContentBlock{ai.TextContent{Type: "text", Text: "List the .go files in the current directory."}},
    }
    if err := a.Run(ctx, []ai.Message{userMsg}, agent.Config{
        StreamOptions: ai.StreamOptions{MaxTokens: 1024},
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
func New(
    provider ai.Provider,
    model string,
    registry *tools.Registry,
    systemPrompt string,
) *Agent
```

Creates a new Agent. All parameters are required.

### `agent.Agent.Run`

```go
func (a *Agent) Run(ctx context.Context, msgs []ai.Message, cfg Config) error
```

Runs the agent loop with the given initial messages. Blocks until the agent
stops (no more tool calls, no follow-up messages, or context cancelled).

Returns a non-nil error only for unrecoverable failures. Provider errors and
tool errors are surfaced as events, not returned.

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

### `agent.Agent.AttachSession`

```go
func (a *Agent) AttachSession(sess *session.Session)
```

Attaches a session for persistence. Every message is appended to the session
file automatically.

---

## Config

```go
type Config struct {
    // ConvertToLLM transforms the full message history to what gets sent to
    // the LLM. Default: keep only user/assistant/tool_result messages.
    ConvertToLLM func([]ai.Message) ([]ai.Message, error)

    // TransformContext applies any pruning/enrichment before ConvertToLLM.
    TransformContext func([]ai.Message) ([]ai.Message, error)

    // GetAPIKey returns a (possibly dynamic) API key for the provider.
    GetAPIKey func(provider string) (string, error)

    // GetSteeringMessages returns messages to inject between tool calls.
    // Return nil to continue normally.
    GetSteeringMessages func() ([]ai.Message, error)

    // GetFollowUpMessages returns messages to send after the agent would stop.
    // Return nil to stop.
    GetFollowUpMessages func() ([]ai.Message, error)

    // StreamOptions are passed to the provider.
    StreamOptions ai.StreamOptions

    // MaxTurns caps the number of LLM calls (turns) per Run.
    // Each turn = one assistant response + its tool calls.
    // 0 means unlimited. When the limit is hit the loop stops cleanly and
    // EventTurnLimitReached is broadcast.
    MaxTurns int
}
```

---

## Events

All agent activity is surfaced via events. Subscribe with `a.Subscribe(fn)`.

```go
type Event struct {
    Type EventType

    // message_* events
    Message     ai.Message
    StreamEvent *ai.StreamEvent   // set on message_update

    // turn_end
    ToolResults  []ai.ToolResultMessage
    ContextUsage ContextUsage     // token snapshot

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
}
```

| Event Type | Description |
|-----------|-------------|
| `agent_start` | Agent loop started |
| `agent_end` | Agent loop finished; `NewMessages` has this turn's messages |
| `turn_start` | One LLM call starting |
| `turn_end` | One LLM call + all its tools finished; `ContextUsage` populated |
| `message_start` | Message added (user, assistant, or tool_result) |
| `message_update` | Streaming delta; `StreamEvent` has the delta |
| `message_end` | Message finalised |
| `tool_start` | Tool execution starting |
| `tool_update` | Streaming progress from tool |
| `tool_end` | Tool execution finished |
| `compaction` | Context compaction completed |
| `turn_limit_reached` | `MaxTurns` hit — loop stopped early; no error returned |

---

## Providers

All providers implement `ai.Provider`:

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

### Provider with custom base URL

```go
import "github.com/nickcecere/agent/pkg/ai/providers/openai"

// OpenAI-compatible endpoint (OpenRouter, Ollama, etc.)
provider := openai.NewWithBaseURL(apiKey, "https://openrouter.ai/api/v1")
```

### Proxy provider

```go
import "github.com/nickcecere/agent/pkg/ai/providers/proxy"

// Connect to a remote agent server
client := proxy.New("https://myserver.example.com", "secret-token")
```

---

## Custom Provider

Implement `ai.Provider` to integrate any LLM backend:

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

        // Call your API, stream results into ch
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
    return tools.TextResult("Result: " + input), nil
}

// Register
reg := tools.NewRegistry()
reg.Register(&MyTool{})
```

---

## Pluggable Bash Executor

Replace the bash tool's execution backend (for remote/sandboxed environments):

```go
import "github.com/nickcecere/agent/pkg/tools/builtin"

type SSHExecutor struct { host string }

func (e *SSHExecutor) Exec(ctx context.Context, command, cwd string, onData func(string)) (int, error) {
    // run command on remote host via SSH
    return runSSH(ctx, e.host, command, cwd, onData)
}

// Register bash tool with custom executor
reg := tools.NewRegistry()
reg.Register(builtin.NewBashToolWithExecutor(".", &SSHExecutor{host: "myserver"}))
```

---

## Session Persistence

```go
import "github.com/nickcecere/agent/pkg/session"

// Create a new session
dir := filepath.Join(os.UserHomeDir(), ".config", "agent", "sessions")
sess, err := session.Create(dir, cwd)

// Attach to agent — all messages auto-persisted
agent.AttachSession(sess)

// On exit
sess.Close()

// Resume later
sess2, err := session.Load(dir, "a3f7c9") // prefix match on session ID
msgs := sess2.Messages()                   // restore conversation history
```

---

## Serving a Proxy

Expose any provider as a network-accessible agent proxy:

```go
import (
    "net/http"
    "github.com/nickcecere/agent/pkg/ai/providers/proxy"
    "github.com/nickcecere/agent/pkg/ai/providers/anthropic"
)

upstream := anthropic.New(os.Getenv("ANTHROPIC_API_KEY"))
handler  := proxy.NewHandler(upstream, "my-secret-token")

http.Handle("/stream", handler)
http.ListenAndServe(":8080", nil)
```

Clients connect using `provider: proxy` with `base_url: http://localhost:8080`.

---

## Compaction

Compaction is configured when building the agent:

```go
import "github.com/nickcecere/agent/pkg/agent"

a := agent.New(provider, model, reg, systemPrompt)
a.SetCompaction(agent.CompactionConfig{
    Enabled:          true,
    ContextWindow:    200000,
    ReserveTokens:    16384,
    KeepRecentTokens: 20000,
})
```

Listen for compaction events:

```go
a.Subscribe(func(e agent.Event) {
    if e.Type == agent.EventCompaction {
        c := e.Compaction
        fmt.Printf("Compacted %d → %d tokens (%d messages removed)\n",
            c.TokensBefore, c.TokensAfter, c.MessagesRemoved)
    }
})
```
