# Roadmap

## Current state (v0.4.8)

Distributed agent platform deployed in k8s. All core features complete.
Marketplace live with 9 plugins and 6 profiles.

### Completed
- Core framework, plugins, profiles, tools, sessions, policy, approvals
- On-demand plugin + profile install from registry
- HTTP workers, MCP server, sub-agent orchestration
- Gateway: routing, auth, webhooks, scheduling, retries, costs, memory, dashboard
- OpenAI Responses API with multi-turn tool calls
- Configurable model resolution (config/CLI/env/per-profile)
- Marketplace with search, download counts, READMEs, browsable UI
- 9 plugins (Go binaries), 6 model-optional profiles
- CI/CD, Docker images, k8s deployment

### Remaining
1. **Multi-user support** — accounts, per-user config, team scoping
2. **Marketplace v2** — publisher accounts, ratings, trending
3. **Production hardening** — multi-arch builds, rate limiting, package signing
