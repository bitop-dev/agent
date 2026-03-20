# Building Plugins

This document explains how plugins work in this framework and how to build one.

If you are still asking yourself questions like:

- "Am I writing a script or a binary?"
- "How does the agent get access to my tool?"
- "What files do I need?"

this is the right place to start.

## The mental model

There are three separate things involved in a plugin-based tool.

### 1. The agent framework

The core agent is responsible for:

- loading plugin manifests
- validating plugin config
- registering plugin contributions
- deciding which tools are available in a profile
- enforcing policy and approvals
- dispatching tool calls to the right runtime

The core framework does **not** need to contain the logic for every integration.

### 2. The plugin bundle

The plugin bundle is the declarative package that tells the framework:

- what the plugin is called
- what tools it contributes
- what config it needs
- what prompts/profile templates/policies it adds
- how the runtime should be reached

Typical bundle structure:

```text
my-plugin/
  plugin.yaml
  tools/
  prompts/
  profiles/
  policies/
```

This is the part you install with:

```bash
./bin/agent plugins install /path/to/my-plugin --link
```

### 3. The plugin runtime

The plugin runtime is the code that actually performs the work.

Depending on the runtime type, that could be:

- an HTTP service
- an MCP server
- a local executable or script (via `command` runtime)
- a privileged host capability bridge

For example:

- `send-email` has a separate runtime binary that talks to SMTP
- `web-research` has a separate runtime binary that handles `/web-search` and `/web-fetch`
- `spawn-sub-agent` is a plugin, but its runtime type is `host`, so it uses a controlled host capability API instead of an HTTP service

## How a plugin becomes usable by an agent

This is the full lifecycle.

### Step 1: Create the plugin bundle

You write:

- `plugin.yaml`
- one or more tool descriptor files
- optional prompts
- optional profile templates
- optional policy snippets

### Step 2: Build or run the plugin runtime

If your plugin uses `runtime.type: http`, `mcp`, `command`, or `rpc`, you also need a runtime implementation.

That runtime is not "embedded" into the core agent.
It runs separately and the framework talks to it.

### Step 3: Install the plugin bundle

```bash
./bin/agent plugins install /path/to/my-plugin --link
```

### Step 4: Configure the plugin

```bash
./bin/agent plugins config set my-plugin baseURL http://127.0.0.1:8092
./bin/agent plugins config set my-plugin apiKey local-test-key
```

### Step 5: Validate config

```bash
./bin/agent plugins validate-config my-plugin
```

### Step 6: Enable the plugin

```bash
./bin/agent plugins enable my-plugin
```

### Step 7: Use a profile that enables the tool IDs

The plugin may contribute tools, but the agent only gets access to them when the selected profile enables those tool IDs.

That means:

- installed plugin != available in all runs
- enabled plugin != automatically active in every profile

Profiles still decide what the agent can use.

## Plugin bundle vs plugin runtime

This is the key distinction that usually confuses people.

### Bundle

The bundle is metadata + assets.

Examples:

- `plugin.yaml`
- `tools/search.yaml`
- `prompts/research-style.md`
- `profiles/researcher.yaml`

### Runtime

The runtime is executable behavior.

Examples:

- a Go HTTP server
- an MCP server
- a sidecar process

So if you want to build `web_search` and `web_fetch`, then **yes**, in practice you are usually writing a separate service or binary that the agent calls.

## Runtime types

### `asset`

Use this when the plugin only contributes prompts, profile templates, or policies.

No runtime process is needed.

### `http`

Use this when your plugin runtime is an HTTP service.

This is the easiest choice if you are building a custom integration today.

Good for:

- web search
- web fetch
- email send/draft
- internal APIs
- Grafana/Jira/Slack integrations

### `mcp`

Use this when an MCP server already exists or you want to expose your tools through an MCP-compatible tool server.

Good for:

- filesystem tools
- database tools
- third-party tool servers already in the MCP ecosystem

### `command`

Use this when you want to wrap an existing CLI tool, a custom binary, or a script.

No persistent service required.
The agent executes the command, passes input, and captures output.

Good for:

- existing CLIs: `gh`, `kubectl`, `jq`, `curl`, `terraform`
- custom Go/Rust/C binaries
- Python, Bash, Ruby, or any scripting language

The `command` runtime supports two I/O modes:

**Argv-template mode** -- for existing CLIs that take arguments on the command line:

```yaml
runtime:
  type: command
  command: ["gh"]

# tool descriptor
execution:
  mode: command
  argv: ["pr", "list", "--repo", "{{repo}}", "--state", "{{state}}"]
```

Placeholders `{{name}}` are expanded from the tool's arguments.
If a value is empty and the preceding element is a flag (`--flag`), both are omitted.
Use `{{config.key}}` to reference plugin config values.

**JSON-stdin/stdout mode** -- for custom binaries and scripts:

```yaml
runtime:
  type: command
  command: ["python3", "scripts/my-tool.py"]

# tool descriptor (no argv = JSON mode)
execution:
  mode: command
  operation: my-operation
```

The agent sends JSON on stdin:

```json
{"plugin":"...","tool":"...","operation":"...","arguments":{...},"config":{...}}
```

The script returns JSON on stdout:

```json
{"output":"result text","data":{"key":"value"}}
```

Or an error: `{"error":"something went wrong"}`

If the script returns non-JSON text, it is used as plain text output.

In both modes, plugin config values are also set as environment variables with
the `AGENT_PLUGIN_` prefix (e.g., `AGENT_PLUGIN_APIKEY`).

### `host`

Use this only for trusted, privileged plugins that need bounded access to the host runtime.

Good for:

- `spawn-sub-agent`
- planner/delegation behaviors

Do **not** use this for ordinary integrations like search or email.

## Example: wrapping `gh` CLI as a plugin

This is the fastest way to give the agent access to an existing CLI tool.

### Plugin bundle

```text
github-cli/
  plugin.yaml
  tools/
    pr-list.yaml
    issue-list.yaml
    repo-view.yaml
```

### `plugin.yaml`

```yaml
apiVersion: agent/v1
kind: Plugin
metadata:
  name: github-cli
  version: 0.1.0
  description: GitHub CLI integration
spec:
  category: integration
  runtime:
    type: command
    command: ["gh"]
  contributes:
    tools:
      - id: gh/pr-list
        path: tools/pr-list.yaml
      - id: gh/issue-list
        path: tools/issue-list.yaml
```

### Tool descriptor (`tools/pr-list.yaml`)

```yaml
id: gh/pr-list
kind: tool
description: List pull requests in a GitHub repository
inputSchema:
  type: object
  properties:
    repo:
      type: string
      description: Repository in owner/name format
    state:
      type: string
      enum: [open, closed, merged, all]
  required: [repo]
execution:
  mode: command
  argv: ["pr", "list", "--repo", "{{repo}}", "--state", "{{state}}", "--json", "number,title,state,url"]
  timeout: 15
risk:
  level: low
```

### Install and use

```bash
./bin/agent plugins install ./github-cli --link
./bin/agent plugins enable github-cli
./bin/agent run --profile my-profile "list open PRs in ncecere/agent"
```

No HTTP server, no separate binary, no build step.

## Example: web search and fetch plugin

If you want a `web-search` / `web-fetch` plugin today, the clean pattern is:

### Plugin bundle

```text
web-research/
  plugin.yaml
  tools/
    search.yaml
    fetch.yaml
  prompts/
  profiles/
  policies/
```

### Runtime implementation

Write a separate HTTP service or binary that exposes endpoints like:

- `POST /web-search`
- `POST /web-fetch`

The agent does not need your source code directly.
It just needs:

- the plugin bundle installed
- the runtime address configured

Then the flow is:

```bash
./bin/agent plugins install ./my-plugin --link
./bin/agent plugins config set my-plugin baseURL http://127.0.0.1:8092
./bin/agent plugins validate-config my-plugin
./bin/agent plugins enable my-plugin
./bin/agent run --profile ./my-profile.yaml "search golang plugin architecture"
```

## Where plugin files live

Normal runtime discovery uses:

- project-local: `.agent/plugins/`
- user-level: `~/.agent/plugins/`

In this repository, example plugin bundles live under:

- `../agent-plugins/`

and example plugin runtimes live under:

- `_testing/runtimes/`

Those are development/testing assets, not special runtime locations.

## How agents get tools

An agent gets a tool only when all of these are true:

1. the plugin bundle is installed
2. the plugin config is valid
3. the plugin is enabled
4. the selected profile includes that tool ID
5. policy allows the tool call

That layered model is intentional.

## Recommended way to build a plugin

If you are building a new integration, start here:

1. write the plugin bundle
2. choose a runtime type
3. build the runtime implementation
4. install and configure locally
5. test with a profile that enables the tool IDs

For wrapping existing CLIs or scripts, I recommend `command` first.
For custom HTTP service integrations, I recommend `http`.

## Related docs

- `docs/plugins.md`
- `docs/plugin-http-example.md`
- `docs/mcp-bridge.md`
- `docs/examples/build-an-mcp-plugin.md`
- `docs/plugin-runtime-choices.md`
- `docs/examples/build-a-web-research-plugin.md`
- `docs/examples/build-a-send-email-plugin.md`
- `docs/plugin-author-checklist.md`
