# Changelog

All notable changes to the `agent` framework and CLI.

## Unreleased

### Added
- Wide-event structured logging in registry server (via `agent-registry`)

---

## v0.2.0

### Added

**Plugin ecosystem**
- Remote registry search — `plugins search` queries `/v1/index.json` on configured registry sources
- Remote registry install — `plugins install <name>` downloads artifact from registry, extracts, registers
- Plugin upgrade — `plugins upgrade <name>` reinstalls latest version from original source
- Plugin publish — `plugins publish <path>` packs directory and POSTs to a registry source
- `--source` flag on install/upgrade to pin to a specific named source
- `envMapping` in plugin config — map plugin config keys to subprocess environment variables
- Version and source tracking — `installedVersion` and `installedSource` recorded in plugin config

**Runtime**
- `command` runtime: argv-template mode for wrapping existing CLIs (e.g. `gh`, `kubectl`)
- `command` runtime: JSON stdin/stdout mode for custom binaries and scripts
- `host` runtime: `SpawnSubRunParallel` — concurrent sub-agent execution via goroutines

**Sessions**
- Session compaction — token-aware context truncation at turn boundaries

**Profiles**
- `instructions.system` resolves registered plugin prompt IDs by name

**CLI**
- `profiles install <path>` — copy a profile directory into the local profiles store

---

## v0.1.0

Initial framework release.

### Added

- Generic Go agent runtime with typed event model
- CLI: `run`, `chat`, `resume`, `plugins`, `profiles`, `sessions`, `config`, `doctor`
- Built-in core tools: `read`, `write`, `edit`, `bash`, `glob`, `grep`
- OpenAI-compatible provider with chat and responses API support, SSE streaming fallback
- SQLite-backed session persistence, resume, export, and transcript replay
- Declarative profile loading with policy overlays
- Plugin system: local install, enable, disable, configure, list, validate
- Plugin config management from the CLI
- HTTP plugin runtime
- Host plugin runtime with `spawn-sub-agent`
- MCP client bridge: stdio, HTTP, and SSE transports
- Approval and policy enforcement for sensitive tool calls and plugin actions
