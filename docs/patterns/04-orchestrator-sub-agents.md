# Pattern 4: Orchestrator with Sub-Agents

A parent agent that breaks a complex task into bounded sub-tasks and delegates each
to a specialist child agent. The parent coordinates; the children execute.

This is the most powerful pattern in the framework. It enables parallel research threads,
clean separation of concerns, and complex multi-step workflows.

---

## What it looks like

```
Orchestrator agent
    └── agent/spawn → researcher sub-agent #1
    │       └── ddg/search "OpenAI news"
    │       └── ddg/fetch  (articles)
    │       └── Returns: structured summary
    │
    └── agent/spawn → researcher sub-agent #2
    │       └── ddg/search "Anthropic news"
    │       └── ddg/fetch  (articles)
    │       └── Returns: structured summary
    │
    └── email/send
            └── Combines both summaries into one email
            └── Delivers to recipient
```

The orchestrator never touches the web. It only coordinates and delivers.
Each sub-agent runs in complete isolation with its own tool set and turn limit.

---

## How sub-agents work

When the orchestrator calls `agent/spawn`, the framework:

1. Loads the requested profile from `~/.agent/profiles/<name>/` or `.agent/profiles/`
2. Resolves that profile's tools from the plugin registry
3. Creates a full new `RunRequest` with the task as the user prompt
4. Runs the complete agent loop — model turns, tool calls, everything
5. Returns the sub-agent's final text output as a tool result to the orchestrator

The orchestrator's model sees the sub-agent's output as a string — the same as
any other tool result. It can then use that string in subsequent steps.

### Safety boundaries

Sub-agents have hard constraints that cannot be overridden:

| Constraint | What it means |
|---|---|
| **Max depth** | Default limit of 2 levels. A sub-agent cannot spawn its own sub-agents by default. |
| **Deny-all approvals** | Approvals are disabled inside sub-agents. Sensitive tool calls fail instead of prompting. |
| **Profile scoped** | Sub-agents only have access to tools in their profile — not the orchestrator's tools. |
| **Turn limit** | `maxTurns` caps how long a sub-agent can run (default 4, configurable per call). |

These constraints prevent runaway delegation and infinite recursion.

---

## Required plugin

The `spawn-sub-agent` plugin contributes the `agent/spawn` tool.
It uses the `host` runtime type — it has controlled access to the agent runtime itself.

```bash
agent plugins install spawn-sub-agent
agent plugins enable spawn-sub-agent
```

---

## Three profiles to create

This pattern requires three profiles:

### Profile 1 — Researcher (sub-agent)

The specialist that does the actual research work.
Must be in `~/.agent/profiles/` or `.agent/profiles/` so the orchestrator can find it by name.

```yaml
# ~/.agent/profiles/researcher/profile.yaml
apiVersion: agent/v1
kind: Profile
metadata:
  name: researcher
  version: 0.1.0
  description: Focused research sub-agent
spec:
  instructions:
    system:
      - ./prompts/system.md
  provider:
    default: openai
    model: nemotron-3-super-120b-a12b
  tools:
    enabled:
      - ddg/search
      - ddg/fetch
  approval:
    mode: never           # sub-agents use deny-all anyway; this is documentation
  workspace:
    required: false
    writeScope: read-only
  session:
    persistence: none     # sub-agents don't need their own sessions
  policy:
    overlays: []
```

```markdown
# ~/.agent/profiles/researcher/prompts/system.md

You are a focused research assistant. Your only job is to research a given topic
and return a structured summary.

## Workflow
1. Search for the topic using ddg/search with a specific, targeted query
2. Search again with a slightly different query to get broader coverage
3. Fetch the 2-3 most relevant and recent article URLs using ddg/fetch
4. Synthesize what you found into a structured summary

## Output format
Return ONLY a structured summary in this exact format — no preamble, no sign-off:

**Topic:** <topic name>

**Key stories:**
- [Story title] — <1-2 sentence summary>. Source: <URL>
- [Story title] — <1-2 sentence summary>. Source: <URL>
- [Story title] — <1-2 sentence summary>. Source: <URL>

**Overall theme:** <1 sentence describing the dominant theme across all stories>

Do not send emails. Do not write files. Just return the structured summary.
```

### Profile 2 — Orchestrator

The coordinator. Only has `agent/spawn` and `email/send` — no research tools.

```yaml
# ~/.agent/profiles/ai-news-orchestrator/profile.yaml
apiVersion: agent/v1
kind: Profile
metadata:
  name: ai-news-orchestrator
  version: 0.1.0
  description: Orchestrates researcher sub-agents then emails a combined summary
spec:
  instructions:
    system:
      - ./prompts/system.md
  provider:
    default: openai
    model: nemotron-3-super-120b-a12b
  tools:
    enabled:
      - agent/spawn
      - email/send
  approval:
    mode: always          # auto-approve spawn and email/send
  workspace:
    required: false
    writeScope: read-only
  session:
    persistence: sqlite
    compaction: auto
  policy:
    overlays: []
```

```markdown
# ~/.agent/profiles/ai-news-orchestrator/prompts/system.md

You are an AI news orchestrator. You coordinate specialist sub-agents to research
topics and produce a final email report.

## Your workflow — follow these steps in order

### Step 1 — Spawn OpenAI researcher
Call agent/spawn with:
- task: "Research the latest OpenAI news. Find recent stories about products,
  models, funding, partnerships, or company news. Return a structured summary."
- profile: "researcher"
- maxTurns: 6

### Step 2 — Spawn Anthropic researcher
Call agent/spawn with:
- task: "Research the latest Anthropic news. Find recent stories about Claude
  models, safety research, funding, partnerships, or company news. Return a
  structured summary."
- profile: "researcher"
- maxTurns: 6

### Step 3 — Compose and send the email
Once you have both research results, send a single email using email/send:
- Subject: "AI News Briefing: OpenAI & Anthropic — <today's date>"
- Opening: 2-3 sentence executive summary of the biggest themes
- OpenAI section: key stories from the first sub-agent's summary
- Anthropic section: key stories from the second sub-agent's summary
- Closing "What to watch": 2-3 developments worth following up on
- Sign off as "AI News Orchestrator"

## Rules
- Always run both researchers before composing the email
- Do not do any web searching yourself — delegate all research to sub-agents
- Keep the email body under 600 words
- Include source URLs for major stories
```

---

## Run it

```bash
agent run --profile ai-news-orchestrator \
  "Research the latest OpenAI and Anthropic news and send a summary email to nick@example.com"
```

```
Running at 2026-03-20T11:23:51-04:00

[tool request] agent/spawn
[policy] tool agent/spawn requires approval
[approval] tool agent/spawn requires approval      ← auto-approved (mode: always)
[tool finished] **Topic:** OpenAI Latest News
                **Key stories:**
                - [$110B Funding Round] — OpenAI raised $110 billion...
                - [Apple Partnership] — ChatGPT integrated into Apple Intelligence...
                ...

[tool request] agent/spawn
[approval] tool agent/spawn requires approval      ← auto-approved
[tool finished] **Topic:** Anthropic Latest News
                **Key stories:**
                - [Claude Opus 4.6] — Anthropic released its most capable model...
                - [Pentagon Lawsuit] — Anthropic sued the Trump administration...
                ...

[tool request] email/send
[approval] tool email/send requires approval       ← auto-approved
[tool finished] sent email to nick@example.com with subject "AI News Briefing: OpenAI & Anthropic — 2026-03-20"

I've completed the AI news briefing:
1. Researched OpenAI news via a researcher sub-agent
2. Researched Anthropic news via a researcher sub-agent
3. Composed and sent a combined briefing to nick@example.com

Session: 20260320T112351.675005000
```

---

## The approval messages explained

```
[tool request] agent/spawn
[policy] tool agent/spawn requires approval
[approval] tool agent/spawn requires approval
```

This sequence is **not** pausing for human input. Here's what each line means:

| Line | Meaning |
|---|---|
| `[tool request] agent/spawn` | The model decided to call agent/spawn |
| `[policy] tool agent/spawn requires approval` | Policy engine checked — sensitiveAction requires approval |
| `[approval] tool agent/spawn requires approval` | Approval resolver invoked — with `mode: always`, it auto-approved |

The 20-30 second gap before `[tool finished]` is the sub-agent running — doing real
web searches and fetching articles. It is not waiting for you.

---

## Tuning sub-agent behavior

### maxTurns

`maxTurns` controls how many model turns a sub-agent gets.
One turn = one model response (which may include multiple tool calls).

```
maxTurns: 4  → search → fetch → fetch → final summary   (tight)
maxTurns: 6  → search → search → fetch → fetch → fetch → final summary   (thorough)
maxTurns: 8  → deep research with multiple search angles   (slow but comprehensive)
```

Set it in the spawn call:
```json
{"task": "...", "profile": "researcher", "maxTurns": 6}
```

Or instruct the orchestrator in its system prompt:
```markdown
Spawn the researcher with maxTurns: 6 for thorough coverage.
```

### Parallel vs sequential

Sub-agents run **sequentially** — the orchestrator waits for sub-agent 1 to finish
before calling sub-agent 2. There is no true parallelism in the current framework.

The model processes the results in order, which means sub-agent 2 cannot be
influenced by sub-agent 1's findings (they run independently). This is intentional
for isolation.

---

## Variations

### Three or more sub-agents

```markdown
## Workflow

### Step 1 — Spawn OpenAI researcher
### Step 2 — Spawn Anthropic researcher
### Step 3 — Spawn Google DeepMind researcher
### Step 4 — Compose comparison email covering all three companies
```

### Sub-agent with different model

Give specialist sub-agents a different model than the orchestrator:

```yaml
# researcher/profile.yaml
spec:
  provider:
    model: claude-3.7-sonnet   # better at synthesis
```

```yaml
# orchestrator/profile.yaml
spec:
  provider:
    model: gpt-4o-mini         # cheaper for coordination
```

### Sub-agent that writes files

Give a sub-agent `core/write` instead of `email/send`:

```yaml
# writer/profile.yaml
spec:
  tools:
    enabled:
      - ddg/search
      - ddg/fetch
      - core/write
  workspace:
    required: true
    writeScope: workspace
```

Orchestrator task:
```
"Research Python packaging best practices and write a markdown summary
 to ./research/python-packaging.md"
```

### Chained sub-agents

Sub-agent output feeds into the next sub-agent's task:

```markdown
### Step 1 — Research
Spawn researcher with task: "Find the top 5 AI papers published this week. Return
titles, authors, and abstracts."

### Step 2 — Summarize
Spawn summarizer with task: "Given these papers: [paste sub-agent 1 output],
write a 200-word plain English explanation of the most important one."

### Step 3 — Send
Send the plain English summary to newsletter@example.com
```

---

## Important: profile discovery

Sub-agent profiles must be discoverable by the framework at runtime.
The orchestrator resolves profiles by name from:

```
~/.agent/profiles/<name>/profile.yaml       ← user-level, always available
.agent/profiles/<name>/profile.yaml         ← project-local, CWD must match
```

If you reference `profile: "researcher"` in your spawn call, the framework
will look for `~/.agent/profiles/researcher/profile.yaml`.

If it cannot find the profile, the sub-agent call fails with:
```
spawn-sub-agent: load profile "researcher": not found in configured profile directories
```

Fix: ensure the profile is installed in `~/.agent/profiles/` or provide an absolute path.

---

## Depth limits

The framework enforces a maximum spawn depth of 2 by default.

```
Level 0: orchestrator          (your top-level profile)
Level 1: researcher            (first sub-agent)
Level 2: sub-researcher        (second level sub-agent — allowed)
Level 3: blocked               (MaxDepth reached — spawn fails)
```

A sub-agent at depth 1 can spawn its own sub-agent (depth 2), but that
sub-agent cannot spawn further. This prevents runaway recursive delegation.

---

## Parallel sub-agents

For independent tasks, use `agent/spawn-parallel` instead of sequential `agent/spawn` calls.
This runs all tasks concurrently using goroutines, cutting total runtime significantly.

### Tool: `agent/spawn-parallel`

```yaml
tools:
  enabled:
    - agent/spawn-parallel
    - email/send
```

### Usage in the system prompt

```markdown
Call agent/spawn-parallel with a tasks array:

tasks:
  - task: "Research OpenAI news. Return a structured summary."
    profile: "researcher"
    maxTurns: 8
  - task: "Research Anthropic news. Return a structured summary."
    profile: "researcher"
    maxTurns: 8
  - task: "Research Google DeepMind news. Return a structured summary."
    profile: "researcher"
    maxTurns: 8
```

### How it works

All tasks execute concurrently. The framework:
1. Spawns a goroutine per task
2. Each runs its own full agent loop (profile, tools, model turns)
3. Waits for all to finish
4. Returns combined output with each task's result labelled

```
=== Task 1 (Research OpenAI news…) ===
**Topic:** OpenAI
**Key stories:** ...

=== Task 2 (Research Anthropic news…) ===
**Topic:** Anthropic
**Key stories:** ...

=== Task 3 (Research Google DeepMind news…) ===
**Topic:** Google DeepMind
**Key stories:** ...
```

### Error handling

If one sub-agent fails, the others still complete. Errors are reported inline:

```
=== Task 2 (Research Anthropic…) — ERROR ===
spawn-sub-agent: load profile "researcher": not found
```

The orchestrator receives all results and can decide how to proceed.

### When to use parallel vs sequential

| Use parallel when... | Use sequential when... |
|---|---|
| Tasks are independent | Task 2 depends on Task 1's output |
| You want speed | You want to chain results |
| Sub-agents don't share state | Sub-agents share a workspace |
| Research across different topics | Multi-step workflow on one topic |

### Example: 3-company news briefing with parallel research

```yaml
# orchestrator profile
spec:
  instructions:
    system:
      - |
        Step 1: Call agent/spawn-parallel with three research tasks:
          - OpenAI news
          - Anthropic news
          - Google DeepMind news
        Use profile "researcher", maxTurns 8 for each.

        Step 2: Combine the three summaries and call email/send.
  tools:
    enabled:
      - agent/spawn-parallel
      - email/send
```

This runs all three searches concurrently — total time is the duration of the
slowest sub-agent, not the sum of all three.

---

## Session compaction

When an agent session grows long (many tool calls, large tool results), the framework
automatically compacts the transcript to free up context window space.

Compaction is triggered when estimated context tokens exceed ~64k tokens.
It follows [pi-mono's compaction design](https://github.com/badlogic/pi-mono):

1. **Token-aware trigger** — not turn-count-based
2. **Turn-boundary cuts** — never splits between a tool call and its result
3. **Serialized conversation** — messages are converted to labeled text (`[User]:`,
   `[Tool result]:`) before summarizing so the LLM doesn't treat it as a conversation
4. **Structured summary format** — Goal, Constraints, Progress, Key Decisions, Next Steps
5. **Session persistence** — compaction summaries are saved as session entries

Compaction is enabled by default when `spec.session.compaction: auto` is set.
The most recent ~20k tokens are always kept verbatim.

---

## Related patterns

- [Pattern 2: Research Agent](./02-research-agent.md) — the sub-agent profile used here
- [Pattern 3: Research and Action Pipeline](./03-research-and-action-pipeline.md) — single-agent alternative
- [Pattern 5: Policy and Approval](./05-policy-and-approval-patterns.md) — controlling agent/spawn permissions
