# Roadmap — agent

## Current state

The core framework is stable. v0.1.0 and v0.2.0 are released. All foundational features are in place: runtime, CLI, plugins, profiles, sessions, MCP, policy, and registry integration. Remote search, install, upgrade, publish, version pinning, and plugin dependency enforcement are all done.

---

## Near term

### `--source` filter on `plugins search`
`plugins search` currently queries all configured sources and combines results. Add source filtering:
```bash
agent plugins search email --source official
```
The `--source` flag already exists on `plugins install` — the same pattern applies here.

### Profile sharing via registry
`profiles install` currently only accepts a local path. Extend it to support registry-backed profiles:
```bash
agent profiles search research
agent profiles install research-profile --source official
```
Profiles are installable units just like plugins — they should be searchable and installable from a registry source.

---

## Medium term

### Improved provider support
- Anthropic native API (currently requires an OpenAI-compatible proxy)
- Google Gemini via OpenAI-compatible endpoint
- Bedrock transport

### Transitive plugin dependency resolution
`requires.plugins` is currently enforced at registration (missing dep = error). The next step is auto-installing declared dependencies at install time, similar to how package managers handle `requires`.

---

## Long term

### Web UI
Browser-based interface for:
- Viewing and searching session history
- Running agents without the CLI
- Managing plugins and profiles

### Multi-agent coordination
Structured protocols for agents to discover and delegate to each other beyond the current `spawn-sub-agent` model.
