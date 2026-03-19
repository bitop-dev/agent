# Go Agent Framework Plan

## Scope

This plan reviews the frameworks fetched under `opensrc/` and uses them to shape a new Go-based agent framework.

Reviewed repositories:

- `opensrc/repos/github.com/openclaw/openclaw`
- `opensrc/repos/github.com/badlogic/pi-mono`
- `opensrc/repos/github.com/NVIDIA/OpenShell`
- `opensrc/repos/github.com/sipeed/picoclaw`
- `opensrc/repos/github.com/zeroclaw-labs/zeroclaw`

## What each project is optimizing for

### OpenClaw

- Personal assistant product, not just an agent SDK.
- Strong channel and device integration.
- Huge plugin and extension surface.
- Gateway-centric architecture.

Strengths:

- Deep real-world UX: onboarding, channels, sessions, apps, tools.
- Mature plugin model and broad ecosystem surface.
- Good separation between gateway, clients, channels, and runtime.

Tradeoffs:

- Very large surface area.
- TypeScript monorepo and plugin API create substantial complexity.
- Easy to accumulate product sprawl before core runtime is crisp.

### Pi Mono

- Clean, embeddable coding-agent runtime.
- Strong event model and terminal ergonomics.
- Explicitly favors extensibility over built-in complexity.

Strengths:

- Clear core/runtime split: `pi-ai`, `pi-agent-core`, coding harness.
- Excellent event stream model for UI and tool execution.
- Sessions and branching are practical and lightweight.

Tradeoffs:

- Fewer batteries included around orchestration and deployment.
- Intentionally avoids some higher-level features users may still want later.

### OpenShell

- Secure runtime and sandbox platform for autonomous agents.
- Treats the agent as a workload that needs policy, isolation, and routing.

Strengths:

- Best security posture of the set.
- Clear control plane and data plane boundaries.
- Provider, policy, sandbox, gateway, and TUI are separated well.

Tradeoffs:

- Heavyweight operational model.
- K3s-inside-container approach is powerful but more than most local users need.
- Better as infrastructure around agents than as the agent framework itself.

### PicoClaw

- Single-binary Go assistant optimized for small hardware.
- Broad capabilities without giving up the Go deployment story.

Strengths:

- Best language fit for our direction.
- Practical module layout: agent, channels, providers, tools, memory, gateway, routing.
- Good proof that a useful multi-capability agent can stay in one Go binary.

Tradeoffs:

- Feature growth is already pushing memory and complexity upward.
- Security roadmap is still catching up to capability expansion.
- Risk of becoming an all-in-one product before the framework layer is stabilized.

### ZeroClaw

- Trait-driven Rust runtime optimized for performance and portability.
- Strong emphasis on pluggability and low resource usage.

Strengths:

- Best architecture discipline of the lightweight projects.
- Good separation of providers, channels, tools, memory, observability, runtime, and peripherals.
- Feature flags keep optional capability out of the hot path.

Tradeoffs:

- Lower-level extensibility may be great for maintainers but heavier for app developers.
- Some plugin and feature breadth may be hard to match without growing complexity.

## Patterns worth stealing

### 1. Small core, optional capabilities

Best source: Pi Mono, ZeroClaw, OpenClaw.

Keep the core runtime tiny:

- conversation loop
- tool calling
- streaming events
- session state
- model/provider abstraction
- policy hooks

Everything else should be optional:

- channels
- browser automation
- web UI
- scheduling
- memory backends
- sandbox adapters

### 2. Event-first runtime

Best source: `pi-agent-core`.

The internal runtime should publish typed events for:

- run start/end
- turn start/end
- assistant deltas
- tool start/progress/end
- approvals requested
- memory writes
- retries and errors

This lets us support:

- CLI
- TUI
- web UI
- logs/traces
- remote control

without coupling the runtime to any single interface.

### 3. Local-first, single binary by default

Best source: PicoClaw and ZeroClaw.

The best Go advantage is distribution simplicity. The default install should be:

- one binary
- one config dir
- sqlite by default
- no Docker or Kubernetes required

Remote or multi-tenant deployment can come later.

### 4. Security as a first-class subsystem

Best source: OpenShell and OpenClaw.

Do not bolt on safety later. Add policy boundaries from day one:

- filesystem scope
- command allow/block lists
- network egress rules
- provider credential isolation
- approval gates for risky tools
- audit logs for tool execution

### 5. Interfaces instead of framework lock-in

Best source: ZeroClaw traits, Pi package split.

In Go, this means interfaces plus registration, not a giant inheritance tree.

Core extension points:

- `Provider`
- `Tool`
- `Memory`
- `SessionStore`
- `PolicyEngine`
- `Channel`
- `RuntimeObserver`
- `SandboxAdapter`

### 6. Product layer separate from framework layer

Best source: Pi Mono and OpenClaw together.

We should not build only a library and we should not build only a giant product.

Recommended split:

- framework/runtime packages
- first-party CLI app
- optional gateway/server
- optional channel packages

## Patterns to avoid

- Building every integration into core.
- Making sub-agents and swarms mandatory for basic tasks.
- Requiring containers/Kubernetes for local use.
- Using Go `plugin` as the primary extension model.
- Treating memory, channels, and browser automation as inseparable from the runtime.

## Recommendation: our product position

Build a Go-native agent framework with three layers:

1. `core`: deterministic runtime, tools, events, state, policies.
2. `app`: a first-party coding/personal assistant CLI built on `core`.
3. `edge`: optional gateway and remote runners for channels, webhooks, and browser tasks.

This gives us:

- Pi-like developer clarity
- PicoClaw-like deployment and performance
- OpenShell-like security posture
- OpenClaw-like growth path without starting there

## Proposed architecture

### Package layout

```text
cmd/
  agent/
  agentd/
internal/
  app/
pkg/
  runtime/
  provider/
  tool/
  memory/
  policy/
  session/
  events/
  config/
  sandbox/
  channel/
  gateway/
  approval/
  observability/
  mcp/
  skills/
```

### Core runtime responsibilities

- manage conversation state
- stream model output
- detect and execute tool calls
- enforce policy before tool execution
- support human approval checkpoints
- publish typed events
- persist session history
- compact context when needed

### Core interfaces

```go
type Provider interface {
    Stream(ctx context.Context, req CompletionRequest) (<-chan Event, error)
}

type Tool interface {
    Name() string
    Schema() ToolSchema
    Run(ctx context.Context, call ToolCall) (ToolResult, error)
}

type PolicyEngine interface {
    CheckToolCall(ctx context.Context, call ToolCall) (Decision, error)
}

type Memory interface {
    Recall(ctx context.Context, query RecallQuery) ([]MemoryItem, error)
    Store(ctx context.Context, item MemoryItem) error
}
```

### State model

Use an append-only event log plus materialized session state.

- append-only log for replay/debugging
- session snapshot for fast resume
- sqlite as default local store
- jsonl export/import for portability

This takes the best parts of Pi session files and adds stronger runtime replay.

### Security model

Default posture:

- tools disabled unless registered and allowed
- command execution limited to workspace
- network tools off by default
- file writes scoped to workspace
- dangerous operations require approval
- credential injection done per provider call, not globally

### Extension model

Do not use Go native plugins as the default.

Prefer three extension modes:

- in-process Go packages for first-party and custom builds
- MCP for external tools and services
- gRPC or JSON-RPC sidecars for isolated extensions

That gives us portability and avoids ABI pain.

## MVP recommendation

### Phase 1: framework core

- runtime loop
- provider abstraction
- tool registry
- event bus
- sqlite session store
- config loading
- policy hooks

### Phase 2: first-party coding agent

- CLI chat loop
- file tools
- bash tool with policy gate
- session resume
- context compaction
- simple approval UI

### Phase 3: ecosystem hooks

- MCP client
- remote gateway API
- web UI or TUI
- memory plugins
- browser automation adapter

### Phase 4: production hardening

- audit trails
- multi-user auth
- sandbox runners
- network policy enforcement
- tracing and metrics

## Opinionated defaults I recommend

- Go for runtime and CLI.
- Cobra only for thin command routing, not business logic.
- SQLite for local state.
- JSON schema for tool definitions.
- SSE/WebSocket event stream for remote clients.
- MCP compatibility, but not MCP-only architecture.
- Human approval checkpoints before high-risk tools.
- Config profile system for local, team, and production modes.

## Design choices to settle early

1. Are we building primarily a coding agent, a personal assistant, or a general agent framework with one flagship app?
2. Will remote execution be a first-class goal in v1, or can it wait until the local runtime is excellent?
3. Do we want multi-agent support in v1, or only a clean single-agent runtime with extensibility for later?
4. Will skills be plain prompt assets, executable workflows, or both?
5. How strict should the default security posture be for shell and network tools?

## My recommendation

Start with a single-agent coding runtime that is:

- local-first
- event-driven
- policy-aware
- single-binary
- MCP-compatible
- easy to embed

Then grow outward into gateway, channels, and sandboxing only after the core loop and developer UX feel excellent.

That is the clearest gap in the current landscape and the best fit for Go.
