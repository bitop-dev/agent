# Go Agent Framework Roadmap

## 1. Purpose and planning assumptions

This roadmap is for turning the current framework into a stable, usable, and maintainable local-first agent platform. The goal is not to maximize feature count. The goal is to prove that the current architecture can ship safely, support third-party capability growth through plugins, and stay disciplined about security and operator ergonomics as complexity increases.

### Planning assumptions

- `v0.1` is a real release, not a placeholder milestone.
- The project is already past the "is this architecture viable?" stage for the local runtime.
- The next work should optimize for release safety, plugin author success, and trust in the policy model before expanding the surface area.
- The framework should remain local-first and single-binary by default.
- Plugins and runtimes are the main extension mechanism; the core should stay intentionally small.
- Security posture must improve before ecosystem reach expands.
- Some important limitations are known and should remain explicit rather than being hidden behind vague roadmap language:
  - no remote plugin registry yet
  - no RPC runtime implementation yet
  - no nested `.gitignore` negation support yet
- Credential rotation for previously exposed secrets must happen outside the repo and is a release-adjacent operational task, not a code task.

### Planning constraints

| Constraint | Current implication |
| --- | --- |
| No `v0.1` tag yet | Release work is still in progress and should be treated as active engineering |
| Credentials were exposed in deleted files | Release should not be treated as complete until rotation is confirmed outside the repository |
| No remote plugin registry | Adoption work should favor local install, examples, and docs over discovery infrastructure |
| No RPC runtime implementation | Runtime expansion should focus on hardening current runtimes before adding a new transport boundary |
| No nested `.gitignore` negation support | File-selection behavior should be documented clearly and treated as a known limitation |

## 2. Current state snapshot

The framework has enough working surface area to justify a controlled `v0.1`.

### Implemented and working for `v0.1`

- Runtime core works.
- CLI host works.
- Profiles work.
- Plugins work.
- Sessions work.
- Policy evaluation works.
- Approval flow works.
- Built-in core tools exist:
  - `read`
  - `write`
  - `edit`
  - `bash`
  - `glob`
  - `grep`
- Runtime types now include:
  - `asset`
  - `http`
  - `command`
  - `mcp`
  - `host`
- `command` runtime has just been implemented for local CLIs, scripts, and binaries.

### Known remaining work before calling the project ready for broader use

- Missing documentation:
  - build-a-send-email-plugin guide
  - plugin-author-checklist guide
- `v0.1` tag has not been created.
- Credential rotation still needs to happen outside the repository.
- Some roadmap-worthy capability gaps remain:
  - no remote plugin registry
  - no RPC runtime implementation
  - no nested `.gitignore` negation support

### Opinionated read on project maturity

The project is not blocked on major architecture work. It is blocked on release discipline, documentation closure, test confidence, and tightening the extension model around policy and operator trust.

## 3. Now / next / later framing

### Now

Ship `v0.1` safely and remove ambiguity around what is stable today.

### Next

Make plugin authors successful with minimal ceremony and strong examples so the ecosystem can grow without adding a registry or remote control plane.

### Later

Harden runtime isolation, policy enforcement, and operator ergonomics only after the current local-first workflow is reliable and teachable.

### What should not happen yet

- Building a registry before plugin install and authoring feel good locally.
- Building RPC infrastructure before the current runtime contract is proven stable.
- Building a large UI before the event and session model are mature enough to support it cleanly.
- Expanding built-in tools aggressively; plugins should carry most capability growth.

## 4. Phase 0: ship v0.1 safely

This phase exists to convert "it works on the main branch" into "it is safe to tag and tell people to use."

### Objectives

- Close release-blocking docs gaps.
- Verify the shipped surface matches the actual code.
- Confirm the `command` runtime is documented and covered enough to include in `v0.1`.
- Complete security-adjacent operational work around leaked credentials.
- Create and publish the `v0.1.0` tag only after validation is complete.

### Milestone 0.1: Release readiness audit

#### Scope

- Review CLI commands and docs for drift.
- Review runtime docs for current runtime list, especially `command`.
- Confirm known limitations are documented, not implied away.
- Confirm release checklist reflects actual shipped features.

#### Acceptance criteria

- `docs/release-checklist-v0.1.md` matches current product scope.
- `README.md` and plugin/runtime docs do not describe features that are absent.
- `command` runtime is described accurately, including intended use cases and trust boundaries.
- Known limitations are present in docs:
  - no remote plugin registry
  - no RPC runtime implementation
  - no nested `.gitignore` negation support

#### Dependencies

- None; this should start immediately.

### Milestone 0.2: Documentation closure for `v0.1`

#### Scope

- Write the build-a-send-email-plugin guide.
- Write the plugin-author-checklist guide.
- Update plugin docs to reflect all currently supported runtimes, including `command`.

#### Acceptance criteria

- A new user can build one plugin from docs without reading internal code.
- A plugin author can use the checklist to validate packaging, policy, config, and approval behavior.
- At least one doc path shows when to choose `http`, `command`, `mcp`, `host`, or `asset`.

#### Dependencies

- Milestone 0.1 findings should be incorporated first.

### Milestone 0.3: Release validation and tagging

#### Scope

- Run the release checklist.
- Verify one end-to-end plugin-backed workflow.
- Verify one approval-gated workflow.
- Verify session create/list/resume/export.
- Confirm credential rotation status outside the repo.
- Create the `v0.1.0` tag.

#### Acceptance criteria

- Release checklist is complete with no intentionally skipped items.
- An explicit release note lists shipped capabilities and non-goals.
- Credential rotation is confirmed by the operator responsible for affected secrets.
- Tag `v0.1.0` exists only after validation is complete.

#### Dependencies

- Milestones 0.1 and 0.2 must be complete.
- External credential rotation confirmation is required before release sign-off.

### Phase 0 exit criteria

- `v0.1.0` is tagged.
- Docs describe the actual product, not the intended product.
- The team can point users to concrete supported workflows with confidence.

## 5. Phase 1: plugin author experience and adoption

This phase should make the framework useful to external builders without adding infrastructure that the project is not ready to operate.

### Objective

Reduce the time from "I installed the binary" to "I built a working plugin with sane defaults" to under one afternoon.

### Strategic stance

Do not solve discovery with a registry yet. Solve it with better local workflows, examples, validation, and docs.

### Milestone 1.1: Plugin authoring path is coherent

#### Scope

- Define a preferred plugin author journey:
  - choose a runtime
  - create manifest
  - define tool inputs/outputs
  - configure policy expectations
  - test locally
  - package and install locally
- Add stronger validation output for plugin manifests and configuration.
- Make runtime choice guidance explicit.

#### Acceptance criteria

- A plugin author can answer "which runtime should I use?" from docs alone.
- `agent plugins validate` surfaces actionable errors and warnings.
- Plugin validation covers at least:
  - manifest shape
  - runtime reference validity
  - config schema validity
  - missing approval hints for risky actions
  - incompatible tool IDs or collisions

#### Dependencies

- Phase 0 docs closure.

### Milestone 1.2: First-class example set

#### Scope

Ship a small, opinionated example set rather than many shallow examples.

Suggested examples:

- send email plugin using `http` or `command`
- local CLI wrapper plugin using `command`
- MCP bridge example for one useful external tool
- minimal read-only asset/plugin example

#### Acceptance criteria

- Each example is runnable and tested at least once in CI or scripted verification.
- Each example includes:
  - purpose
  - runtime choice rationale
  - config requirements
  - policy/approval considerations
  - verification steps
- At least one example uses `command` for a local binary or script.

#### Dependencies

- Milestone 1.1 runtime guidance.

### Milestone 1.3: Local install and iteration loop feels good

#### Scope

- Tighten plugin install/enable/disable/remove UX.
- Improve config inspection for installed plugins.
- Add better errors when runtime dependencies are missing, such as absent local binaries for `command`.

#### Acceptance criteria

- A user can install a local plugin, set config, validate config, and run it without reading source.
- Error output tells the user what to fix rather than only reporting failure.
- `command` runtime failures distinguish among:
  - binary not found
  - permission denied
  - non-zero exit
  - invalid arguments
  - timeout or cancellation

#### Dependencies

- Milestone 1.1.

### Phase 1 exit criteria

- Plugin authors can build and test locally without maintainers walking them through internals.
- There are 2-4 high-quality examples that cover the major runtime choices.
- Plugin runtime selection is teachable and consistent.

## 6. Phase 2: runtime hardening and policy model

This phase should improve trust, not just add more control knobs.

### Objective

Make runtime execution safer and more predictable, especially as plugins move beyond trivial read-only use cases.

### Strategic stance

Harden the existing runtime families before adding a new RPC-based one. The system should prove it can enforce policy consistently across `http`, `command`, `mcp`, and `host` first.

### Milestone 2.1: Runtime behavior contracts

#### Scope

Define and document consistent execution behavior across runtimes:

- argument validation
- timeout behavior
- cancellation behavior
- stdout/stderr or output capture rules
- error shape
- retryability guidance
- audit/event emission expectations

#### Acceptance criteria

- Every runtime has an explicit contract document or section in runtime docs.
- Runtime behavior is covered by integration tests.
- Event emission is consistent enough for future UI and operator tooling.

#### Dependencies

- Stable current runtime implementations.

### Milestone 2.2: Policy model tightening

#### Scope

- Clarify what policy can decide today and where enforcement actually happens.
- Normalize policy checks across built-in tools and plugin-backed tools.
- Add clearer risk classes, especially for `command`, `bash`, and outbound `http`.

#### Acceptance criteria

- High-risk actions cannot bypass policy because of runtime differences.
- Policy decisions are observable in event streams and logs.
- Approval requests include enough context for a human to make a decision safely.
- Policy docs explain current limits honestly.

#### Dependencies

- Milestone 2.1 contracts.

### Milestone 2.3: Workspace and ignore semantics hardening

#### Scope

- Tighten workspace boundary behavior for file-related tools.
- Document and test `.gitignore` handling limits, especially lack of nested negation support.
- Decide whether to fix nested negation now or keep it as an explicit limitation through `v0.x`.

#### Acceptance criteria

- Current ignore behavior is deterministic and tested.
- Users are warned when behavior differs from Git expectations.
- A maintainers-only decision is recorded:
  - implement nested negation in a later milestone, or
  - defer until a stronger workspace/file-selection subsystem exists

#### Dependencies

- Existing workspace and file tool behavior.

### Milestone 2.4: Auditability baseline

#### Scope

- Improve event/log output for tool execution, policy decisions, approvals, and failures.
- Make it easier to answer "what happened in this run?" after the fact.

#### Acceptance criteria

- Tool execution history can be reconstructed from stored events or session records.
- Policy and approval decisions are attached to the relevant tool action.
- Operators can distinguish model failure, policy denial, approval denial, and runtime failure.

#### Dependencies

- Milestones 2.1 and 2.2.

### Phase 2 exit criteria

- Runtimes behave consistently enough that plugin authors do not need per-runtime tribal knowledge.
- Policy and approvals are credible controls, not just UX prompts.
- Known workspace/ignore limitations are either fixed or deliberately deferred with documentation.

## 7. Phase 3: operator and UI ergonomics

This phase should improve daily usability for people running the framework locally or in controlled environments. It should not become a premature product surface explosion.

### Objective

Make the framework easier to operate, inspect, and recover without introducing a large new architecture.

### Strategic stance

Prefer CLI and event-driven ergonomics before building a web app or full TUI.

### Milestone 3.1: Operator-focused CLI improvements

#### Scope

- Improve `doctor`, validation, and inspection commands.
- Add clearer views into sessions, plugins, profiles, and policy decisions.
- Improve command naming and help text where current UX is noisy or inconsistent.

#### Acceptance criteria

- A user can diagnose a failed run from CLI output and inspection commands.
- Common misconfigurations are detectable with `doctor` or validation commands.
- Session inspection exposes enough metadata to troubleshoot without reading raw storage.

#### Dependencies

- Phase 2 auditability improvements.

### Milestone 3.2: Better session and run introspection

#### Scope

- Improve session export/import and human-readable inspection.
- Add summaries of tool usage, approvals, and failures.
- Make event data easier to consume from tooling.

#### Acceptance criteria

- Users can inspect a session and understand:
  - which tools ran
  - which actions were blocked
  - which approvals were requested
  - why a run failed
- Export format is stable enough for downstream tooling.

#### Dependencies

- Milestone 2.4.

### Milestone 3.3: Lightweight UI surface only if event model is ready

#### Scope

If pursued, keep it small:

- a minimal TUI or local web inspector
- session viewer
- approval prompt surface
- event stream viewer

#### Acceptance criteria

- Any UI consumes the same event/session model the CLI uses.
- No UI-specific runtime behavior is introduced.
- The UI does not become the only way to access important operational data.

#### Dependencies

- Event model stability.
- Session/run introspection maturity.

### Phase 3 exit criteria

- The framework is practical to operate without internal maintainers.
- Failure diagnosis is fast.
- Any UI surface is layered on the runtime instead of distorting it.

## 8. Phase 4: architectural bets to defer until proven necessary

This phase is intentionally conservative. These are bets that could be valuable, but they should not pull focus until earlier phases prove demand.

### Objective

Name the likely future bets explicitly so the team can avoid accidental partial implementations.

### Bet 1: Remote plugin registry

#### Why defer

- Discovery is not the current bottleneck; trust, docs, examples, and local iteration are.
- Registry work adds package hosting, version policy, signing questions, and operational burden.

#### Preconditions

- Plugin packaging is stable.
- Validation rules are stable.
- There is evidence of multiple external plugin authors.
- There is a clear trust/distribution model.

### Bet 2: RPC runtime implementation

#### Why defer

- Current runtimes already cover many practical cases.
- RPC adds protocol stability, process lifecycle management, auth/trust questions, and versioning costs.

#### Preconditions

- Current runtime contracts are stable.
- There are real plugin use cases that do not fit `http`, `command`, `mcp`, or `host`.
- The team can define a durable RPC boundary instead of a stopgap transport.

### Bet 3: Rich UI product surface

#### Why defer

- UI can hide missing runtime discipline if built too early.
- The event and session model should prove itself first.

#### Preconditions

- Event schema is stable.
- Session inspection is strong in CLI form.
- Approval and policy data are already reliable and structured.

### Bet 4: More advanced workspace semantics

#### Why defer

- File-selection behavior should be correct and simple before it is flexible and clever.
- Nested `.gitignore` negation support is real work, but may belong in a more deliberate filesystem subsystem.

#### Preconditions

- There is repeated user pain from current ignore handling.
- There is a clear test corpus for expected behavior.

## 9. Cross-cutting workstreams

These should run across all phases rather than being treated as cleanup.

### Testing

#### Priorities

- Integration coverage for each runtime type.
- Deterministic end-to-end tests with a fake provider.
- Policy and approval regression tests.
- Session persistence and replay tests.
- Example/plugin verification tests.

#### Acceptance criteria

- Every shipped runtime has at least one integration test.
- Every built-in core tool has policy coverage.
- At least one plugin example is validated in automated checks.
- Release confidence does not depend on ad hoc manual runs alone.

### Documentation

#### Priorities

- Close remaining `v0.1` docs gaps first.
- Keep runtime choice docs current.
- Document limitations explicitly.
- Add operator troubleshooting content, not just author guides.

#### Acceptance criteria

- Docs match implementation at release time.
- New users can install, configure, and run at least one plugin from docs alone.
- Troubleshooting sections exist for common failures, especially around `command` and `http`.

### Examples

#### Priorities

- Favor a few maintained examples over many stale ones.
- Cover runtime choice, policy expectations, and config shape.
- Use examples as compatibility tests where possible.

#### Acceptance criteria

- Each example has an owner or maintenance expectation.
- Each example states why its runtime was chosen.
- Examples fail loudly when the framework changes incompatibly.

### Release hygiene

#### Priorities

- Keep a real release checklist.
- Track docs drift explicitly.
- Confirm secret rotation operationally, not informally.
- Publish release notes that separate shipped features from deferred work.

#### Acceptance criteria

- Release decisions are checklist-based.
- No tag is created without sign-off on code, docs, and security-adjacent tasks.
- Known limitations are carried forward in notes until fixed.

## 10. Prioritization rubric

Use this rubric to decide what should ship first when tradeoffs appear.

| Criterion | Weight | What high score means |
| --- | --- | --- |
| Release safety | 5 | Reduces the chance of shipping something misleading or unsafe |
| User unblock value | 5 | Helps a user complete a real workflow now |
| Architecture leverage | 4 | Improves multiple runtimes, tools, or workflows at once |
| Trust and security | 5 | Strengthens policy, approvals, boundaries, or auditability |
| Doc/support reduction | 4 | Prevents repeated confusion or maintainer hand-holding |
| Implementation cost | 3 | Can be completed without major churn |
| Deferrability | 4 | Low score means it can wait without harming current users |

### How to apply it

- Prioritize work that scores high on release safety, user unblock value, and trust.
- Deprioritize work that is strategically interesting but highly deferrable, such as registry or RPC infrastructure.
- If two items are equal, choose the one that improves plugin author success or reduces operator confusion.

### Default decision rule

If a task does not make `v0.1` safer, make plugin authors more successful, or improve policy/runtime trust, it probably belongs later.

## 11. Suggested issue backlog broken into small actionable items

The backlog below is intentionally small-grained so work can be scheduled and verified incrementally.

### Phase 0 backlog

| Issue | Outcome | Dependencies |
| --- | --- | --- |
| Audit `README.md` against current CLI/runtime behavior | Public docs match shipped behavior | None |
| Update runtime docs to include `command` runtime everywhere | Runtime list is accurate | None |
| Write `build-a-send-email-plugin` doc | Missing `v0.1` author guide is closed | Runtime docs audit |
| Write `plugin-author-checklist` doc | Authors have a repeatable validation path | Runtime docs audit |
| Add known limitations section to plugin/runtime docs | No ambiguity about missing registry/RPC/ignore support | None |
| Review `docs/release-checklist-v0.1.md` for current scope | Release checklist reflects reality | None |
| Run end-to-end plugin-backed validation flow | Confirms real plugin path works | Docs and runtime state ready |
| Run approval-gated validation flow | Confirms policy/approval path works | None |
| Confirm external credential rotation completion | Security-adjacent release blocker is closed | External operator |
| Create `v0.1.0` tag and release notes | First release is formally cut | All Phase 0 items |

### Phase 1 backlog

| Issue | Outcome | Dependencies |
| --- | --- | --- |
| Add runtime selection matrix to plugin docs | Authors know when to use `asset/http/command/mcp/host` | Phase 0 docs |
| Improve `agent plugins validate` error messages | Validation is actionable | Existing validation commands |
| Add warnings for risky plugin actions lacking approval guidance | Safer plugin defaults | Policy model clarity |
| Create `command` runtime example plugin wrapping a local CLI | New runtime has a canonical example | Phase 0 release |
| Create send-email plugin example with full config and troubleshooting | Practical external-use example exists | Missing doc completion |
| Add example verification script or test harness | Examples stay working | Example docs |
| Improve plugin config inspection output | Easier local iteration | Existing config commands |
| Add clearer missing-binary and non-zero-exit diagnostics for `command` runtime | Faster author/operator debugging | Runtime hardening start |

### Phase 2 backlog

| Issue | Outcome | Dependencies |
| --- | --- | --- |
| Define shared runtime error model | Runtime failures become consistent | Phase 1 runtime docs |
| Add timeout and cancellation tests for each runtime | Execution behavior is predictable | Runtime contracts |
| Normalize event emission across all runtimes | Operator tooling becomes reliable | Shared event model |
| Add risk classes for `command`, `bash`, and outbound `http` | Policy decisions become clearer | Policy docs |
| Ensure plugin-backed tools always pass through central policy checks | No runtime-specific bypasses | Policy model |
| Improve approval request payloads with tool arguments and risk summary | Human approval is informed | Policy risk classes |
| Add ignore semantics test corpus for workspace/file tools | File behavior is explicit and stable | Current workspace logic |
| Decide disposition of nested `.gitignore` negation support | Limitation is either planned or intentionally deferred | Test corpus |

### Phase 3 backlog

| Issue | Outcome | Dependencies |
| --- | --- | --- |
| Expand `agent doctor` for plugin/runtime dependency checks | Operators can self-diagnose setup problems | Phase 2 contracts |
| Add session inspection summary command | Run history is understandable | Session metadata improvements |
| Add policy decision inspection in session output | Operators can see why actions were blocked | Auditability baseline |
| Stabilize session export format | Downstream tooling becomes possible | Session introspection work |
| Prototype minimal event/session viewer using existing event model | UI feasibility is tested without runtime distortion | Stable event schema |

### Phase 4 backlog

| Issue | Outcome | Dependencies |
| --- | --- | --- |
| Write decision record for remote plugin registry prerequisites | Registry work stays disciplined | Evidence of demand |
| Write decision record for RPC runtime boundary | RPC work is problem-driven | Stable runtime contracts |
| Define metrics that would justify richer UI investment | UI work becomes evidence-based | Operator pain signals |
| Reassess advanced ignore semantics after user feedback | Filesystem work is driven by real usage | Support and bug data |

## 12. Explicit non-goals

These are deliberate non-goals for the near-term roadmap.

- Building a remote plugin registry before local plugin authoring and validation are strong.
- Implementing RPC runtime support in the next release cycle just to claim architectural completeness.
- Expanding the built-in core tools far beyond `read`, `write`, `edit`, `bash`, `glob`, and `grep`.
- Treating nested `.gitignore` negation support as a silent compatibility promise before it is implemented and tested.
- Shipping a large TUI or web UI before event, session, and audit models are stable.
- Turning the framework into a multi-tenant hosted control plane in the `v0.x` window.
- Hiding known limitations in docs to make the project look more complete than it is.
- Declaring security work "done" once leaked files are deleted; credential rotation must happen outside the repo and remain tracked until confirmed.

## Recommended sequencing summary

1. Finish release truthfulness and safety work.
2. Tag `v0.1.0` only after docs, validation, and secret rotation confirmation are done.
3. Make plugin authoring excellent locally before adding distribution infrastructure.
4. Harden runtime and policy behavior before adding new runtime classes.
5. Improve operator ergonomics through CLI and event/session introspection before investing in richer UI surfaces.
6. Keep registry, RPC, and advanced UI work deferred until user demand and current architecture maturity justify them.
