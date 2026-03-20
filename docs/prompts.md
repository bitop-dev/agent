# Prompts

The system prompt tells the agent who it is, what its job is, and how it should behave.
This document explains how system prompts are built, how plugins can contribute prompt
fragments, and how profiles combine multiple sources into a single system instruction.

Related docs:
- `docs/profiles.md` — the full profile reference including `spec.instructions`
- `docs/building-plugins.md` — how plugins contribute prompts

---

## How system prompts work

When an agent runs, the framework builds a single system prompt string from the profile's
`spec.instructions.system` list. Each entry in the list is resolved and all resolved values
are joined together with a blank line separator.

```yaml
spec:
  instructions:
    system:
      - ./prompts/system.md          # file path
      - email/style-default          # plugin prompt ID
      - "Always cite your sources."  # inline text
```

This would produce a single system prompt that is the concatenation of all three, in order.

---

## Resolution order

Each entry in `instructions.system` is resolved in this order:

### 1. Plugin prompt ID

If an enabled plugin has registered a prompt with this exact ID, the framework loads
that prompt's file content.

```yaml
# send-email/plugin.yaml
spec:
  contributes:
    prompts:
      - id: email/style-default
        path: prompts/style-default.md
```

Once the plugin is installed and enabled, you can reference it from any profile by ID:

```yaml
spec:
  instructions:
    system:
      - email/style-default   # resolved from the plugin registry
```

The file content of `~/.agent/plugins/send-email/prompts/style-default.md` is loaded
and included in the system prompt. No file path needed in the profile.

**This is the recommended way to use plugin-contributed prompts.** It decouples your
profile from the plugin's install location and allows the plugin author to update the
prompt content without requiring changes to your profile.

### 2. File path

If the entry is not a registered prompt ID, the framework tries to load it as a file.
Relative paths are resolved from the profile's directory.

```yaml
spec:
  instructions:
    system:
      - ./prompts/system.md         # relative to the profile directory
      - ./prompts/extra.md
      - /absolute/path/to/base.md   # absolute path
```

This is the standard way to include your own custom instructions.

### 3. Inline text

If neither lookup succeeds (not a registered ID, not a file on disk), the entry is used
as literal prompt text.

```yaml
spec:
  instructions:
    system:
      - "You are a concise assistant. Answer in plain text."
```

Inline text is convenient for short instructions or quick experiments.
For anything more than a sentence, a file is easier to maintain.

---

## Combining multiple sources

All resolved entries are joined with `\n\n` (a blank line). Order is preserved.

```yaml
spec:
  instructions:
    system:
      - ./prompts/base.md            # your base persona
      - email/style-default          # plugin's email writing style
      - web/research-style           # plugin's research style
      - "Today's date is 2026-03-20."  # dynamic inline instruction
```

This produces:
```
<contents of base.md>

<contents of send-email plugin's style-default.md>

<contents of web-research plugin's research-style.md>

Today's date is 2026-03-20.
```

You control the order. Put your most important instructions first.

---

## Writing effective system prompts

### Persona and role

```markdown
You are a research assistant specialized in AI news.
Your job is to search for recent developments, fetch article content,
synthesize findings, and send concise summary emails.
```

### Workflow instructions

```markdown
Always follow this workflow:
1. Search for the topic using ddg/search
2. Fetch the 2-3 most relevant articles using ddg/fetch
3. Synthesize the content into a clear summary
4. Draft the email with email/draft
5. Send it with email/send

Do not skip steps. Do not stop after drafting.
```

### Output format

```markdown
When writing emails:
- Subject line should summarize the key topic
- Open with a 2-sentence executive summary
- Use bullet points for individual findings
- Include source URLs for each key story
- Sign off as "AI Research Assistant"
- Keep the body under 400 words
```

### Constraints

```markdown
Never write directly to the filesystem.
Never execute shell commands.
If you are unsure about something, say so rather than guessing.
```

---

## Plugin-contributed prompts

Plugins can contribute reusable prompt fragments that any profile can include.

### How plugins register prompts

In `plugin.yaml`:

```yaml
spec:
  contributes:
    prompts:
      - id: email/style-default
        path: prompts/style-default.md
      - id: email/formal-mode
        path: prompts/formal.md
```

The `id` is the key used in profile `instructions.system`.
The `path` is relative to the plugin bundle directory.

Multiple prompts can be contributed. Each gets its own ID.

### What plugin prompts are for

Plugin authors use contributed prompts to ship:

- **Style guides** — how the agent should format tool outputs
- **Usage instructions** — how to use the plugin's tools correctly
- **Persona fragments** — domain-specific behavior rules
- **Safety guidelines** — constraints relevant to the plugin's risk area

For example, the `send-email` plugin ships `email/style-default` which says:

```
Write concise, professional emails.
Prefer drafts first unless explicitly asked to send.
```

Including this in your profile means you get the plugin author's recommended
behavior without copying the text into your own prompt files.

### When plugin prompts are available

A plugin prompt is available in the registry as soon as:

1. The plugin bundle is installed
2. The plugin is enabled

Check what prompts are registered:

```bash
agent doctor
# registered_prompts  3
```

To inspect a specific prompt's content, look at the plugin's installed files:

```bash
cat ~/.agent/plugins/send-email/prompts/style-default.md
```

### Overriding plugin prompts

Plugin prompts are included by choice — you reference them by ID or not at all.
There is no auto-injection. If you do not include `email/style-default` in your profile,
it has no effect on your agent.

To override a plugin prompt's content, do not reference it by ID.
Instead, write your own instructions in a local file:

```yaml
spec:
  instructions:
    system:
      - ./prompts/my-email-style.md   # your version, instead of email/style-default
```

To extend a plugin prompt with your own additions, include both:

```yaml
spec:
  instructions:
    system:
      - email/style-default           # plugin baseline
      - ./prompts/my-additions.md     # your additions, appended after
```

---

## Practical examples

### Example 1 — Local file only

The simplest approach. Write your instructions in a markdown file next to the profile.

```
my-agent/
  profile.yaml
  prompts/
    system.md
```

```yaml
# profile.yaml
spec:
  instructions:
    system:
      - ./prompts/system.md
```

```markdown
# prompts/system.md
You are a coding assistant. Help the user read, understand, and modify code.
Always read files before editing them. Prefer small targeted edits over rewrites.
```

---

### Example 2 — Plugin prompt by ID

Use the send-email plugin's writing style without copying its content.

```yaml
spec:
  instructions:
    system:
      - ./prompts/research-agent.md
      - email/style-default           # loaded from installed plugin
```

If the `send-email` plugin is updated and ships a better `style-default.md`,
your profile automatically benefits from that update.

---

### Example 3 — Multiple plugins with combined prompts

A research-and-email agent using prompts from two plugins:

```yaml
spec:
  instructions:
    system:
      - ./prompts/base-persona.md
      - web/research-style      # from web-research plugin
      - email/style-default     # from send-email plugin
```

---

### Example 4 — Inline text for dynamic context

Inject runtime context as inline text. Useful for date, environment, or persona tweaks.

```yaml
spec:
  instructions:
    system:
      - ./prompts/system.md
      - "The current date is 2026-03-20. Prioritize news from the last 7 days."
```

---

### Example 5 — No system prompt

An agent with no system instructions at all. The model uses its default behavior.

```yaml
spec:
  instructions:
    system: []
```

Useful for debugging or when you want a neutral agent with no persona.

---

## Where prompt files live

Prompt files belong next to their profile:

```
my-project/
  .agent/
    profiles/
      research/
        profile.yaml
        prompts/
          system.md
          research-focus.md
```

For user-level prompts that are reused across multiple profiles:

```
~/.agent/
  profiles/
    coding/
      profile.yaml
      prompts/
        system.md
    researcher/
      profile.yaml
      prompts/
        system.md
```

Absolute paths are allowed but make profiles harder to share across machines.
Relative paths (relative to the profile directory) are always preferred.
