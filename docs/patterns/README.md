# Agent Patterns

This directory documents the core patterns for building and composing agents with this framework.
Each pattern is a proven approach backed by working examples that were built and tested.

Patterns are ordered from simplest to most complex. Start with the first one and work forward.

---

## Pattern index

| Pattern | What it is | When to use it |
|---|---|---|
| [1. Single-tool agent](./01-single-tool-agent.md) | One plugin, one focused task | Wrapping a CLI, script, or API call |
| [2. Research agent](./02-research-agent.md) | Search → fetch → synthesize | Any task requiring real web data |
| [3. Research and action pipeline](./03-research-and-action-pipeline.md) | Research → produce output → deliver | News briefings, reports, notifications |
| [4. Orchestrator with sub-agents](./04-orchestrator-sub-agents.md) | Parent delegates tasks to child agents | Parallel research, multi-step workflows |
| [5. Policy and approval patterns](./05-policy-and-approval-patterns.md) | Controlling what agents can do | Production deployments, sensitive tools |
| [6. Prompt composition patterns](./06-prompt-composition.md) | Building effective system prompts | Any agent that needs consistent behavior |

---

## Core concepts before you start

### A plugin is not an agent

A plugin contributes tools. An agent is a profile that selects which tools to use,
which model to use, and how to behave. The same plugin can be used by dozens of
different agents with different behaviors.

```
Plugin (capability)          Profile (agent definition)
─────────────────            ──────────────────────────
ddg-research                 ai-news-researcher
  ddg/search         →         tools: [ddg/search, ddg/fetch, email/send]
  ddg/fetch                    model: nemotron-3-super-120b-a12b
                               system: "Research and email a summary..."
send-email
  email/draft        →         (same plugin, different agent)
  email/send                   tools: [email/draft]
                               model: gpt-4o-mini
                               system: "Draft emails in formal style..."
```

### The tool call cycle

Every agent run follows this loop:

```
1. User prompt arrives
2. Model receives: system prompt + conversation history + available tool definitions
3. Model either:
   a. Calls a tool → framework executes it → result added to history → back to step 2
   b. Produces a final text response → run ends
```

### Profiles live in two places

```
~/.agent/profiles/<name>/profile.yaml     # user-level, available everywhere
.agent/profiles/<name>/profile.yaml       # project-local, available in this directory
```

Sub-agents spawned by the orchestrator pattern also use these locations.
If a sub-agent profile isn't in one of these locations, the spawn will fail.

---

## Prerequisites for all patterns

All examples assume:

- The agent binary is built: `go build -o bin/agent ./cmd/agent`
- A provider is configured in `~/.agent/config.yaml`
- Plugins are installed and enabled as noted in each pattern

Quick provider setup:

```bash
# ~/.agent/config.yaml
providers:
  openai:
    baseURL: https://your-provider.com/v1
    apiKey: your-api-key
    apiMode: chat
```

Or via environment:

```bash
export OPENAI_BASE_URL=https://your-provider.com/v1
export OPENAI_API_KEY=your-api-key
```
