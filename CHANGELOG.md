# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [0.1.0] — 2026-02-27

### Added

#### Core Framework
- `pkg/agent` — agent loop, event system, compaction, session management
- `pkg/ai` — provider interface, streaming events, message types, model registry (30+ models)
- `pkg/tools` — tool interface, registry, JSON Schema validation, plugin protocol
- `pkg/session` — JSONL persistence, fork/branch, HTML export
- `pkg/skills` — skill discovery and loading from `~/.config/agent/skills/` and `.agent/skills/`
- `pkg/prompts` — prompt template loading and `$1`/`$2` expansion

#### Providers
- Anthropic Messages API (Claude) with prompt caching
- OpenAI Chat Completions and Responses API
- Google Generative AI (Gemini)
- Azure OpenAI Service
- Amazon Bedrock (ConverseStream)
- Proxy provider — serve any upstream as an HTTP endpoint

#### Built-in Tools (9)
- `read` — file reader with offset/limit and 50 KB truncation
- `bash` — shell executor with pluggable `Executor` interface
- `edit` — exact-string surgical file editor
- `write` — file writer with auto-mkdir
- `grep` — pure-Go regex search with gitignore support and streaming progress
- `find` — pure-Go glob file finder with streaming progress
- `ls` — directory listing with sizes and timestamps
- `web_search` — DuckDuckGo Lite search (no API key)
- `web_fetch` — URL fetcher with HTML-to-text conversion and streaming progress

#### Plugin Tools (any language)
- JSON-over-stdin/stdout protocol (`describe` / `call` / result)
- Python example — `stats` (descriptive statistics, stdlib only)
- TypeScript/Deno example — `json_query` (dot-path JSON extraction)
- Node.js ESM example — `json_query` (same tool, no compilation)
- Rust example — `file_info` (file/dir metadata, zero crates)
- Bash example — `sys_info` (OS, CPU, memory, disk, processes)
- Ruby example — `template` (Mustache-style rendering, stdlib only)

#### Reliability & Safety
- Retry with exponential backoff for transient LLM errors (429, 5xx, network)
- Panic recovery in tool execution — panics converted to error results
- `MaxTurns` loop cap with `EventTurnLimitReached` event
- `MaxCostUSD` budget cap — stops loop when cumulative cost exceeds limit
- `ToolTimeout` per-tool execution deadline via `context.WithTimeout`

#### Control & Confirmation
- `ConfirmToolCall` callback — allow / deny / abort per tool call
- `AutoApproveAll` helper for unattended/autonomous operation
- `auto_approve` YAML field for config-driven approval
- Parallel tool execution via `MaxToolConcurrency` with semaphore

#### Observability
- `OnMetrics` callback with `TurnMetrics` (latency, token counts, tool durations, cost)
- `CostUsage` — per-turn and cumulative USD cost using model registry pricing
- `TurnDuration` in `EventTurnEnd`
- Structured logging via `*slog.Logger` (silent by default)
- `EventRetry`, `EventToolDenied`, `EventConfigReloaded` event types

#### Sub-Agent Delegation
- `SubAgent` — run a child agent and get its final text response
- `SubAgentTool` — wrap a child agent as a callable tool for the parent LLM

#### Config Hot-Reload
- `ConfigReloader` — polls YAML config every 2s, applies mutable changes live
- Mutable at runtime: `model`, `max_tokens`, `temperature`, `thinking_level`, `cache_retention`, `context_window`

#### Multimodal
- `ai.ImageContent` — image blocks in user messages and tool results
- All providers serialize images correctly (Anthropic base64, OpenAI data URI, Google inlineData)

#### CLI (`cmd/agent`)
- Interactive REPL with history
- One-shot mode via `-prompt`
- Session resume via `-session <prefix>`
- REPL commands: `/clear`, `/state`, `/model`, `/session`, `/sessions`, `/export`, `/fork`, `/skills`, `/templates`
- Config file with `${ENV_VAR}` expansion

#### CI/CD
- GitHub Actions CI — `go vet`, `go test -race`, `staticcheck` on push/PR
- GitHub Actions Release — GoReleaser + Docker on `vX.X.X` tags
- GoReleaser — linux/darwin/windows × amd64/arm64 binaries with GitHub Release
- Docker image — `ghcr.io/bitop-dev/agent` with Python 3.12, Node.js 18, Ruby 3.2, Bash

#### Documentation
- `docs/quickstart.md` — install, first config, first run
- `docs/config.md` — complete YAML reference
- `docs/providers.md` — all providers with credentials
- `docs/tools.md` — built-in tools reference
- `docs/custom-tools.md` — writing compiled-in and plugin tools (all languages)
- `docs/sdk.md` — Go library API reference
- `docs/session.md` — session format and JSONL spec
- `docs/compaction.md` — context compaction
- `docs/skills.md` — skills system
- `docs/prompt-templates.md` — prompt templates
- `docs/proxy.md` — proxy provider
- `docs/models.md` — model registry

[0.1.0]: https://github.com/bitop-dev/agent/releases/tag/v0.1.0
