# Gaps vs Pi-Mono

Features present in pi-mono's TypeScript agent that are missing from this Go implementation.
Listed in recommended implementation order. Check off each item as it is completed.

---

## 🔴 High Priority — Core missing functionality

### 1. Context Overflow Detection
- [x] **Status:** Done — `pkg/ai/overflow.go`
- **Effort:** Low | **Impact:** High

Pi has `isContextOverflow()` which matches error message patterns from every known provider
(Anthropic, OpenAI, Google, xAI, Groq, Cerebras, Mistral, OpenRouter, llama.cpp, LM Studio,
Kimi, MiniMax, etc.) plus a silent-overflow fallback for providers like z.ai that accept
oversized requests and just truncate (detected by checking `usage.input > contextWindow`).

Without this, context overflow errors are indistinguishable from other fatal errors and the
agent simply crashes the loop with no actionable information.

**What to build:** `pkg/ai/overflow.go` — `IsContextOverflow(msg *ai.AssistantMessage, contextWindow int) bool`
with a table of provider-specific regex patterns and the silent-overflow numeric check.

---

### 2. Tool Argument Validation
- [x] **Status:** Done — `pkg/tools/validate.go`, wired into `pkg/agent/loop.go`
- **Effort:** Low | **Impact:** High

Pi validates every LLM-provided tool call's arguments against the tool's JSON Schema using AJV
(with type coercion enabled) before `execute()` is called. LLMs regularly produce minor type
mismatches — e.g., passing `"5"` where an integer is expected, or omitting an optional field.

Our current `coerceParams()` in `pkg/agent/loop.go` is a no-op stub that passes arguments
through unchanged. Bad arguments reach the tool and cause confusing errors.

**What to build:** Replace the stub with real JSON Schema validation using a pure-Go library
(e.g., `github.com/santhosh-tekuri/jsonschema/v6`). Coerce simple type mismatches
(string→number, number→string) before returning an error.

---

### 3. Session Persistence
- [x] **Status:** Done — `pkg/session/`: JSONL session files (`Create`/`Load`/`AppendMessage`/`AppendCompaction`); full message serialisation/deserialisation for all content block types; `ParseMessages` handles compaction entries. Agent auto-writes every message to session. CLI: `-session <id>` to resume, `-sessions` to list, `/session` and `/sessions` REPL commands
- **Effort:** High | **Impact:** High

Pi saves every conversation turn to a JSONL file in `~/.pi/agent/sessions/`. Each entry has a
UUID, parent UUID, and ISO timestamp. The format supports:
- Resuming sessions across process restarts
- Session branching (fork at any point, explore an alternative, return)
- HTML export for sharing
- Compaction history (which messages were summarised and when)

We have no persistence — everything lives in-memory and is lost on exit.

**What to build:** `pkg/session/` package with a JSONL writer/reader, session header,
message entries, and compaction entries. Wire into the agent so every message append
also writes to disk. Add `-session` and `-resume` CLI flags.

---

### 4. Context Compaction
- [x] **Status:** Done — `pkg/agent/compaction.go`: `ShouldCompact()`, `FindCutPoint()` (never splits tool-call/result pairs; cuts at user message boundaries), `GenerateSummary()` (LLM-generated structured Markdown with Goal/Progress/Decisions/NextSteps), incremental updates via `prevSummary`. `maybeCompact()` on agent runs before each LLM call. Session persistence of compaction entries. Config: `compaction.enabled`, `context_window`, `reserve_tokens`, `keep_recent_tokens`. `EventCompaction` event. CLI prints compaction stats
- **Effort:** High | **Impact:** High

Pi's compaction pipeline triggers when the estimated context token count approaches the
model's context window limit. It:
- Estimates token count using a chars/4 heuristic anchored on the last real `usage` object
- Finds the optimal cut point without splitting tool-call/tool-result pairs
- Calls the LLM to generate a structured Markdown summary (Goal, Progress, Key Decisions,
  Next Steps, Critical Context) of the discarded history
- Handles "split turns" (cut falls mid-turn) with a second summarization call for the prefix
- Maintains continuity across multiple compactions via incremental summary updates
- Tracks which files were read/modified in the compacted portion
- Stores the compaction entry in the session file for replay

Without compaction, any session that approaches the model's context window silently breaks
(or fails with an overflow error that is not recovered from).

**What to build:** `pkg/agent/compaction.go` — token estimation, cut-point detection,
LLM-based summarisation, and a `MaybeCompact()` hook called before each `streamResponse`.
Requires session persistence (#3) to store compaction entries.

---

## 🟡 Medium Priority — Useful but workable without

### 5. System Prompt Builder
- [x] **Status:** Done — `pkg/agent/systemprompt.go`, wired into `cmd/agent/main.go`
- **Effort:** Low | **Impact:** Medium

Pi's `buildSystemPrompt()` constructs the system prompt dynamically at startup based on which
tools are active. It injects:
- A tool list with one-line descriptions
- Context-aware usage guidelines (e.g., "prefer grep/find over bash" only when all three
  are active; "use read before editing" only when both read and edit are active)
- Current date and time (formatted, with timezone)
- Current working directory

We accept a raw `system_prompt` string from config with no enrichment. The agent doesn't know
what tools are available, what time it is, or where it's running.

**What to build:** `pkg/agent/systemprompt.go` — `BuildSystemPrompt(opts SystemPromptOptions) string`
that takes the active tool names, cwd, and any user-supplied base prompt and constructs the
full prompt. Call it in `cmd/agent/main.go` at startup.

---

### 6. Context Files (AGENTS.md / CLAUDE.md)
- [x] **Status:** Done — `LoadContextFiles()` in `pkg/agent/systemprompt.go`, called by `BuildSystemPrompt()`
- **Effort:** Low | **Impact:** Medium

Pi auto-discovers `AGENTS.md` or `CLAUDE.md` in the project working directory and in the
global `~/.pi/agent/` directory, then injects their contents into the system prompt under a
"Project Context" section. This gives the agent project-specific instructions (coding
conventions, architecture notes, do-not-touch paths) without cluttering the config file.

We have no auto-loading of context files.

**What to build:** Add context file discovery to `BuildSystemPrompt()` (gap #5). Search for
`AGENTS.md` then `CLAUDE.md` in cwd and in `~/.config/agent/`. Append found content under a
`# Project Context` header. Add `context_files: []` override to `FileConfig`.

---

### 7. Thinking Levels
- [x] **Status:** Done — `ThinkingLevel`/`ThinkingBudgets`/`CacheRetention` added to `ai.StreamOptions`; Anthropic (budget + adaptive effort), OpenAI Responses (`reasoning_effort`), Google Gemini (`thinkingBudget`) all wired. Config: `thinking_level`, `cache_retention`
- **Effort:** Medium | **Impact:** Medium

Pi has a `ThinkingLevel` type (`"off" | "minimal" | "low" | "medium" | "high" | "xhigh"`)
that maps to provider-specific controls:
- **Anthropic:** sets `budget_tokens` in the `thinking` content block
- **OpenAI Responses:** sets `reasoning_effort` (`"low"` | `"medium"` | `"high"`)
- **Google Gemini:** sets `thinkingConfig.thinkingBudget` (token count)
- **Bedrock (Claude):** same as Anthropic via the converse API

Without this, reasoning models use their provider defaults, which may be excessive (slow,
expensive) or absent entirely.

**What to build:** Add `ThinkingLevel string` to `ai.StreamOptions`. Each provider reads it
and maps to its own wire format. Add `thinking_level` to `FileConfig`.

---

### 8. Prompt Cache Headers (Anthropic)
- [x] **Status:** Done — `cache_control: ephemeral` breakpoints on system prompt and last user message when `cache_retention != "none"`. Default is caching enabled (`"short"`)
- **Effort:** Medium | **Impact:** Medium

Pi sends `cache_control: {"type": "ephemeral"}` breakpoints on the system prompt block and on
the oldest user message that is likely to remain stable across turns. Anthropic's prompt
caching makes cache reads ~10× cheaper than fresh input tokens and reduces latency for long
contexts.

We never send cache control headers. For any session longer than a few turns this is a
meaningful cost difference.

**What to build:** In `pkg/ai/providers/anthropic/anthropic.go`, add cache breakpoints to the
system prompt and to the last user message before the trailing exchange. Add
`cache_retention: "none" | "short" | "long"` to `FileConfig` / `StreamOptions`.

---

### 9. Cache Token Tracking
- [x] **Status:** Done — Anthropic `cache_read_input_tokens`/`cache_creation_input_tokens`, OpenAI `prompt_tokens_details.cached_tokens`, Google `cachedContentTokenCount` all parsed into `ai.Usage.CacheRead`/`CacheWrite`
- **Effort:** Low | **Impact:** Medium

Pi's `Usage` struct has `cacheRead` and `cacheWrite` token fields returned by Anthropic and
Google. Our `ai.Usage` only has `Input`, `Output`, `TotalTokens`. Without cache token fields:
- Cost estimates are wrong for cached sessions (cache reads are billed differently)
- You cannot tell whether the prompt cache is being hit

**What to build:** Add `CacheRead int` and `CacheWrite int` to `ai.Usage`. Parse from
Anthropic's `cache_read_input_tokens` / `cache_creation_input_tokens` and Google's equivalent
fields. Update all providers that return cache usage.

---

### 10. Skills System
- [x] **Status:** Done — `pkg/skills/` discovers SKILL.md files from `~/.config/agent/skills/` and `.agent/skills/`. `FormatSkillsForPrompt()` injects `<available_skills>` XML block. `/skills` REPL command. Wired into CLI and `BuildSystemPrompt()`
- **Effort:** Medium | **Impact:** Medium

Pi loads Markdown files with YAML frontmatter from `~/.pi/agent/skills/` (user-level) and
`.pi/skills/` (project-level). Each skill has a `name`, `description`, and body content. The
system prompt gets an `<available_skills>` XML block listing skills with their descriptions
and file paths. When the agent encounters a relevant task it reads the skill file using the
`read` tool to get specialised instructions.

This is a lightweight form of prompt-based RAG — reusable instruction libraries that the
agent pulls in on demand without polluting the base system prompt.

We have no skill loading.

**What to build:** `pkg/skills/` package — `LoadSkills(cwd, agentDir string) []Skill` that
scans for `SKILL.md` files under subdirectories. Add `FormatSkillsForPrompt(skills) string`
that produces the `<available_skills>` XML block. Wire into `BuildSystemPrompt()`.

---

### 11. Prompt Templates
- [x] **Status:** Done — `pkg/prompts/` loads `.md` files from `~/.config/agent/prompts/` and `.agent/prompts/`. `Expand()` handles `$1`, `$@`, `$ARGUMENTS`, `${@:N}`, `${@:N:L}`. `/templates` REPL command. Input auto-expanded before sending to agent
- **Effort:** Medium | **Impact:** Medium

Pi loads Markdown files from `~/.pi/agent/prompts/` and `.pi/prompts/`. In the REPL, typing
`/template-name arg1 arg2` expands the template, substituting `$1`, `$2`, `$@`, `$ARGUMENTS`,
and `${@:N:L}` (bash-style slicing). The expansion happens before the text is sent to the
agent, so the template content becomes the actual user message.

We have no `/command` expansion in the REPL.

**What to build:** `pkg/prompts/` package — `LoadTemplates(cwd, agentDir string) []Template`
and `Expand(text string, templates []Template) string`. Integrate into the REPL input loop in
`cmd/agent/main.go` so any `/name ...` input is expanded before being sent to the agent.

---

### 12. Turn-level Token Tracking
- [x] **Status:** Done — `pkg/agent/tokens.go`: `EstimateContextTokens()` (chars/4 heuristic anchored on last real usage). `ContextUsage` emitted in `EventTurnEnd`. `State.ContextTokens` field. `/state` REPL shows context token count
- **Effort:** Medium | **Impact:** Medium

Pi exposes the estimated context token count after each turn (via `ContextUsage` in the
extension and session APIs). This is the prerequisite for auto-compaction (#4): you can't
know when to compact if you don't know how full the context window is.

Pi uses a two-part estimate: the `usage` object from the last assistant message gives the
exact count up to that point; any messages added after that (steering, tool results, the next
user message) are estimated at chars/4.

**What to build:** Add `EstimateContextTokens(msgs []ai.Message) int` to `pkg/agent/`.
Expose `ContextTokens int` on `agent.State`. Emit a `EventContextUsage` event (or include it
in `EventTurnEnd`) after each turn so callers can monitor context growth.

---

## 🟢 Lower Priority — Nice-to-have

### 13. Session Branching
- [ ] **Status:** Not started
- **Effort:** High | **Impact:** Low
- **Requires:** Session persistence (#3)

Pi can fork a session at any message boundary, let the agent explore an alternative path in a
child session, generate a branch summary, and optionally return to the parent. Useful for
comparing approaches or recovering from a bad turn without losing the original history.

**What to build:** Add `Fork(fromMessageID string) (*Session, error)` and
`GenerateBranchSummary(model, apiKey string) (string, error)` to the session manager. Add
a `/fork` REPL command.

---

### 14. HTML Export
- [x] **Status:** Done — `pkg/session/export.go`: `ExportHTML(data, opts)` renders a self-contained dark-mode HTML file with user/assistant/tool/thinking/compaction blocks, inline usage stats, and zero external dependencies. `/export` REPL command writes `session-<id>.html` to cwd. `--export` flag available via CLI
- **Effort:** Medium | **Impact:** Low
- **Requires:** Session persistence (#3)

Pi can export a full session — messages, tool calls with arguments, diffs, thinking blocks —
as a self-contained HTML file suitable for sharing or archiving.

**What to build:** `pkg/session/export.go` — `ExportHTML(session *Session) ([]byte, error)`
that renders messages to an HTML template with syntax highlighting for code blocks and diffs.
Add a `/export` REPL command and `--export` CLI flag.

---

### 15. Pluggable Bash Operations
- [x] **Status:** Done — `pkg/tools/builtin/executor.go`: `Executor` interface with `Exec(ctx, command, cwd, onData)` → `(exitCode, error)`. `LocalExecutor` (default). `NewBashToolWithExecutor(cwd, exec)` for custom backends (Docker, SSH, sandbox). BashTool fully refactored to delegate all process management to the executor
- **Effort:** Medium | **Impact:** Low

Pi's bash tool accepts a `BashOperations` interface (`exec(command, cwd, opts)`) and a
`BashSpawnHook` that intercepts the command before execution, allowing delegation to SSH,
Docker, remote hosts, or sandboxed environments without changing the tool interface.

Our bash tool is hardcoded to spawn a local shell subprocess.

**What to build:** Extract an `Executor` interface from `pkg/tools/builtin/bash.go` with a
single `Exec(ctx, command, cwd string, onData func([]byte)) (exitCode int, err error)` method.
Default to `LocalExecutor`. Pass it through `BashOptions` when constructing the tool.

---

### 16. Proxy Stream Function
- [ ] **Status:** Not started
- **Effort:** Medium | **Impact:** Low

Pi has `streamProxy()` for routing all LLM calls through a central server that manages
authentication and proxies requests to providers. The server sends a bandwidth-optimised
event stream (partial message reconstructed client-side). Useful for team deployments where
a shared API key or rotating OAuth token is managed server-side.

**What to build:** `pkg/ai/providers/proxy/proxy.go` — `Provider` that POSTs to a configurable
server URL with a bearer token, reads the pi-compatible SSE event format, and reconstructs
`StreamEvent`s. Add `provider: proxy` / `base_url` / `auth_token` to config.

---

### 17. Model Registry
- [x] **Status:** Done — `pkg/ai/models/models.go`: 30+ models across Anthropic, OpenAI, Google, Groq, xAI, Mistral, Bedrock with context window, max output, cost/1M tokens, vision, thinking support. `Lookup(id)` with exact + fuzzy prefix matching. `ContextWindowFor(id)` auto-fills compaction config at startup. `/model` REPL command. CLI auto-discovers context window from registry if not set in config
- **Effort:** Medium | **Impact:** Low

Pi maintains a database of known models with metadata: context window size, max output tokens,
cost per million input/output/cache tokens, supported input modalities (text, image), and
whether the model supports reasoning. This data is used for cost estimation, overflow
detection thresholds, and UI display.

We have no model metadata — everything is inferred from the raw API responses.

**What to build:** `pkg/ai/models/models.go` — a `ModelInfo` struct and a package-level map
of well-known model IDs to their metadata. Add a `LookupModel(id string) *ModelInfo` helper.
Wire into cost tracking and as the default `contextWindow` for compaction.

---

### 18. Prompt Caching for Other Providers
- [ ] **Status:** Not started
- **Effort:** Medium | **Impact:** Low
- **Requires:** Cache token tracking (#9), Prompt cache headers (#8)

Google Gemini supports implicit prompt caching (no explicit breakpoints needed — the API
caches automatically based on prefix stability) and explicit caching via the Caches API.
OpenAI also caches automatically for inputs over 1024 tokens. Bedrock supports caching for
Claude models via `cacheConfig` in the system prompt.

Gap #8 covers Anthropic (the most impactful because it requires explicit breakpoints). This
gap tracks making the other providers cache-aware, primarily by ensuring our message
construction produces stable prefixes across turns and by exposing cache hit metrics.

**What to build:** Ensure Google, OpenAI, and Bedrock providers produce stable request
prefixes (system prompt always first, consistent ordering). Add cache hit reporting to usage
events. Document caching behaviour per provider in README.

---

## Implementation Order

| # | Gap | Effort | Impact | Status |
|---|-----|--------|--------|--------|
| 1 | Context overflow detection | Low | 🔴 High | ✅ Done |
| 2 | Tool argument validation | Low | 🔴 High | ✅ Done |
| 5 | System prompt builder | Low | 🟡 Medium | ✅ Done |
| 6 | Context files (AGENTS.md) | Low | 🟡 Medium | ✅ Done |
| 9 | Cache token tracking | Low | 🟡 Medium | ✅ Done |
| 7 | Thinking levels | Medium | 🟡 Medium | ✅ Done |
| 8 | Prompt cache headers (Anthropic) | Medium | 🟡 Medium | ✅ Done |
| 10 | Skills system | Medium | 🟡 Medium | ✅ Done |
| 11 | Prompt templates | Medium | 🟡 Medium | ✅ Done |
| 12 | Turn-level token tracking | Medium | 🟡 Medium | ✅ Done |
| 3 | Session persistence | High | 🔴 High | ✅ Done |
| 4 | Context compaction | High | 🔴 High | ✅ Done |
| 13 | Session branching | High | 🟢 Low | ✅ Done |
| 14 | HTML export | Medium | 🟢 Low | ✅ Done |
| 15 | Pluggable bash operations | Medium | 🟢 Low | ✅ Done |
| 16 | Proxy stream function | Medium | 🟢 Low | ✅ Done |
| 17 | Model registry | Medium | 🟢 Low | ✅ Done |
| 18 | Prompt caching (other providers) | Medium | 🟢 Low | ✅ Done |
