# agent

A Go framework and CLI for building and running AI agents powered by large language models.

## What it is

`agent` provides the runtime, CLI, and plugin system for composing LLM-backed agents.
Define an agent via a profile YAML — model, tools, plugins, policies — then run it
from the CLI, as an HTTP worker, or as an MCP server.

```bash
agent run --profile researcher "Summarize today's AI news"
agent chat --profile coding
agent serve --addr :9898                    # HTTP worker
agent serve --profile researcher            # MCP server
```

## Features

- **Multi-mode CLI** — `run`, `chat`, `resume`, `serve`, `plugins`, `profiles`, `sessions`, `doctor`
- **Plugin system** — install, upgrade, publish, on-demand install, env-based config, dependencies
- **Plugin runtimes** — `http`, `mcp`, `command` (argv-template + JSON stdin/stdout), `host`
- **Built-in tools** — `read`, `write`, `edit`, `bash`, `glob`, `grep`
- **Profile system** — declarative YAML with inheritance, discovery metadata, MCP server, reactive triggers
- **Model fallback** — `provider.fallback: [model1, model2]` — auto-tries next on failure
- **Agent memory** — `agent/remember` + `agent/recall` — persistent knowledge across tasks
- **Providers** — OpenAI-compatible + native Anthropic Messages API
- **Policy and approval** — configurable gates for sensitive tools
- **MCP client bridge** — connect to external MCP servers
- **Session compaction** — pi-mono style structured summarization
- **Sub-agent orchestration** — sequential, parallel, pipelines, discovery
- **Tool name sanitization** — works with Bedrock, Azure, strict backends
- **On-demand everything** — blank workers pull profiles + plugins from registry

## HTTP worker mode

```bash
agent serve --addr :9898                           # dynamic — any profile
GATEWAY_URL=http://gateway:8080 agent serve --addr :9898  # with gateway integration
```

Workers auto-register with the gateway, install profiles and plugins on demand,
and report token usage for cost tracking.

## MCP server mode

```bash
agent serve --profile researcher
```

Exposes the agent as an MCP tool for opencode, Claude Desktop, Cursor.

## k8s deployment

Workers start blank. Tasks arrive via the gateway, workers pull what they need:

```
Gateway → Worker (blank) → pulls profile → pulls plugins → executes → result
```

Scale: `kubectl -n agent-system scale deployment/agent-workers --replicas=10`

## Related repos

| Repo | Purpose |
|---|---|
| [agent-gateway](https://github.com/bitop-dev/agent-gateway) | Task routing, auth, webhooks, scheduling, costs, dashboard |
| [agent-registry](https://github.com/bitop-dev/agent-registry) | Plugin + profile package server |
| [agent-plugins](https://github.com/bitop-dev/agent-plugins) | Plugin packages |
| [agent-profiles](https://github.com/bitop-dev/agent-profiles) | Agent profile definitions |
| [agent-docs](https://github.com/bitop-dev/agent-docs) | Full documentation |
