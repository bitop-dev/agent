# Tools

Tools are functions the LLM can call during a conversation. The agent executes
them and feeds the results back to the model.

There are three categories:

1. **Built-in tools** — compiled into the binary, selected by preset
2. **Plugin tools** — external executables communicating over JSON/stdin/stdout
3. **Compiled-in custom tools** — Go types implementing `tools.Tool`, registered in code

For a comprehensive guide to writing your own tools (both compiled-in and plugins,
with examples in Python, TypeScript, Rust, Bash, and Ruby) see
[custom-tools.md](custom-tools.md).

---

## Built-in Tool Presets

Select a preset in your config:

```yaml
tools:
  preset: coding     # read, bash, edit, write  (default)
  work_dir: .        # working directory for file tools
```

| Preset | Tools |
|--------|-------|
| `coding` | `read`, `bash`, `edit`, `write` |
| `readonly` | `read`, `grep`, `find`, `ls` |
| `web` | `web_search`, `web_fetch` |
| `all` | All of the above |
| `none` | No built-in tools |

---

## Built-in Tools Reference

### `read`

Read a file, returning its contents as text.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `path` | string | ✓ | File path (absolute or relative to `work_dir`) |
| `offset` | number | | 1-based line number to start from |
| `limit` | number | | Maximum number of lines to return |

**Truncation:** Output is capped at 50 KB and 2000 lines (whichever is hit first). A summary of total size is appended if truncated.

**Example LLM call:**
```json
{"path": "pkg/agent/loop.go", "offset": 1, "limit": 50}
```

---

### `bash`

Execute a shell command and return its combined stdout/stderr.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `command` | string | ✓ | Shell command to run |
| `timeout` | number | | Timeout in seconds (default: 120) |

**Behaviour:**
- Runs in a subprocess with the `work_dir` as working directory
- stdout and stderr are merged
- Exit code is appended to output on non-zero exit
- Output is capped at 50 KB / 2000 lines

**Example LLM call:**
```json
{"command": "go test ./... 2>&1", "timeout": 60}
```

The `bash` tool supports a pluggable `Executor` interface, so you can swap in
a remote or sandboxed executor at the Go level. See [sdk.md](sdk.md).

---

### `edit`

Replace an exact string in a file. The `old_text` must match character-for-
character (including whitespace and newlines).

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `path` | string | ✓ | File to edit |
| `old_text` | string | ✓ | Exact string to find and replace |
| `new_text` | string | ✓ | Replacement string |

**Errors:**
- File not found
- `old_text` not found in file
- `old_text` found more than once (ambiguous)

**Example LLM call:**
```json
{
  "path": "main.go",
  "old_text": "fmt.Println(\"hello\")",
  "new_text": "fmt.Println(\"world\")"
}
```

---

### `write`

Create a new file or completely overwrite an existing one. Parent directories
are created automatically.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `path` | string | ✓ | File to write |
| `content` | string | ✓ | Full file contents |

**Example LLM call:**
```json
{"path": "notes.md", "content": "# Notes\n\n- item one\n"}
```

---

### `grep`

Search for a regular expression pattern across files. Pure Go implementation —
no external tools required.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `pattern` | string | ✓ | Regular expression (Go `regexp` syntax) |
| `path` | string | | File or directory to search (default: `work_dir`) |
| `glob` | string | | File glob filter (e.g. `"*.go"`, `"**/*.spec.ts"`) |
| `ignoreCase` | boolean | | Case-insensitive search (default: `false`) |
| `literal` | boolean | | Treat pattern as literal string, not regex (default: `false`) |
| `context` | number | | Lines of context before/after each match (default: `0`) |
| `limit` | number | | Cap on matches returned (default: `100`) |

**Notes:**
- Respects `.gitignore`. Skips `.git`, `node_modules`, and common binary files.
- Long lines are truncated to 500 chars; use the `read` tool to see full lines.
- Emits streaming progress updates every 100 files scanned in large trees.

**Example LLM call:**
```json
{"pattern": "func.*Handler", "path": ".", "glob": "*.go", "context": 2}
```

---

### `find`

Recursively find files/directories matching a pattern. Pure Go — no `find`
binary required.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `pattern` | string | ✓ | Glob pattern to match files, e.g. `"*.go"`, `"**/*.json"` |
| `path` | string | | Root directory to search (default: `work_dir`) |
| `limit` | number | | Cap on results returned (default: `1000`) |

**Notes:**
- Respects `.gitignore`. Skips `.git` and `node_modules`.
- Supports `**` glob patterns for recursive matching.
- Emits streaming progress updates every 200 entries scanned.
- Only matches files (not directories).

**Example LLM call:**
```json
{"pattern": "*_test.go", "path": "."}
```

---

### `ls`

List directory contents with sizes and modification times.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `path` | string | | Directory to list (default: `work_dir`) |
| `show_hidden` | boolean | | Include dotfiles (default: `false`) |

**Example LLM call:**
```json
{"path": "pkg/agent", "show_hidden": false}
```

---

### `web_search`

Search the web via DuckDuckGo Lite. No API key required.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `query` | string | ✓ | Search query |
| `max_results` | number | | Maximum results (default: 10, max: 20) |

Returns titles, URLs, and snippets for each result.

**Example LLM call:**
```json
{"query": "golang 1.24 release notes", "max_results": 5}
```

---

### `web_fetch`

Fetch a URL and return its content as clean plain text. HTML is converted to
readable Markdown-like text (headings, lists, code blocks preserved). Images
are rendered as `[Image: alt text]`.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `url` | string | ✓ | URL to fetch |
| `max_bytes` | number | | Max response bytes (default: 51200, max: 102400) |

**Notes:**
- Emits a `Fetching URL…` streaming progress update before the HTTP request.
- Follows redirects (up to 10). Appended redirect notice when URL changes.
- Content-Type is detected; HTML is converted, plain text/JSON returned as-is.

**Example LLM call:**
```json
{"url": "https://go.dev/doc/go1.24", "max_bytes": 51200}
```

---

## Output Truncation

**All tools truncate their output** to avoid overwhelming the LLM context.
The default limits are:

- **50 KB** (51,200 bytes)
- **2000 lines**

Whichever is hit first. When output is truncated, a notice is appended to the
result, for example:

```
[50KB limit reached. Use offset/limit parameters to read further.]
```

The `builtin` package exposes helpers for use in your own tools:

```go
import "github.com/bitop-dev/agent/pkg/tools/builtin"

tr := builtin.TruncateHead(output, maxLines, maxBytes)
// tr.Content   — truncated text
// tr.Truncated — true if content was cut

size := builtin.FormatSize(51200) // "50 KB"
```

---

## Writing a Compiled-In Tool

Implement `tools.Tool` and register it with the agent's registry.
See [custom-tools.md](custom-tools.md) for the full reference.

```go
package mytools

import (
    "context"
    "fmt"

    "github.com/bitop-dev/agent/pkg/ai"
    "github.com/bitop-dev/agent/pkg/tools"
)

// WeatherTool fetches current weather for a city.
type WeatherTool struct{ apiKey string }

func NewWeatherTool(apiKey string) *WeatherTool {
    return &WeatherTool{apiKey: apiKey}
}

func (t *WeatherTool) Definition() ai.ToolDefinition {
    return ai.ToolDefinition{
        Name:        "get_weather",
        Description: "Get the current weather for a city.",
        Parameters: tools.MustSchema(tools.SimpleSchema{
            Properties: map[string]tools.Property{
                "city":  {Type: "string", Description: "City name, e.g. 'Berlin'"},
                "units": {Type: "string", Description: "celsius or fahrenheit",
                          Enum: []any{"celsius", "fahrenheit"}},
            },
            Required: []string{"city"},
        }),
    }
}

func (t *WeatherTool) Execute(
    ctx context.Context, _ string,
    params map[string]any, onUpdate tools.UpdateFn,
) (tools.Result, error) {
    city, _ := params["city"].(string)
    units, _ := params["units"].(string)
    if units == "" {
        units = "celsius"
    }
    if onUpdate != nil {
        onUpdate(tools.TextResult(fmt.Sprintf("Fetching weather for %s…", city)))
    }
    temp, condition, err := fetchWeather(ctx, t.apiKey, city, units)
    if err != nil {
        return tools.ErrorResult(err), nil
    }
    return tools.TextResult(fmt.Sprintf("Weather in %s: %.1f° %s, %s",
        city, temp, units, condition)), nil
}
```

Register it with the agent:

```go
reg := tools.NewRegistry()
reg.Register(mytools.NewWeatherTool(os.Getenv("WEATHER_API_KEY")))

a := agent.New(agent.Options{
    Provider: provider,
    Model:    "claude-sonnet-4-5",
    Tools:    reg,
})
```

---

## Writing a Plugin Tool

Plugin tools are external executables that communicate with the agent over
newline-delimited JSON on stdin/stdout. They can be written in any language.

See [custom-tools.md](custom-tools.md) for the full protocol spec and examples
in Python, TypeScript, Rust, Bash, and Ruby.

### Protocol (summary)

**Step 1 — describe** (once, at startup):

```
agent → plugin:  {"type":"describe"}
plugin → agent:  {"name":"my_tool","description":"...","parameters":{...}}
```

**Step 2 — call** (once per tool invocation):

```
agent → plugin:  {"type":"call","call_id":"c1","params":{"input":"hello"}}
plugin → agent:  {"content":[{"type":"text","text":"result"}],"error":false}
```

On error, set `"error": true` and put the message in `content`.

### Minimal Go Example

```go
package main

import (
    "bufio"
    "encoding/json"
    "fmt"
    "os"
)

func main() {
    scanner := bufio.NewScanner(os.Stdin)
    enc := json.NewEncoder(os.Stdout)

    for scanner.Scan() {
        var msg map[string]any
        if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
            continue
        }
        switch msg["type"] {
        case "describe":
            enc.Encode(map[string]any{
                "name":        "echo",
                "description": "Echoes the input back.",
                "parameters": map[string]any{
                    "type": "object",
                    "properties": map[string]any{
                        "input": map[string]any{"type": "string", "description": "Text to echo"},
                    },
                    "required": []string{"input"},
                },
            })
        case "call":
            params := msg["params"].(map[string]any)
            input, _ := params["input"].(string)
            enc.Encode(map[string]any{
                "content": []map[string]string{{"type": "text", "text": fmt.Sprintf("Echo: %s", input)}},
                "error":   false,
            })
        }
    }
}
```

### Registering Plugin Tools

```yaml
tools:
  preset: coding
  plugins:
    - path: python3
      args: ["./plugins/my_tool.py"]
    - path: ./plugins/my_tool       # compiled binary
      args: []
```

The plugin process starts once and stays alive for the session.
Each tool call is one JSON round-trip.

---

## Tool Validation

Before calling `Execute`, the agent validates the LLM's arguments against the
tool's JSON Schema. Invalid or missing required parameters produce an error
result sent back to the LLM, with a message like:

```
validation error: missing required parameter "city"
```

The agent never calls `Execute` with invalid arguments.
