# Go Agent Framework Comparison And Direction

## Goal

This document does two things:

1. builds a practical comparison matrix across the reviewed agent frameworks
2. recommends the best product direction for a new Go-based agent system

## Repositories compared

- `opensrc/repos/github.com/openclaw/openclaw`
- `opensrc/repos/github.com/badlogic/pi-mono`
- `opensrc/repos/github.com/NVIDIA/OpenShell`
- `opensrc/repos/github.com/sipeed/picoclaw`
- `opensrc/repos/github.com/zeroclaw-labs/zeroclaw`

## Comparison matrix

Scoring is relative and opinionated:

- `High` = strong differentiator
- `Medium` = solid but not defining
- `Low` = weak or not a primary focus

| Project | Primary identity | Runtime clarity | UX and onboarding | Security model | Extensibility | Deployment simplicity | Product breadth | Best lesson for us | Main risk if copied too literally |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| OpenClaw | Personal assistant platform | Medium | High | Medium-High | High | Low-Medium | Very High | Build around real user workflows, not just a loop | We inherit a huge surface area too early |
| Pi Mono | Minimal coding-agent harness + SDK | High | High | Low-Medium | High | Medium | Medium | Keep the runtime event-first and composable | We underbuild safety and ops boundaries |
| OpenShell | Secure agent runtime platform | High | Medium | Very High | Medium-High | Low | Medium | Treat policy and sandboxing as first-class architecture | We build infrastructure before product fit |
| PicoClaw | Lightweight Go assistant | Medium-High | Medium-High | Medium-Low | Medium-High | High | High | Single-binary Go distribution is a superpower | Feature growth erodes simplicity fast |
| ZeroClaw | Lightweight trait-driven runtime OS | High | Medium | High | High | High | Medium-High | Keep modules swappable and optional | We over-abstract before validating usage |

## Category breakdown

### 1. Core runtime loop

| Project | Notes |
| --- | --- |
| Pi Mono | Best model of turns, message events, tool lifecycle, and session branching. The runtime is explicit and easy to reason about. |
| ZeroClaw | Strong separation of runtime concerns via traits and modules. Good architecture discipline. |
| OpenShell | Runtime is clear, but much of the design is about controlled execution environments rather than app-level conversation UX. |
| PicoClaw | Practical runtime with lots of features, but less obviously minimal than Pi Mono. |
| OpenClaw | Runtime exists inside a much larger product system, so it is harder to extract as a clean standalone model. |

### 2. Developer ergonomics

| Project | Notes |
| --- | --- |
| Pi Mono | Strongest developer ergonomics for embedding, customizing, and extending. |
| OpenClaw | Best end-user ergonomics, onboarding, and real-world interface coverage. |
| PicoClaw | Good CLI posture and approachable single-binary story. |
| ZeroClaw | Good maintainability ergonomics, slightly less approachable for app builders. |
| OpenShell | Great for security-minded operators, heavier for everyday local builder workflows. |

### 3. Security posture

| Project | Notes |
| --- | --- |
| OpenShell | Best-in-class among the set. Policies, sandboxing, gateway boundary, egress controls, and provider isolation are central. |
| ZeroClaw | Security is a strong architectural theme with sandboxing and allowlists, though less infrastructure-heavy than OpenShell. |
| OpenClaw | Serious about safe defaults and DM trust boundaries, but the overall product is much broader than just secure execution. |
| PicoClaw | Security is clearly on the roadmap, but current docs still warn against production deployment. |
| Pi Mono | Leaves more security choices to the operator or extension author, which is flexible but risky as a default framework stance. |

### 4. Extensibility model

| Project | Notes |
| --- | --- |
| OpenClaw | Most expansive extension and plugin surface. Strong precedent for ecosystem growth. |
| Pi Mono | Very clean customization model through extensions, skills, themes, and packages. |
| ZeroClaw | Trait-driven modularity is excellent for long-term architecture health. |
| PicoClaw | Modular package structure is promising, especially for a Go codebase. |
| OpenShell | Extensible, but more controlled and infrastructure-oriented. |

### 5. Deployment and operations

| Project | Notes |
| --- | --- |
| PicoClaw | Best direct proof that a featureful Go agent can stay easy to deploy. |
| ZeroClaw | Strong binary-first and portable story. |
| Pi Mono | Good local developer experience but still tied to Node and package workflows. |
| OpenClaw | Powerful, but Node monorepo plus apps/plugins increases operational complexity. |
| OpenShell | Most operationally sophisticated, also the heaviest to adopt. |

## What to borrow from each project

### Borrow from Pi Mono

- typed event stream around the agent loop
- explicit turn and tool lifecycle
- session branching and compaction
- minimal core with heavy customization around it

### Borrow from OpenShell

- policy engine as a first-class subsystem
- workspace, process, and network boundaries
- approval-oriented posture for risky actions
- clean separation between local runtime and managed control plane

### Borrow from PicoClaw

- one-binary Go-first install story
- practical package decomposition for agent, channels, tools, memory, and gateway
- focus on low overhead and hardware portability

### Borrow from ZeroClaw

- interface-first architecture
- optional capabilities behind boundaries, not entangled in the core path
- strong distinction between providers, tools, channels, memory, runtime, and observability

### Borrow from OpenClaw

- product thinking: users care about workflows, not architecture purity
- onboarding and setup must feel deliberate
- plugin ecosystem should exist, but not contaminate the core

## What not to borrow directly

- OpenClaw's full breadth as an MVP
- OpenShell's heavy operational footprint for local-first v1
- Pi Mono's relatively loose default security posture
- PicoClaw's tendency toward rapid capability expansion before hardening
- ZeroClaw's temptation to abstract every extension point before usage justifies it

## Product direction options

There are three realistic directions for us.

### Option A: Go coding agent first

Definition:

- build the best local coding agent runtime in Go
- focus on terminal UX, tools, sessions, approvals, and repo workflows
- delay channels, remote devices, and broad assistant surfaces

Advantages:

- fastest path to a sharp product
- strongest overlap with Pi Mono's clean runtime ideas
- easiest place to validate event model, tool model, and safety model

Risks:

- may feel narrow if we eventually want a broader personal assistant platform
- can become just another coding harness if we do not differentiate on architecture and safety

### Option B: Personal assistant platform first

Definition:

- start with channels, messaging, gateway, notifications, voice, and personal workflows
- build the OpenClaw-style product path in Go from day one

Advantages:

- very ambitious and defensible if executed well
- clearer consumer-facing product identity

Risks:

- too much surface area too early
- channels and device integrations will dominate roadmap and obscure framework quality
- much harder to keep Go codebase tight while product explodes outward

### Option C: General framework plus one flagship app

Definition:

- build a reusable Go agent runtime as the core product
- ship one excellent first-party app on top, most likely a coding agent CLI
- keep the path open for later gateway, channels, browser automation, and remote execution

Advantages:

- best balance between product focus and architectural leverage
- lets us validate the framework through real daily use
- avoids premature sprawl while still creating a platform, not just a demo app

Risks:

- requires discipline to keep the framework honest and not over-generic
- some users may initially only see the flagship app and miss the broader platform value

## Recommendation

Choose **Option C: general framework plus one flagship host**.

More specifically:

- framework identity: a local-first, policy-aware Go agent runtime
- flagship host: a CLI for loading and running agent profiles
- first bundled example: a coding-oriented profile
- later expansion: remote runner, gateway, browser adapter, channels, and installable plugins

This is the strongest move because:

- it matches Go's strengths: binaries, portability, concurrency, predictable deployment
- it gives us a concrete daily-use host immediately
- it avoids the trap of building abstract framework code with no proving ground
- it avoids the trap of building a giant consumer product before the runtime is excellent

## Recommended product thesis

> Build the safest and cleanest local-first agent runtime in Go, then prove it with a first-party CLI that loads installable agent profiles.

That thesis is stronger than:

- “build an OpenClaw clone in Go”
- “build the smallest possible agent SDK”
- “build a swarm platform before single-agent UX is solved”

## Suggested positioning

### For developers

- embed a policy-aware agent runtime in Go apps
- stream structured events to CLI, TUI, or web UIs
- attach tools, memory, and providers through stable interfaces

### For end users

- run one binary locally
- work safely inside a repo or workspace
- approve risky actions explicitly
- resume sessions and inspect event history easily
- load or install new profiles and capability plugins over time

## V1 boundaries

To keep the direction sharp, v1 should include:

- single-agent runtime
- profile-driven first-party CLI host
- session persistence
- typed events
- provider abstraction
- tool registry
- policy engine hooks
- approval flow
- sqlite local state
- MCP client support

V1 should not include:

- full messaging-channel matrix
- mobile/device ecosystem
- mandatory sandbox containers
- swarm orchestration as a core default
- distributed multi-tenant control plane

## Decision summary

If we compress all of the repo review into one sentence:

- **Pi Mono** shows how the core loop should feel
- **OpenShell** shows how the safety model should feel
- **PicoClaw** shows how the Go deployment story should feel
- **ZeroClaw** shows how the architecture should stay modular
- **OpenClaw** shows how a serious product can eventually grow from that foundation

So our move should be:

**Build a Go framework first, but make it real immediately through a first-party CLI that runs loaded profiles.**
