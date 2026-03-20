# WHERE-WE-ARE

Fast handoff for the `agent` repository.

## Release status

- `v0.1.0` — initial framework release
- **Phase 1 — complete**: plugin source management, registry search/install, registry publish
- **Phase 2 — complete**: session compaction (pi-mono style), parallel sub-agents
- **Phase 3 — complete**: kubectl plugin, Slack plugin, Grafana alerts custom binary plugin

---

## What was shipped (all sessions combined)

### Framework (`agent`)

| Commit | Feature |
|---|---|
| `fix(plugin): cmd.Dir` | Command plugins resolve relative paths correctly |
| `fix(provider): sanitize tool names` | Tool IDs with `/` work on Bedrock, Azure, vLLM |
| `fix(provider): omit tool_choice` | Empty tools list no longer errors on strict backends |
| `feat(plugin): remote registry search/install` | `plugins search` and `plugins install <name>` from registry |
| `feat(plugin): envMapping` | Plugin config keys map to subprocess env vars |
| `feat(cli): prompt ID resolution` | `instructions.system` resolves registered plugin prompt IDs |
| `feat(plugin): version tracking` | InstalledVersion, InstalledSource tracked in config |
| `feat(plugin): --source filter` | `plugins install <name> --source official` |
| `feat(plugin): upgrade` | `plugins upgrade <name>` — checks registry, removes, reinstalls |
| `feat(plugin): publish` | `plugins publish <path> [--registry name]` — packs + POSTs tarball |
| `feat(runtime): session compaction` | Pi-mono style: token-aware, structured summary format, turn-boundary cuts |
| `feat(host): SpawnSubRunParallel` | Concurrent sub-agent execution with goroutines |

### Plugins (`agent-plugins`)

| Plugin | Runtime | What it does |
|---|---|---|
| `ddg-research` | command/Go binary | Real DuckDuckGo search + page fetch with `df=` date filtering |
| `grafana-mcp` | mcp | MCP bridge to mcp-grafana with envMapping auth |
| `grafana-alerts` | command/Go binary | Alert events, PromQL, LogQL via Grafana REST API directly |
| `kubectl` | command/argv-template | Wraps kubectl — pods, deployments, events, logs, describe |
| `slack` | command/Go binary | Post messages and Block Kit blocks via Webhook or Web API |
| `spawn-sub-agent` | host | Added `agent/spawn-parallel` for concurrent sub-agents |

### Registry (`agent-registry`)

| Feature | What it does |
|---|---|
| `POST /v1/packages` | Publish endpoint — accepts tarball, validates, stores, updates live index |
| `--publish-token` | Bearer token auth for publishing |

---

## Plugin inventory (14 total)

| Name | Type | Status |
|---|---|---|
| core-tools | host | ✅ built-in |
| ddg-research | command (Go binary) | ✅ tested |
| github-cli | command (argv-template) | ✅ needs `gh` installed |
| grafana-alerts | command (Go binary) | ✅ tested end-to-end |
| grafana-mcp | mcp | ✅ tested |
| json-tool | command (Go binary) | ✅ tested |
| kubectl | command (argv-template) | ✅ new |
| mcp-filesystem | mcp | ✅ tested |
| python-tool | command (Python script) | ✅ tested |
| send-email | http | ✅ tested end-to-end |
| slack | command (Go binary) | ✅ new |
| spawn-sub-agent | host | ✅ tested (sequential + parallel) |
| web-research | http | ✅ tested |

---

## Agent patterns validated

| Pattern | Tools | Status |
|---|---|---|
| Single-tool | python-tool | ✅ word count |
| Research pipeline | ddg/search + ddg/fetch | ✅ real DDG results |
| Research + email | ddg + email/send | ✅ emails delivered |
| Orchestrator + sub-agents | agent/spawn + email/send | ✅ parallel research |
| Grafana ops summary | grafana-alerts + email/send | ✅ 293 real alert events |
| MCP filesystem | read_file + list_directory | ✅ real /tmp listing |

---

## Documentation

| Doc | What it covers |
|---|---|
| `docs/profiles.md` | Complete profile reference |
| `docs/policy.md` | Policy system, overlay format, precedence |
| `docs/prompts.md` | System instruction resolution, plugin prompt IDs |
| `docs/building-plugins.md` | All contribution types, override behavior |
| `docs/plugins.md` | Plugin CLI workflow, registry sources |
| `docs/patterns/` | 6-pattern agent design guide with worked examples |

---

## Quick validation

```bash
# Core
go test ./...
go run ./cmd/agent doctor
go run ./cmd/agent plugins list

# Registry
cd ../agent-registry && go run ./cmd/registry-server --plugin-root ../agent-plugins --addr 127.0.0.1:9080 --publish-token dev-token-123

# Install from registry
go run ./cmd/agent plugins install ddg-research --source official
go run ./cmd/agent plugins search kubectl

# Publish
go run ./cmd/agent plugins publish ../agent-plugins/kubectl --registry official

# Upgrade
go run ./cmd/agent plugins upgrade ddg-research

# Run research agent
go run ./cmd/agent run --profile ai-news-orchestrator "Today is $(date +%Y-%m-%d). Research OpenAI and Anthropic news and send to nick@bitop.dev"

# Run grafana summary
go run ./cmd/agent run --profile grafana-alert-summary "Today is $(date +%Y-%m-%d). Get last week's ops summary for team ict-aipe. Send to nick@bitop.dev"
```

---

## Logical next steps

1. **Grafana alert deduplication** — group repeated alert instances by name+host, collapse into "fired N times, last at X"
2. **Plugin dependency resolution** — `requires.plugins` field is declared but not enforced at install time
3. **Multi-version support in registry** — store and serve multiple versions per package, not just latest
4. **Profile sharing** — `profiles install <name>` from registry, profile packages
5. **Agent-as-MCP-server** — expose agent tools over MCP protocol for other clients
6. **Web UI** for session/agent management
