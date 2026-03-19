# Go Agent Framework Plugin Specification

## Goal

This document defines the formal plugin model for the framework.

It answers:

- how plugins are structured
- how plugins are configured
- how plugins are installed and enabled
- how plugins become active at runtime
- whether plugins are hotloaded

This spec is designed to keep the core framework small while allowing capabilities like:

- email sending
- web search and fetch
- Grafana integrations
- sub-agent spawning
- prompt packs and profile templates

## Design goals

The plugin system should optimize for:

- local-first installation
- explicit capability declaration
- safe defaults
- low binary bloat
- portability across environments
- support for both simple asset bundles and executable integrations

The plugin system should avoid:

- arbitrary code execution during manifest load
- hidden capability injection
- requiring recompilation of the core binary for most extensions
- mutation of active sessions without explicit reload or restart behavior

## Core plugin principles

### 1. Plugins add capability, profiles select capability

Plugins contribute possible functionality.

Profiles decide which of that functionality is available to a specific agent run.

That means:

- plugin install does not imply automatic access
- plugin enable does not imply every profile can use it
- profile enablement still does not bypass policy

### 2. Plugin config is separate from plugin code or assets

Plugins declare what configuration they need.

Users provide configuration through agent config or environment variables.

Do not hardcode secrets or environment-specific values in plugin manifests.

### 3. Most plugins should not be compiled into the binary

The core binary should only include:

- the runtime
- the plugin loader
- the profile loader
- the policy engine
- the approval system
- session persistence
- a very small built-in tool set

Most integrations should be installable plugin bundles.

### 4. Active sessions should use a stable capability snapshot

A plugin change should not silently mutate a running session.

New sessions can see new plugin state.
Existing sessions should keep the tool and capability snapshot they started with unless explicitly reloaded in a controlled way later.

## Plugin categories

There are four plugin categories.

### Category A: Asset plugins

Purpose:

- contribute non-executable assets

Examples:

- prompt packs
- profile templates
- policy templates
- documentation helpers

These are the safest and simplest plugins.

### Category B: Integration plugins

Purpose:

- connect the framework to external systems

Examples:

- `send_email`
- `web_search`
- `grafana`
- `jira`
- `slack`

These usually expose one or more tools and require configuration.

### Category C: Bridge plugins

Purpose:

- connect to executable external runtimes or tool servers

Examples:

- MCP bridges
- gRPC or JSON-RPC sidecars
- browser automation sidecars

These allow rich capabilities without linking them into the core binary.

### Category D: Orchestration plugins

Purpose:

- change how work is delegated or coordinated inside the framework

Examples:

- `spawn_sub_agent`
- planner plugins
- batch task executors

These are more privileged than ordinary integrations.
They may require access to host runtime capabilities, not just external APIs.

## Plugin runtime modes

Each plugin must declare a runtime mode.

### `asset`

- no executable runtime
- contributes prompts, profiles, or policies only

### `http`

- plugin tools call an HTTP API directly
- good for services like search, email APIs, Grafana, or internal platforms

### `command`

- plugin launches a local executable or sidecar process
- good for local adapters and wrappers

### `mcp`

- plugin connects to an MCP server
- good for interoperable tool providers

### `rpc`

- plugin talks to a gRPC or JSON-RPC sidecar
- good for richer typed integrations or isolated privileged logic

### `host`

- plugin depends on controlled host runtime capabilities
- intended for trusted orchestration features like `spawn_sub_agent`

Important:

- `host` plugins should be treated as privileged
- they are still plugins conceptually, but they must not have unrestricted access to core internals

## Recommended v0.1 and v0.2 scope

### v0.1

- `asset`
- `http`
- `mcp`
- manifest-driven local bundles

### v0.2+

- `command`
- `rpc`
- selected `host` capabilities for trusted first-party orchestration plugins

## Plugin manifest

## Required manifest fields

```yaml
apiVersion: agent/v1
kind: Plugin
metadata:
  name: send-email
  version: 0.1.0
  description: Send email through SMTP or API providers

spec:
  category: integration

  runtime:
    type: http

  contributes:
    tools:
      - id: email/send
        path: tools/send.yaml
      - id: email/draft
        path: tools/draft.yaml

    prompts:
      - id: email/style-default
        path: prompts/style-default.md

    profileTemplates:
      - id: email/assistant
        path: profiles/email-assistant.yaml

  configSchema:
    type: object
    properties:
      provider:
        type: string
        enum: [smtp, resend, sendgrid]
      smtpHost:
        type: string
      smtpPort:
        type: integer
      username:
        type: string
      password:
        type: string
        secret: true
      from:
        type: string
    required:
      - provider

  permissions:
    network:
      outbound:
        - smtp
        - https
    sensitiveActions:
      - email/send

  requires:
    framework: ">=0.1.0"
    plugins: []
```

## Manifest sections

### `metadata`

- `name`
- `version`
- `description`

### `spec.category`

Allowed values:

- `asset`
- `integration`
- `bridge`
- `orchestration`

### `spec.runtime`

Defines how the plugin becomes executable or active.

Suggested shape:

```yaml
runtime:
  type: http | command | mcp | rpc | asset | host
  command: []
  endpoint: ""
  protocol: ""
```

Not every field is used for every runtime type.

### `spec.contributes`

Defines what the plugin adds to the system.

Possible contribution types:

- `tools`
- `providers`
- `prompts`
- `profileTemplates`
- `policies`
- `approvalAdapters`
- `eventObservers`
- `hostCapabilities` for privileged future cases

### `spec.configSchema`

Defines required user-supplied configuration.

This should be declarative and schema-driven.

It should support:

- strings
- booleans
- numbers
- enums
- arrays
- objects
- `secret: true` annotation

The framework should never assume a plugin can run without its required config.

### `spec.permissions`

Declares the expected capability footprint of the plugin.

Examples:

- network egress needs
- filesystem access expectations
- sensitive tools it contributes
- whether it can trigger sub-runs or host capabilities

This does not replace runtime policy enforcement, but it gives the loader and user visibility.

### `spec.requires`

Compatibility constraints:

- minimum framework version
- required companion plugins
- maybe minimum host capabilities later

## Contribution types in detail

### Tool contributions

Tool contributions are the most important plugin output.

A plugin tool contribution should define:

- tool ID
- description
- argument schema
- runtime binding information
- optional approval or risk hints

Example tool descriptor:

```yaml
id: web/search
description: Search the web using a configured provider
inputSchema:
  type: object
  properties:
    query:
      type: string
    topK:
      type: integer
  required:
    - query

execution:
  mode: http
  operation: search

risk:
  level: low
```

### Provider contributions

These should come later unless needed immediately.

Provider plugins could contribute:

- a model provider adapter
- provider prompt formatting helpers
- capability flags like tool-call support

### Prompt contributions

Useful for:

- style packs
- domain instructions
- incident playbooks
- email templates

### Profile template contributions

These make plugins easier to adopt.

Examples:

- `web-researcher`
- `grafana-ops`
- `email-assistant`

The user can install the plugin and then start from a provided profile template.

### Policy contributions

Useful for:

- safe defaults for plugin tools
- service-specific approval rules
- domain-specific allowlists

Example:

- `send_email` can recommend `require_approval` for `email/send`

## Plugin configuration model

Plugin config must be supplied outside the plugin bundle.

Recommended config locations:

- project config
- user config
- environment variables

Suggested config shape:

```yaml
plugins:
  send-email:
    enabled: true
    config:
      provider: smtp
      smtpHost: ${SMTP_HOST}
      smtpPort: ${SMTP_PORT}
      username: ${SMTP_USERNAME}
      password: ${SMTP_PASSWORD}
      from: ${SMTP_FROM}

  web-search:
    enabled: true
    config:
      provider: tavily
      apiKey: ${TAVILY_API_KEY}

  spawn-sub-agent:
    enabled: false
    config:
      maxDepth: 2
      maxAgents: 4
      allowedProfiles:
        - coding
        - readonly
```

## Config rules

- plugin config belongs to the host config, not the plugin bundle
- secrets should be supported through env expansion or secret stores later
- missing required config should make the plugin unavailable for active use
- unavailable plugin capabilities should fail clearly during validation or startup

## Installation model

Plugin lifecycle should be explicit.

### Install

Command:

- `agent plugins install /path/to/plugin`

Behavior:

- validate manifest
- validate contribution references
- validate compatibility constraints
- copy or link bundle into plugin directory
- do not automatically enable dangerous functionality

Optional v0.1 flag:

- `--link` for local development using symlinks

### Enable

Command:

- `agent plugins enable <name>`

Behavior:

- mark plugin enabled in config
- plugin becomes available to new runs

### Disable

Command:

- `agent plugins disable <name>`

Behavior:

- remove plugin from active config
- new runs no longer see it

### Remove

Command:

- `agent plugins remove <name>`

Behavior:

- uninstall bundle from managed plugin directory
- warn if profiles still reference it

## Managed plugin directories

Recommended locations:

- project-local: `./plugins/`
- user-level: `~/.agent/plugins/`

Later we can add:

- remote registry installs
- versioned cache directories
- signed bundles

## Plugin activation model

There are three states to keep distinct:

### Installed

- plugin bundle exists locally

### Enabled

- config says plugin may contribute capabilities

### Active in a run

- current profile actually uses the contributed capability
- runtime has registered the contribution and policy allows it

This separation is critical.

## Reload and hotloading behavior

My recommendation is:

- discover and validate plugins dynamically
- do not silently mutate active sessions

### v0.1 rule

- new runs see newly enabled plugins
- existing runs do not automatically reload capabilities

### Why

If a plugin changes during an active session:

- tool schemas may drift
- model-visible capabilities may no longer match runtime state
- policy assumptions may change mid-conversation

That is too dangerous and confusing.

### Recommended future behavior

Maybe later add:

- `agent plugins reload`

But even then:

- current live session should keep its original capability snapshot unless user explicitly starts a new session or performs a controlled reload step

So the short answer is:

- plugin metadata can be rediscovered quickly
- active tool availability should be stable per session
- no full hotloading into running sessions by default

## Trusted versus untrusted plugins

Not all plugins should be treated equally.

### Unprivileged plugins

Examples:

- web search
- web fetch
- email drafting
- prompt packs

These should only use declared runtime bridges and tool contracts.

### Privileged plugins

Examples:

- `spawn_sub_agent`
- planner plugins
- anything using host runtime capabilities

These should be:

- explicitly marked
- opt-in only
- likely first-party or trusted-only at first
- always subject to policy and approval boundaries

## Example plugin designs

### Example: `send_email`

Category:

- `integration`

Runtime:

- `http` or `command`

Contributions:

- `email/send`
- `email/draft`
- `email/templates`

Config needs:

- SMTP or email API credentials
- sender identity

Recommended policy:

- drafting allowed
- sending requires approval

### Example: `web_search`

Category:

- `integration`

Runtime:

- `http`

Contributions:

- `web/search`

Config needs:

- search provider API key

Recommended policy:

- low risk
- outbound HTTPS only

### Example: `web_fetch`

Category:

- `integration`

Runtime:

- `http`

Contributions:

- `web/fetch`
- `web/extract`

Config needs:

- timeout
- optional domain allowlist
- user agent settings

### Example: `spawn_sub_agent`

Category:

- `orchestration`

Runtime:

- `host`

Contributions:

- `agent/spawn`
- `agent/delegate`

Config needs:

- max depth
- max sub-agents
- allowed profiles
- token/time budgets

Important:

- this should be treated as privileged
- it should use a host capability API, not unrestricted runtime access

## Host capability API for privileged plugins

This is future-facing, but worth defining now.

Privileged orchestration plugins may need controlled access to host services like:

- create sub-run
- load profile by name
- restrict tool set
- cap time or token budget
- collect final result

That should happen through a bounded host capability interface, not by exposing internal runtime packages directly.

## Validation rules

A plugin validator should check:

- schema version
- manifest completeness
- contribution references exist
- runtime mode is supported
- config schema is valid
- compatibility requirements are satisfiable
- declared permissions are syntactically valid

Validation should happen:

- on install
- on explicit validate command
- on startup or plugin discovery when needed

## What the first implementation should support

To keep implementation honest, first registration work should support:

- manifest parsing
- plugin category and runtime typing
- config schema parsing
- tool contribution registration
- prompt contribution registration
- profile template registration
- policy snippet registration
- enabled-versus-installed distinction

Do not try to build immediately:

- remote registries
- signed bundle distribution
- arbitrary in-process executable plugins
- automatic hot mutation of active sessions
- general third-party privileged host plugins

## Recommended implementation sequence

### Phase 1

- update plugin manifest types in `pkg/plugin`
- extend config model for plugin config
- implement contribution registration for tools, prompts, profiles, and policies

### Phase 2

- implement plugin install, enable, disable, and remove commands
- support `--link` local development install

### Phase 3

- implement `http` and `mcp` runtime-backed plugin tools
- validate config schema against user config

### Phase 4

- design host capability API for trusted orchestration plugins
- prototype `spawn_sub_agent`

## Recommendation summary

The plugin system should behave like this:

- plugins are manifest-driven bundles
- plugins declare capabilities explicitly
- config is provided outside the plugin
- install and enable are separate
- most plugins are not compiled into the binary
- active sessions keep a stable capability snapshot
- privileged orchestration plugins exist, but through a controlled host API

That model supports the behavior you want while keeping the framework understandable, secure, and extensible.
