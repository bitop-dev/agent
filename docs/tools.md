# Tools

Tools are functions the LLM can call during a conversation. The agent executes
them and feeds the results back to the model.

There are three categories:

1. **Built-in tools** — compiled into the binary, selected by preset
2. **Plugin tools** — external executables communicating over JSON/stdin/stdout
3. **Compiled-in custom tools** — Go types implementing `tools.Tool`, registered in code

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
| `glob` | string | | File glob filter (e.g. `"*.go"`) |
| `case_sensitive` | boolean | | Default: `false` (case-insensitive) |
| `context_lines` | number | | Lines of context around each match |
| `max_results` | number | | Cap on matches returned (default: 100) |

**Example LLM call:**
```json
{"pattern": "func.*Handler", "path": ".", "glob": "*.go", "context_lines": 2}
```

---

### `find`

Recursively find files/directories matching a pattern. Pure Go — no `find`
binary required.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `path` | string | | Root directory to search (default: `work_dir`) |
| `pattern` | string | | Glob or substring to match against file names |
| `type` | string | | `"file"` \| `"dir"` \| `""` (both) |
| `max_results` | number | | Cap on results returned (default: 200) |

**Example LLM call:**
```json
{"path": ".", "pattern": "*_test.go", "type": "file"}
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

Whichever is hit first. When output is truncated, a note is appended:

```
[Output truncated at 50KB. Full output saved to: /tmp/agent-out-abc123.txt]
```

The `truncate` package provides these utilities for use in your own tools:

```go
import "github.com/nickcecere/agent/pkg/tools/builtin"

result := builtin.TruncateHead(output, builtin.DefaultMaxBytes, builtin.DefaultMaxLines)
result := builtin.TruncateTail(output, builtin.DefaultMaxBytes, builtin.DefaultMaxLines)
size   := builtin.FormatSize(bytes)   // "50KB", "1.5MB"
```

---

## Writing a Compiled-In Tool

Implement `tools.Tool` and register it with the agent's registry.

```go
package mytools

import (
    "context"
    "fmt"

    "github.com/nickcecere/agent/pkg/ai"
    "github.com/nickcecere/agent/pkg/tools"
)

// WeatherTool fetches current weather for a city.
type WeatherTool struct {
    apiKey string
}

func NewWeatherTool(apiKey string) *WeatherTool {
    return &WeatherTool{apiKey: apiKey}
}

func (t *WeatherTool) Definition() ai.ToolDefinition {
    return ai.ToolDefinition{
        Name:        "get_weather",
        Description: "Get the current weather for a city.",
        Parameters: tools.MustSchema(tools.SimpleSchema{
            Properties: map[string]tools.Property{
                "city": {
                    Type:        "string",
                    Description: "City name, e.g. 'San Francisco'",
                },
                "units": {
                    Type:        "string",
                    Description: "Temperature units: 'celsius' or 'fahrenheit'",
                    Enum:        []any{"celsius", "fahrenheit"},
                },
            },
            Required: []string{"city"},
        }),
    }
}

func (t *WeatherTool) Execute(
    ctx context.Context,
    callID string,
    params map[string]any,
    onUpdate tools.UpdateFn,
) (tools.Result, error) {
    city, _ := params["city"].(string)
    units, _ := params["units"].(string)
    if units == "" {
        units = "celsius"
    }

    // Stream a progress update while fetching
    if onUpdate != nil {
        onUpdate(tools.TextResult(fmt.Sprintf("Fetching weather for %s...", city)))
    }

    // Fetch weather (pseudocode)
    temp, condition, err := fetchWeather(ctx, t.apiKey, city, units)
    if err != nil {
        return tools.ErrorResult(err), nil
    }

    return tools.TextResult(fmt.Sprintf(
        "Weather in %s: %d°%s, %s",
        city, temp, unitSymbol(units), condition,
    )), nil
}
```

Register it with the agent:

```go
reg := tools.NewRegistry()
reg.Register(mytools.NewWeatherTool(os.Getenv("WEATHER_API_KEY")))

agent := agent.New(provider, "model", reg, systemPrompt)
```

See [sdk.md](sdk.md) for a full example.

---

## Writing a Plugin Tool

Plugin tools are external executables that communicate with the agent over
JSON-encoded messages on stdin/stdout. They can be written in any language.

### Protocol

1. **Startup:** Agent starts your executable and writes a `{"type":"init","tools":[...]}` message. Your process reads this and responds with the tool schemas.
2. **Execution:** Agent writes `{"type":"call","name":"...","call_id":"...","params":{...}}`. Your process executes and writes back `{"type":"result","call_id":"...","content":[...],"is_error":false}`.

### Go Plugin Example

```go
package main

import (
    "bufio"
    "encoding/json"
    "fmt"
    "os"
)

type inMsg struct {
    Type   string          `json:"type"`
    Tools  json.RawMessage `json:"tools,omitempty"`
    Name   string          `json:"name,omitempty"`
    CallID string          `json:"call_id,omitempty"`
    Params map[string]any  `json:"params,omitempty"`
}

type outMsg struct {
    Type    string `json:"type"`
    CallID  string `json:"call_id,omitempty"`
    Content []struct {
        Type string `json:"type"`
        Text string `json:"text"`
    } `json:"content,omitempty"`
    IsError bool             `json:"is_error,omitempty"`
    Tools   []map[string]any `json:"tools,omitempty"`
}

func main() {
    scanner := bufio.NewScanner(os.Stdin)
    enc := json.NewEncoder(os.Stdout)

    for scanner.Scan() {
        var msg inMsg
        if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
            continue
        }
        switch msg.Type {
        case "init":
            enc.Encode(outMsg{
                Type: "init",
                Tools: []map[string]any{{
                    "name":        "my_plugin_tool",
                    "description": "Does something useful.",
                    "parameters": map[string]any{
                        "type": "object",
                        "properties": map[string]any{
                            "input": map[string]any{"type": "string"},
                        },
                        "required": []string{"input"},
                    },
                }},
            })
        case "call":
            input, _ := msg.Params["input"].(string)
            result := fmt.Sprintf("Processed: %s", input)
            enc.Encode(outMsg{
                Type:   "result",
                CallID: msg.CallID,
                Content: []struct {
                    Type string `json:"type"`
                    Text string `json:"text"`
                }{{Type: "text", Text: result}},
            })
        }
    }
}
```

### Registering Plugin Tools

```yaml
tools:
  preset: none    # or coding, etc.
  plugins:
    - path: ./plugins/my-tool
      args: []
    - path: /usr/local/bin/my-plugin
      args: ["--verbose"]
```

The plugin process is started once at agent startup and kept alive for the
session. Each tool call is a JSON round-trip over stdin/stdout.

See `examples/tools/bash_tool/main.go` for a complete working plugin.

---

## Tool Validation

Before calling `Execute`, the agent validates the LLM's arguments against the
tool's JSON Schema. Invalid or missing required parameters produce an error
result sent back to the LLM, with a message like:

```
validation error: missing required parameter "city"
```

The agent never calls `Execute` with invalid arguments.
