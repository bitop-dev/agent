# Go Agent Framework CLI Surface

## Goal

This document defines the first CLI surface for the framework.

The CLI is a host for the runtime.
It should let users:

- load profiles
- inspect available capabilities
- run agent sessions
- manage local plugins
- resume prior work

It should not make assumptions that the framework is only for coding.

## CLI philosophy

The first CLI should feel:

- local-first
- profile-driven
- explicit about safety
- small enough to learn quickly
- extensible without becoming confusing

The CLI should expose the framework shape clearly:

- profiles define behavior
- plugins provide capability
- sessions persist work
- policy and approvals mediate risky actions

## Top-level command model

Recommended executable name for now:

- `agent`

Top-level command groups for `v0.1`:

- `agent chat`
- `agent run`
- `agent resume`
- `agent profiles ...`
- `agent plugins ...`
- `agent sessions ...`
- `agent config ...`
- `agent doctor`

This keeps the surface compact while still showing the framework clearly.

## Core execution commands

## `agent chat`

Purpose:

- start an interactive session with a selected profile

Example:

```bash
agent chat --profile coding
```

Responsibilities:

- load config
- resolve profile
- discover enabled plugins
- start or create a session
- render streamed events interactively
- handle approval prompts inline

Recommended flags:

- `--profile <name|path>`
- `--model <provider/model>`
- `--session <id>`
- `--cwd <path>`
- `--approval <never|on-request|always>`
- `--no-session`
- `--var key=value`

Notes:

- this is the default human-friendly mode
- if no profile is specified, use configured default or prompt clearly

## `agent run`

Purpose:

- execute a one-shot or non-interactive run

Examples:

```bash
agent run --profile coding "Summarize this repository"
agent run --profile readonly --input prompt.txt
```

Responsibilities:

- run a single request without entering a persistent REPL loop
- still persist a session unless `--no-session` is set
- print final output cleanly
- optionally stream progress events

Recommended flags:

- `--profile <name|path>`
- `--input <file>`
- `--model <provider/model>`
- `--cwd <path>`
- `--format <text|json|events>`
- `--approval <never|on-request|always>`
- `--no-session`
- `--var key=value`

Notes:

- `text` is for humans
- `json` is for scripting
- `events` is useful for debugging and future integrations

## `agent resume`

Purpose:

- continue a previous session quickly

Examples:

```bash
agent resume
agent resume 8b4f7b3a
```

Responsibilities:

- load the most recent session when no ID is given
- reopen with the original profile and context
- continue in interactive mode by default

Recommended flags:

- `--profile <name|path>` only as an override with clear warning behavior
- `--cwd <path>`

Notes:

- profile override should be rare and explicit because changing profiles on old sessions may be unsafe or confusing

## Profile management commands

## `agent profiles list`

Purpose:

- show all discoverable profiles

Example:

```bash
agent profiles list
```

Output should include:

- profile name
- version
- source location
- short description
- whether bundled or user-installed

Recommended flags:

- `--json`
- `--verbose`

## `agent profiles show`

Purpose:

- inspect one profile in detail

Example:

```bash
agent profiles show coding
```

Output should include:

- profile metadata
- default provider and model
- enabled tools
- approval mode
- workspace requirements
- referenced policy overlays
- resolved source path

Recommended flags:

- `--json`
- `--resolved`

`--resolved` should show the profile after config overlays and defaults are applied.

## `agent profiles validate`

Purpose:

- validate one or more profiles without running them

Examples:

```bash
agent profiles validate coding
agent profiles validate ./profiles/grafana-ops
```

Why it matters:

- fast feedback for profile authors
- useful before sessions fail at runtime

Recommended flags:

- `--json`

## `agent profiles init`

Purpose:

- scaffold a new local profile

Example:

```bash
agent profiles init my-profile
```

For `v0.1`, this can be a simple template generator.

If this feels too much for the first cut, it can move to `v0.2`.

## Plugin management commands

## `agent plugins list`

Purpose:

- show discovered plugins and their status

Example:

```bash
agent plugins list
```

Output should include:

- plugin name
- version
- source path
- enabled or disabled state
- contributed capability types

Recommended flags:

- `--json`
- `--verbose`

## `agent plugins show`

Purpose:

- inspect one plugin manifest and contributions

Example:

```bash
agent plugins show grafana
```

Output should include:

- metadata
- compatibility requirements
- contributed tools
- contributed prompts
- contributed profile templates
- contributed policy snippets

Recommended flags:

- `--json`

## `agent plugins validate`

Purpose:

- validate one plugin bundle before it is used

Example:

```bash
agent plugins validate ./plugins/grafana
```

Why it matters:

- validates manifest correctness
- checks contribution references
- catches compatibility issues early

## `agent plugins install`

Purpose:

- install a plugin into the local plugin directory

Examples for `v0.1`:

```bash
agent plugins install ./plugins-local/grafana
agent plugins install /tmp/web-tools
```

Important:

- `v0.1` should support local path install only
- remote registry install can wait

Responsibilities:

- copy or link the plugin into the managed plugin directory
- validate before enabling
- do not auto-enable dangerous capabilities silently

Recommended flags:

- `--link`
- `--force`

## `agent plugins enable`

Purpose:

- enable an installed plugin for discovery and use

Example:

```bash
agent plugins enable grafana
```

Notes:

- install and enable should remain separate
- enablement can update user config, not plugin contents

## `agent plugins disable`

Purpose:

- disable a plugin without uninstalling it

Example:

```bash
agent plugins disable grafana
```

## `agent plugins remove`

Purpose:

- uninstall a plugin from the local system

Example:

```bash
agent plugins remove grafana
```

Safety note:

- this should confirm if the plugin is referenced by active profiles

## Session commands

## `agent sessions list`

Purpose:

- show local sessions available for resume

Example:

```bash
agent sessions list
```

Output should include:

- session ID
- profile name
- cwd or workspace
- last updated time
- short label if available

Recommended flags:

- `--json`
- `--limit <n>`

## `agent sessions show`

Purpose:

- inspect one session metadata record

Example:

```bash
agent sessions show 8b4f7b3a
```

Output should include:

- session metadata
- profile used
- creation and update times
- workspace root
- event or message counts

## `agent sessions export`

Purpose:

- export session history for debugging or sharing

Examples:

```bash
agent sessions export 8b4f7b3a --format json
agent sessions export 8b4f7b3a --format jsonl
```

This is useful in `v0.1` even if branch management stays simple.

## Config commands

## `agent config show`

Purpose:

- show resolved configuration

Example:

```bash
agent config show
```

Output should include:

- config file sources
- default profile
- enabled plugins
- session directory
- provider config summary without secrets

Recommended flags:

- `--json`

## `agent config paths`

Purpose:

- show where the CLI looks for config, profiles, plugins, and sessions

Example:

```bash
agent config paths
```

This is a high-value command for support and debugging.

## Diagnostics command

## `agent doctor`

Purpose:

- run local diagnostics for common setup issues

Checks for `v0.1`:

- config file readability
- session directory access
- profile discovery
- plugin discovery
- provider credential presence
- SQLite writability

Example:

```bash
agent doctor
```

This command is especially helpful when profiles and plugins are user-provided.

## Global flags

These should work across most commands where they make sense.

- `--config <path>`
- `--cwd <path>`
- `--profile <name|path>`
- `--json`
- `--verbose`
- `--quiet`

Use them sparingly. Too many global flags can make the CLI muddy.

## Output modes

For `v0.1`, support three output styles where relevant:

- `text` for normal human use
- `json` for automation and tooling
- `events` for debugging the runtime or building future UIs

Not every command needs all three, but `agent run` should support them.

## Interactive-mode behavior

Inside `agent chat`, we should support a small slash-command set.

Recommended initial in-session commands:

- `/help`
- `/profile`
- `/session`
- `/tools`
- `/approve`
- `/quit`

Recommended behavior:

- `/profile` shows current resolved profile summary
- `/session` shows current session metadata
- `/tools` lists tools currently available in this run
- `/approve` shows current approval mode

Keep this small in `v0.1`.

## What not to include in the first CLI

- giant command trees for every future idea
- separate top-level commands for `research` or `monitor`
- hardcoded agent identities in the CLI surface
- marketplace or cloud account flows
- daemon-first UX

If the CLI starts to imply a fixed set of agent types, we are slipping away from the architecture we chose.

## Recommended minimum `v0.1` command set

If we trim to the smallest honest surface, I would ship:

- `agent chat`
- `agent run`
- `agent resume`
- `agent profiles list`
- `agent profiles show`
- `agent profiles validate`
- `agent plugins list`
- `agent plugins show`
- `agent plugins validate`
- `agent plugins install`
- `agent plugins enable`
- `agent plugins disable`
- `agent sessions list`
- `agent sessions show`
- `agent config show`
- `agent config paths`
- `agent doctor`

Everything else can wait.

## Recommended implementation order

### Phase 1

- `agent chat`
- `agent run`
- `agent resume`

### Phase 2

- `agent profiles list`
- `agent profiles show`
- `agent profiles validate`

### Phase 3

- `agent plugins list`
- `agent plugins show`
- `agent plugins validate`
- `agent plugins install`
- `agent plugins enable`
- `agent plugins disable`

### Phase 4

- `agent sessions list`
- `agent sessions show`
- `agent config show`
- `agent config paths`
- `agent doctor`

## Recommendation summary

The first CLI should teach the framework model through its commands:

- you run profiles
- you install plugins
- you resume sessions
- you inspect config and capabilities

That will make the architecture legible to users from day one.
