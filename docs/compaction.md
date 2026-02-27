# Context Compaction

LLMs have finite context windows. When a conversation grows too long, the
agent automatically summarizes the older portion and replaces it with a
structured Markdown summary — freeing up space while preserving the essential
context.

---

## How It Works

Before every LLM call, the agent checks:

```
context_tokens > context_window - reserve_tokens
```

If this threshold is exceeded, compaction runs:

1. **Find cut point** — Walk backward from the newest message, accumulating
   token estimates until `keep_recent_tokens` are collected. The cut always
   falls on a `user` message boundary (tool call / tool result pairs are
   never split).

2. **Summarise** — Send the older portion to the LLM with a structured
   summary prompt. The LLM produces a Markdown document tracking goal,
   progress, key decisions, and next steps.

3. **Persist** — A `compaction` entry is appended to the session JSONL file
   with the summary and the ID of the first message to keep verbatim.

4. **Continue** — The next LLM call sees:
   - System prompt
   - Compaction summary (as a user message)
   - Messages from `first_kept_entry_id` onwards

```
Before compaction:
  [user] [assistant/tools] [user] [assistant/tools] [user] [assistant]
  |___________ summarized _________|_______________ kept ______________|
                                   ↑
                          first_kept_entry_id

What the LLM sees after compaction:
  [system] [summary] [user] [assistant/tools] [user] [assistant]
                     |_________________ kept __________________|
```

---

## Configuration

```yaml
compaction:
  enabled: true           # Default: true
  context_window: 200000  # Tokens in context window (auto-filled from model registry if omitted)
  reserve_tokens: 16384   # Tokens to reserve for response (default: 16384)
  keep_recent_tokens: 20000  # Tokens to keep verbatim (default: 20000)
```

Compaction triggers when:

```
context_tokens > context_window - reserve_tokens
              > 200000 - 16384
              > 183616 tokens
```

Messages newer than `keep_recent_tokens` (~20k tokens) are always kept
verbatim and never summarized.

### Context Window Resolution

The compaction `context_window` is resolved in this order:

1. `compaction.context_window` (explicit config field)
2. Top-level `context_window` config field
3. Model registry lookup for the configured `model`
4. 0 (compaction disabled if no window is known)

---

## Summary Format

The generated summary follows a structured Markdown format:

```markdown
## Goal
[What the user is trying to accomplish]

## Constraints & Preferences
- [Requirements and constraints mentioned by user]

## Progress
### Done
- [x] [Completed tasks]

### In Progress
- [ ] [Current work]

### Blocked
- [Blockers, if any]

## Key Decisions
- **[Decision]**: [Rationale and context]

## Next Steps
1. [What should happen next]
2. [...]

## Critical Context
- [Data, values, or state needed to continue]

<read-files>
path/to/file1.go
path/to/file2.go
</read-files>

<modified-files>
path/to/changed.go
</modified-files>
```

The `<read-files>` and `<modified-files>` sections list files the agent
accessed/changed, helping it re-read relevant context after compaction.

---

## Incremental Compaction

When a previous compaction summary exists, the new summary prompt includes it
as `prevSummary`. The LLM is instructed to merge the old summary with the new
messages, producing a single coherent document. This prevents information loss
across multiple compaction cycles.

---

## Cut Point Rules

The compaction algorithm walks backward through messages and finds a cut point:

- **Valid cut points:** user messages, assistant messages, bash-execution messages
- **Never cut at:** tool_result messages (they must stay paired with their tool call)
- **Fall-through:** If even the newest single turn exceeds `keep_recent_tokens`,
  the cut lands mid-turn at an assistant message. Both the history and the
  partial turn are summarized separately and merged.

---

## Compaction Event

When compaction runs, an `EventCompaction` is broadcast:

```go
agent.Subscribe(func(e agent.Event) {
    if e.Type == agent.EventCompaction && e.Compaction != nil {
        fmt.Printf("Compacted: %d → %d tokens, removed %d messages\n",
            e.Compaction.TokensBefore,
            e.Compaction.TokensAfter,
            e.Compaction.MessagesRemoved,
        )
    }
})
```

---

## Disabling Compaction

Set `compaction.enabled: false` in your config. The agent will continue
conversations until the provider returns a context-overflow error.

```yaml
compaction:
  enabled: false
```

---

## Token Estimation

The agent estimates token counts using a simple heuristic:

```
tokens ≈ characters / 4
```

This is conservative and intentionally over-estimates to avoid triggering
compaction too late. The estimate is updated after each turn using the
actual usage reported by the provider.

After each turn, `EventTurnEnd` includes a `ContextUsage` snapshot:

```go
type ContextUsage struct {
    Tokens         int  // estimated total tokens in context
    UsageTokens    int  // tokens from last provider usage report
    TrailingTokens int  // estimated tokens added since last report
}
```
