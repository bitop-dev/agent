# agent — Go LLM agent framework

A minimal, config-driven agentic framework inspired by pi-mono's `packages/agent`.
Compile one binary, point it at a YAML config, wire in tools.

---

## Project layout

```
agent/
├── cmd/agent/            # CLI binary
├── pkg/
│   ├── ai/               # Core types: messages, content blocks, streaming events
│   │   ├── sse/          # SSE reader used by providers
│   │   └── providers/
│   │       ├── openai/   # OpenAI chat-completions (+ any OpenAI-compatible)
│   │       └── anthropic/ # Anthropic messages API
│   ├── tools/            # Tool interface, registry, subprocess plugin protocol
│   └── agent/            # Agent struct, event system, loop, config loader
├── examples/tools/
│   └── bash_tool/        # Example external tool plugin
└── agent.example.yaml    # Annotated config example
```

---

## Quick start

```bash
# 1. Copy and edit the config
cp agent.example.yaml agent.yaml
$EDITOR agent.yaml

# 2. Build the agent binary
go build -o agent ./cmd/agent

# 3. Run interactive mode
./agent -config agent.yaml

# 4. Or one-shot
./agent -config agent.yaml -prompt "What is the capital of France?"
```

---

## Config file

```yaml
provider: anthropic          # openai | anthropic | openrouter | groq | ollama | …
model: claude-opus-4-5
api_key: ${ANTHROPIC_API_KEY} # env-var expansion supported anywhere

system_prompt: |
  You are a helpful assistant.

max_tokens: 4096
# temperature: 0.7
# base_url: https://openrouter.ai/api/v1  # required for non-default endpoints

plugins:
  - path: ./examples/tools/bash_tool/bash_tool
```

All `${ENV_VAR}` references are expanded before parsing, so you never have to
hard-code secrets.

---

## Built-in tool plugins

### `bash_tool`

Runs arbitrary bash commands and returns stdout+stderr.

```bash
go build -o bash_tool ./examples/tools/bash_tool
```

---

## Writing your own tool

### Option A — compile into the binary

Implement `tools.Tool`:

```go
type MyTool struct{}

func (t MyTool) Definition() ai.ToolDefinition {
    return ai.ToolDefinition{
        Name:        "my_tool",
        Description: "Does something useful.",
        Parameters: tools.MustSchema(tools.SimpleSchema{
            Properties: map[string]tools.Property{
                "input": {Type: "string", Description: "The input value."},
            },
            Required: []string{"input"},
        }),
    }
}

func (t MyTool) Execute(ctx context.Context, callID string, params map[string]any, onUpdate tools.UpdateFn) (tools.Result, error) {
    input, _ := params["input"].(string)
    return tools.TextResult("you said: " + input), nil
}
```

Register it before creating the agent:

```go
registry := tools.NewRegistry()
registry.Register(MyTool{})

ag := agent.New(agent.Options{
    Provider: openai.New(""),
    Model:    "gpt-4o",
    Tools:    registry,
})
```

### Option B — external subprocess plugin

Write any executable that speaks the JSON-over-stdin/stdout protocol:

```
← {"type":"describe"}
→ {"name":"my_tool","description":"...","parameters":{...}}

← {"type":"call","call_id":"abc123","params":{"input":"hello"}}
→ {"content":[{"type":"text","text":"you said: hello"}],"error":false}
```

Then add it to the config:

```yaml
plugins:
  - path: ./my_tool_binary
```

The agent starts the process once and keeps it alive for the session.
See `examples/tools/bash_tool/main.go` for a complete example.

---

## Using the agent in your own Go code

```go
package main

import (
    "context"
    "fmt"

    "github.com/nickcecere/agent/pkg/agent"
    "github.com/nickcecere/agent/pkg/ai"
    "github.com/nickcecere/agent/pkg/ai/providers/anthropic"
    "github.com/nickcecere/agent/pkg/tools"
)

func main() {
    registry := tools.NewRegistry()
    // registry.Register(MyTool{})

    ag := agent.New(agent.Options{
        SystemPrompt: "You are a helpful assistant.",
        Model:        "claude-opus-4-5",
        Provider:     anthropic.New(""),
        Tools:        registry,
    })

    // Stream output to terminal
    ag.Subscribe(func(ev agent.Event) {
        if ev.Type == agent.EventMessageUpdate && ev.StreamEvent != nil {
            if ev.StreamEvent.Type == ai.StreamEventTextDelta {
                fmt.Print(ev.StreamEvent.Delta)
            }
        }
        if ev.Type == agent.EventMessageEnd && ev.Message.GetRole() == ai.RoleAssistant {
            fmt.Println()
        }
    })

    cfg := agent.Config{
        StreamOptions: ai.StreamOptions{
            APIKey:    "sk-ant-...",
            MaxTokens: 4096,
        },
    }

    if err := ag.Prompt(context.Background(), "Hello!", cfg); err != nil {
        panic(err)
    }
}
```

---

## Architecture

```
Prompt()
  └─ runLoop()                     agent/loop.go
       ├─ streamResponse()         → provider.Stream() → SSE events
       │    └─ broadcast(message_update) for each text delta
       └─ executeToolCalls()
            ├─ registry.Get(name).Execute()
            └─ broadcast(tool_start / tool_end)
```

**Steering** — call `ag.SteerText("…")` while the agent is running.
The message is injected after the current tool call finishes, remaining
tool calls are skipped.

**Follow-up** — call `ag.FollowUpText("…")` while running.
The message is injected only when the agent would otherwise stop.

**Abort** — call `ag.Abort()` to cancel the current run via context.
