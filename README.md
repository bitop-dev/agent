# agent

A Go framework and CLI for building and running AI agents powered by large language models.

## What it is

`agent` provides the runtime, CLI, and plugin system for composing LLM-backed agents. You configure an agent via a profile YAML — defining which model, tools, plugins, and policies to use — then run it from the CLI.

```bash
# One-shot task
go run ./cmd/agent run --profile ./profiles/research.yaml "Summarize today's AI news"

# Interactive chat
go run ./cmd/agent chat --profile ./profiles/coding.yaml

# Resume a previous session
go run ./cmd/agent resume --session abc123
```

## Features

- **Multi-mode CLI** — `run`, `chat`, `resume`, `plugins`, `profiles`, `sessions`, `config`, `doctor`
- **Plugin system** — install, enable, disable, configure, upgrade, publish, and search plugins from local paths or a remote registry
- **Plugin runtimes** — `http`, `mcp` (stdio/HTTP/SSE), `command` (argv-template + JSON stdin/stdout), `host`
- **Built-in tools** — `read`, `write`, `edit`, `bash`, `glob`, `grep`
- **Profile system** — declarative YAML profiles with policy overlays and prompt composition
- **Policy and approval** — configurable approval gates for sensitive tool calls
- **MCP client bridge** — connects to external MCP tool servers over stdio, HTTP, or SSE
- **Session persistence** — SQLite-backed sessions with export and replay
- **Registry integration** — remote plugin search, install, and publish against `agent-registry`
- **Parallel sub-agents** — orchestrate concurrent sub-agent runs via the host runtime

## Installation

```bash
git clone https://github.com/bitop-dev/agent
cd agent
go build -o bin/agent ./cmd/agent
```

## Plugin management

```bash
# Search the registry
agent plugins search email

# Install from registry
agent plugins install send-email --source official

# Install from local path
agent plugins install ../agent-plugins/send-email --link

# Upgrade
agent plugins upgrade send-email

# Publish to registry
agent plugins publish ../agent-plugins/my-plugin --registry official

# Configure a plugin
agent plugins config set send-email baseURL http://localhost:3001
```

## Related repos

| Repo | Purpose |
|---|---|
| [agent-plugins](https://github.com/bitop-dev/agent-plugins) | Official plugin packages |
| [agent-registry](https://github.com/bitop-dev/agent-registry) | Plugin registry server |
| [agent-docs](https://github.com/bitop-dev/agent-docs) | Full documentation |

## Development

```bash
go test ./...
go build ./...
go run ./cmd/agent doctor
```
