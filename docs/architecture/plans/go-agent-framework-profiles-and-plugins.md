# Go Agent Framework Profiles And Plugins

## Goal

This document defines two critical extension concepts for the framework:

- `profiles`: how an agent is configured to behave
- `plugins`: how an agent gains new capabilities

The framework should stay small and generic.
Profiles and plugins are how it becomes useful for specific jobs.

## The distinction

### Profile

A profile answers:

- what kind of agent behavior do I want
- which tools are available
- which provider and model should be used by default
- what approval posture should apply
- what workspace or session constraints exist

Profiles shape behavior.

### Plugin

A plugin answers:

- what new capabilities can be added to the framework
- what tools, providers, prompts, policies, or hooks can be contributed

Plugins expand capability.

Profiles choose from available capability.

## Rule of thumb

- profile = configuration and composition
- plugin = contribution and extension

If something adds a new tool, provider, or integration, it is probably a plugin.
If something selects defaults and behavior for a use case, it is probably a profile.

## Profile model

## What a profile should contain

A profile should be a loadable manifest plus optional assets.

Minimum profile fields:

- profile name
- version
- description
- default system prompt or instruction sources
- provider selection rules
- model defaults
- enabled tool references
- approval defaults
- session defaults
- workspace requirements
- policy overlays

Recommended extended fields:

- output style hints
- retry and timeout defaults
- context compaction strategy
- memory mode references
- trigger definitions for non-interactive modes later

## Suggested profile structure

```yaml
apiVersion: agent/v1
kind: Profile
metadata:
  name: coding
  version: 0.1.0
  description: General coding assistant for local repositories

spec:
  instructions:
    system:
      - ./prompts/system.md
      - ./prompts/style.md

  provider:
    default: anthropic
    model: claude-sonnet

  tools:
    enabled:
      - core/read
      - core/write
      - core/edit
      - shell/bash

  approval:
    mode: on-request
    requireFor:
      - shell/bash
      - fs/delete

  workspace:
    required: true
    writeScope: workspace

  session:
    persistence: sqlite
    compaction: auto

  policy:
    overlays:
      - ./policy/default.yaml
```

## Profile behavior rules

Profiles should be able to:

- enable tools by reference
- override provider defaults
- apply policy overlays
- set approval posture
- choose session behavior

Profiles should not be able to:

- bypass policy engine internals
- execute arbitrary code during load
- mutate the runtime contract
- silently expand permissions beyond declared policy

That last part is important. Profiles compose behavior; they should not be executable code.

## Profile sources

V0.1 should support local profiles from predictable locations.

Suggested lookup order:

- explicit `--profile path-or-name`
- project-local `./profiles/`
- user-level `~/.agent/profiles/`
- bundled profiles inside the install

Later we can add:

- registry-based installs
- Git-based installs
- signed profile packages

## Profile validation

A profile loader should validate:

- schema version
- referenced tools exist
- referenced providers exist
- referenced policy overlays exist
- required workspace constraints are coherent
- unknown fields are rejected or warned on clearly

Profiles should fail fast at load time, not midway through a run.

## Plugin model

## What a plugin can contribute

Plugins should be able to contribute a bounded set of things.

Recommended contribution types:

- tools
- providers
- prompts or instruction bundles
- policy snippets or policy templates
- approval adapters
- profile templates
- event observers
- MCP bridge definitions

Not every plugin needs all of these.

## What a plugin should not do by default

Plugins should not be allowed to:

- replace core runtime internals silently
- bypass policy checks
- auto-enable dangerous tools without explicit install and enablement
- inject hidden prompts or capabilities without clear declaration

Plugins should declare what they contribute up front.

## Suggested plugin manifest

```yaml
apiVersion: agent/v1
kind: Plugin
metadata:
  name: grafana
  version: 0.1.0
  description: Grafana and Prometheus integration tools

spec:
  contributes:
    tools:
      - id: grafana/query-panels
        entrypoint: tools/query_panels.yaml
      - id: grafana/query-alerts
        entrypoint: tools/query_alerts.yaml

    prompts:
      - id: grafana/incident-analysis
        path: prompts/incident-analysis.md

    profileTemplates:
      - id: grafana/ops
        path: profiles/grafana-ops.yaml

  requires:
    framework: ">=0.1.0"
    plugins: []
```

The exact format can change, but the pattern should hold:

- explicit manifest
- explicit contributions
- explicit compatibility requirements

## Plugin loading strategies

We should not depend primarily on Go native dynamic plugins.

Recommended plugin mechanisms:

### 1. Manifest-driven local bundles

Best for v0.1.

Each plugin is a directory with:

- manifest
- prompts
- profile templates
- tool descriptors
- config or policy snippets

This is simple and portable.

### 2. External tool bridges

Use MCP, gRPC, or JSON-RPC for plugins that need executable logic outside the process.

Best for:

- browser automation
- SaaS integrations
- isolated privileged operations
- non-Go extensions

### 3. In-process compiled modules

Useful for first-party or advanced custom builds, but should not be the main install story.

Reason:

- ABI portability is painful
- distribution gets harder
- cross-platform behavior gets brittle

## Plugin lifecycle

V0.1 lifecycle should be intentionally simple.

Suggested lifecycle:

1. discover plugin
2. validate manifest
3. register contributions
4. make contributions available to profiles
5. enable only when config or profile references them

Important:

- install is not the same as enable
- available is not the same as allowed
- allowed is still filtered by policy

## Plugin contribution boundaries

To keep the framework safe and understandable:

- plugins contribute capabilities
- profiles select capabilities
- policy controls execution
- approvals gate risky actions

That separation is the architecture.

## Minimal built-in tools

The initial framework should keep built-ins small.

Recommended built-ins:

- `core/read`
- `core/write`
- `core/edit`
- maybe `core/bash` with strict policy

Possible early plugin packs:

- `plugin/web`
- `plugin/mcp`
- `plugin/browser`
- `plugin/grafana`
- `plugin/git`

This gives us a small trusted base with optional expansion.

## Example flow

### Example 1: coding assistant

- install `core-tools`
- load `coding` profile
- profile enables `read`, `write`, `edit`, `bash`
- policy limits writes to workspace and shells to approved operations

### Example 2: research assistant

- install `web-tools`
- load `research` profile
- profile enables web fetch, source ranking, note synthesis
- no shell tool needed at all

### Example 3: Grafana monitoring assistant

- install `grafana` plugin
- load `grafana-ops` profile
- profile enables Grafana query tools and alert analysis prompt bundle
- policy disables file write and enables only specific outbound API access

That is exactly why profiles and plugins should be separate.

## V0.1 recommendation

For the first milestone, keep it narrow.

Build:

- one profile schema
- one plugin schema
- local discovery for both
- a tiny built-in tool set
- explicit plugin install and enable flow

Do not build yet:

- full remote plugin registries
- arbitrary executable plugin code during profile load
- hot-reload of all extensions everywhere
- a giant marketplace protocol

## Recommendation summary

If we want a truly general framework:

- profiles should be declarative
- plugins should be explicit and bounded
- core tools should stay tiny
- new agent behaviors should be assembled, not hardcoded

That gives you the freedom to create whatever agent you want later without turning the framework itself into a giant monolith.
