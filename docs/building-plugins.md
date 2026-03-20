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

## What plugins can contribute beyond tools

A plugin is not limited to contributing tools. The `contributes` section of `plugin.yaml`
supports four types of contributions:

```yaml
spec:
  contributes:
    tools:            # callable tools
    prompts:          # reusable system prompt fragments
    profileTemplates: # suggested agent profiles
    policies:         # suggested policy overlay files
```

### Tools

Tools are the primary contribution. Each tool descriptor file defines the tool's
ID, description, input schema, execution mode, and risk level.

The tool ID is what the profile lists in `spec.tools.enabled` and what the model
sees in the API request. Tool IDs use the pattern `namespace/name`
(e.g. `email/send`, `ddg/search`, `core/read`).

```yaml
contributes:
  tools:
    - id: email/send
      path: tools/send.yaml
    - id: email/draft
      path: tools/draft.yaml
```

### Prompts

Prompt contributions are reusable system prompt fragments that any profile can
include by referencing the prompt's ID in `spec.instructions.system`.

```yaml
contributes:
  prompts:
    - id: email/style-default
      path: prompts/style-default.md
```

Once the plugin is installed and enabled, any profile can include this prompt:

```yaml
# profile.yaml
spec:
  instructions:
    system:
      - ./prompts/my-base.md
      - email/style-default     # resolved from the plugin registry by ID
```

The framework loads the file at the registered path and includes its content
in the system prompt. No hard-coded path is needed in the profile.

**Profiles are always in control of prompts.** Plugin prompts are never
auto-injected. A profile must explicitly include a prompt ID to use it.
If you do not reference `email/style-default`, it has no effect.

To override a plugin prompt, simply do not include its ID and write
your own instructions instead.

See `docs/prompts.md` for the full resolution order and examples.

### Profile templates

Profile templates are example agent configurations shipped with the plugin.
They show the recommended way to use the plugin and its tools.

```yaml
contributes:
  profileTemplates:
    - id: email/assistant
      path: profiles/email-assistant.yaml
```

Profile templates are **registered when the plugin is enabled** but are
**never auto-applied**. They are reference material only — suggestions from
the plugin author about how to compose the plugin into a useful agent.

You can:
- Copy the template as a starting point and modify it freely
- Ignore it entirely and write your own profile from scratch
- Use some of its suggestions while changing others

Your profile YAML is always the final word.

### Policies (sensitiveActions and overlay files)

Plugins influence policy in two distinct ways:

#### `permissions.sensitiveActions` — automatic

Any tool listed under `sensitiveActions` is automatically marked as requiring
approval when the plugin is enabled. No profile configuration needed.

```yaml
spec:
  permissions:
    sensitiveActions:
      - email/send   # always requires approval
```

This is the plugin author saying: "this tool has real-world side effects."

**Profiles can override this** by adding a policy overlay that sets the
tool's decision to `allow`:

```yaml
# profile's policy overlay
version: 1
rules:
  - id: allow-send-in-pipeline
    action: tool
    tool: email/send
    decision: allow     # profile overrides the plugin's require_approval
```

Profile overlay rules take precedence over `sensitiveActions`. See `docs/policy.md`.

#### `contributes.policies` — opt-in

Plugins can also ship suggested policy overlay files:

```yaml
contributes:
  policies:
    - id: email/default
      path: policies/default.yaml
```

These are registered but **not automatically applied**. The profile must
explicitly reference them:

```yaml
# profile.yaml
spec:
  policy:
    overlays:
      - /path/to/installed/plugin/policies/default.yaml
```

Or, more commonly, you copy the plugin's policy file into your profile directory
and reference it locally so your profile is self-contained.

---

## Full example: a plugin that contributes all four types

```yaml
# my-integration/plugin.yaml
apiVersion: agent/v1
kind: Plugin
metadata:
  name: my-integration
  version: 0.1.0
  description: Full-featured example plugin

spec:
  category: integration
  runtime:
    type: http

  contributes:
    tools:
      - id: my/search
        path: tools/search.yaml
      - id: my/send
        path: tools/send.yaml

    prompts:
      - id: my/style-guide
        path: prompts/style.md        # referenced as 'my/style-guide' in profiles

    profileTemplates:
      - id: my/standard-agent
        path: profiles/standard.yaml  # shown in 'agent doctor' output

    policies:
      - id: my/default-policy
        path: policies/default.yaml   # opt-in by profiles

  permissions:
    sensitiveActions:
      - my/send                        # auto-requires approval

  configSchema:
    type: object
    properties:
      baseURL:
        type: string
      apiKey:
        type: string
        secret: true
    required:
      - baseURL
```

A profile using this plugin might look like:

```yaml
apiVersion: agent/v1
kind: Profile
metadata:
  name: my-agent
  version: 0.1.0

spec:
  instructions:
    system:
      - ./prompts/base.md
      - my/style-guide         # includes the plugin's prompt by ID

  provider:
    default: openai
    model: gpt-4o

  tools:
    enabled:
      - my/search
      - my/send

  approval:
    mode: on-request           # pauses on my/send (flagged as sensitiveAction)

  policy:
    overlays:
      - ./policies/overrides.yaml  # local file, overrides my/send behavior if desired
```

---

## Override summary

| Contribution type | Auto-applied? | How to override |
|---|---|---|
| `sensitiveActions` | ✅ Yes — approval required | Profile overlay with `decision: allow` or `mode: always` |
| `contributes.policies` | ❌ No — opt-in only | Profile lists it in `policy.overlays` or ignores it |
| `contributes.prompts` | ❌ No — opt-in only | Profile references the ID, or writes its own instructions |
| `contributes.profileTemplates` | ❌ No — reference only | Copy and modify, or ignore entirely |

---

## Related docs

- `docs/plugins.md` — plugin CLI workflow (install, configure, enable)
- `docs/profiles.md` — complete profile reference
- `docs/policy.md` — policy system, overlay format, override precedence
- `docs/prompts.md` — system instructions and plugin prompt IDs
- `docs/plugin-http-example.md`
- `docs/mcp-bridge.md`
- `docs/examples/build-an-mcp-plugin.md`
- `docs/plugin-runtime-choices.md`
- `docs/examples/build-a-web-research-plugin.md`
- `docs/examples/build-a-send-email-plugin.md`
- `docs/plugin-author-checklist.md`
