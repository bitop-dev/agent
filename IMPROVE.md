# Improvements

Prioritized list of features, functionality, and code quality improvements.

---

## High Impact — Reliability & Safety

### 1. Retry with Backoff

If an LLM call hits a rate limit, 503, or network error, the loop returns an
error immediately. Production agents need exponential backoff with configurable
max retries.

**Approach:** Add `MaxRetries int` and `RetryBackoff time.Duration` to
`agent.Config`. Wrap the `provider.Stream()` call in a retry loop that catches
transient HTTP errors (429, 500, 502, 503, 504, connection reset) and backs off
exponentially. Emit an `EventRetry` so callers can log/display retry attempts.

**Priority:** Highest — this is the single biggest reliability gap.

---

### 2. Panic Recovery in Tool Execution

If a tool (especially a user-written or plugin tool) panics, the entire agent
process crashes. This is unacceptable in production.

**Approach:** Wrap `tool.Execute()` in `executeSingleTool()` with a
`defer recover()` that converts the panic into a `tools.ErrorResult`. The agent
loop continues normally with the error surfaced to the LLM as a tool result.

**Priority:** High — minimal code change, large safety improvement.

---

### 3. Permission / Confirmation Hooks

There is no way to gate dangerous operations (file writes, bash commands,
destructive edits). Pi-mono has a confirmation flow where the user can
approve/reject tool calls before execution.

**Approach:** Add a `ConfirmToolCall func(name string, args map[string]any) (bool, error)`
callback to `agent.Config`. The loop checks it before executing each tool call.
If the callback returns `false`, the tool result is set to "User denied
execution" and the LLM is informed. A nil callback means auto-approve
(current behaviour).

**Priority:** High — critical for interactive and safety-sensitive use cases.

---

### 4. Parallel Tool Execution

When the LLM returns multiple independent tool calls in one turn, they
currently run sequentially. This adds unnecessary latency.

**Approach:** Run tool calls concurrently with a `sync.WaitGroup`, collecting
results in order. Add `MaxToolConcurrency int` to `agent.Config` (default 1 =
sequential for backward compatibility). Use a semaphore channel to cap
concurrency. Steering checks still happen after all tools complete.

**Priority:** High — significant latency reduction on multi-tool turns.

---

## Medium Impact — Real Usage Gaps

### 5. Structured Logging

Errors in session persistence and compaction are currently silently swallowed
(`_ = err`). There is no way for embedders to observe internal warnings.

**Approach:** Add a `Logger` field to `agent.Options` accepting an `*slog.Logger`
(from the Go stdlib `log/slog` package). Default to `slog.Default()`. Replace
all `_ = err` sites with `logger.Warn(...)`. This adds no external dependency
and integrates with any slog handler (zerolog, zap, logfmt, JSON, etc.).

**Priority:** Medium — important for debugging and production observability.

---

### 6. Cost Estimation

The model registry already stores context windows. Adding pricing data would
enable per-turn and cumulative cost tracking.

**Approach:** Add `InputPricePer1M float64` and `OutputPricePer1M float64`
fields to `models.ModelInfo`. Compute cost in the `EventTurnEnd` handler using
the turn's input/output token counts. Add a `Cost` field to `ContextUsage`
and a cumulative `TotalCost` to `State`. Useful for budget caps
(`MaxCostUSD float64` in Config).

**Priority:** Medium — valuable for teams tracking API spend.

---

### 7. Image / Multimodal Input (End-to-End Verification)

The `ai.ImageContent` type exists in `types.go`, but the full path from user
input → provider request serialization → API call needs end-to-end
verification for all vision-capable providers (OpenAI, Anthropic, Gemini).

**Approach:** Add an integration test or example that sends an image (base64
and URL variants) through each provider that supports vision. Fix any
serialization gaps found. Document which providers support images and any
size/format constraints.

**Priority:** Medium — multimodal is increasingly expected.

---

### 8. Streaming Progress for Long Tools

The `UpdateFn` callback exists but most built-in tools don't use it. Only
bash streams output incrementally.

**Tools that would benefit:**
- `grep` on large codebases — emit match count periodically
- `find` with thousands of results — emit count as walking
- `web_fetch` on slow URLs — emit bytes downloaded / status
- `read` on large files — emit progress through offset/limit

**Approach:** Add `UpdateFn` calls at regular intervals in each tool's main
loop. Keep updates lightweight (just a count or percentage).

**Priority:** Medium — improves UX for long-running operations.

---

## Lower Impact — Production Nice-to-Haves

### 9. Configurable Timeout per Tool

Bash has a timeout parameter, but `web_fetch` has none, and there's no way to
set a global per-tool timeout to prevent hung agents.

**Approach:** Add `ToolTimeout time.Duration` to `agent.Config`. In
`executeSingleTool()`, wrap the `tool.Execute()` call with
`context.WithTimeout(ctx, timeout)`. Individual tools can still override via
their own parameters (e.g. bash's `timeout` param takes precedence). Default:
120s.

**Priority:** Lower — prevents hung agents in edge cases.

---

### 10. Metrics / Observability

For teams running agents in production, there's no way to emit structured
metrics (latency, error rates, token usage over time).

**Approach:** Add an optional `OnMetrics func(Metrics)` callback to
`agent.Config`, where `Metrics` contains turn duration, provider latency,
token counts, tool execution times, and error counts. Alternatively, emit
OpenTelemetry spans if an OTEL tracer is configured on the context. Start
with the callback approach (zero dependencies) and add OTEL as an opt-in
later.

**Priority:** Lower — important at scale, not needed for single-user CLI.

---

### 11. Sub-Agent / Delegation

Spawning a child `Agent` with a scoped system prompt and tool set, collecting
its result, and feeding it back to the parent. This is the multi-agent
pattern.

**Current state:** The architecture already supports this — you can call
`agent.New()` inside a custom tool's `Execute()` method. But there's no
first-class support.

**Approach:** Create a `SubAgentTool` helper that wraps agent creation:

```go
sub := agent.NewSubAgent(agent.SubAgentOptions{
    Parent:       parentAgent,
    SystemPrompt: "You are a code reviewer...",
    Tools:        readonlyTools,
    MaxTurns:     5,
})
result, err := sub.Run(ctx, "Review this diff: ...")
```

The helper handles session isolation, token budget inheritance, and result
formatting.

**Priority:** Lower — useful for complex workflows, not core functionality.

---

### 12. Config Hot-Reload

For long-running agents (proxy server, daemon mode), watching `agent.yaml` for
changes and applying them without restart.

**Approach:** Use `fsnotify` to watch the config file. On change, re-parse and
update mutable fields (model, max_tokens, max_turns, thinking_level) via a
`Reconfigure()` method. Provider and tool changes require a full restart
(document this). Emit an `EventConfigReloaded` event.

**Priority:** Lower — only relevant for long-running server deployments.

---

## Architecture / Code Quality

### 13. Merge `provider.go` into `types.go`

`pkg/ai/provider.go` is 25 lines containing only the `Provider` interface.
Having a separate file adds navigation overhead for no structural benefit.
Merge it into `types.go` where all other core AI types live.

---

### 14. Evaluate `paths.go` Scope

`pkg/tools/builtin/paths.go` contains path resolution helpers (@ prefix
stripping, relative-to-cwd resolution). Verify whether multiple tools use it
or only `read.go`. If only one tool uses it, inline the logic. If multiple
tools share it, add a doc comment explaining the shared contract.

---

### 15. Harden `IsContextOverflow` Detection

`pkg/ai/overflow.go` relies on string pattern matching against error messages
to detect context overflow. This is fragile across provider API changes.

**Options:**
- Add provider-specific overflow detection methods that check HTTP status
  codes and structured error responses (not just string matching)
- Document the current approach as a known limitation
- Add a fallback: if usage tokens exceed the model's context window from the
  registry, treat it as overflow regardless of the error message

---

## Recommended Order of Implementation

1. **Panic recovery (#2)** — 10 lines of code, immediate safety win
2. **Retry with backoff (#1)** — biggest reliability improvement
3. **Confirmation hooks (#3)** — critical for interactive safety
4. **Structured logging (#5)** — unblocks debugging everything else
5. **Parallel tool execution (#4)** — performance win
6. **Cost estimation (#6)** — quick to add with existing model registry
7. Everything else as needed
