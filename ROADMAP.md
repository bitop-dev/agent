# Roadmap

## Current state (v0.3.x)

The framework is a working distributed agent system. Blank workers in k8s
pull profiles and plugins from a registry on demand, execute tasks using
LLM-driven tool chains, and return results. Multi-agent orchestration
(discovery, delegation, pipelines) works end to end.

What's deployed:
- Registry server with plugin + profile publishing
- Dynamic HTTP workers with on-demand install
- MCP server mode for external clients (opencode, Claude Desktop)
- Agent discovery across local profiles and registry
- Sub-agent orchestration (sequential, parallel, pipeline)
- Session compaction, message bus, worker registration

---

## Near term — operational improvements

### Multi-arch plugin builds
Plugins with compiled binaries (ddg-research, grafana-alerts, slack) fail
when the binary was built for a different OS/arch than the worker. Need:
- CI workflow in agent-plugins that builds linux/amd64 + darwin/arm64
- Plugin tarball includes both, worker selects the right one at install
- Or: plugins declare a `build` command and workers compile on install

### Provider resilience
LLM API calls from k8s are slower and less reliable than local. Need:
- Configurable retry with exponential backoff (partially exists)
- Request timeout per-model (some models are slower)
- Model fallback chain — if primary model times out, try a secondary
- Provider health check at startup

### Plugin config from environment
Plugins like send-email need config (SMTP host, credentials) that currently
requires `plugins config set` commands. In k8s, config should come from:
- Environment variables mapped via envMapping (exists but not used for all plugins)
- Secrets mounted as config
- Default config values in plugin.yaml so plugins work with minimal setup

### Worker observability
No visibility into what workers are doing without `kubectl logs`. Need:
- `GET /v1/status` — current task, profile, duration, tool calls in progress
- `GET /v1/tasks/history` — last N completed tasks with results and timing
- Structured JSON logs with task IDs for correlation
- Prometheus metrics: tasks completed, duration histogram, errors, cache hits

### Task history and audit
Task results disappear after the HTTP response. Need:
- Workers store completed tasks in a local SQLite or send to a central store
- `GET /v1/tasks` — list recent tasks
- `GET /v1/tasks/{id}` — get full result, tool call trace, timing
- Retention policy (keep last 100 tasks or last 7 days)

---

## Medium term — workflow features

### Scheduled tasks (cron)
Run agents on a schedule without external triggers:
```yaml
# schedule.yaml
schedules:
  - name: daily-ops-report
    cron: "0 8 * * *"
    profile: grafana-alert-summary
    task: "Generate daily ops report for team ict-aipe. Send to nick@bitop.dev"
    context:
      team: ict-aipe
```
Workers pick up scheduled tasks from the registry or a shared schedule store.

### Webhook triggers
External systems trigger agent tasks:
- `POST /v1/webhook/slack` — Slack message → agent task
- `POST /v1/webhook/github` — GitHub event → agent task
- `POST /v1/webhook/grafana` — Grafana alert → agent task
- `POST /v1/webhook/generic` — any JSON payload → agent task with template

Example: Grafana fires an alert → webhook hits worker → worker runs
grafana-researcher to investigate → emails the team.

### Profile inheritance and composition
Profiles should be composable:
```yaml
# base-researcher.yaml — shared research behavior
metadata:
  name: base-researcher
spec:
  tools:
    enabled: [ddg/search, ddg/fetch]

# security-researcher.yaml — extends base
metadata:
  name: security-researcher
  extends: base-researcher
spec:
  instructions:
    system:
      - base-researcher/system    # inherit base prompt
      - ./prompts/security.md     # add security focus
  tools:
    enabled:
      - ddg/search     # inherited
      - ddg/fetch      # inherited
      - cve/search     # additional
```

### Agent memory / persistent knowledge
Agents forget everything between tasks. For long-term projects:
- Per-profile knowledge store (key-value or vector)
- `agent/remember` tool — store a fact for future tasks
- `agent/recall` tool — retrieve relevant facts from memory
- Memory shared across all tasks using the same profile

### Cost tracking
Track LLM token usage per task, profile, and model:
- Token count per request/response
- Cost estimate based on model pricing
- Budget limits per profile or per worker
- `GET /v1/costs` — usage dashboard data

---

## Long term — platform features

### Web dashboard
Browser UI for managing the agent system:
- Worker status (health, current task, uptime)
- Task history with search and filtering
- Agent/profile browser with live testing
- Plugin management (install, enable, configure)
- Session viewer for debugging agent behavior
- Cost and usage charts

### Multi-provider support
Different agents use different LLM providers:
```yaml
# Fast cheap researcher
spec:
  provider:
    default: openai
    model: gpt-4o-mini

# Precise expensive analyst
spec:
  provider:
    default: anthropic
    model: claude-4-sonnet
```
Requires native provider implementations beyond the current
OpenAI-compatible proxy.

### Agent marketplace
Community-contributed plugins and profiles:
- Public registry at registry.bitop.dev
- `agent plugins search --marketplace`
- Rating, downloads, verified publishers
- One-command install: `agent plugins install community/slack-bot`

### Reactive agent mesh
Long-lived agents that react to events (extends Phase 5d message bus):
- Monitor agent watches Grafana → detects anomaly
- Sends message to diagnostic agent
- Diagnostic investigates → sends message to report agent
- Report agent compiles findings → sends email
- All happens automatically without an orchestrator

### Smart routing
Registry-based task routing that matches tasks to the best available worker:
- Workers report their installed profiles and current load
- Registry routes tasks to workers with the right profiles
- Load-aware: prefer idle workers over busy ones
- Capability-aware: route grafana tasks to workers with VPN access

### Guardrails and safety
Production safety controls:
- Rate limiting per profile, per user, per worker
- Content filtering on inputs and outputs
- Maximum task duration enforcement
- Approval workflows for high-risk tasks (already exists, needs UI)
- Audit log for compliance

---

## Priority matrix

| Feature | Value | Effort | Priority |
|---|---|---|---|
| Multi-arch plugin builds | High | Medium | 1 |
| Provider resilience | High | Small | 2 |
| Worker observability | High | Medium | 3 |
| Task history | High | Medium | 4 |
| Plugin config from env | Medium | Small | 5 |
| Scheduled tasks | High | Medium | 6 |
| Webhook triggers | High | Medium | 7 |
| Cost tracking | Medium | Medium | 8 |
| Web dashboard | High | Large | 9 |
| Profile inheritance | Medium | Medium | 10 |
| Agent memory | Medium | Large | 11 |
| Multi-provider | Medium | Large | 12 |
| Smart routing | Medium | Large | 13 |
| Reactive mesh | Medium | Large | 14 |
| Marketplace | Low | Large | 15 |
