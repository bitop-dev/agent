# Go Agent Plugin Package Model

## Purpose

This document defines the recommended package boundary as the project moves toward a plugin ecosystem more like npm, Homebrew, or the Terraform Registry.

The goal is to separate:

- core framework code
- plugin packages
- runtime implementations
- framework-owned test fixtures

## Current direction

The workspace is now split conceptually like this:

- `agent/` - core framework, CLI, policy engine, runtime manager, tests
- `agent-plugins/` - plugin packages and plugin-owned example assets

This is the right foundation for a future registry because plugin packages now have a home outside the core runtime repository.

## Recommended ownership model

### Core framework-owned

These stay in `agent/`:

- `cmd/agent`
- `internal/`
- `pkg/`
- framework integration tests
- framework-owned example profiles
- framework-owned runtime fixtures used primarily to test the framework itself

### Plugin package-owned

These should live in `agent-plugins/` or another plugin package repository:

- `plugin.yaml`
- tool descriptors
- plugin prompts
- plugin policies
- plugin profile templates
- plugin-owned example profiles
- package metadata for versioning and distribution

### Runtime implementation-owned

These may live in different places depending on what they are:

- if the runtime is only a framework test fixture, keep it in `agent/_testing/runtimes/`
- if the runtime is the real implementation of a plugin, it should eventually live with the plugin package or in its own dedicated repo

That distinction matters because a future registry may distribute plugin metadata without distributing the runtime binary the same way.

## Current profile classification

### Framework-owned profiles

These remain in `agent/_testing/profiles/`:

- `readonly`
- `coding`
- `coding-no-approval`

Why they stay:

- they test the core tool/runtime/policy surface
- they are not primarily attached to one plugin package
- they are useful as baseline verification profiles

### Plugin-owned profiles

These moved to `agent-plugins/`:

- `delegator` -> `../agent-plugins/spawn-sub-agent/examples/profiles/delegator/`
- `email-local` -> `../agent-plugins/send-email/examples/profiles/email-local/`
- `email-openai` -> `../agent-plugins/send-email/examples/profiles/email-openai/`
- `email-openai-review` -> `../agent-plugins/send-email/examples/profiles/email-openai-review/`
- `email-openai-no-approval` -> `../agent-plugins/send-email/examples/profiles/email-openai-no-approval/`
- `web-local` -> `../agent-plugins/web-research/examples/profiles/web-local/`

Why they moved:

- they exist to demonstrate or validate a specific plugin package
- they are better shipped with the package they enable
- they belong in the eventual package/install story

### Incomplete fixture

- `openai-coding/` remains in `agent/_testing/profiles/` as an incomplete experimental fixture and should not be treated as part of the current supported package model

## Recommended plugin package contents

The package unit for a future registry should look like this:

```text
my-plugin/
  plugin.yaml
  tools/
  prompts/
  policies/
  profiles/
  examples/
    profiles/
  README.md
```

### Required

- `plugin.yaml`

### Usually included

- `tools/`
- `prompts/`
- `policies/`
- `profiles/` for profile templates contributed by the plugin

### Strongly recommended

- `examples/profiles/` for runnable example profiles that enable the plugin tools
- `README.md` with install/config/run instructions

## Registry-oriented packaging rules

If we want a registry later, the installable package should be the plugin bundle plus assets, not necessarily the runtime executable.

Why:

- `http` plugins may point to a separately deployed service
- `mcp` plugins may point to a remote endpoint or a separately installed stdio server
- `command` plugins may wrap an existing local executable
- some runtimes may be installed by another package manager entirely

So the registry should probably distribute:

- plugin manifest and assets
- package metadata
- docs/examples
- optional install hints for the runtime

It should not assume every plugin ships one self-contained binary artifact.

## Install model recommendation

### Phase 1: path and link installs

Support and document:

- `plugins install ../agent-plugins/send-email --link`

### Phase 2: named package installs

Add a package index so users can do something like:

- `agent plugins install send-email`
- `agent plugins install context7-mcp@0.2.1`

### Phase 3: versioned package metadata

Add:

- package versions
- compatibility constraints
- checksums/signatures
- publishing workflow
- trust policy for install sources

## Runtime ownership recommendation

Do not move all runtimes out of `agent/` yet.

Keep framework test runtimes in `agent/_testing/runtimes/` until there is a clearer split between:

- framework validation fixtures
- actual plugin runtime implementations meant for package distribution

When a runtime becomes the real shipped implementation of a plugin package, then consider moving it to:

- the plugin package repo, or
- a separate runtime repo owned by that plugin

## Decision summary

- move plugin bundles out of the core repo now
- move plugin-owned example profiles with their plugins now
- keep framework-owned profiles in the core repo
- keep framework test runtimes in the core repo for now
- design the future registry around plugin packages, not around assuming every plugin ships a binary

## Near-term next steps

1. keep updating docs to use `../agent-plugins/...` paths
2. add package metadata/index files inside `agent-plugins/`
3. define a package naming/versioning scheme
4. design `plugins search`, `plugins install <name>`, and `plugins publish`
5. add trust and signature rules before enabling remote installs by default
