# Roadmap

## Current state (v0.4.5)

Distributed agent platform deployed in k8s with 7 pods. All core features
implemented, tested, and running in production.

### Completed

Everything in the platform is built, wired, and tested:

- Core framework, plugins, profiles, tools, sessions, policy, approvals
- On-demand plugin + profile install from registry
- HTTP workers (blank, self-bootstrapping, pod IP registration)
- MCP server mode for external clients
- Agent discovery, structured handoff, pipelines, checkpoints
- Sequential + parallel sub-agents + gateway-distributed parallel
- Gateway: routing, auth, webhooks, scheduling, retries, dead worker eviction
- NATS event bus, SSE stream, web dashboard
- Agent memory (agent/remember, agent/recall, PostgreSQL)
- Cost tracking (models.dev pricing, token usage pipeline)
- Model fallback chain (stream error detection, auto-advance)
- Plugin config from environment (EnvVar, Default, convention)
- Profile inheritance (extends with merge)
- Anthropic native provider
- Reactive triggers (service-mode profiles → gateway webhooks)
- CI/CD, Docker images, k8s deployment

### Remaining

1. **Enhanced dashboard** — task detail views, submit from UI, SSE real-time, cost charts
2. **Marketplace** — public registry, community plugins/profiles
3. **Note:** Cost tracking shows $0 for self-hosted models (UFL proxy doesn't return usage tokens) — revisit when using direct API keys
