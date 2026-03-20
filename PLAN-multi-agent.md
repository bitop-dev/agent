# Multi-Agent Coordination — Design Plan

This document defines the phased plan for evolving the agent framework from
single-agent and simple orchestrator patterns into a full multi-agent
coordination system capable of handling complex, long-running tasks.

---

## Vision

A user should be able to describe a complex goal in natural language, and the
system should be able to:

1. Break it into sub-tasks
2. Assign sub-tasks to the right specialist agents
3. Run those agents in parallel where possible
4. Pass results between agents
5. Handle failures, retries, and partial completions
6. Run for hours or days if needed (long-running workflows)
7. Allow human oversight at configurable checkpoints

The same framework should also work for simple single-agent tasks where none
of this complexity is needed.

---

## Current state (what exists today)

| Capability | Status |
|---|---|
| Single agent with tools | ✅ Working |
| `agent/spawn` — sequential sub-agent | ✅ Working |
| `agent/spawn-parallel` — concurrent sub-agents | ✅ Working |
| Session persistence and resume | ✅ Working |
| Session compaction (pi-mono style) | ✅ Working |
| Depth limit (max 2 levels) | ✅ Working |
| Sub-agent isolation (deny-all approvals) | ✅ Working |

### Current limitations

| Limitation | Impact |
|---|---|
| One-way communication (parent → child → parent) | Child can't ask parent for clarification |
| No agent discovery | Parent must hard-code profile names |
| No shared state between children | Each child starts from scratch |
| No result routing between children | Parent manually passes output from A to B |
| No progress visibility | Parent blocks until child finishes |
| No long-running support | Everything runs in a single CLI invocation |
| No failure recovery | If a child fails, the parent gets an error string |
| No agent-to-agent direct communication | Everything routes through the orchestrator |

---

## Phased plan

### Phase 1: Agent Registry and Discovery

**Goal:** Agents can discover what other agents are available and what they can do.

**What it is:**
Today a profile is just a YAML file. There is no way for an agent to ask
"what agents exist and what are they good at?" An orchestrator must hard-code
profile names in its system prompt.

**What to build:**

1. **Agent manifest** — extend the profile YAML with discovery metadata:

   ```yaml
   # ~/.agent/profiles/researcher/profile.yaml
   metadata:
     name: researcher
     version: 0.1.0
     description: Focused web research agent using DuckDuckGo
     capabilities:
       - web-search
       - content-extraction
       - summarization
     accepts: "A topic or question to research"
     returns: "Structured summary with titles, URLs, and key findings"
   ```

2. **`agent/discover` tool** — a new host tool that lists available agents
   with their capabilities, descriptions, and input/output contracts:

   ```json
   {
     "agents": [
       {
         "name": "researcher",
         "description": "Focused web research agent using DuckDuckGo",
         "capabilities": ["web-search", "content-extraction"],
         "accepts": "A topic or question to research",
         "returns": "Structured summary with titles, URLs, and key findings"
       },
       {
         "name": "grafana-researcher",
         "description": "Queries Grafana for alerts, metrics, and logs",
         "capabilities": ["alert-query", "prometheus", "loki"],
         "accepts": "A team name and time range",
         "returns": "Structured alert and metric summary"
       }
     ]
   }
   ```

3. **Dynamic orchestration** — the orchestrator's system prompt no longer
   needs to hard-code profile names. Instead:

   ```markdown
   Step 1: Call agent/discover to see what agents are available.
   Step 2: Based on the user's request, choose the right agents.
   Step 3: Delegate using agent/spawn or agent/spawn-parallel.
   ```

**Why this matters:** The orchestrator becomes a general-purpose coordinator
that adapts to whatever agents are installed, instead of being a rigid
pipeline tied to specific profile names.

**Effort:** Small — extend profile spec, build one new host tool, update
the Profiles loader to expose discovery metadata.

---

### Phase 2: Structured Handoff Protocol

**Goal:** Agents communicate with structured context, not raw text.

**What it is:**
Today, the parent passes a task string to `agent/spawn` and gets back a text
string. There's no structured way to pass context, constraints, prior results,
or expected output format.

**What to build:**

1. **Handoff envelope** — structured JSON passed alongside the task string:

   ```json
   {
     "task": "Research Anthropic news from this week",
     "context": {
       "date": "2026-03-20",
       "prior_results": ["OpenAI section already completed"],
       "constraints": ["Use only sources from the past 7 days"],
       "output_format": "structured-summary"
     }
   }
   ```

2. **Extend SubRunRequest** to carry the context:

   ```go
   type SubRunRequest struct {
       Task         string
       Profile      string
       MaxTurns     int
       AllowedTools []string
       Context      map[string]any  // NEW: structured context
   }
   ```

3. **Inject context into the sub-agent's prompt** — the framework prepends
   the context as a structured block before the task:

   ```
   [Context from parent agent]
   Date: 2026-03-20
   Prior results: OpenAI section already completed
   Constraints: Use only sources from the past 7 days
   Expected output format: structured-summary

   [Task]
   Research Anthropic news from this week
   ```

**Why this matters:** Enables chained workflows where agent B receives
agent A's output as structured context, not just a paste of raw text.
Also enables constraints to flow downward (e.g., "only use these data sources").

**Effort:** Small-medium — extend SubRunRequest, update the runner to
inject context, update spawn tool descriptors.

---

### Phase 3: Result Routing and Pipelines

**Goal:** Agent A's output automatically becomes Agent B's input.

**What it is:**
Today the orchestrator manually sequences: spawn A → read output → spawn B
with A's output pasted into the task. This is brittle and wastes context window.

**What to build:**

1. **Pipeline definition** — declare a sequence of agents where outputs flow:

   ```yaml
   # pipeline.yaml or as part of a profile
   pipeline:
     - agent: researcher
       task: "Research {{topic}} news"
       as: research_output
     - agent: writer
       task: "Write a summary email from the research"
       context:
         research: "{{research_output}}"
       as: email_draft
     - agent: emailer
       task: "Send the email to {{recipient}}"
       context:
         body: "{{email_draft}}"
   ```

2. **`agent/pipeline` tool** — a new host tool that executes a pipeline:

   ```json
   {
     "pipeline": [
       {"agent": "researcher", "task": "...", "as": "research"},
       {"agent": "writer", "task": "...", "context": {"data": "{{research}}"}}
     ]
   }
   ```

3. **Pipeline runner in the framework** — executes steps in order,
   wires outputs to inputs, handles partial failures.

**Why this matters:** Complex multi-step workflows become declarative rather
than requiring a clever orchestrator prompt that might drift or skip steps.

**Effort:** Medium — new pipeline runner, template variable expansion,
pipeline YAML schema.

---

### Phase 4: Long-Running Tasks and Checkpoints

**Goal:** Agents can run for hours or days, surviving restarts and allowing
human review at checkpoints.

**What it is:**
Today everything runs in a single CLI invocation. If you Ctrl+C, the work is
lost (except what's in the session store). There's no way to run a 4-hour
research job that checks in periodically.

**What to build:**

1. **Task persistence** — save the full task state (pipeline position,
   intermediate results, agent states) to the session store:

   ```go
   type TaskState struct {
       ID              string
       Pipeline        []PipelineStep
       CurrentStep     int
       IntermediateResults map[string]string
       Status          string   // "running", "paused", "completed", "failed"
       CreatedAt       time.Time
       UpdatedAt       time.Time
   }
   ```

2. **Checkpoint/resume** — the framework periodically saves state so a
   task can be resumed after a crash or Ctrl+C:

   ```bash
   agent tasks list
   agent tasks resume <task-id>
   agent tasks status <task-id>
   ```

3. **Human checkpoints** — configurable pause points where the system
   stops and waits for human input before continuing:

   ```yaml
   pipeline:
     - agent: researcher
       task: "Research competitors"
     - checkpoint:
         message: "Review the research before proceeding to the report"
         requires: approval
     - agent: writer
         task: "Write the competitive analysis report"
   ```

4. **Background execution** — run tasks detached from the terminal:

   ```bash
   agent tasks start --background --profile ops-daily \
     "Run the daily ops check for all teams and email results"
   ```

**Why this matters:** Unlocks real-world use cases like nightly ops reports,
weekly research briefings, CI/CD-triggered analysis, and long research projects
that take multiple model calls across hours.

**Effort:** Large — task persistence, resume logic, background daemon,
checkpoint UI.

---

### Phase 5: Agent-to-Agent Communication

**Goal:** Agents can communicate directly without routing through an orchestrator.

**What it is:**
Today all communication goes: parent → child → parent. There's no way for
child A to ask child B a question, or for a worker agent to notify a monitor agent.

**What to build:**

1. **Message bus** — a simple in-process pub/sub system where agents can
   send and receive messages:

   ```go
   type AgentMessage struct {
       From      string         // agent ID
       To        string         // agent ID or "broadcast"
       Type      string         // "request", "response", "notification"
       Content   string
       Data      map[string]any
       ReplyTo   string         // message ID for threading
   }
   ```

2. **`agent/send-message` and `agent/receive-messages` tools** — agents
   can communicate without the orchestrator relaying:

   ```json
   // Agent A sends to Agent B
   {"to": "agent-b", "type": "request", "content": "What's the CPU usage on host X?"}

   // Agent B receives and responds
   {"from": "agent-a", "type": "request", "content": "What's the CPU usage on host X?"}
   // Agent B calls its tools, then responds:
   {"to": "agent-a", "type": "response", "content": "CPU is at 85% for the last hour"}
   ```

3. **Agent lifecycle management** — agents that stay alive and listen for
   messages, rather than running once and exiting:

   ```yaml
   # profile with "service" mode
   spec:
     mode: service       # "oneshot" (default) or "service"
     listen:
       - type: message
         from: "*"
   ```

**Why this matters:** Enables reactive multi-agent systems where a monitoring
agent detects an issue and asks a diagnostic agent to investigate, without
a central orchestrator managing the flow.

**Effort:** Large — message bus, agent lifecycle, service mode.

---

### Phase 6: Agent-as-MCP-Server

**Goal:** Expose agent capabilities over MCP so external tools (Claude Desktop,
Cursor, other MCP clients) can use agents as tool providers.

**What it is:**
Today the agent framework is an MCP client — it can call MCP servers.
But it can't be called as an MCP server by other tools.

**What to build:**

1. **MCP server mode** — run the agent as a tool server:

   ```bash
   agent serve --profile researcher --transport stdio
   agent serve --profile grafana-researcher --transport sse --addr localhost:8000
   ```

2. **Auto-generate MCP tool definitions** from the profile's enabled tools
   and system prompt. External clients see the agent as a single tool:

   ```json
   {
     "name": "researcher",
     "description": "Focused web research agent using DuckDuckGo",
     "inputSchema": {
       "type": "object",
       "properties": {
         "task": {"type": "string", "description": "Research topic or question"}
       }
     }
   }
   ```

3. **Bidirectional MCP** — the agent is both a client (calling plugins)
   and a server (being called by external tools) simultaneously.

**Why this matters:** Agents become composable across tools. A Claude Desktop
user could call "researcher" as a tool. A Cursor user could call "code-reviewer"
as a tool. Agents become a reusable service layer.

**Effort:** Medium — MCP server transport, tool definition generation.

---

## Implementation priority

| Phase | Value | Effort | Dependencies |
|---|---|---|---|
| 1. Agent Discovery | High | Small | None |
| 2. Structured Handoff | High | Small | Phase 1 helps but not required |
| 3. Result Routing / Pipelines | High | Medium | Phase 2 |
| 4. Long-Running / Checkpoints | High | Large | Phase 3 |
| 5. Agent-to-Agent Comms | Medium | Large | Phase 4 |
| 6. Agent-as-MCP-Server | High | Medium | Phase 1 |

**Recommended order:** 1 → 2 → 6 → 3 → 4 → 5

Rationale: Phase 1 and 2 are quick wins that make the current orchestrator
pattern dramatically more powerful. Phase 6 (MCP server) is high value and
independent of 3-5. Phases 3-5 are the long-term architecture for complex
multi-agent workflows.

---

## Design principles

1. **Simple things should be simple.** A single-agent task with one tool
   should not require understanding any of this multi-agent machinery.

2. **Complexity is opt-in.** Each phase adds capability without changing
   how existing simpler patterns work.

3. **Profiles are the unit of composition.** An agent is defined by its
   profile. Multi-agent coordination composes profiles, not code.

4. **Text is the universal interface.** Agents communicate via text (natural
   language or structured formats). No binary protocols between agents.

5. **The framework coordinates, models decide.** The framework provides
   the plumbing (discovery, routing, lifecycle). The models decide what
   to do with it.

6. **Fail gracefully.** A failed sub-agent should not crash the parent.
   Partial results are better than no results.

7. **Human oversight is configurable, not mandatory.** Fully automated
   pipelines and human-in-the-loop workflows use the same primitives.

---

## Example: what a complex task looks like at Phase 4

```bash
agent tasks start --profile ops-weekly-report \
  "Generate the weekly ops report for all teams. Research any incidents,
   check Grafana alerts, pull deployment logs, and send a summary email
   to the engineering leads."
```

Behind the scenes:

```
ops-weekly-report (orchestrator)
├── agent/discover → finds: grafana-researcher, researcher, emailer
├── agent/spawn-parallel:
│   ├── grafana-researcher: "Alert summary for team ict-aipe, last 7 days"
│   ├── grafana-researcher: "Alert summary for team ict-lps, last 7 days"
│   ├── grafana-researcher: "Alert summary for team ict-mps, last 7 days"
│   └── researcher: "Recent deployment incidents across UF IT"
├── checkpoint: "Review the collected data before writing the report"
├── writer: "Compose the weekly ops report from: {{all_results}}"
├── checkpoint: "Review the draft report before sending"
└── emailer: "Send the report to eng-leads@ufl.edu"
```

Total runtime: 15-20 minutes. Runs in the background. Pauses at checkpoints.
Resumes after human approval. Sends the final email automatically.

---

## Example: what it looks like at Phase 6

```json
// claude-desktop config
{
  "mcpServers": {
    "researcher": {
      "command": "agent",
      "args": ["serve", "--profile", "researcher", "--transport", "stdio"]
    },
    "grafana": {
      "command": "agent",
      "args": ["serve", "--profile", "grafana-researcher", "--transport", "stdio"]
    }
  }
}
```

Now Claude Desktop can call "researcher" and "grafana" as tools — each one
is a full agent with its own model, tools, and system prompt running behind
the MCP interface.
