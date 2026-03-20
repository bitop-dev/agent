# Roadmap — agent

## Current state

The core framework is stable. v0.1.0 is released. All foundational features are in place: runtime, CLI, plugins, profiles, sessions, MCP, policy, and registry integration.

---

## Near term

### Multi-version registry support
Currently `plugins install <name>` always resolves to the latest version. Add version pinning:
```bash
agent plugins install send-email@0.2.0
```
- Version resolution in the registry client
- Store pinned version in plugin config
- `plugins upgrade` should respect pinned versions

### Plugin dependency enforcement
`requires.plugins` is declared in `plugin.yaml` but not checked at install time. When installing a plugin that declares dependencies, install them automatically or warn.

### `--source` on search
```bash
agent plugins search email --source official
```
Currently search combines all sources. Add source filtering.

---

## Medium term

### Profile sharing via registry
```bash
agent profiles install research-profile --source official
```
- Profile packages as a first-class registry concept
- `profiles search`, `profiles install`, `profiles upgrade`

### Agent-as-MCP-server
Expose the agent's tools over the MCP protocol so other clients (Claude Desktop, Cursor, etc.) can use an agent as a tool provider.

### Improved provider support
- Anthropic native API (currently requires OpenAI-compatible proxy)
- Google Gemini via OpenAI-compatible endpoint
- Bedrock transport

---

## Long term

### Web UI
Browser-based interface for:
- Viewing and searching session history
- Running agents without the CLI
- Managing plugins and profiles

### Plugin dependency graph
Resolve transitive plugin dependencies at install time, similar to how package managers handle `requires`.

### Multi-agent coordination
Structured protocols for agents to discover and delegate to each other beyond the current `spawn-sub-agent` model.
