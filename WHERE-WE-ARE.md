# WHERE-WE-ARE

This file is a fast handoff for work in the `agent` repository.

## What this repo owns

- core framework and CLI
- runtime loop, providers, policy, approvals, sessions
- built-in core tools (read, write, edit, bash, glob, grep)
- plugin loading, install/config/enable flows
- framework-owned test profiles and runtime fixtures

Sibling repos:
- `../agent-plugins` — plugin package bundles
- `../agent-registry` — plugin registry HTTP server

---

## Release status

- `v0.1.0` — tagged, initial framework release
- Phase 1 (plugin source management + registry integration) — **complete**
- Phase 2 (in progress) — plugin ecosystem, real-world agent patterns

---

## What is implemented and working

### Core framework
- Generic runtime loop with typed events
- CLI: `run`, `chat`, `resume`, `profiles`, `plugins`, `sessions`, `config`, `doctor`
- OpenAI-compatible provider with chat/responses support and SSE streaming
- Tool name sanitization for strict API backends (Bedrock, Azure — `[a-zA-Z0-9_-]+`)
- SQLite-backed session persistence, resume, export
- Declarative profile loading with policy overlays
- Approval and policy enforcement
- MCP client bridge with remote (HTTP/SSE) and stdio transport

### Plugin system
- Install from local path, filesystem source, or registry server
- `plugins sources add/remove/list` — configure filesystem and registry sources
- `plugins search [query]` — searches both filesystem and registry sources
- `plugins install <name>` — resolves from configured sources, downloads tarball,
  verifies SHA256, extracts with permission preservation
- `plugins config set/unset/show`, `plugins validate-config`
- `plugins enable/disable/remove`
- `envMapping` in Runtime — maps plugin config keys to subprocess env vars

### Plugin contributions
- `tools` — command (argv-template + JSON-stdin/stdout), HTTP, MCP, host runtimes
- `prompts` — registered by ID, referenceable from profile `instructions.system`
- `profileTemplates` — registered, available as reference material
- `policies` — registered, opt-in by profiles
- `sensitiveActions` — automatically enforced on enable
- `cmd.Dir` set from plugin directory — relative paths in command work correctly

### System instructions
- Three-step resolution: plugin prompt ID → file path → inline text
- Works in both top-level agents and sub-agents (capabilities.go)

### Sub-agent orchestration (spawn-sub-agent)
- Bounded sub-runs with depth limit (default 2)
- Deny-all approvals inside sub-agents
- Profile-scoped tool sets

---

## What was built this session (Phase 1 completion + Phase 2 start)

### Framework fixes
1. `fix(plugin)`: `cmd.Dir` set from `PluginDir` for command runtime
2. `fix(provider)`: tool name sanitization for Bedrock/Azure backends
3. `fix(provider)`: omit `tool_choice` when no tools
4. `feat(plugin)`: remote registry search and install (`internal/plugin/remote.go`)
5. `feat(plugin)`: `envMapping` in Runtime for env var injection
6. `feat(cli)`: plugin prompt IDs resolve in system instructions (both CLI and sub-agents)

### New plugins (`../agent-plugins`)
- `ddg-research` — real DuckDuckGo web search + page fetch (Go binary, command runtime)
  - `timeRange` parameter maps to DDG `df=` date filter — prevents stale results
- `grafana-mcp` — MCP bridge to `mcp-grafana` binary with envMapping auth
- `grafana-alerts` — custom Go binary calling Grafana REST APIs directly
  - `grafana/alert-events` — alertmanager + Prometheus rules API, time-range aware
  - `grafana/query-metrics` — PromQL via datasource proxy
  - `grafana/query-logs` — LogQL via datasource proxy
  - `grafana/datasources` — UID discovery

### Working agent patterns (verified end-to-end)
- Single-tool agent (python-tool word count)
- Research + action pipeline (DDG → email)
- Orchestrator + sub-agents (parallel research → combined email)
- Grafana ops summary (alert events + logs + metrics → email)

---

## New documentation

- `docs/profiles.md` — complete profile reference
- `docs/policy.md` — policy system, overlay format, precedence
- `docs/prompts.md` — system instruction resolution, plugin prompt IDs
- `docs/building-plugins.md` — updated with all contribution types
- `docs/plugins.md` — updated with registry source workflow
- `docs/patterns/` — 6-pattern agent design guide with worked examples

---

## Next steps

See "What logical next steps are" section at bottom for prioritized list.

### Immediate / high value
1. **Plugin versioning + upgrade** — `plugins upgrade <name>` command, version pinning
   (`send-email@0.2.0`), installed version tracking in config
2. **Source filtering on install** — `plugins install send-email --source official`
3. **`grafana-alerts` enhancements** — alert history API per rule UID, k8s event logs,
   smarter deduplication of repeated alert instances
4. **Streaming output improvements** — show tool call arguments, not just names;
   progress indicator for long-running sub-agents

### Medium priority
5. **Profile discovery improvements** — `profiles list` should show user + local
   profiles; `profiles install <name>` from registry
6. **Session compaction** — implement auto compaction for long conversations
7. **`plugins publish`** — workflow for publishing to registry from agent-plugins
8. **More plugins** — Slack, GitHub Issues, Jira, k8s kubectl

### Longer term
9. **Parallel sub-agents** — run multiple spawn calls concurrently
10. **Agent-as-MCP-server** — expose agent tools over MCP protocol for other clients
11. **Web UI** for session/agent management

---

## Quick validation commands

```bash
# Core
go test ./...
go run ./cmd/agent doctor

# Registry (in ../agent-registry)
go run ./cmd/registry-server --plugin-root ../agent-plugins --addr 127.0.0.1:9080

# Install from registry
go run ./cmd/agent plugins sources add official http://127.0.0.1:9080 --type registry
go run ./cmd/agent plugins search
go run ./cmd/agent plugins install ddg-research

# Run a research + email agent
go run ./cmd/agent run --profile ai-news-orchestrator \
  "Today is $(date +%Y-%m-%d). Research OpenAI and Anthropic news and send to nick@bitop.dev"

# Run Grafana ops summary
go run ./cmd/agent run --profile grafana-alert-summary \
  "Today is $(date +%Y-%m-%d). Get last week's ops summary for team ict-aipe. Send to nick@bitop.dev"
```
