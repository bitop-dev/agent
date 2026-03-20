# Pattern 5: Policy and Approval Patterns

How to control what agents can and cannot do, and when to require human confirmation.
These patterns apply to any agent, but are especially important when tools have real-world
side effects like sending email, writing files, or calling external APIs.

---

## The two layers

```
Plugin layer                 Profile layer
─────────────────            ──────────────────────────────────────
sensitiveActions:            approval.mode: always | on-request | never
  - email/send      →        policy.overlays: [./my-policy.yaml]
  - agent/spawn
                             Overlay rules override plugin defaults.
                             Profile always wins.
```

See `docs/policy.md` for the complete reference.

---

## Pattern: fully automated (no human review)

Use when the agent runs unattended — scheduled jobs, CI/CD pipelines, trusted environments.

```yaml
spec:
  approval:
    mode: always          # auto-approve everything — no prompts ever
  policy:
    overlays: []          # rely on plugin defaults
```

All approval requests are automatically granted. Tools marked as `sensitiveActions`
by plugins still fire the approval event (visible in logs) but proceed immediately.

**Risk:** mistakes go unreviewed. Only use with well-tested profiles on trusted tasks.

---

## Pattern: human reviews before sensitive actions

Use when the agent is sending to external recipients, modifying shared resources,
or taking actions you want to sign off on.

```yaml
spec:
  approval:
    mode: on-request      # pause and prompt at the terminal
  policy:
    overlays: []
```

When the agent reaches `email/send` or `agent/spawn`, the terminal shows:
```
Approve send for tool email/send? [y/N]:
```

Type `y` to proceed or `N` to abort.

**Add specific tools to always prompt for** regardless of mode:

```yaml
approval:
  mode: on-request
  requireFor:
    - email/send          # always prompt before this tool
    - agent/spawn         # always prompt before spawning sub-agents
    - core/bash           # always prompt before shell commands
```

---

## Pattern: allow specific tools, require approval for others

The most precise approach. Override per-tool behavior with a policy file.

```yaml
# profile.yaml
spec:
  approval:
    mode: on-request
  policy:
    overlays:
      - ./policies/overrides.yaml
```

```yaml
# policies/overrides.yaml
version: 1
rules:
  - id: allow-draft-free
    action: tool
    tool: email/draft
    decision: allow           # draft is low risk — no approval needed

  - id: require-send-approval
    action: tool
    tool: email/send
    decision: require_approval  # always confirm before actual sending

  - id: allow-search-free
    action: tool
    tool: ddg/search
    decision: allow           # searching is read-only — no approval needed

  - id: allow-fetch-free
    action: tool
    tool: ddg/fetch
    decision: allow           # fetching is read-only — no approval needed
```

Result: search and draft run freely; sending always pauses for confirmation.

---

## Pattern: deny specific tools entirely

Block a tool from being called at all, even if it's in the profile's `tools.enabled`.

```yaml
# policies/deny-send.yaml
version: 1
rules:
  - id: deny-email-send
    action: tool
    tool: email/send
    decision: deny           # model tries to call it → receives an error
```

Use this to create a "draft-only" agent that can compose but never deliver:

```yaml
tools:
  enabled:
    - email/draft
    - email/send         # listed so the model knows it exists conceptually
policy:
  overlays:
    - ./policies/deny-send.yaml   # but always denied at runtime
```

The model will draft emails. If it tries to send, it receives:
```
policy denied tool email/send
```

---

## Pattern: deny shell commands

Block `core/bash` entirely for agents that should never execute system commands.

```yaml
# policies/no-shell.yaml
version: 1
rules:
  - id: deny-shell
    action: shell
    decision: deny
```

```yaml
spec:
  tools:
    enabled:
      - core/read
      - core/write
      - core/bash           # listed in profile
  policy:
    overlays:
      - ./policies/no-shell.yaml   # but bash is always denied
```

Shell rules apply globally to all shell execution, not per-command.

---

## Pattern: read-only agent (no writes anywhere)

The safest configuration. The agent can read the filesystem but not modify it.

```yaml
spec:
  tools:
    enabled:
      - core/read
      - core/glob
      - core/grep
  workspace:
    required: true
    writeScope: read-only     # all writes denied at the workspace level
  approval:
    mode: never
```

`writeScope: read-only` is enforced by the workspace policy engine, independent of
tool policy rules. Even if a tool tries to write, the workspace check blocks it first.

---

## Pattern: workspace-scoped writes

Allow writes only within the current project directory.
Writes outside the working directory are denied automatically.

```yaml
spec:
  tools:
    enabled:
      - core/read
      - core/write
      - core/edit
  workspace:
    required: true
    writeScope: workspace     # writes only inside CWD
  approval:
    mode: on-request
    requireFor:
      - core/bash             # require approval for shell, not file writes
```

Attempting to write outside the workspace:
```
[policy] denied: path /etc/hosts is outside workspace
```

---

## Pattern: sub-agent approval control

The `agent/spawn` tool is marked as `sensitiveAction` by the spawn plugin.
Sub-agents themselves always use deny-all approvals internally.

### Orchestrator that auto-spawns (pipeline mode)

```yaml
# orchestrator profile
spec:
  approval:
    mode: always       # auto-approve both agent/spawn and email/send
```

### Orchestrator that pauses before each spawn

```yaml
spec:
  approval:
    mode: on-request
  policy:
    overlays:
      - ./policies/spawn-control.yaml
```

```yaml
# policies/spawn-control.yaml
version: 1
rules:
  - id: require-spawn-approval
    action: tool
    tool: agent/spawn
    decision: require_approval   # show the user what task is being delegated

  - id: allow-email-send
    action: tool
    tool: email/send
    decision: allow              # email is fine without approval
```

---

## Pattern: different policies for different environments

Use separate overlay files for dev vs production:

```
my-agent/
  profile.yaml
  policies/
    dev.yaml      ← permissive, allow everything
    prod.yaml     ← strict, require approval for sensitive tools
```

```bash
# dev run — use the profile directly (dev.yaml is referenced in profile)
agent run --profile my-agent "do the task"

# prod run — override approval mode at the CLI
agent run --profile my-agent --approval on-request "do the task"
```

The `--approval` flag overrides the profile's approval mode for that run.

---

## Policy decision reference

| Decision | What happens |
|---|---|
| `allow` | Tool call proceeds immediately |
| `deny` | Tool call fails — model receives an error and can react |
| `require_approval` | Approval resolver is invoked — behavior depends on approval mode |

## Approval mode reference

| Mode | Behavior when approval is requested |
|---|---|
| `always` | Auto-approved — no prompt, proceeds immediately |
| `on-request` | Terminal prompt — user types y/N |
| `never` | Auto-denied — tool call fails |

## Precedence order

```
1. Profile policy overlay rule for this tool  → wins if present
2. Profile approval.requireFor list           → wins if tool is listed
3. Plugin sensitiveActions marking            → wins if no overlay
4. Default                                    → allow
```

---

## Checklist: choosing the right approval strategy

```
Is this an automated/unattended pipeline?
  Yes → mode: always

Is this an interactive session where you want control?
  Yes → mode: on-request

Does the agent send emails or post to external services?
  Yes → add email/send or similar to requireFor

Does the agent run shell commands?
  Yes → add core/bash to requireFor, or use deny-shell policy

Does the agent spawn sub-agents?
  Yes → add agent/spawn to requireFor

Should the agent only read, never write?
  Yes → writeScope: read-only
```

---

## Related patterns

- [Pattern 4: Orchestrator with Sub-Agents](./04-orchestrator-sub-agents.md) — approval in orchestrators
- [`docs/policy.md`](../policy.md) — complete policy engine reference
- [`docs/profiles.md`](../profiles.md) — approval fields in the profile spec
