# Pattern 2: Research Agent

An agent that searches the web, fetches page content, and synthesizes a response.
This is the foundation for any agent that needs real-world information.

---

## What it looks like

```
User prompt
    └── Model calls ddg/search (query)
        └── Gets: list of titles, URLs, snippets
    └── Model calls ddg/fetch (URL 1)
        └── Gets: extracted page text
    └── Model calls ddg/fetch (URL 2)
        └── Gets: extracted page text
    └── Model synthesizes all results
        └── Produces final response
```

The model decides which URLs to fetch based on the search results.
It may call search multiple times with different queries to broaden coverage.

---

## The ddg-research plugin

The `ddg-research` plugin provides real web search via DuckDuckGo HTML and
page content extraction. It ships as a compiled Go binary — no external service needed.

```
ddg-research/
  plugin.yaml
  bin/
    web-tool          ← compiled Go binary (chmod +x)
  cmd/
    web-tool/
      main.go         ← source for rebuilding
      go.mod
  tools/
    search.yaml
    fetch.yaml
```

The binary reads a JSON request from stdin and writes a JSON response to stdout.
The plugin runtime type is `command` — the framework spawns the binary per tool call.

### Install and enable

```bash
agent plugins install ddg-research
agent plugins enable ddg-research
```

The binary is extracted from the registry tarball with its executable permissions intact.

---

## Profile

```yaml
# ~/.agent/profiles/researcher/profile.yaml
apiVersion: agent/v1
kind: Profile
metadata:
  name: researcher
  version: 0.1.0
  description: Web research agent — searches, fetches, and summarizes
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
    mode: never
  workspace:
    required: false
    writeScope: read-only
  session:
    persistence: none    # no session persistence for focused sub-tasks
  policy:
    overlays: []
```

### System prompt

The system prompt is what shapes the research behavior. This is the most important
part of the research agent pattern.

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

---

## Run it

```bash
agent run --profile researcher "What is the latest news about Anthropic?"
```

```
[tool request] ddg/search
[tool finished] Search results for "Anthropic latest news": 1. Anthropic unveils Claude...

[tool request] ddg/search
[tool finished] Search results for "Anthropic Claude AI 2025": ...

[tool request] ddg/fetch
[tool finished] Anthropic unveils new AI model Claude Opus 4.6...

[tool request] ddg/fetch
[tool finished] Panicked OpenAI execs as Anthropic pulls ahead...

**Topic:** Anthropic Latest News

**Key stories:**
- [Anthropic Unveils Claude Opus 4.6] — Anthropic released its most capable model yet...
  Source: https://...
- [Anthropic Sues Trump Administration] — Anthropic filed a lawsuit after the Pentagon
  designated it a "supply-chain risk"...
  Source: https://...

**Overall theme:** Anthropic is navigating both technical milestones and political
challenges as the AI race with OpenAI intensifies.
```

---

## Controlling search depth

The model decides how many searches and fetches to do. You can influence this with the system prompt:

**Shallow (fast, 1-2 searches):**
```markdown
Search once for the topic, fetch the top 1-2 results, and summarize.
Do not search more than twice.
```

**Deep (thorough, 3-5 searches):**
```markdown
Search multiple times with different queries to ensure broad coverage.
Fetch at least 3 articles before synthesizing. Look for primary sources.
```

**Targeted:**
```markdown
Search specifically for news from the last 7 days. Skip any results
older than one week. If you can't find recent content, say so.
```

---

## Preventing infinite loops

Without constraints, a model may loop — searching for more information because
each page only partially answers the question. The system prompt is your main tool:

```markdown
## Hard limits
- Maximum 3 ddg/search calls
- Maximum 4 ddg/fetch calls
- Stop and summarize what you have — do not continue searching if you have
  found at least 2 relevant articles
```

You can also set `maxTurns` when spawning this as a sub-agent (see Pattern 4).
The default is 4 turns — enough for 2 searches + 2 fetches + final response.

---

## Handling paywalled or blocked pages

Some pages (`nytimes.com`, etc.) return minimal content. The fetch tool extracts
whatever text is present. The model typically handles this gracefully and tries
another URL. Reinforce this in the system prompt:

```markdown
If a fetched page returns very little content (under 200 words), skip it
and fetch a different URL from the search results instead.
```

---

## Variations

### Focused domain search

Lock the search to a specific site or topic area:

```markdown
When searching, always add "site:techcrunch.com OR site:wired.com" to your
queries to focus results on technology journalism.
```

### Citation-heavy research

Require URLs for every claim:

```markdown
Every fact you include in the summary must have a source URL.
Never include a claim you cannot attribute to a specific fetched article.
```

### Comparative research

Give the agent two things to compare:

```bash
agent run --profile researcher \
  "Compare the latest news about OpenAI vs Anthropic. What is each company focused on?"
```

---

## Related patterns

- [Pattern 3: Research and Action Pipeline](./03-research-and-action-pipeline.md) — add email delivery
- [Pattern 4: Orchestrator with Sub-Agents](./04-orchestrator-sub-agents.md) — run multiple researchers in parallel
