# Agent

`github.com/ncecere/agent` is a local-first Go agent framework with:

- a generic runtime loop with tool call cycle
- declarative profiles (agent definitions)
- installable plugins with a registry ecosystem
- built-in core tools: `read`, `write`, `edit`, `bash`, `glob`, `grep`
- policy and approval gates for sensitive actions
- SQLite-backed sessions with resume and pi-mono-style compaction
- sub-agent orchestration with sequential and parallel spawn
- plugin runtimes: `http`, `command` (argv-template + JSON-stdin/stdout), `mcp`, `host`
- tool name sanitisation for strict API backends (Bedrock, Azure)

## Status

The framework is past `v0.1` with a working plugin registry, version tracking,
upgrade/publish workflow, and a library of 14 tested plugins.

```bash
go test ./...   # all tests pass
go run ./cmd/agent doctor
```

## Repository layout

```
cmd/agent/       CLI host
internal/        runtime, services, providers, registries, persistence, policies
pkg/             public framework contracts
docs/            reference docs, patterns guide, examples
_testing/        repository-local profiles, runtimes, and fixtures
```

Sibling repos:
- `../agent-plugins` — 14 plugin packages (source of truth)
- `../agent-registry` — HTTP registry server

## Quick start

```bash
# Configure provider
export OPENAI_BASE_URL=https://your-provider.com/v1
export OPENAI_API_KEY=sk-...

# Add a registry source and install plugins
go run ./cmd/agent plugins sources add official http://127.0.0.1:9080 --type registry
go run ./cmd/agent plugins search
go run ./cmd/agent plugins install ddg-research
go run ./cmd/agent plugins enable ddg-research

# Run an agent
go run ./cmd/agent run --profile researcher "Research the latest AI news"
```

## Plugin workflow

```bash
# Install (local, by name, or with version pinning)
agent plugins install ../agent-plugins/send-email --link
agent plugins install send-email --source official
agent plugins install send-email@0.2.0

# Configure
agent plugins config set send-email provider smtp
agent plugins config set send-email smtpPort 587
agent plugins validate-config send-email
agent plugins enable send-email

# Upgrade and publish
agent plugins upgrade send-email
agent plugins publish ../agent-plugins/my-plugin --registry official
```

## Profile workflow

```bash
# List, run, install
agent profiles list
agent run --profile researcher "Research OpenAI news"
agent profiles install ./my-profiles/researcher
```

## Key features

| Feature | Details |
|---|---|
| **Plugin sources** | Filesystem directories and HTTP registry servers |
| **Version tracking** | Installed version and source recorded in config |
| **Version pinning** | `plugins install name@0.2.0` for exact versions |
| **Upgrade** | `plugins upgrade <name>` fetches latest from source |
| **Publish** | `plugins publish <path> --registry <name>` with bearer auth |
| **Dependencies** | `requires.plugins` enforced at registration time |
| **envMapping** | Map plugin config keys to subprocess environment variables |
| **Prompt IDs** | Profile `instructions.system` resolves plugin prompt IDs |
| **Session compaction** | Pi-mono style: token-aware, structured summaries, turn-boundary cuts |
| **Parallel sub-agents** | `agent/spawn-parallel` for concurrent sub-agent execution |
| **Tool name sanitisation** | `py/word-count` → `py_word-count` for Bedrock/Azure compatibility |

## Documentation

| Doc | What it covers |
|---|---|
| `docs/profiles.md` | Complete profile reference |
| `docs/policy.md` | Policy system, overlay format, precedence |
| `docs/prompts.md` | System instruction resolution, plugin prompt IDs |
| `docs/plugins.md` | Plugin CLI: install, upgrade, publish, sources, config |
| `docs/building-plugins.md` | How to build plugins, all contribution types, envMapping |
| `docs/patterns/` | 6-pattern agent design guide with worked examples |

## Plugin inventory (14 plugins)

| Name | Runtime | Description |
|---|---|---|
| `core-tools` | host | Built-in read/write/edit/bash |
| `ddg-research` | command (Go) | Real DuckDuckGo search + page fetch with date filtering |
| `github-cli` | command (argv) | Wraps `gh` CLI — PRs, issues, repos |
| `grafana-alerts` | command (Go) | Alert events, PromQL, LogQL via Grafana REST API |
| `grafana-mcp` | mcp | MCP bridge to mcp-grafana |
| `json-tool` | command (Go) | JSON echo example |
| `kubectl` | command (argv) | Kubernetes — pods, deployments, events, logs, describe |
| `mcp-filesystem` | mcp | MCP bridge to filesystem server |
| `python-tool` | command (Python) | Word count example |
| `send-email` | http | SMTP email draft and send |
| `slack` | command (Go) | Post messages via Webhook or Web API |
| `spawn-sub-agent` | host | Sequential and parallel sub-agent delegation |
| `web-research` | http | Web search and fetch (HTTP runtime) |

## Agent patterns (tested end-to-end)

| Pattern | Description |
|---|---|
| Single-tool agent | Wrap a CLI/script, one tool, one focused task |
| Research agent | DDG search → fetch → synthesize |
| Research + email | Research → email delivery pipeline |
| Orchestrator + sub-agents | Parallel research → combined email |
| Grafana ops summary | Alert events + Loki logs + PromQL metrics → email |

See `docs/patterns/` for detailed walkthroughs with working profiles and prompts.
