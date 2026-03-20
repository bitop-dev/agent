# Pattern 1: Single-Tool Agent

The simplest possible agent: one plugin, one tool, one focused task.
This is the right starting point for wrapping an existing CLI, script, or API.

---

## What it looks like

```
User prompt
    └── Model decides to call the tool
        └── Tool executes (script / binary / HTTP / CLI)
            └── Result returned to model
                └── Model produces final response
```

---

## Real example: word-count agent

This is the `python-tool` plugin. It ships a Python script that counts words.
The agent's only job is to receive text and call the tool.

### Plugin structure

```
python-tool/
  plugin.yaml
  tools/
    word-count.yaml
  scripts/
    tool.py            ← the actual implementation
```

`plugin.yaml` declares the runtime and tools:
```yaml
spec:
  runtime:
    type: command
    command: ["python3", "scripts/tool.py"]
  contributes:
    tools:
      - id: py/word-count
        path: tools/word-count.yaml
```

`tools/word-count.yaml` tells the model what the tool does and what it needs:
```yaml
id: py/word-count
description: Count words in a text string
inputSchema:
  type: object
  properties:
    text:
      type: string
      description: Text to count words in
  required:
    - text
execution:
  mode: command
  operation: word-count
  timeout: 5
```

### Install and enable

```bash
agent plugins install python-tool
agent plugins enable python-tool
```

### Profile

```yaml
# ~/.agent/profiles/word-counter/profile.yaml
apiVersion: agent/v1
kind: Profile
metadata:
  name: word-counter
  version: 0.1.0
  description: Counts words in text
spec:
  instructions:
    system:
      - "You are a word counting assistant. Use the py/word-count tool to count words."
  provider:
    default: openai
    model: nemotron-3-super-120b-a12b
  tools:
    enabled:
      - py/word-count
  approval:
    mode: never
  workspace:
    required: false
    writeScope: read-only
  session:
    persistence: sqlite
    compaction: auto
  policy:
    overlays: []
```

### Run it

```bash
agent run --profile word-counter "Count the words in: the quick brown fox jumps over the lazy dog"
```

Output:
```
[tool request] py/word-count
[policy] tool allowed
[tool finished] 9 words

The sentence contains 9 words.
```

---

## The general pattern

### Step 1 — Choose a runtime type

| Your tool is... | Use runtime |
|---|---|
| An existing CLI (`gh`, `kubectl`, `jq`) | `command` with argv-template mode |
| A custom script (Python, Bash, Ruby) | `command` with JSON-stdin/stdout mode |
| A custom Go/Rust/C binary | `command` with JSON-stdin/stdout mode |
| An HTTP service you control | `http` |
| An MCP server | `mcp` |

### Step 2 — Write the tool descriptor

Every tool descriptor needs:

```yaml
id: namespace/tool-name       # what the profile lists in tools.enabled
description: ...              # what the model reads to decide when to call it
inputSchema:                  # what arguments the model must provide
  type: object
  properties:
    arg1:
      type: string
      description: ...
  required: [arg1]
execution:
  mode: command               # or http
  operation: my-operation     # the operation name sent to your runtime
  timeout: 10
risk:
  level: low                  # low | medium | high
```

**Write the description carefully.** The model reads it to decide whether and when
to call the tool. A vague description leads to missed calls or wrong calls.

Good: `"Count the exact number of words in a string of text"`
Bad: `"Word counter"`

### Step 3 — Write the profile

Keep single-tool agent profiles minimal:

```yaml
spec:
  instructions:
    system:
      - "You are a [purpose] assistant. Use [tool-id] to [what it does]."
  tools:
    enabled:
      - your/tool-id
  approval:
    mode: never        # single low-risk tools rarely need approval
```

### Step 4 — Test the tool directly first

Before involving the model, test your tool binary/script directly with the
JSON-stdin/stdout protocol:

```bash
echo '{"plugin":"my-plugin","tool":"my/tool","operation":"my-op","arguments":{"input":"hello world"},"config":{}}' \
  | python3 scripts/tool.py
```

Expected output:
```json
{"output": "result text", "data": {"key": "value"}}
```

If that works, the agent will work.

---

## Wrapping an existing CLI (argv-template mode)

For tools like `gh`, `kubectl`, or `jq` that you don't control:

```yaml
# plugin.yaml
spec:
  runtime:
    type: command
    command: ["gh"]     # the CLI binary

# tools/pr-list.yaml
execution:
  mode: command
  argv: ["pr", "list", "--repo", "{{repo}}", "--state", "{{state}}", "--json", "number,title,url"]
```

`{{repo}}` and `{{state}}` are filled from the tool call arguments.
If a value is empty, the framework drops both the flag and the value automatically
(so `--state {{state}}` disappears if state isn't provided).

Use `{{config.key}}` to inject plugin config values:
```yaml
argv: ["--token", "{{config.apiKey}}", "repo", "view", "{{repo}}"]
```

---

## Practical tips

**Keep the tool focused.** A tool that does one thing is easier for the model
to use correctly than a tool that does many things based on a `mode` argument.
Split complex tools into multiple simple ones.

**Set a realistic timeout.** CLI tools that call external APIs can be slow.
Set `execution.timeout` to something generous (15-30 seconds) for network calls.

**Return structured data alongside text output.** The model reads `output` (text)
but the `data` field (JSON object) is available for downstream processing.
Return both:

```json
{
  "output": "found 3 results",
  "data": {
    "count": 3,
    "results": [{"title": "...", "url": "..."}]
  }
}
```

**Mark risky tools correctly.** If your tool has side effects (sends data, modifies files,
calls APIs that cost money), set `risk.level: high` and list it in
`permissions.sensitiveActions` in `plugin.yaml`. This automatically requires approval.

---

## Related patterns

- [Pattern 2: Research Agent](./02-research-agent.md) — chain multiple tools
- [Pattern 5: Policy and Approval](./05-policy-and-approval-patterns.md) — control risky tools
