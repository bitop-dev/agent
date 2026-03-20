# Pattern 6: Prompt Composition

How to write system prompts that produce consistent, reliable agent behavior.
The system prompt is the most important lever you have — it determines whether the
agent does its job well, gets confused, or goes in circles.

---

## The three sources

A profile's system prompt is assembled from a list of sources, joined in order:

```yaml
spec:
  instructions:
    system:
      - ./prompts/base.md           # your local file
      - email/style-default         # a plugin-contributed prompt by ID
      - "Always cite sources."      # inline literal text
```

Resolved in order:
1. Plugin prompt ID → loads the file registered by the enabled plugin
2. File path → loaded relative to the profile directory
3. Inline text → used as-is

All entries are joined with a blank line (`\n\n`). See `docs/prompts.md` for the full spec.

---

## Pattern: persona + workflow + format

The most effective system prompts have three sections:

### 1. Persona (who the agent is)

```markdown
You are an AI news research assistant specialized in technology and AI industry coverage.
Your job is to research recent developments, synthesize findings, and deliver clear
summaries to busy professionals.
```

Sets the tone, the domain, and the purpose. Kept short — 2-3 sentences.

### 2. Workflow (what to do, in order)

```markdown
## Workflow — follow these steps in order

### Step 1 — Research
1. Search for the topic using ddg/search with a specific query
2. Search again with a different angle for broader coverage
3. Fetch the 2-3 most relevant articles using ddg/fetch

### Step 2 — Synthesize
Identify the 3-5 most significant stories. Note:
- The dominant theme
- What changed or was announced
- Who is affected and how

### Step 3 — Deliver
Send the email to the recipient specified in the user's message.
```

Numbered steps prevent the model from skipping actions. Explicit sequencing
is especially important for pipelines that must complete every step.

### 3. Output format (what the result looks like)

```markdown
## Email format

Subject: "AI News Briefing: [Topic] — [Date]"

Opening paragraph: 2-3 sentence executive summary of the major themes.

Key stories:
- **[Story title]** — 1-2 sentence summary. Source: URL
- **[Story title]** — 1-2 sentence summary. Source: URL

What to watch: 2-3 bullet points on follow-up items.

Sign off as: AI Research Assistant

Keep the body under 500 words.
```

Format instructions prevent the model from improvising structure that varies run to run.

---

## Pattern: rules and hard limits

Add explicit rules to prevent common failure modes:

```markdown
## Rules
- Always complete the full workflow — do not stop after researching
- Do not ask clarifying questions — infer from context and proceed
- Do not write files — your only output channel is email/send
- Maximum 3 ddg/search calls, maximum 4 ddg/fetch calls
- If a fetched page has very little content, skip it and try another URL
```

Rules should be:
- **Specific** — "maximum 3 searches" not "don't search too much"
- **Action-oriented** — "do X" or "do not do X"
- **Addressing known failure modes** — add a rule when you've seen the model fail

---

## Pattern: sub-agent prompt design

Sub-agents have a narrower job than top-level agents. Their prompts should reflect this.

### Sub-agent principles

**1. Tell it what NOT to do** — a researcher sub-agent should never send email:
```markdown
Do not send emails. Do not write files. Just return the structured summary as your final output.
```

**2. Define the exact output format** — the orchestrator reads the sub-agent's output
as a tool result. Structure it so the orchestrator can use it reliably:
```markdown
## Output format

Return ONLY a structured summary in this exact format:

**Topic:** <topic name>
**Key stories:**
- [Title] — <summary>. Source: <URL>
**Overall theme:** <one sentence>
```

**3. No preamble** — sub-agent output is consumed by the orchestrator, not shown to a human:
```markdown
Return ONLY the structured summary — no preamble, no "Here's what I found:", no sign-off.
```

**4. Keep it focused** — sub-agent prompts should be shorter than orchestrator prompts.
The sub-agent has one job. Don't distract it with policies or format rules that
don't apply to its task.

---

## Pattern: orchestrator prompt design

The orchestrator coordinates. Its prompt must:

**1. Tell it exactly what to delegate** — be specific about what task to give each sub-agent:

```markdown
### Step 1 — Spawn OpenAI researcher
Call agent/spawn with:
- task: "Research the latest OpenAI news. Find recent stories about products,
  models, funding, partnerships, or company news. Return a structured summary."
- profile: "researcher"
- maxTurns: 6
```

Without this specificity, the model may write its own task descriptions that
produce inconsistent sub-agent behavior.

**2. Tell it not to do the sub-agents' work itself**:

```markdown
Do not do any web searching yourself — delegate all research to sub-agents.
Do not summarize until you have received output from all sub-agents.
```

**3. Define what to do with the combined results**:

```markdown
### Step 3 — Compose and send the email
Once you have both research results, send a single email using email/send.
Combine the key stories from both sub-agents into a unified briefing.
```

---

## Pattern: composing plugin prompts with local prompts

Plugin prompts provide reusable fragments. Your local prompt provides context-specific behavior.
Combine them:

```yaml
spec:
  instructions:
    system:
      - ./prompts/persona.md         # who this specific agent is
      - email/style-default          # plugin: email writing conventions
      - ./prompts/constraints.md     # agent-specific rules and limits
```

**Order matters.** Later entries append to earlier ones.
Put the persona first — it sets the frame for everything that follows.

### Using a plugin prompt as a baseline

If the plugin author's prompt is 90% right, include it and add a small override after:

```yaml
system:
  - email/style-default              # plugin baseline: "concise professional emails"
  - "Exception: this agent writes to a technical audience. Use technical terms freely."
```

The model sees both instructions and applies both.

### Fully replacing a plugin prompt

Simply don't include the plugin's ID:

```yaml
system:
  - ./prompts/my-email-style.md     # your version entirely replaces the plugin's
```

---

## Pattern: inline text for dynamic context

Inline literal text is useful for injecting context that changes per run:

```yaml
system:
  - ./prompts/system.md
  - "Today's date is 2026-03-20. Focus on news from the last 7 days."
```

For truly dynamic values (dates, user names, etc.) you currently inject them
via the user prompt rather than the system prompt:

```bash
agent run --profile researcher \
  "Today is March 20 2026. Research AI news from this week. Send to nick@example.com"
```

The model picks up the date from the user message and applies it to its behavior.

---

## Common prompt failures and fixes

### Failure: agent stops after first tool call

Model calls `ddg/search` and then summarizes without fetching.

**Fix:**
```markdown
Always fetch at least 2 articles using ddg/fetch before synthesizing.
Do not produce a final summary based only on search snippets.
```

### Failure: agent sends email after drafting

When both `email/draft` and `email/send` are available, some models send
without confirming. The approval system handles this, but you can also be explicit:

```markdown
If you are unsure whether to send or draft, always draft first and say
"I've drafted the email — reply 'send it' to deliver it."
```

### Failure: orchestrator searches the web itself

The orchestrator has `agent/spawn` and `email/send` but no search tools.
If the model hallucinates tool calls to `ddg/search`, it will fail. Prevent confusion:

```markdown
You do not have web search tools. All research must be done by sub-agents.
Your only tools are agent/spawn and email/send.
```

### Failure: sub-agent output is inconsistent

Sub-agent returns free-form text instead of the structured format the orchestrator expects.

**Fix:** Make the format exact and use markers the orchestrator can reliably parse:

```markdown
Return your summary using EXACTLY this structure. Do not deviate:

RESEARCH_START
Topic: <topic>
Stories:
1. <title> | <1-2 sentence summary> | <URL>
2. <title> | <1-2 sentence summary> | <URL>
Theme: <one sentence>
RESEARCH_END
```

### Failure: agent loops on web research

Model keeps searching for more information without concluding.

**Fix:** Add hard limits and a stopping condition:

```markdown
Hard limits:
- Maximum 3 ddg/search calls
- Maximum 4 ddg/fetch calls
- If you have found 2 or more relevant articles, stop searching and synthesize
```

### Failure: long session context degrades behavior

On long sessions with many tool calls, the model's behavior can drift as
the context window fills up. The session compaction setting handles this:

```yaml
session:
  compaction: auto    # summarizes old turns when the context gets long
```

---

## Prompt file organization

Keep prompts next to their profile, organized by concern:

```
my-agent/
  profile.yaml
  prompts/
    system.md          ← main instructions (persona + workflow + format)
    constraints.md     ← rules and limits (optional, for long constraint lists)
    format.md          ← output format spec (optional, for complex formats)
```

Splitting into multiple files keeps each file focused. The profile combines them:

```yaml
spec:
  instructions:
    system:
      - ./prompts/system.md
      - ./prompts/constraints.md
      - ./prompts/format.md
```

---

## Related docs

- [`docs/prompts.md`](../prompts.md) — prompt resolution reference
- [`docs/profiles.md`](../profiles.md) — full profile spec including instructions field
- [Pattern 2: Research Agent](./02-research-agent.md) — example prompt for research agents
- [Pattern 4: Orchestrator](./04-orchestrator-sub-agents.md) — example prompts for orchestrators and sub-agents
