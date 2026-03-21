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
- **Agent discovery** — `agent/discover` tool lets orchestrators find agents dynamically
- **Structured handoff** — pass context (date, constraints, prior results) to sub-agents
- **MCP server mode** — `agent serve --profile <name>` exposes agents as MCP tools
- **Sub-agent progress** — parent sees child tool calls in real time with `[sub:profile]` prefix
- **Transitive dependencies** — `plugins install` auto-installs missing `requires.plugins`
- **Session compaction** — pi-mono style structured summarization for long conversations

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

# Install (auto-installs dependencies)
agent plugins install send-email --source official
agent plugins install grafana-alerts@0.1.0

# Upgrade and publish
agent plugins upgrade send-email
agent plugins publish ../agent-plugins/my-plugin --registry official

# Configure
agent plugins config set send-email baseURL http://localhost:3001
```

## MCP server mode

Expose any agent profile as an MCP tool for external clients:

```bash
# Start as MCP server (stdio transport)
agent serve --profile researcher
```

Configure in Claude Desktop, Cursor, or opencode:

```json
{
  "mcp": {
    "researcher": {
      "type": "local",
      "command": ["agent", "serve", "--profile", "researcher"]
    }
  }
}
```

External clients see the agent as a single callable tool with the profile's
name, description, and capabilities. The agent runs its full tool chain
internally and returns the final output.

## HTTP worker mode

Run as a distributed worker that accepts tasks via HTTP:

```bash
# Dynamic worker — loads any profile on demand
agent serve --addr :9898

# With gateway integration — parallel sub-agents distributed across pods
GATEWAY_URL=http://gateway:8080 agent serve --addr :9898
```

Workers auto-install profiles and plugins from the registry when a task
requires them. No pre-configuration needed.

## k8s deployment

Workers start blank in k8s. When a task arrives, the worker pulls the
profile and plugins from the registry automatically:

```
Task → Gateway → Worker (blank)
                   ↓ pulls profile from registry
                   ↓ pulls plugins from registry
                   ↓ executes task
                   → result stored in PostgreSQL
```

## Related repos

| Repo | Purpose |
|---|---|
| [agent-gateway](https://github.com/bitop-dev/agent-gateway) | Task routing, auth, webhooks, scheduling, dashboard |
| [agent-registry](https://github.com/bitop-dev/agent-registry) | Plugin + profile package server |
| [agent-plugins](https://github.com/bitop-dev/agent-plugins) | Plugin packages |
| [agent-profiles](https://github.com/bitop-dev/agent-profiles) | Agent profile definitions |
| [agent-docs](https://github.com/bitop-dev/agent-docs) | Full documentation |

## Development

```bash
go test ./...
go build ./...
go run ./cmd/agent doctor
```
