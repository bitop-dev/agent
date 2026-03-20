# Pattern 3: Research and Action Pipeline

An agent that researches a topic and then takes an action with the results —
in this case sending an email, but the pattern applies to any "gather then deliver" workflow.

---

## What it looks like

```
User prompt
    └── Model calls ddg/search
        └── Real search results
    └── Model calls ddg/fetch (top articles)
        └── Real page content
    └── Model synthesizes findings
    └── Model calls email/send
        └── Email delivered
    └── Model confirms completion
```

One agent. One uninterrupted workflow. No human-in-the-loop unless a tool is marked sensitive.

---

## Why a single agent instead of two

For simple pipelines, a single agent with all the tools is sufficient and simpler.
The model is capable of sequencing research → email without explicit orchestration.

Use a single agent when:
- The research and delivery are tightly coupled
- You want the model to write the email itself based on what it found
- The workflow is a fixed sequence with no branching

Use sub-agents (Pattern 4) when:
- You want multiple independent research threads
- The research topics are unrelated
- You want cleaner separation of concerns

---

## Plugins required

| Plugin | Tools used | What it provides |
|---|---|---|
| `ddg-research` | `ddg/search`, `ddg/fetch` | Real web search and page extraction |
| `send-email` | `email/send` | SMTP email delivery |

### Install and configure

```bash
# Install from registry
agent plugins install ddg-research
agent plugins install send-email
agent plugins enable ddg-research
agent plugins enable send-email

# Configure SMTP
agent plugins config set send-email provider smtp
agent plugins config set send-email smtpHost mail.privateemail.com
agent plugins config set send-email smtpPort 587
agent plugins config set send-email username support@yourdomain.com
agent plugins config set send-email password 'your-password'
```

---

## Profile

```yaml
# ~/.agent/profiles/ai-news-researcher/profile.yaml
apiVersion: agent/v1
kind: Profile
metadata:
  name: ai-news-researcher
  version: 0.1.0
  description: Researches AI news and sends a summary email
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
      - email/send
  approval:
    mode: always        # auto-approve email/send — this agent is trusted
  workspace:
    required: false
    writeScope: read-only
  session:
    persistence: sqlite
    compaction: auto
  policy:
    overlays: []
```

### System prompt

The system prompt drives the entire workflow. Be explicit about the sequence.

```markdown
# ~/.agent/profiles/ai-news-researcher/prompts/system.md

You are an AI news research assistant. Your job is to research a topic,
synthesize the findings, and send a summary email.

## Workflow — follow these steps in order

### Step 1 — Research
1. Search for the topic using ddg/search with a specific query
2. Search again with a different angle to get broader coverage
3. Fetch the 2-3 most relevant and recent articles using ddg/fetch

### Step 2 — Synthesize
From the fetched content, identify:
- The 3-5 most significant stories or developments
- The dominant theme across all stories
- 2-3 things worth watching or following up on

### Step 3 — Send the email
Call email/send with:
- **to:** the recipient address provided in the user's request
- **subject:** a specific subject line (e.g. "AI News Briefing: [Topic] — [Date]")
- **body:** a well-structured email (see format below)

## Email format

Opening: 2-3 sentence executive summary of the major themes.

Key stories section:
- **[Story title]** — 1-2 sentence summary. Source: URL
- **[Story title]** — 1-2 sentence summary. Source: URL
- (repeat for each major story)

Closing: "What to watch" — 2-3 brief bullet points on follow-up items.

Sign off as: AI Research Assistant

Keep the body under 500 words. Always complete the full workflow.
Do not stop after researching — always send the email.
```

---

## Run it

```bash
agent run --profile ai-news-researcher \
  "Research the latest OpenAI news and send a summary to nick@example.com"
```

```
Running at 2026-03-20T10:39:05-04:00

[tool request] ddg/search
[tool finished] Search results for "latest OpenAI news": 1. OpenAI shifts to coding...

[tool request] ddg/fetch
[tool finished] OpenAI shifts to coding and enterprise as Anthropic pulls ahead...

[tool request] ddg/fetch
[tool finished] Anthropic unveils new AI model Claude Opus 4.6...

[tool request] email/send
[approval] tool email/send requires approval
[tool finished] sent email to nick@example.com with subject "AI News Briefing: OpenAI — March 2026"

Email sent. The briefing covers OpenAI's strategic shift toward coding and enterprise
in response to competition from Anthropic...

Session: 20260320T103905.675005000
```

---

## Approval strategies

`email/send` is marked as a sensitive action by the `send-email` plugin.
How you handle approval depends on your use case:

### Automated pipeline (no human review)

```yaml
approval:
  mode: always       # auto-approve everything
```

Use when: scheduled runs, CI/CD, trusted environments.

### Human reviews before sending

```yaml
approval:
  mode: on-request   # pause and prompt before email/send
```

Use when: the agent is sending to external recipients and you want to review first.

### Draft only (never sends)

```yaml
tools:
  enabled:
    - ddg/search
    - ddg/fetch
    - email/draft    # draft only — no send tool available
approval:
  mode: never
```

Use when: you want to see what the agent would send before committing.

---

## Adapting the pattern

### Different delivery channel

Swap `email/send` for any other action tool:

```yaml
tools:
  enabled:
    - ddg/search
    - ddg/fetch
    - slack/post-message    # post to Slack instead
```

### Save to file instead of sending

Add `core/write` and remove email tools:

```yaml
tools:
  enabled:
    - ddg/search
    - ddg/fetch
    - core/write
workspace:
  required: true
  writeScope: workspace
```

Update the system prompt:
```markdown
After researching, write a markdown file called `briefing.md` in the current
directory using core/write. Include all sources and a structured summary.
```

### Multiple topics in one run

The agent can handle complex prompts:

```bash
agent run --profile ai-news-researcher \
  "Research both OpenAI and Anthropic news this week. Compare their recent
   announcements and send a comparative briefing to team@example.com"
```

The model will search for both, fetch relevant articles, and write a comparative email
— all in one uninterrupted run.

---

## Session and resume

Because `session.persistence: sqlite` is set, every run is saved.
Resume a previous research session to ask follow-up questions:

```bash
agent sessions list
agent resume "What were the sources used for the Anthropic section?"
```

---

## Related patterns

- [Pattern 2: Research Agent](./02-research-agent.md) — research without the delivery step
- [Pattern 4: Orchestrator with Sub-Agents](./04-orchestrator-sub-agents.md) — parallel research threads
- [Pattern 5: Policy and Approval](./05-policy-and-approval-patterns.md) — controlling email delivery
