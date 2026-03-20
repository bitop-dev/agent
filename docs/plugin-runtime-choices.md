# Plugin Runtime Choices

Use this guide when deciding how a new plugin should run.

## Quick rule

- if you want to wrap an existing CLI or local script, use `command`
- if you are building a custom HTTP service integration, use `http`
- if an MCP server already exists, use `mcp`
- if the plugin only contributes assets, use `asset`
- if the plugin needs bounded access to host runtime behavior, use `host`

## Decision table

| Runtime | Best for | Write separate code? | Typical examples |
| --- | --- | --- | --- |
| `asset` | prompts, profiles, policies | No | prompt packs, policy templates |
| `http` | custom integrations | Yes | web search, web fetch, email, Grafana |
| `mcp` | existing MCP ecosystem servers | Usually yes, but external | filesystem, database, external tool servers |
| `host` | trusted orchestration | No separate service required | spawn-sub-agent |
| `command` | existing CLIs, scripts, custom binaries | No separate service | `gh`, `jq`, Python scripts, compiled tools |
| `rpc` | richer sidecars | Yes | typed local/remote integrations |

## Recommended defaults

### Choose `http` when

- you are building your own plugin runtime
- you want the simplest implementation path
- your integration talks to web APIs or external services

### Choose `mcp` when

- an MCP server already exists
- you want to reuse MCP tooling and discovery

### Choose `command` when

- you want to wrap an existing CLI tool (e.g., `gh`, `kubectl`, `jq`, `curl`)
- you have a custom binary or script (Go, Python, Bash, etc.) that should be a tool
- you do not want to run a persistent HTTP server
- the tool runs locally and produces output on stdout

The `command` runtime supports two modes:

**Argv-template mode** -- for wrapping existing CLIs. Define `execution.argv` with
`{{placeholder}}` templates that get expanded from the tool's arguments:

```yaml
execution:
  mode: command
  argv: ["pr", "list", "--repo", "{{repo}}", "--state", "{{state}}"]
```

If a placeholder resolves to empty and the preceding element is a flag (`--flag`),
both are omitted automatically, making optional arguments work naturally.

**JSON-stdin/stdout mode** -- for custom binaries and scripts. Omit `execution.argv`
and the runtime sends a JSON object on stdin and reads a JSON response from stdout:

```
stdin:  {"plugin":"...","tool":"...","operation":"...","arguments":{...},"config":{...}}
stdout: {"output":"...","data":{...}}  -- or {"error":"..."}
```

If the command returns non-JSON text, it is treated as plain text output.

In both modes, plugin config values are injected as `AGENT_PLUGIN_<KEY>` environment
variables. In argv templates, use `{{config.key}}` to reference config values directly.

### Choose `host` when

- the plugin needs access to controlled framework internals
- the behavior is orchestration, not just external I/O

## Examples

### `send-email`

- runtime: `http`
- why: external integration, credentials, side effects, easy separation from core

### `web-research`

- runtime: `http`
- why: straightforward search/fetch service, no need for privileged host access

### `spawn-sub-agent`

- runtime: `host`
- why: it needs bounded access to the host runtime to create sub-runs

### `mcp-filesystem`

- runtime: `mcp`
- why: it bridges to an external MCP server rather than reimplementing the tool logic inside the core framework

### `github-cli`

- runtime: `command` (argv-template mode)
- why: wraps the existing `gh` CLI, no new binary or server needed
- the agent calls `gh pr list --repo owner/name --json ...` directly

### `python-tool`

- runtime: `command` (JSON-stdin/stdout mode)
- why: custom Python script that reads JSON from stdin, returns JSON on stdout
- no HTTP server to deploy, just a script in the plugin bundle
