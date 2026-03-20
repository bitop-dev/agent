# AGENTS.md — agent

Instructions for AI coding agents working in this repository.

## What this repo is

The `agent` Go module is the core framework and CLI for building and running LLM-backed agents. It is the runtime — not the plugins, not the registry server, not the documentation.

**Module:** `github.com/bitop-dev/agent`
**Go version:** 1.26+
**Entry point:** `cmd/agent/`

## What this repo does NOT own

| Concern | Where it lives |
|---|---|
| Plugin package bundles | `../agent-plugins` |
| Registry HTTP server | `../agent-registry` |
| Documentation | `../agent-docs` |

Do not add plugin bundle files here. Do not add registry server code here. Do not add broad documentation here — put it in `agent-docs`.

## Key directories

```
cmd/agent/              — main entrypoint
internal/
  cli/run.go            — all CLI command dispatch (single file)
  approval/             — approval flow and prompt
  host/                 — host runtime (spawn-sub-agent, parallel)
  loader/               — combined plugin + profile loader
  mcp/                  — MCP client (stdio / HTTP / SSE)
  plugin/
    manage.go           — install, search, upgrade, publish, remove
    register.go         — in-memory plugin registry
    loader.go           — load plugin manifests from disk
    remote.go           — registry HTTP client calls
    validate.go         — plugin manifest validation
  policy/               — policy evaluation and overlays
  profile/              — profile loading and validation
  providers/openai/     — OpenAI-compatible provider
  registry/             — prompt and tool registries
  runtime/              — dispatch to http / command / mcp / host runtimes
  service/              — agent service loop
  store/sqlite/         — session persistence
  tools/core/           — built-in tools (read, write, edit, bash, glob, grep)
  tools/host/           — host tools (spawn-sub-agent)
pkg/                    — public types shared across packages
  config/config.go      — full config schema (profiles, plugins, sources)
  plugin/plugin.go      — plugin manifest and spec types
  policy/, profile/, session/, provider/, runtime/, tool/, events/, workspace/
_testing/               — local test fixtures (not shipped, not in agent-plugins)
```

## How to validate changes

Always run before committing:

```bash
go build ./...
go test ./...
go run ./cmd/agent doctor
```

## CLI command map

| Command | What it does |
|---|---|
| `run` | One-shot agent run with a prompt |
| `chat` | Interactive multi-turn session |
| `resume` | Resume a saved session |
| `plugins list` | List installed plugins |
| `plugins search [query]` | Search local + registry sources |
| `plugins install <name\|path>` | Install from registry or local path |
| `plugins upgrade <name>` | Reinstall latest from original source |
| `plugins publish <path>` | Pack and POST to a registry source |
| `plugins enable/disable <name>` | Toggle plugin active state |
| `plugins remove <name>` | Uninstall a plugin |
| `plugins config set/unset` | Configure a plugin's settings |
| `plugins sources add/list/remove` | Manage plugin sources |
| `profiles list/install/show` | Profile management |
| `sessions list/show/export` | Session management |
| `config get/set` | Global config management |
| `doctor` | Validate environment and config |

## Plugin source types

```yaml
pluginSources:
  - name: local-dev
    type: filesystem
    path: ../agent-plugins

  - name: official
    type: registry
    url: http://127.0.0.1:9080
    publishToken: dev-token-123   # optional, only for publish
```

## Things to watch out for

- All CLI dispatch is in `internal/cli/run.go` — one large file by design
- Plugin config is stored per-plugin in the agent's config directory
- `_testing/` is for local development fixtures only — do not treat it as the plugin source
- The MCP client supports three transports: `stdio`, `http`, and `sse` — each has different lifecycle
- `envMapping` in plugin config translates config keys to subprocess env vars at runtime
- The `host` runtime is special — it runs Go code directly, not a subprocess or HTTP call

## Running locally

```bash
# Build
go build -o bin/agent ./cmd/agent

# Run with a profile
./bin/agent run --profile ./_testing/profiles/coding/profile.yaml "explain go interfaces"

# Run with the registry (start agent-registry first)
./bin/agent plugins install send-email --source official
```
