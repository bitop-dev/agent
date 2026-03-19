# Go Agent Framework v0.1 Feature List

## Goal

This document turns the revised architecture into a realistic `v0.1` milestone.

It assumes:

- the runtime is generic
- profiles are loaded assets
- plugins expand capabilities
- built-in tools stay intentionally small
- the first shipping host is a CLI

The point of `v0.1` is not to be broad.
The point is to prove the core framework shape is correct.

## v0.1 product definition

`v0.1` should be:

- a local-first Go agent runtime
- a CLI host that can load and run profiles
- a small built-in tool set
- a local plugin discovery model
- a policy and approval system that already matters

`v0.1` should not try to be:

- a marketplace
- a multi-tenant gateway
- a full personal-assistant platform
- a browser automation suite
- a channel integration product

## Success criteria

We should call `v0.1` successful if a user can:

- install one binary
- load a local profile
- run a session against a supported model provider
- use a tiny safe tool set
- get approval prompts for risky actions
- persist and resume sessions locally
- install a local plugin that adds new tools or profile templates

If those things work well, the framework is real.

## Required v0.1 capabilities

## 1. Runtime core

Status: required

Packages:

- `pkg/runtime`
- `internal/runtime`
- `pkg/events`

Must include:

- single-agent run loop
- turn-based execution model
- provider streaming support
- tool-call detection and dispatch
- typed event emission for the full run lifecycle
- cancellation support through context
- bounded retry behavior for provider or tool failures

Must not include yet:

- multi-agent orchestration
- planner trees
- swarm mode
- distributed task runners

Definition of done:

- one prompt can go through provider -> tool call -> tool result -> final assistant response
- all major lifecycle steps emit typed events

## 2. Event model

Status: required

Packages:

- `pkg/events`

Must include:

- `run_started`
- `run_finished`
- `turn_started`
- `turn_finished`
- `assistant_delta`
- `tool_requested`
- `tool_started`
- `tool_finished`
- `policy_decision`
- `approval_requested`
- `approval_resolved`
- `session_saved`
- `error`

Definition of done:

- CLI can render a run entirely from event flow rather than hidden runtime state

## 3. Provider abstraction

Status: required

Packages:

- `pkg/provider`
- `internal/registry`

Must include:

- provider interface
- model reference abstraction
- streaming completion contract
- credential loading from environment or config
- at least one production provider implementation

Recommended for `v0.1`:

- Anthropic first
- OpenAI second if easy

Must not include yet:

- every provider under the sun
- provider marketplace features
- remote provider credential sync

Definition of done:

- profile can reference a provider and model
- runtime can stream a real response from at least one provider

## 4. Tool system

Status: required

Packages:

- `pkg/tool`
- `internal/registry`

Must include:

- tool interface
- tool schema and argument validation
- tool registry
- tool execution result model
- tool IDs by stable namespace, such as `core/read`

Built-in tools for `v0.1`:

- `core/read`
- `core/write`
- `core/edit`
- optional `core/bash` if approval and policy land first

Nice to have, not required:

- `core/glob`
- `core/grep`

Must not include yet:

- giant built-in tool catalog
- plugin-contributed executable code that bypasses validation

Definition of done:

- profile can enable tools by ID
- runtime can validate, dispatch, and record tool usage cleanly

## 5. Policy engine

Status: required

Packages:

- `pkg/policy`

Must include:

- central policy check interface
- decisions like `allow`, `deny`, `require_approval`
- file path scope checks
- shell risk classification if `core/bash` exists
- plugin-contributed tool checks through the same policy interface

Initial policy scope:

- read inside workspace
- write inside workspace
- deny writes outside workspace
- shell disabled by default or approval-gated
- network disabled unless explicitly allowed by tool policy

Must not include yet:

- full sandbox containers
- kernel-level isolation
- network proxy enforcement

Definition of done:

- dangerous tool execution cannot happen without policy evaluation

## 6. Approval flow

Status: required

Packages:

- `pkg/approval`
- `internal/cli`

Must include:

- approval request model
- approval resolver interface
- CLI approval prompt implementation
- profile-configurable approval mode

Initial approval cases:

- shell command execution
- overwrite of existing files
- deletes if delete tools exist later
- any policy rule marked `require_approval`

Approval modes for `v0.1`:

- `never`
- `on-request`
- `always`

Definition of done:

- runtime pauses safely for approval and resumes deterministically after a decision

## 7. Session persistence

Status: required

Packages:

- `pkg/session`
- `internal/store/sqlite`

Must include:

- create session
- append entries
- load session
- resume most recent session
- save enough structured state to replay prior turns

Recommended storage choice:

- SQLite only for `v0.1`

Nice to have:

- JSONL export for debugging

Must not include yet:

- cross-device sync
- cloud session storage
- complex branch management UI

Definition of done:

- user can exit and resume later with context intact

## 8. Profile system

Status: required

Packages:

- `pkg/profile`
- `internal/profile`
- `profiles/`

Must include:

- one declarative profile schema
- local profile discovery
- explicit profile selection by name or path
- profile validation before execution
- support for referenced prompts, tools, provider defaults, approval defaults, and policy overlays

Bundled examples for `v0.1`:

- one coding-oriented profile
- maybe one minimal read-only profile for testing

Important:

- bundled examples are not special runtime cases
- they load through the same profile loader as user profiles

Definition of done:

- a user can add a new local profile without recompiling the binary

## 9. Plugin system

Status: required, but intentionally narrow

Packages:

- `pkg/plugin`
- `internal/plugin`
- `plugins/`

Must include:

- one plugin manifest schema
- local plugin discovery from known directories
- plugin validation and compatibility checks
- registration of plugin contributions into registries
- explicit enablement through config or profile references

Supported contribution types in `v0.1`:

- tool descriptors
- prompt bundles
- profile templates
- policy snippets

Optional in `v0.1` if easy:

- provider contributions

Must not include yet:

- remote plugin marketplace
- signed plugin registry
- hot upgrade daemon
- arbitrary code execution during plugin load

Definition of done:

- user can install or drop in a local plugin and have it contribute at least one new tool or profile template

## 10. CLI host

Status: required

Packages:

- `cmd/agent`
- `internal/cli`
- `internal/service`

Must include:

- interactive chat mode
- run with explicit profile
- resume previous session
- list profiles
- inspect profile
- list installed plugins

Recommended minimal commands:

- `agent run --profile <name|path>`
- `agent chat --profile <name|path>`
- `agent resume [session-id]`
- `agent profiles list`
- `agent profiles show <name|path>`
- `agent plugins list`

Optional if easy:

- `agent plugins inspect <name>`
- `agent doctor`

Must not include yet:

- full TUI
- web console
- daemon control plane

Definition of done:

- a user can discover profiles and plugins, then start or resume a run without touching internal config files manually

## 11. Config system

Status: required

Packages:

- `pkg/config`

Must include:

- global config file
- project config override
- environment variable support for credentials
- profile and plugin enablement settings
- validation on startup

Recommended config lookup:

- project-local config
- user config directory
- environment variables

Definition of done:

- users can set provider credentials and default behavior without recompiling or editing profile manifests directly

## 12. Workspace model

Status: required

Packages:

- `pkg/workspace`

Must include:

- workspace root detection
- safe path normalization
- repo-aware boundaries for file tools
- integration with policy checks

Definition of done:

- file tools cannot accidentally wander outside the allowed root when profile or policy says they should not

## 13. Testing baseline

Status: required

Directories:

- `test/integration`
- `test/e2e`
- `test/fixtures`

Must include:

- profile loader tests
- plugin loader tests
- policy decision tests
- session persistence tests
- end-to-end run test with a fake provider

Recommended:

- a fake streaming provider for deterministic runtime tests

Definition of done:

- we can verify the framework behavior without needing live model API calls for every test

## Deferred from v0.1

These are good ideas, but they should not be in the first milestone.

- remote plugin registry
- signed package distribution
- browser automation runtime
- Grafana, research, or monitoring plugins as first-class product work
- remote gateway and webhook server
- MCP as a deep runtime architecture instead of a bridge or plugin
- full memory subsystem separate from sessions
- multi-agent orchestration
- container sandbox execution
- SaaS auth flows beyond simple credential config

## Suggested v0.1 milestone breakdown

## Milestone A: Core loop

- runtime contracts
- event model
- one provider
- one fake provider for tests

## Milestone B: Safe local operation

- workspace model
- file tools
- policy engine
- approval flow

## Milestone C: Persistence and host UX

- SQLite session store
- CLI run and resume
- config loading

## Milestone D: Extensibility proof

- profile schema and loader
- plugin schema and loader
- one bundled example profile
- one example plugin adding a tool or prompt pack

## Milestone E: Hardening

- validation failures are clean
- event stream is stable enough for future UIs
- integration tests cover the happy path and key blocked actions

## Recommended first example assets

To keep `v0.1` grounded, I would ship:

- bundled profile: `coding`
- bundled profile: `readonly`
- bundled plugin: `core-tools`
- optional example plugin: `git-tools` or `web-tools`

Even here, the important rule is:

- the profile is loaded through the profile system
- the plugin is loaded through the plugin system
- nothing is special-cased in the runtime

## Release bar

Do not call it `v0.1` unless all of these are true:

- runtime can execute a real tool-calling session end to end
- policy blocks at least one unsafe action correctly
- approval flow works for at least one risky tool
- sessions resume cleanly from SQLite
- a local profile can be added without recompiling
- a local plugin can add capability without recompiling the runtime core

## Recommendation summary

The right `v0.1` is not “many features.”
It is a narrow but honest proof that the architecture works:

- generic runtime
- loaded profiles
- installable plugins
- tiny tool surface
- real safety boundaries
- one solid CLI host

If we ship that, the framework is ready to grow in the directions you care about later.
