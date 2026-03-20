# Policy

The policy system controls what the agent is allowed to do at runtime. It operates as a
gate between the model's decisions and the actual execution of those decisions.

Policy is separate from approval. Policy decides whether an action is permissible at all.
Approval decides what happens when an action requires human confirmation.

Related docs:
- `docs/profiles.md` — how profiles configure policy overlays and approval mode
- `docs/building-plugins.md` — how plugins declare sensitive actions

---

## What policy controls

Policy gates four types of actions:

| Action | What it covers |
|---|---|
| `tool` | Any tool call — `core/read`, `email/send`, `ddg/search`, etc. |
| `shell` | Bash/shell execution via `core/bash` |
| `write` / `edit` | File writes and edits via `core/write`, `core/edit` |
| `read` | File reads via `core/read` |
| `net` | Network access (reserved for future use) |

Every tool call goes through the policy engine before execution.

---

## The two policy layers

There are two independent sources of policy that apply together:

### Layer 1 — Plugin `sensitiveActions`

When a plugin is installed and enabled, any tool listed under `permissions.sensitiveActions`
in its `plugin.yaml` is automatically marked as requiring approval:

```yaml
# send-email/plugin.yaml
spec:
  permissions:
    sensitiveActions:
      - email/send   # always requires approval when this plugin is enabled
```

This is the plugin author saying: "this tool has side effects — require confirmation."

`sensitiveActions` are applied **automatically** whenever the plugin is enabled.
The profile does not need to list them. They are always active.

### Layer 2 — Profile policy overlays

The profile can apply its own policy overlay files via `spec.policy.overlays`:

```yaml
# profile.yaml
spec:
  policy:
    overlays:
      - ./policies/my-overrides.yaml
```

Overlay files can **allow**, **deny**, or **require_approval** for specific tools,
and can also configure global shell and network rules.

---

## Override precedence

Profile overlay rules take precedence over plugin `sensitiveActions`.

The policy engine checks in this order:

```
1. Profile overlay rule for this tool ID   → wins if present
2. Plugin sensitiveAction for this tool ID → wins if no overlay
3. Default                                 → allow
```

This means:

- A profile can **remove** the approval requirement on a sensitive tool by setting its decision to `allow`
- A profile can **deny** a tool entirely with `decision: deny`
- A profile can **add** approval requirements to tools the plugin left unrestricted

**Example — override a plugin's approval requirement:**

```yaml
# policies/allow-draft.yaml
version: 1
rules:
  - id: allow-email-draft
    action: tool
    tool: email/draft
    decision: allow   # plugin doesn't mark this sensitive, but making it explicit
  - id: allow-send-anyway
    action: tool
    tool: email/send
    decision: allow   # overrides the plugin's require_approval
```

**Example — deny a tool entirely:**

```yaml
version: 1
rules:
  - id: deny-bash
    action: tool
    tool: core/bash
    decision: deny    # model can never execute shell commands
```

**Example — require approval on a tool the plugin left unrestricted:**

```yaml
version: 1
rules:
  - id: careful-search
    action: tool
    tool: ddg/fetch
    decision: require_approval   # ask before fetching any URL
```

---

## Policy overlay file format

```yaml
version: 1
rules:
  - id: unique-rule-id        # required, used in logs
    action: tool              # tool | shell | net
    tool: email/send          # required when action is 'tool'
    decision: require_approval # allow | deny | require_approval
```

### `action` values

| Value | What it applies to |
|---|---|
| `tool` | A specific tool call — requires `tool` field |
| `shell` | All shell/bash execution globally |
| `net` | All network access globally |

### `decision` values

| Value | Behavior |
|---|---|
| `allow` | Permit without prompting |
| `deny` | Block the action entirely — the model receives an error |
| `require_approval` | Pause and ask for human confirmation |

### Multiple rules

Rules are evaluated in file order. The **first matching rule** wins.
If multiple overlay files are listed in the profile, they are merged — later files do not
override earlier ones on a per-rule basis. Rules are merged by tool ID and the last
definition for a given tool ID wins.

---

## Workspace scoping

File access is also controlled by workspace scope, independent of tool policy rules.

When `spec.workspace.writeScope` is `workspace` in the profile:
- `core/read` is allowed anywhere inside the current working directory
- `core/write` and `core/edit` are allowed only inside the current working directory
- Any path outside the working directory is denied regardless of policy rules

When `writeScope` is `read-only`:
- All writes and edits are denied regardless of policy rules or tool lists

Workspace scoping happens before tool policy rules are checked.

---

## Shell policy

`core/bash` is treated specially. By default it always requires approval:

```
default shell policy: require_approval
```

You can change this with a shell action rule:

```yaml
# allow bash freely (dangerous — only for trusted environments)
version: 1
rules:
  - id: allow-shell
    action: shell
    decision: allow

# deny bash entirely
version: 1
rules:
  - id: deny-shell
    action: shell
    decision: deny
```

Shell rules apply globally to all shell execution, not per-command.

---

## How approval mode interacts with policy

When a policy decision is `require_approval`, the approval resolver is invoked.
What happens next depends on the profile's `spec.approval.mode`:

| Approval mode | What happens on `require_approval` |
|---|---|
| `on-request` | The agent pauses and prompts the user interactively |
| `always` | The action is automatically approved — no prompt |
| `never` | The action is automatically denied — the tool call fails |

`spec.approval.requireFor` adds additional tool IDs that always go through
the approval resolver, even if the tool is not marked sensitive by the plugin
and no overlay sets it to `require_approval`.

```yaml
approval:
  mode: on-request
  requireFor:
    - core/bash      # always prompt before shell, even if policy would allow it
    - email/send     # always prompt before sending
```

---

## Full worked examples

### 1. Fully unrestricted agent (automated pipeline use)

Allow everything without any human-in-the-loop. Only appropriate for trusted
environments where the agent runs autonomously.

```yaml
# profile.yaml
spec:
  approval:
    mode: always          # auto-approve all approval requests
  policy:
    overlays:
      - ./policies/allow-all.yaml
```

```yaml
# policies/allow-all.yaml
version: 1
rules:
  - id: allow-shell
    action: shell
    decision: allow
  - id: allow-all-tools
    action: tool
    tool: email/send
    decision: allow
  - id: allow-all-tools-2
    action: tool
    tool: core/bash
    decision: allow
```

---

### 2. Strict read-only research agent

The agent can search and fetch pages but cannot write files, send email, or run shell.

```yaml
# profile.yaml
spec:
  tools:
    enabled:
      - ddg/search
      - ddg/fetch
  approval:
    mode: never           # no approval prompts — sensitive tools fail if called
  workspace:
    writeScope: read-only
  policy:
    overlays: []          # rely on defaults + workspace scoping
```

---

### 3. Email agent where drafting is free but sending requires confirmation

```yaml
# profile.yaml
spec:
  tools:
    enabled:
      - email/draft
      - email/send
  approval:
    mode: on-request
  policy:
    overlays:
      - ./policies/email.yaml
```

```yaml
# policies/email.yaml
version: 1
rules:
  - id: allow-draft
    action: tool
    tool: email/draft
    decision: allow           # override plugin: draft doesn't need approval
  - id: require-send
    action: tool
    tool: email/send
    decision: require_approval # keep approval on actual sending
```

---

### 4. Coding agent where bash is allowed but only inside the workspace

No explicit bash policy needed — the workspace scoping handles it.
`core/bash` still requires approval by default (shell default), but writes
are bounded by the workspace root automatically.

```yaml
# profile.yaml
spec:
  tools:
    enabled:
      - core/read
      - core/write
      - core/edit
      - core/bash
  approval:
    mode: on-request
    requireFor:
      - core/bash
  workspace:
    required: true
    writeScope: workspace    # writes outside CWD are denied by workspace policy
  policy:
    overlays: []
```

---

## Where policy files live

Policy files are loaded relative to the profile that references them.
A common layout is:

```
my-agent/
  profile.yaml
  policies/
    overrides.yaml
  prompts/
    system.md
```

```yaml
# profile.yaml
spec:
  policy:
    overlays:
      - ./policies/overrides.yaml
```

Plugin-contributed policy files are installed alongside the plugin:

```
~/.agent/plugins/send-email/
  policies/
    default.yaml     # contributed by the plugin
```

These are registered when the plugin is enabled but are **not** automatically applied.
To use a plugin's policy file in your profile:

```yaml
spec:
  policy:
    overlays:
      - /Users/you/.agent/plugins/send-email/policies/default.yaml
```

Or copy the file into your profile directory and reference it locally — that way
your policy doesn't depend on the plugin's installed path.
