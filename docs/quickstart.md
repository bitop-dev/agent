# Quick Start

## Prerequisites

- Go 1.23+
- An API key for at least one supported provider

## Install

```bash
git clone https://github.com/nickcecere/agent
cd agent
go build -o agent ./cmd/agent
```

Or install directly:

```bash
go install github.com/nickcecere/agent/cmd/agent@latest
```

## Your First Config

Create `agent.yaml`:

```yaml
provider: anthropic
model: claude-sonnet-4-5
api_key: ${ANTHROPIC_API_KEY}
max_tokens: 4096

tools:
  preset: coding   # read, bash, edit, write
```

All `${ENV_VAR}` references are expanded from your environment before parsing.

## Run

```bash
# Interactive REPL
./agent -config agent.yaml

# One-shot prompt (no REPL)
./agent -config agent.yaml -prompt "List all .go files and summarize each one."

# Override working directory for file tools
./agent -config agent.yaml -cwd /path/to/project

# Resume a previous session
./agent -config agent.yaml -session abc123
```

## Interactive Commands

| Command | Effect |
|---------|--------|
| Any text + Enter | Send prompt to agent |
| `/clear` | Reset conversation history |
| `/state` | Show message count, streaming status |
| `/model <id>` | Switch model for the current session |
| `/session` | Show the current session ID and file path |
| `/sessions` | List recent sessions |
| `/export` | Export current session as self-contained HTML |
| `/skills` | List loaded skills |
| `/templates` | List loaded prompt templates |
| `exit` / `quit` / Ctrl+D | Exit |

## CLI Flags

| Flag | Description |
|------|-------------|
| `-config <path>` | YAML config file (default: `agent.yaml`) |
| `-prompt <text>` | One-shot prompt; skips interactive REPL |
| `-cwd <dir>` | Working directory for file tools |
| `-session <id>` | Resume session by ID prefix |
| `-sessions` | List recent sessions and exit |

## Minimal Config by Provider

### Anthropic

```yaml
provider: anthropic
model: claude-sonnet-4-5
api_key: ${ANTHROPIC_API_KEY}
```

### OpenAI

```yaml
provider: openai-responses
model: gpt-4o
api_key: ${OPENAI_API_KEY}
```

### Google Gemini

```yaml
provider: google
model: gemini-2.0-flash
api_key: ${GEMINI_API_KEY}
```

### Any OpenAI-compatible endpoint

```yaml
provider: openai-completions
base_url: https://openrouter.ai/api/v1
api_key: ${OPENROUTER_API_KEY}
model: meta-llama/llama-3.3-70b-instruct
```

See [providers.md](providers.md) for the full list and all options.
