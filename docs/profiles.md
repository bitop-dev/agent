# Profiles

A **profile** is the complete definition of an agent. It specifies which language model to use,
which tools are available, what the system prompt says, how approvals are handled, and which
policy rules apply.

Plugins supply the raw capabilities — tools, prompts, policies, and profile templates.
The profile is where you compose those capabilities into a specific agent with a specific purpose.

Related docs:
- `docs/building-plugins.md` — how plugins contribute tools and other assets
- `docs/policy.md` — how policy overlays and sensitiveActions interact
- `docs/prompts.md` — how system instructions and plugin prompts work

---

## Profile discovery

The agent looks for profiles in two locations, in this order:

| Location | Purpose |
|---|---|
| `.agent/profiles/` in the current working directory | project-local profiles |
| `~/.agent/profiles/` in the user's home directory | user-level profiles |

Any subdirectory that contains a `profile.yaml` or `profile.yml` is discovered automatically.
Profiles can be referenced by name (`coding`) or by path (`./my-profiles/researcher/profile.yaml`).

---

## Full profile reference

```yaml
apiVersion: agent/v1
kind: Profile
metadata:
  name: my-agent          # identifier used with --profile
  version: 0.1.0
  description: What this agent does

spec:
  instructions:
    system:               # list of system prompt sources (see docs/prompts.md)
      - ./prompts/system.md
      - email/style-default   # a plugin-contributed prompt referenced by ID
      - "Always be concise."  # inline literal text

  provider:
    default: openai       # provider name (must be registered)
    model: gpt-4o         # model passed to the provider

  tools:
    enabled:              # tool IDs the agent can call
      - core/read
      - core/write
      - email/draft
      - email/send

  approval:
    mode: on-request      # never | always | on-request (default)
    requireFor:           # additional tools that always require approval
      - email/send        # even if mode is 'never', these still prompt

  workspace:
    required: false       # if true, agent must be run from a workspace
    writeScope: workspace # workspace | read-only

  session:
    persistence: sqlite   # sqlite | none
    compaction: auto      # auto | none

  policy:
    overlays:             # policy files applied on top of plugin defaults
      - ./policies/strict.yaml
```

---

## Field reference

### `metadata`

| Field | Type | Description |
|---|---|---|
| `name` | string | Profile identifier used with `--profile` and in session records |
| `version` | string | Semantic version of the profile |
| `description` | string | Human-readable description |

---

### `spec.instructions.system`

A list of system prompt sources loaded in order and joined with a blank line.

Each entry is resolved in this priority order:

1. **Plugin prompt ID** — if a plugin registers a prompt with this ID (e.g. `email/style-default`),
   its file is loaded. This works for any prompt contributed by an enabled plugin.
2. **File path** — resolved relative to the profile directory, or absolute.
3. **Inline text** — if neither lookup succeeds, the string itself is used as prompt text.

See `docs/prompts.md` for full details and examples.

---

### `spec.provider`

| Field | Type | Description |
|---|---|---|
| `default` | string | Provider name — currently `openai` or `mock` |
| `model` | string | Model identifier passed directly to the provider |

The `openai` provider is OpenAI-compatible and works with any proxy that exposes a `/chat/completions` endpoint.
Configure base URL and API key in `~/.agent/config.yaml` or via environment variables:

```bash
export OPENAI_BASE_URL=https://api.openai.com/v1
export OPENAI_API_KEY=sk-...
```

---

### `spec.tools.enabled`

The list of tool IDs this agent can call. A tool is only available when:

1. The plugin that contributes it is installed
2. The plugin config is valid
3. The plugin is enabled
4. The tool ID appears in this list
5. Policy allows the call at runtime

Tool IDs follow the pattern `namespace/name` (e.g. `core/read`, `email/draft`, `ddg/search`).
MCP-bridged tools use the name reported by the MCP server directly (e.g. `read_file`, `list_directory`).

**Ordering does not matter.** The agent can call tools in any order the model decides.

---

### `spec.approval`

Controls when the agent must pause and ask for human approval before executing a tool.

| Mode | Behavior |
|---|---|
| `on-request` | Default. Prompts interactively for tools marked as sensitive by plugins. |
| `always` | Auto-approves all approval requests without prompting. |
| `never` | Denies all approval requests. Sensitive tool calls will fail. |

`requireFor` adds additional tool IDs that always trigger an approval prompt,
regardless of what the plugin declares as sensitive.

**Approval interacts with policy.** Policy decides whether a tool call needs
approval at all. Approval mode decides what happens when it does.
See `docs/policy.md` for the full interaction model.

---

### `spec.workspace`

| Field | Type | Description |
|---|---|---|
| `required` | bool | If true, the agent must be run from inside a workspace directory |
| `writeScope` | string | `workspace` allows writes; `read-only` denies all writes |

When `writeScope` is `workspace`, the policy engine allows reads and writes only within the
current working directory tree. Writes outside the workspace are denied.

When `writeScope` is `read-only`, `core/write`, `core/edit`, and any other write tools
are blocked by policy regardless of which tools are listed in `tools.enabled`.

---

### `spec.session`

| Field | Type | Description |
|---|---|---|
| `persistence` | string | `sqlite` saves the session to `~/.agent/sessions/`; `none` disables persistence |
| `compaction` | string | `auto` compacts long sessions automatically; `none` disables |

Sessions allow resuming previous conversations with `agent resume`.

---

### `spec.policy.overlays`

A list of policy overlay files applied when this profile runs.
Overlay files are loaded relative to the profile directory.

Overlay rules take precedence over plugin-declared `sensitiveActions`.
This means a profile can explicitly allow a tool that a plugin marked as sensitive,
or deny a tool that a plugin left unrestricted.

See `docs/policy.md` for the full overlay format and precedence rules.

---

## How profiles compose plugins

The profile is the composition layer. The same plugins can be used in many different
profiles with different behaviors.

```
Installed plugins         Profile
─────────────────         ──────────────────────────────
ddg-research         →    tools: [ddg/search, ddg/fetch]
  ddg/search               model: nemotron
  ddg/fetch                approval: never
                           system: "You are a researcher..."

send-email           →    tools: [email/draft, email/send]
  email/draft              approval: always
  email/send               policy overlays: [./allow-send.yaml]

core-tools           →    (not included in this profile)
  core/read
  core/write
  core/edit
  core/bash
```

One plugin installation, many possible agent configurations.

---

## Profile examples

### Minimal read-only assistant

```yaml
apiVersion: agent/v1
kind: Profile
metadata:
  name: assistant
  version: 0.1.0
spec:
  instructions:
    system:
      - "You are a helpful assistant. Answer clearly and concisely."
  provider:
    default: openai
    model: gpt-4o-mini
  tools:
    enabled: []
  approval:
    mode: on-request
  workspace:
    required: false
    writeScope: read-only
  session:
    persistence: sqlite
    compaction: auto
  policy:
    overlays: []
```

---

### Coding agent with file system access

```yaml
apiVersion: agent/v1
kind: Profile
metadata:
  name: coding
  version: 0.1.0
  description: General-purpose coding assistant
spec:
  instructions:
    system:
      - ./prompts/system.md
  provider:
    default: openai
    model: claude-3.7-sonnet
  tools:
    enabled:
      - core/read
      - core/write
      - core/edit
      - core/bash
      - core/glob
      - core/grep
  approval:
    mode: on-request
    requireFor:
      - core/bash
  workspace:
    required: true
    writeScope: workspace
  session:
    persistence: sqlite
    compaction: auto
  policy:
    overlays: []
```

---

### Research and email agent using plugin-contributed prompts by ID

This profile references the `send-email` plugin's built-in style prompt
directly by its registered ID, without hard-coding a file path.

```yaml
apiVersion: agent/v1
kind: Profile
metadata:
  name: ai-news-researcher
  version: 0.1.0
  description: Researches AI news and emails a summary
spec:
  instructions:
    system:
      - ./prompts/research-instructions.md  # local file
      - email/style-default                 # from send-email plugin, by ID
  provider:
    default: openai
    model: nemotron-3-super-120b-a12b
  tools:
    enabled:
      - ddg/search
      - ddg/fetch
      - email/draft
      - email/send
  approval:
    mode: always
  workspace:
    required: false
    writeScope: read-only
  session:
    persistence: sqlite
    compaction: auto
  policy:
    overlays: []
```

`./prompts/research-instructions.md` is a local file next to the profile.
`email/style-default` is resolved from the plugin registry — no path needed.

---

### Strict agent with custom policy override

This profile overrides the `send-email` plugin's default approval requirement
for `email/draft` (which is low-risk) while keeping approval on `email/send`.

```yaml
apiVersion: agent/v1
kind: Profile
metadata:
  name: email-drafter
  version: 0.1.0
spec:
  instructions:
    system:
      - "Draft professional emails. Always confirm before sending."
  provider:
    default: openai
    model: gpt-4o
  tools:
    enabled:
      - email/draft
      - email/send
  approval:
    mode: on-request
  workspace:
    required: false
    writeScope: read-only
  session:
    persistence: sqlite
    compaction: auto
  policy:
    overlays:
      - ./policies/email-overrides.yaml
```

`./policies/email-overrides.yaml`:
```yaml
version: 1
rules:
  - id: allow-draft-no-approval
    action: tool
    tool: email/draft
    decision: allow          # draft is low risk, skip approval
  - id: require-send-approval
    action: tool
    tool: email/send
    decision: require_approval  # always confirm before sending
```

---

## Where to place profiles

### Project-local (recommended for team agents)

```
my-project/
  .agent/
    profiles/
      research/
        profile.yaml
        prompts/
          system.md
        policies/
          custom.yaml
```

Run from the project root:
```bash
agent run --profile research "summarize the repo"
agent run --profile .agent/profiles/research "summarize the repo"
```

### User-level (personal agents)

```
~/.agent/
  profiles/
    coding/
      profile.yaml
      prompts/
        system.md
```

```bash
agent run --profile coding "refactor this function"
```

### Explicit path (quickest for one-offs)

```bash
agent run --profile /tmp/test-profiles/my-agent.yaml "do something"
```

### Install a profile from a directory

Copy a profile directory to `~/.agent/profiles/` for permanent availability:

```bash
agent profiles install ./my-profiles/researcher
```

This reads the `profile.yaml` from the source directory, determines the profile name
from `metadata.name`, and copies the entire directory to `~/.agent/profiles/<name>/`.

Useful for sharing profiles between projects or installing profile templates from
plugin examples:

```bash
# Install a plugin's example profile for customisation
agent profiles install ../agent-plugins/send-email/examples/profiles/email-openai
```

---

## Plugin-contributed profile templates

Many plugins ship example profiles under `contributes.profileTemplates` in their `plugin.yaml`.
These are registered by the framework when the plugin is enabled and can be listed with:

```bash
agent doctor
# registered_profile_templates  3
```

**Profile templates are reference material, not enforced configuration.** They show the
plugin author's recommended way to use the plugin. You can:

- Copy a template as the starting point for your own profile
- Ignore it entirely
- Use only some of its suggestions

Your profile YAML is always the final word. Templates are never auto-applied.

To see what a template looks like, find it in the installed plugin directory:

```bash
ls ~/.agent/plugins/send-email/profiles/
cat ~/.agent/plugins/send-email/profiles/email-assistant.yaml
```
