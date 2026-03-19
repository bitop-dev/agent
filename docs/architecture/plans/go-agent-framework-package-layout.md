# Go Agent Framework Package Layout

## Goal

This document revises the package layout for the framework after one important decision:

- profiles should be loaded, not baked in
- tools should start small
- new capabilities should arrive through installable plugins or external integrations

So the framework is not a built-in set of agent types like `code`, `research`, or `monitor`.
It is a general runtime that can load those behaviors later.

## Core stance

We are building:

- a local-first, policy-aware Go runtime
- a generic CLI host for running agent profiles
- a small built-in tool set
- a plugin system for expanding agent capabilities over time

The first bundled profile may be coding-oriented, but that is an example and proving ground, not a hardcoded architectural identity.

## Design principles

### 1. Runtime is generic

The runtime should not know whether it is running:

- a coding agent
- a research agent
- a monitoring agent
- something we have not thought of yet

It should only know how to:

- run turns
- call providers
- execute tools
- enforce policy
- request approvals
- persist sessions
- emit events

### 2. Profiles are loaded assets

Profiles define behavior by configuration and references.

They should be loadable from disk, registries, plugin bundles, or remote sources later.

That means no baked-in package layout like:

- `internal/app/code`
- `internal/app/research`
- `internal/app/monitor`

Instead we should have:

- profile contracts
- profile loaders
- profile registries

### 3. Plugins expand capability

The framework should ship with a tiny built-in tool surface.

Everything else should be addable through plugins or external tool bridges.

Examples of expansion paths:

- install a Grafana plugin
- install a browser plugin
- connect MCP servers
- add a research-tool bundle

### 4. Public packages stay small

Only stable contracts belong in `pkg/`.

Fast-moving implementations, loaders, registries, CLI flows, and persistence backends belong in `internal/`.

### 5. Policy remains first-class

Even in a plugin-driven world, every dangerous action still flows through policy boundaries.

That includes:

- file writes
- shell commands
- network access
- plugin-contributed tools
- remote integrations

## Recommended repository shape

```text
.
├── cmd/
│   └── agent/
├── internal/
│   ├── cli/
│   ├── loader/
│   ├── plugin/
│   ├── profile/
│   ├── registry/
│   ├── runtime/
│   ├── service/
│   └── store/
├── pkg/
│   ├── approval/
│   ├── config/
│   ├── events/
│   ├── plugin/
│   ├── policy/
│   ├── profile/
│   ├── provider/
│   ├── runtime/
│   ├── session/
│   ├── tool/
│   └── workspace/
├── profiles/
├── plugins/
├── test/
│   ├── e2e/
│   ├── fixtures/
│   └── integration/
└── docs/
```

## Package responsibilities

## `cmd/agent`

This is the main executable host.

Responsibilities:

- parse commands
- load config
- load profiles
- discover plugins
- bootstrap registries
- start interactive or non-interactive runs

It should be wiring only.

## `pkg/` public framework surface

### `pkg/runtime`

Defines the generic agent execution contract.

Responsibilities:

- run request and result types
- turn orchestration contracts
- execution context contracts
- runtime interfaces

This package should not know any specific profile or plugin implementation.

### `pkg/events`

Defines the typed event model shared by runtime, tools, policy, approvals, and sessions.

Examples:

- run started
- assistant delta
- tool requested
- tool completed
- approval requested
- policy blocked
- session checkpointed

### `pkg/tool`

Defines tool interfaces and shared types.

Responsibilities:

- tool contract
- schema and argument model
- result model
- registry contract

This package defines tools as capabilities, not implementations.

### `pkg/provider`

Defines model provider contracts.

Responsibilities:

- provider interface
- completion request model
- streaming event model
- model references
- credential source contracts

### `pkg/policy`

Defines all policy checks across the runtime.

Responsibilities:

- check requests
- decisions
- risk levels
- policy engine interface

Every tool, including plugin-contributed tools, should pass through this boundary.

### `pkg/approval`

Defines approval workflows independent of UI.

Responsibilities:

- approval request model
- approval decision model
- approval resolver interface

The CLI can implement one resolver, and a future remote gateway can implement another.

### `pkg/session`

Defines session and replay contracts.

Responsibilities:

- session metadata
- session entries
- append and load contracts
- checkpoints and branching model

### `pkg/profile`

Defines the loaded profile contract.

Responsibilities:

- profile structure
- profile source abstraction
- validation rules
- profile resolver contracts

This package is important because the runtime should consume a generic profile, not a hardcoded app type.

### `pkg/plugin`

Defines what plugins are allowed to contribute.

Responsibilities:

- plugin manifest model
- plugin capability model
- contribution contracts
- lifecycle hooks for registration only

This is not Go's native `plugin` package. It is our framework-level plugin model.

### `pkg/config`

Defines config loading and merge rules.

Responsibilities:

- root config structure
- environment overrides
- profile/plugin enablement config
- validation and merge logic

### `pkg/workspace`

Defines workspace detection and safe path semantics.

Responsibilities:

- workspace root resolution
- safe relative path handling
- repo boundary helpers

This package is shared by tools and policy.

## `internal/` implementation packages

### `internal/runtime`

Concrete implementation of the runtime contracts.

Responsibilities:

- run loop
- tool dispatch
- provider streaming fan-out
- retries
- context management
- event emission

### `internal/profile`

Concrete profile loading and assembly.

Responsibilities:

- load profile manifests
- resolve referenced tools and providers
- merge profile defaults with user config
- validate profile wiring before execution

### `internal/plugin`

Concrete plugin discovery and registration.

Responsibilities:

- discover installed plugins
- load plugin manifests
- register contributions into registries
- enforce plugin compatibility rules

### `internal/loader`

Shared loading utilities used by profile and plugin systems.

Responsibilities:

- read manifests from disk
- validate versions
- resolve references
- manage local cache if needed later

### `internal/registry`

Owns runtime registries.

Responsibilities:

- tool registry
- provider registry
- approval resolver registry
- profile registry
- plugin contribution registry

This is where concrete implementations get wired together.

### `internal/store`

Concrete persistence backends.

Suggested subpackages:

- `internal/store/sqlite`
- `internal/store/jsonl`
- `internal/store/fs`

Use this for:

- sessions
- checkpoints
- audit logs
- local plugin metadata

### `internal/service`

Application orchestration layer.

Responsibilities:

- run service
- session service
- profile resolution service
- plugin installation service
- diagnostics service

These services compose framework packages for the CLI host.

### `internal/cli`

The first host shell around the runtime.

Responsibilities:

- commands
- interactive loop
- text rendering
- streaming output
- approval prompts
- install/list/show profile and plugin commands

## `profiles/`

Holds bundled example profiles and local profile assets.

Examples:

- `profiles/coding/`
- `profiles/research/`
- `profiles/grafana-ops/`

The key point is that these are data-driven profile assets, not baked-in application packages.

## `plugins/`

Holds bundled or installed plugins.

Examples:

- `plugins/core-tools/`
- `plugins/browser/`
- `plugins/grafana/`
- `plugins/mcp-bridge/`

Some plugins may be local bundles, while others may eventually be installed from registries.

## Small built-in surface

The framework should start with a minimal built-in capability set.

Recommended built-ins for v0.1:

- profile loader
- plugin loader
- config loader
- session persistence
- provider abstraction
- policy engine
- approval flow
- only a very small base tool set

Recommended base tools:

- `read`
- `write`
- `edit`
- optional `bash`, but behind strict policy and approval

Everything else should be installed or bridged in.

## Dependency rules

### Allowed directions

- `cmd/*` -> `internal/*` and `pkg/*`
- `internal/cli` -> `internal/service`, `internal/profile`, `internal/plugin`, `pkg/*`
- `internal/service` -> `internal/runtime`, `internal/registry`, `internal/store`, `pkg/*`
- `internal/profile` -> `internal/loader`, `internal/registry`, `pkg/profile`, `pkg/config`
- `internal/plugin` -> `internal/loader`, `internal/registry`, `pkg/plugin`, `pkg/tool`, `pkg/provider`
- `internal/runtime` -> `pkg/runtime`, `pkg/tool`, `pkg/provider`, `pkg/policy`, `pkg/events`, `pkg/session`, `pkg/approval`

### Forbidden directions

- `pkg/*` must not import `internal/*`
- `pkg/runtime` must not know about any concrete profile names
- `pkg/runtime` must not depend on CLI concerns
- `pkg/tool` must not import concrete tool implementations
- `pkg/profile` must not hardcode bundled profile identities
- `pkg/plugin` must not assume one plugin transport mechanism only

## Recommended v0.1 package set

### Build now

- `cmd/agent`
- `pkg/runtime`
- `pkg/events`
- `pkg/tool`
- `pkg/provider`
- `pkg/policy`
- `pkg/approval`
- `pkg/session`
- `pkg/profile`
- `pkg/plugin`
- `pkg/config`
- `pkg/workspace`
- `internal/runtime`
- `internal/profile`
- `internal/plugin`
- `internal/loader`
- `internal/registry`
- `internal/store/sqlite`
- `internal/service`
- `internal/cli`

### Wait until later

- remote gateway packages
- browser automation packages
- monitoring-specific services
- research-specific services
- web UI
- external SDKs

Those should arrive as profiles, plugins, or optional host modes after the core framework is stable.

## Why this layout is better

This revised layout keeps the core honest:

- the runtime stays general
- profiles stay loadable
- tools stay modular
- plugins stay installable
- new agent behaviors become configuration plus capabilities, not framework rewrites

That is the right architecture if the goal is to let you build whatever agent you want later.
