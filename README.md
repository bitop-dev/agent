# agent — Go LLM Agent Framework

A minimal, config-driven agentic framework written in pure Go. Point it at a
YAML config and run. Embed it in your own programs. Extend it with custom
tools in Go or any other language.

Inspired by [pi-mono](https://github.com/badlogic/pi-mono)'s TypeScript agent.

---

## Features

- **6 LLM providers** — Anthropic, OpenAI (Responses + Completions), Google Gemini, Azure OpenAI, Amazon Bedrock, and any OpenAI-compatible endpoint
- **9 built-in tools** — `read`, `bash`, `edit`, `write`, `grep`, `find`, `ls`, `web_search`, `web_fetch`
- **Plugin tools** — external executables over JSON/stdin/stdout, any language
- **Sub-agent delegation** — compose agents; a parent calls a child agent as a tool
- **Session persistence** — JSONL files, resume any session, fork into branches
- **Context compaction** — auto-summarise old context to stay within the model's window
- **Retry with backoff** — automatic retry on rate limits and transient errors
- **Parallel tool execution** — run multiple tool calls concurrently
- **Confirmation hooks** — gate tool execution with allow/deny/abort callbacks
- **Cost tracking** — per-turn and cumulative USD cost with budget cap
- **Observability** — `OnMetrics` callback with per-turn latency, tokens, and tool durations
- **Structured logging** — `*slog.Logger` with zero-dep default
- **Config hot-reload** — apply model/token changes to a running agent without restart
- **Multimodal input** — image blocks in user messages and tool results
- **Skills** — Markdown files with specialised instructions, discovered automatically
- **Prompt templates** — `/template arg1 arg2` expansion in the REPL
- **Model registry** — 30+ models with context windows, costs, and capability flags
- **Proxy** — serve any provider as an HTTP proxy; connect via `provider: proxy`
- **HTML export** — self-contained dark-mode HTML from any session

---

## Quick Start

```bash
# Build
git clone https://github.com/nickcecere/agent && cd agent
go build -o agent ./cmd/agent

# Configure
cat > agent.yaml << 'EOF'
provider: anthropic
model: claude-sonnet-4-5
api_key: ${ANTHROPIC_API_KEY}
max_tokens: 4096
tools:
  preset: coding
EOF

# Run interactively
export ANTHROPIC_API_KEY=sk-ant-...
./agent -config agent.yaml

# One-shot
./agent -config agent.yaml -prompt "Summarise this repository."
```

---

## Project Layout

```
agent/
├── cmd/agent/                    # CLI binary
├── docs/                         # Full documentation
│   ├── quickstart.md
│   ├── config.md                 # All config fields
│   ├── providers.md              # Provider setup
│   ├── tools.md                  # Built-in tools + writing your own
│   ├── session.md                # Session persistence + JSONL format
│   ├── compaction.md             # Automatic context compaction
│   ├── skills.md                 # Skills system
│   ├── prompt-templates.md       # Prompt templates
│   ├── sdk.md                    # Go library usage
│   ├── proxy.md                  # Proxy provider
│   └── models.md                 # Model registry
├── examples/
│   ├── basic/                    # Minimal embedded agent
│   ├── custom-tool/              # Compiled-in custom tools
│   ├── web-agent/                # Agent with web search + fetch
│   ├── proxy-server/             # HTTP proxy server
│   ├── session-reader/           # Read and export sessions
│   ├── tools/bash_tool/          # External subprocess plugin
│   ├── skills/go-expert/         # Example skill file
│   └── prompts/                  # Example prompt templates
└── pkg/
    ├── ai/                       # Core types, streaming events, Provider interface
    │   ├── sse/                  # SSE reader (no external deps)
    │   ├── models/               # Model registry (30+ models)
    │   └── providers/
    │       ├── anthropic/        # Anthropic Messages API
    │       ├── openai/           # Chat Completions + Responses API
    │       ├── google/           # Google Generative AI
    │       ├── azure/            # Azure OpenAI
    │       ├── bedrock/          # Amazon Bedrock ConverseStream
    │       └── proxy/            # HTTP proxy client + server handler
    ├── agent/                    # Agent struct, event loop, compaction
    ├── tools/                    # Tool interface, registry, plugin protocol
    │   └── builtin/              # read, bash, edit, write, grep, find, ls,
    │                             # web_search, web_fetch
    ├── session/                  # JSONL persistence, fork, HTML export
    ├── skills/                   # Skill discovery and loading
    └── prompts/                  # Prompt template loading and expansion
```

---

## Configuration

```yaml
provider: anthropic              # Required
model: claude-sonnet-4-5         # Required
api_key: ${ANTHROPIC_API_KEY}    # ${ENV_VAR} expanded

system_prompt: |
  You are a helpful coding assistant.

max_tokens: 4096
temperature: 0.7
thinking_level: medium           # off | minimal | low | medium | high | xhigh
cache_retention: short           # none | short | long  (Anthropic caching)

# Max LLM turns per prompt (0 = unlimited).
# Each turn = one assistant response + its tool calls.
# Prevents infinite loops when a model repeatedly calls tools.
# Recommended: 50 for general use, 200 for long agentic/research tasks.
max_turns: 50

context_window: 200000           # Auto-filled from model registry if known

compaction:
  enabled: true
  reserve_tokens: 16384
  keep_recent_tokens: 20000

tools:
  preset: coding                 # coding | readonly | web | all | none
  work_dir: .
  plugins:
    - path: ./my-plugin
```

See [docs/config.md](docs/config.md) for the complete reference.

---

## Providers

| Provider | Config value | Notes |
|----------|-------------|-------|
| Anthropic | `anthropic` | Claude 3.x / 4.x, caching, thinking |
| OpenAI Responses | `openai-responses` | GPT-4o, o1, o3 |
| OpenAI Completions | `openai-completions` | Also for OpenRouter, Ollama, Groq, etc. |
| Google | `google` | Gemini 1.5 / 2.0 / 2.5 |
| Azure OpenAI | `azure` | Chat Completions |
| Amazon Bedrock | `bedrock` | ConverseStream, IAM auth |
| Proxy | `proxy` | Connect to another agent instance |

See [docs/providers.md](docs/providers.md) for details and environment variables.

---

## Built-in Tools

| Tool | Preset | Description |
|------|--------|-------------|
| `read` | coding, readonly, all | Read a file (with offset/limit) |
| `bash` | coding, all | Execute shell commands |
| `edit` | coding, all | Replace exact text in a file |
| `write` | coding, all | Create or overwrite a file |
| `grep` | readonly, all | Regex search across files (pure Go) |
| `find` | readonly, all | Recursive file finder (pure Go) |
| `ls` | readonly, all | List directory contents |
| `web_search` | web, all | DuckDuckGo search (no API key) |
| `web_fetch` | web, all | Fetch URL as clean plain text |

All tools truncate output to 50 KB / 2000 lines. See [docs/tools.md](docs/tools.md).

---

## CLI Flags

```bash
./agent [flags]

Flags:
  -config <path>     Config file (default: agent.yaml)
  -prompt <text>     One-shot prompt; skips interactive REPL
  -cwd <dir>         Working directory for file tools
  -session <id>      Resume session by ID prefix
  -sessions          List recent sessions and exit
```

## Interactive Commands

```
/clear              Reset conversation history
/state              Show message count, streaming status
/model <id>         Switch model (e.g. /model gpt-4o)
/session            Show current session ID and file
/sessions           List recent sessions
/export             Export session as HTML
/skills             List loaded skills
/templates          List loaded prompt templates
/fork [N]           Fork session keeping last N messages
exit / quit         Exit
```

---

## Using as a Go Library

```go
import (
    "github.com/nickcecere/agent/pkg/agent"
    "github.com/nickcecere/agent/pkg/ai"
    "github.com/nickcecere/agent/pkg/ai/providers/anthropic"
    "github.com/nickcecere/agent/pkg/tools"
    "github.com/nickcecere/agent/pkg/tools/builtin"
)

provider := anthropic.New(os.Getenv("ANTHROPIC_API_KEY"))

reg := tools.NewRegistry()
builtin.Register(reg, builtin.PresetCoding, ".")

a := agent.New(agent.Options{
    Provider:     provider,
    Model:        "claude-sonnet-4-5",
    Tools:        reg,
    SystemPrompt: "You are helpful.",
    Logger:       slog.Default(), // structured logging to stderr
})

a.Subscribe(func(e agent.Event) {
    switch e.Type {
    case agent.EventMessageUpdate:
        if se := e.StreamEvent; se != nil && se.Type == ai.StreamEventTextDelta {
            fmt.Print(se.Delta)
        }
    case agent.EventRetry:
        fmt.Printf("[retry %d] %v\n", e.RetryAttempt, e.RetryError)
    case agent.EventTurnEnd:
        fmt.Printf("[cost: $%.4f | %d tokens]\n",
            e.CostUsage.TotalCost, e.ContextUsage.Tokens)
    }
})

a.Prompt(ctx, "List .go files here.", agent.Config{
    StreamOptions:      ai.StreamOptions{MaxTokens: 2048},
    MaxTurns:           50,
    MaxRetries:         3,
    MaxToolConcurrency: 4,
    MaxCostUSD:         0.50,
    ConfirmToolCall:    agent.AutoApproveAll,
})
```

See [docs/sdk.md](docs/sdk.md) for the full API reference.

---

## Skills

Put Markdown files in `~/.config/agent/skills/` (global) or
`.agent/skills/` (project) with YAML frontmatter:

```markdown
---
name: go-expert
description: Expert Go programming guidance. Use for Go code tasks.
---

# Instructions
...
```

The agent lists available skills in its system prompt and reads them on demand
via the `read` tool. See [docs/skills.md](docs/skills.md) and
`examples/skills/go-expert/SKILL.md`.

---

## Prompt Templates

Put Markdown files in `~/.config/agent/prompts/` or `.agent/prompts/`:

```markdown
---
description: Review a file for correctness and style.
---

Review `$1` for correctness, idiomatic style, and test coverage.
```

Invoke with `/review pkg/agent/loop.go`. See [docs/prompt-templates.md](docs/prompt-templates.md).

---

## Session Files

Sessions are stored at `~/.config/agent/sessions/YYYYMMDD-HHMMSS-<id>.jsonl`.

```bash
# List sessions
./agent -sessions

# Resume by ID prefix
./agent -session a3f7c9

# Export as HTML
# (in REPL)
/export
```

See [docs/session.md](docs/session.md) and `examples/session-reader/`.

---

## Context Compaction

When the conversation grows close to the model's context limit, the agent
automatically summarises old messages and continues. Config:

```yaml
compaction:
  enabled: true
  reserve_tokens: 16384      # room for response
  keep_recent_tokens: 20000  # always kept verbatim
```

See [docs/compaction.md](docs/compaction.md).

---

## Examples

| Example | What it shows |
|---------|---------------|
| `examples/basic/` | Minimal embedded agent with REPL |
| `examples/custom-tool/` | Calculator + dice roller custom tools |
| `examples/web-agent/` | Research agent with `web_search` + `web_fetch` |
| `examples/sub-agent/` | Sub-agent delegation via `SubAgent` + `SubAgentTool` |
| `examples/confirmation/` | Tool confirmation hooks (interactive + policy-based) |
| `examples/proxy-server/` | HTTP proxy for any upstream provider |
| `examples/session-reader/` | List, display, export session files |
| `examples/tools/bash_tool/` | External subprocess plugin (JSON protocol) |
| `examples/skills/go-expert/` | Example skill file |
| `examples/prompts/` | `review.md`, `test.md`, `explain.md` templates |

Run any example:

```bash
ANTHROPIC_API_KEY=sk-... go run ./examples/basic
OPENAI_API_KEY=sk-...    go run ./examples/web-agent "Latest news in Go"
ANTHROPIC_API_KEY=sk-... go run ./examples/sub-agent "Summarise this repo"
ANTHROPIC_API_KEY=sk-... go run ./examples/confirmation
                         go run ./examples/session-reader list
```

---

## Documentation

| File | Topic |
|------|-------|
| [docs/quickstart.md](docs/quickstart.md) | Install, first config, first run |
| [docs/config.md](docs/config.md) | Complete configuration reference |
| [docs/providers.md](docs/providers.md) | All providers with credentials |
| [docs/tools.md](docs/tools.md) | Built-in tools reference |
| [docs/custom-tools.md](docs/custom-tools.md) | Writing compiled-in and plugin tools (Go, Python, TS, Rust, Bash, Ruby) |
| [docs/session.md](docs/session.md) | Session format, JSONL, branching |
| [docs/compaction.md](docs/compaction.md) | Context compaction |
| [docs/skills.md](docs/skills.md) | Skills system |
| [docs/prompt-templates.md](docs/prompt-templates.md) | Prompt templates |
| [docs/sdk.md](docs/sdk.md) | Go library API |
| [docs/proxy.md](docs/proxy.md) | Proxy provider |
| [docs/models.md](docs/models.md) | Model registry |

---

## Contributing

1. Fork the repo
2. `go test ./...` — all tests must pass
3. `go vet ./...` — no vet errors
4. Open a pull request

---

## License

MIT
