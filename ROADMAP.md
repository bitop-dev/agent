# Roadmap

## Current state (v0.3.6)

The platform is a working distributed agent system deployed in k8s with 6 services:
gateway, workers, registry, PostgreSQL, NATS, and a web dashboard.

### What's deployed and working

| Feature | Status |
|---|---|
| Core agent framework (runtime, CLI, tools) | ✅ |
| Plugin system (install, upgrade, publish, dependencies) | ✅ |
| Profile system (discovery metadata, MCP server, registry) | ✅ |
| On-demand plugin + profile install from registry | ✅ |
| HTTP worker mode (dynamic profile loading) | ✅ |
| MCP server mode (opencode, Claude Desktop) | ✅ |
| Agent discovery (local + registry) | ✅ |
| Structured handoff (context parameter) | ✅ |
| Agent pipelines (agent/pipeline with {{var}} routing) | ✅ |
| Pipeline checkpoints | ✅ |
| Sequential sub-agents (agent/spawn) | ✅ |
| Parallel sub-agents (agent/spawn-parallel) | ✅ |
| Gateway-distributed parallel (POST /v1/tasks/parallel) | ✅ |
| Gateway task routing (capability-based, load-aware) | ✅ |
| Gateway retries on transient failures | ✅ |
| API key auth with scopes | ✅ |
| Webhooks with template expansion | ✅ |
| Cron scheduling | ✅ |
| Task history in PostgreSQL | ✅ |
| NATS event bus | ✅ |
| SSE event stream | ✅ |
| Web dashboard (embedded) | ✅ |
| Worker auto-registration with gateway | ✅ |
| Session compaction (pi-mono style) | ✅ |
| Tool name sanitization (Bedrock/Azure) | ✅ |
| Sub-agent progress visibility | ✅ |
| CI/CD with GitHub Actions | ✅ |
| Docker images (multi-arch) | ✅ |
| k8s deployment (6 services) | ✅ |

---

## What to work on next (priority order)

### 1. Multi-arch plugin builds (CI)
**Status:** Manual cross-compile before publishing. No automation.

Plugins with Go binaries (ddg-research, grafana-alerts, slack) must be
compiled for linux/amd64 before publishing to the registry for k8s workers.
Currently done by hand.

**What to build:**
- GitHub Actions workflow in agent-plugins that cross-compiles on push/tag
- Plugin tarball includes binaries for linux/amd64 + darwin/arm64
- Worker selects the correct binary at install time based on GOOS/GOARCH
- Or: plugins declare a `build` step and workers compile from source

**Why first:** Every k8s deployment hits this. Without it, publishing
a plugin from macOS produces a broken binary in k8s.

---

### 2. Agent memory / persistent knowledge
**Status:** PostgreSQL table exists (`agent_memory`). No tools or API.

Agents forget everything between tasks. For recurring workflows (daily ops
reports, weekly research), agents should remember previous findings.

**What to build:**
- `agent/remember` host tool — store a key-value fact for this profile
- `agent/recall` host tool — retrieve facts stored by previous tasks
- Gateway API: `GET/POST /v1/memory?profile=researcher`
- Memory scoped per-profile (researcher remembers different things than orchestrator)
- Optional TTL for expiring facts

**Why second:** Enables the daily ops report to say "compared to yesterday's
report..." and the research agent to avoid re-fetching the same articles.

---

### 3. Cost tracking
**Status:** Not started.

No visibility into LLM token usage or cost per task.

**What to build:**
- Count input/output tokens per LLM API call in the provider
- Store token counts per task in PostgreSQL
- Calculate estimated cost based on model pricing table
- Gateway API: `GET /v1/costs?profile=&since=`
- Budget limits per profile (soft warning) or per worker (hard cap)
- Dashboard: cost chart by profile and model

**Why third:** Running 5 workers with nemotron-120b costs real money.
Need visibility before scaling further.

---

### 4. Model fallback chain
**Status:** Gateway retries on transient errors. No model-level fallback.

When a model times out or returns garbage (nemotron's `<|channel|>` garbling),
the agent should try a different model instead of failing.

**What to build:**
- Profile spec: `provider.fallback: [gpt-oss-120b, gpt-4o-mini]`
- Runtime: if primary model fails, retry with fallback model
- Separate from gateway retries (which pick a different worker)
- Log which model was used in the task result

**Why fourth:** The nemotron garbling issue and LLM timeouts are the
most common failure mode in production.

---

### 5. Plugin config from environment
**Status:** `envMapping` exists. Not all plugins use it. No defaults.

Plugins like send-email need SMTP config that should come from k8s
secrets, not manual `plugins config set` commands.

**What to build:**
- Default config values in plugin.yaml configSchema
- Auto-populate config from matching environment variables
- `AGENT_PLUGIN_<NAME>_<KEY>` convention for env-based config
- On-demand install sets defaults so plugins work with minimal setup

**Why fifth:** Reduces friction for k8s deployments where every plugin
needs manual config after install.

---

### 6. Profile inheritance
**Status:** Not started.

Profiles can't extend other profiles. Every new agent type copies
the full YAML instead of inheriting common settings.

**What to build:**
- `extends: base-researcher` field in profile metadata
- Merged tools, instructions, policies from parent
- Child overrides parent fields
- Registry resolves parent profiles on demand

---

### 7. Worker auto-registration with gateway
**Status:** Workers register with registry. Gateway registration is manual.

Workers should auto-register with the gateway on startup, not require
manual curl commands after deployment.

**What to build:**
- Worker reads `GATEWAY_URL` and registers at startup
- Heartbeat to gateway every 5 minutes (already heartbeats to registry)
- Deregister on shutdown
- Remove manual registration step from k8s deployment

---

### 8. Multi-provider support
**Status:** Only OpenAI-compatible API via proxy.

**What to build:**
- Native Anthropic provider (direct API, not proxied)
- Native Google Gemini provider
- Provider selection per profile
- Different models for different agent types

---

### 9. Reactive agent mesh
**Status:** NATS deployed. Message bus on workers exists. No reactive profiles.

**What to build:**
- Service-mode profiles that stay alive and listen for NATS events
- Grafana alert → NATS event → monitor agent reacts → spawns investigation
- Fully automated alert-to-email pipeline without human trigger

---

### 10. Enhanced dashboard
**Status:** Basic dashboard (workers, tasks, agents, refresh).

**What to build:**
- Task detail view with output, tool call trace, timing breakdown
- Real-time event feed via NATS/SSE (currently polling)
- Submit tasks from the UI
- Plugin and profile management
- Cost charts
- Session viewer for debugging

---

### 11. Marketplace
**Status:** Not started.

**What to build:**
- Public registry at registry.bitop.dev
- Community plugin and profile submissions
- Rating and download counts
- Verified publishers

---

## Completed items (moved from previous roadmap)

- ~~Scheduled tasks~~ → Gateway v0.1.0
- ~~Webhook triggers~~ → Gateway v0.1.0
- ~~Task history~~ → Gateway v0.1.0 (PostgreSQL)
- ~~Worker observability~~ → Gateway dashboard + NATS events
- ~~Smart routing~~ → Gateway capability-based routing
- ~~Web dashboard~~ → Gateway v0.3.0 (embedded)
- ~~Gateway-level parallel~~ → Gateway v0.3.1
- ~~Retries~~ → Gateway v0.3.2
