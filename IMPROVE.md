# Improvements

All 15 improvements implemented. ✅

---

## High Impact — Reliability & Safety

### ✅ 1. Retry with Backoff

`Config.MaxRetries` + `Config.RetryBaseDelay` with exponential backoff.
Detects transient errors (429, 5xx, connection reset, timeout) via pattern
matching on error messages and assistant stop reasons. Emits `EventRetry`
for each attempt. Caps delay at 60s.

**Files:** `pkg/agent/loop.go` (`streamResponseWithRetry`, `isTransientError`)
**Config:** `max_retries` in YAML, `Config.MaxRetries` in Go
**Tests:** `TestRetry_RecoversFromTransient`, `TestRetry_ExhaustsRetries`, `TestRetry_ZeroRetries_NoRetry`

---

### ✅ 2. Panic Recovery in Tool Execution

`executeSingleTool()` wraps `tool.Execute()` with `defer recover()`. Panics
are converted to `tools.ErrorResult` and surfaced to the LLM as an error
tool result. The agent loop continues normally.

**Files:** `pkg/agent/loop.go` (`executeSingleTool`)
**Tests:** `TestPanicRecovery`

---

### ✅ 3. Permission / Confirmation Hooks

`Config.ConfirmToolCall` callback gates tool execution:

- `ConfirmAllow` — proceed (default when nil)
- `ConfirmDeny` — skip, LLM gets "Tool call denied by user" result
- `ConfirmAbort` — stop the entire loop with error

`AutoApproveAll` is a convenience function for unattended/autonomous mode.
Setting `auto_approve: true` in YAML or `ConfirmToolCall: agent.AutoApproveAll`
in Go ensures the agent runs without human intervention.

When `ConfirmToolCall` is nil (the default), all tools auto-approve — this
preserves backward compatibility and supports the "send it off to work" pattern.

**Files:** `pkg/agent/types.go` (types), `pkg/agent/loop.go` (execution)
**Config:** `auto_approve` in YAML, `Config.ConfirmToolCall` in Go
**Tests:** `TestConfirmToolCall_Deny`, `TestConfirmToolCall_Abort`,
`TestConfirmToolCall_AutoApprove`, `TestConfirmToolCall_Nil_AutoApproves`

---

### ✅ 4. Parallel Tool Execution

`Config.MaxToolConcurrency` controls concurrent tool execution. When > 1,
tool calls dispatch via goroutines with a semaphore channel. Results are
collected in the original order. Confirmation hooks still run serially
(before dispatch) for safety.

**Files:** `pkg/agent/loop.go` (`executeToolCallsParallel`)
**Config:** `max_tool_concurrency` in YAML, `Config.MaxToolConcurrency` in Go
**Tests:** `TestParallelToolExecution` (verifies wall-clock time is ~half)

---

## Medium Impact — Real Usage Gaps

### ✅ 5. Structured Logging

`Options.Logger` accepts `*slog.Logger` from the Go stdlib. Default is a
no-op logger (silent). All internal warnings (session write errors, compaction
failures, retry attempts, tool panics) use structured logging.

**Files:** `pkg/agent/types.go`, `pkg/agent/agent.go`, `pkg/agent/loop.go`
**Usage:** `agent.New(agent.Options{Logger: slog.Default()})`

---

### ✅ 6. Cost Estimation

`CostUsage` tracks cumulative input/output tokens and USD cost using pricing
from the model registry. `EventTurnEnd` includes `CostUsage` with per-turn
and cumulative cost. `State.CumulativeCost` provides a snapshot.

`Config.MaxCostUSD` is a budget cap — when cumulative cost exceeds this value
the loop stops cleanly with `EventTurnLimitReached`.

**Files:** `pkg/agent/types.go` (`CostUsage`, `computeTurnCost`),
`pkg/agent/loop.go` (tracking), `pkg/ai/models/models.go` (pricing data)
**Tests:** `TestCostTracking`, `TestMaxCostUSD`

---

### ✅ 7. Image / Multimodal Input

Verified end-to-end: `ai.ImageContent` is correctly serialized by all three
major providers (OpenAI → `image_url` with data URI, Anthropic → `image`
with base64 source, Google → `inlineData`). Works in both `UserMessage` and
`ToolResultMessage` contexts. All common MIME types supported.

**Files:** `pkg/ai/types.go` (type), `pkg/ai/providers/*/` (serialization)
**Tests:** `pkg/ai/image_test.go` — `TestImageContent_InUserMessage`,
`TestImageContent_InToolResult`, `TestImageContent_SatisfiesContentBlock`,
`TestImageContent_AllMIMETypes`

---

### ✅ 8. Streaming Progress for Long Tools

Added `UpdateFn` progress callbacks to grep, find, and web_fetch:

- **grep** — emits progress every 100 files scanned with match count
- **find** — emits progress every 200 entries scanned with match count
- **web_fetch** — emits "Fetching URL…" before the HTTP request

Bash already had streaming progress. Read and write are instantaneous and
don't benefit from progress updates.

**Files:** `pkg/tools/builtin/grep.go`, `pkg/tools/builtin/find.go`,
`pkg/tools/builtin/webfetch.go`

---

## Lower Impact — Production Nice-to-Haves

### ✅ 9. Configurable Timeout per Tool

`Config.ToolTimeout` wraps each `tool.Execute()` call with
`context.WithTimeout`. Individual tools (e.g. bash) can still override via
their own parameters.

**Files:** `pkg/agent/loop.go` (`executeSingleTool`)
**Config:** `Config.ToolTimeout` in Go
**Tests:** `TestToolTimeout`

---

### ✅ 10. Metrics / Observability

`Config.OnMetrics` callback receives `TurnMetrics` at the end of each turn:
turn number, provider latency, per-tool durations, token counts, and cost.

**Files:** `pkg/agent/types.go` (`TurnMetrics`), `pkg/agent/loop.go`
**Tests:** `TestMetricsCallback`

---

### ✅ 11. Sub-Agent / Delegation

First-class `SubAgent` helper and `SubAgentTool` wrapper:

```go
// Standalone sub-agent
sub := agent.NewSubAgent(agent.SubAgentOptions{
    Provider:     provider,
    Model:        "gpt-4o",
    SystemPrompt: "You are a code reviewer.",
    Tools:        readonlyTools,
    MaxTurns:     10,
})
result, err := sub.Run(ctx, "Review this diff: ...")

// As a tool the parent agent can call
reviewTool := agent.NewSubAgentTool("code_review",
    "Reviews code and returns feedback",
    agent.SubAgentOptions{...},
)
registry.Register(reviewTool)
```

Features:
- `SubAgent.Run(ctx, prompt)` → final text response
- `SubAgent.RunMessages(ctx, msgs)` → for pre-built messages
- `SubAgent.LastResponse()` → extract text from last assistant message
- `SubAgent.Agent()` → access underlying Agent for advanced use
- `SubAgentTool` — wraps a SubAgent as a `tools.Tool` for parent agents
- `OnEvent` callback for forwarding sub-agent events to parent
- Progress streaming from sub-agent through `UpdateFn`

**Files:** `pkg/agent/subagent.go`
**Tests:** `pkg/agent/subagent_test.go` — `TestSubAgent_Run`,
`TestSubAgent_LastResponse`, `TestSubAgent_OnEvent`, `TestSubAgent_Agent`,
`TestSubAgent_WithTools`, `TestSubAgentTool_Execute`,
`TestSubAgentTool_MissingPrompt`

---

### ✅ 12. Config Hot-Reload

`ConfigReloader` watches a YAML config file and applies mutable changes to a
running Agent. Poll-based (2s interval) — no external dependencies needed.

Mutable fields (applied at runtime):
- model, max_tokens, temperature, thinking_level, cache_retention,
  context_window

Immutable fields (require restart):
- provider, tools, base_url, api_key

```go
reloader := agent.NewConfigReloader("agent.yaml", myAgent, slog.Default())
reloader.Start()
defer reloader.Stop()

// Or manual reload:
reloader.ReloadOnce()
```

Emits `EventConfigReloaded` on successful reload. Optional `OnReload`
callback for custom handling.

**Files:** `pkg/agent/reload.go`
**Tests:** `pkg/agent/reload_test.go` — `TestConfigReloader_ReloadOnce`,
`TestConfigReloader_EmitsEvent`, `TestConfigReloader_OnReloadCallback`,
`TestConfigReloader_StartStop`, `TestConfigReloader_InvalidConfig`

---

## Architecture / Code Quality

### ✅ 13. Merge `provider.go` into `types.go`

The `Provider` interface (25 lines) has been merged into `pkg/ai/types.go`.
`pkg/ai/provider.go` is removed.

---

### ✅ 14. Evaluate `paths.go` Scope

Verified: `resolvePath()` from `paths.go` is used by 6 tools (read, write,
edit, grep, find, ls). The shared utility is well-justified.

---

### ✅ 15. Harden `IsContextOverflow` Detection

Added documentation of the three detection strategies and their limitations
directly in the `overflow.go` file header. The string-matching approach is
documented as a known limitation with guidance for contributors.

---

## Summary

| # | Improvement | Status |
|---|------------|--------|
| 1 | Retry with backoff | ✅ |
| 2 | Panic recovery | ✅ |
| 3 | Confirmation hooks (with auto-approve) | ✅ |
| 4 | Parallel tool execution | ✅ |
| 5 | Structured logging | ✅ |
| 6 | Cost estimation + budget cap | ✅ |
| 7 | Image/multimodal verification | ✅ |
| 8 | Streaming progress for tools | ✅ |
| 9 | Tool timeout | ✅ |
| 10 | Metrics/observability | ✅ |
| 11 | Sub-agent delegation | ✅ |
| 12 | Config hot-reload | ✅ |
| 13 | Merge provider.go | ✅ |
| 14 | Evaluate paths.go | ✅ |
| 15 | Harden overflow detection | ✅ |

**All 15 improvements implemented.**
