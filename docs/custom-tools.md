# Custom Tools

There are two ways to extend the agent with custom tools:

| Method | Language | Distribution |
|--------|----------|-------------|
| **Compiled-in** | Go only | Baked into your binary |
| **Plugin** | Any language | External subprocess |

Compiled-in tools have lower overhead and can access Go types directly.
Plugin tools can be written in Python, TypeScript, Rust, Bash, Ruby — anything
that can read from stdin and write to stdout.

---

## Part 1 — Compiled-In Go Tools

### The `tools.Tool` Interface

```go
// Tool is the interface every tool must implement.
type Tool interface {
    Definition() ai.ToolDefinition
    Execute(ctx context.Context, callID string, params map[string]any, onUpdate UpdateFn) (Result, error)
}

// UpdateFn is a callback for streaming progress updates.
// Call it while the tool is running to show intermediate results.
type UpdateFn func(Result)
```

### Defining Parameters

Use `tools.MustSchema` with `tools.SimpleSchema` to define the JSON Schema
the LLM sees:

```go
Parameters: tools.MustSchema(tools.SimpleSchema{
    Properties: map[string]tools.Property{
        "city": {
            Type:        "string",
            Description: "City name, e.g. 'Berlin'",
        },
        "units": {
            Type:        "string",
            Description: "Temperature units",
            Enum:        []any{"celsius", "fahrenheit"},
        },
        "count": {
            Type:        "integer",
            Description: "How many results to return (default 5)",
        },
    },
    Required: []string{"city"}, // fields not here are optional
}),
```

Supported `Type` values: `"string"`, `"number"`, `"integer"`, `"boolean"`, `"array"`, `"object"`.

### Returning Results

```go
// Plain text
return tools.TextResult("The answer is 42"), nil

// Error (surfaced to the LLM as an error result)
return tools.ErrorResult(fmt.Errorf("city not found: %s", city)), nil

// Multiple content blocks (text + optional metadata)
return tools.Result{
    Content: []ai.ContentBlock{
        ai.TextContent{Type: "text", Text: "Result: ..."},
    },
    Details: map[string]any{
        "raw_value": 42.0,
        "unit":      "celsius",
    },
}, nil

// Image result (for vision-capable models)
return tools.Result{
    Content: []ai.ContentBlock{
        ai.TextContent{Type: "text", Text: "Screenshot captured:"},
        ai.ImageContent{Type: "image", MIMEType: "image/png", Data: base64Data},
    },
}, nil
```

`Details` are not sent to the LLM — they are available in the `EventToolEnd`
event for logging and metrics.

### Streaming Progress

For long-running tools, call `onUpdate` periodically:

```go
func (t *MyTool) Execute(ctx context.Context, _ string, params map[string]any, onUpdate tools.UpdateFn) (tools.Result, error) {
    urls := params["urls"].([]any)

    for i, u := range urls {
        if ctx.Err() != nil {
            return tools.ErrorResult(ctx.Err()), nil
        }

        // Emit progress — shown as a tool_update event
        if onUpdate != nil {
            onUpdate(tools.TextResult(fmt.Sprintf("[%d/%d] fetching %s…", i+1, len(urls), u)))
        }

        result, err := fetch(ctx, u.(string))
        // …
    }
    return tools.TextResult(combined), nil
}
```

### Context Cancellation

Always check `ctx.Err()` in long loops. The agent cancels the context when:
- The user types `quit` / hits Ctrl-C
- `agent.Abort()` is called
- `Config.ToolTimeout` is exceeded

### Registering

```go
reg := tools.NewRegistry()
reg.Register(&MyTool{apiKey: os.Getenv("MY_API_KEY")})
reg.Register(&AnotherTool{})

a := agent.New(agent.Options{
    Provider: provider,
    Model:    "claude-sonnet-4-5",
    Tools:    reg,
})
```

You can also add tools after construction:

```go
a.Tools().Register(&LateAddedTool{})
```

### Complete Example

```go
package main

import (
    "context"
    "fmt"
    "math"

    "github.com/bitop-dev/agent/pkg/ai"
    "github.com/bitop-dev/agent/pkg/tools"
)

// StatsTool computes descriptive statistics on a list of numbers.
type StatsTool struct{}

func (t *StatsTool) Definition() ai.ToolDefinition {
    return ai.ToolDefinition{
        Name:        "stats",
        Description: "Compute descriptive statistics (min, max, mean, stddev) on a list of numbers.",
        Parameters: tools.MustSchema(tools.SimpleSchema{
            Properties: map[string]tools.Property{
                "numbers": {
                    Type:        "array",
                    Description: "List of numeric values",
                },
                "precision": {
                    Type:        "integer",
                    Description: "Decimal places in output (default 2)",
                },
            },
            Required: []string{"numbers"},
        }),
    }
}

func (t *StatsTool) Execute(
    _ context.Context, _ string,
    params map[string]any, _ tools.UpdateFn,
) (tools.Result, error) {
    raw, ok := params["numbers"].([]any)
    if !ok || len(raw) == 0 {
        return tools.ErrorResult(fmt.Errorf("numbers must be a non-empty array")), nil
    }

    precision := 2
    if p, ok := params["precision"].(float64); ok {
        precision = int(p)
    }

    vals := make([]float64, len(raw))
    for i, v := range raw {
        switch n := v.(type) {
        case float64:
            vals[i] = n
        default:
            return tools.ErrorResult(fmt.Errorf("element %d is not a number", i)), nil
        }
    }

    // Compute
    min, max := vals[0], vals[0]
    sum := 0.0
    for _, v := range vals {
        sum += v
        if v < min { min = v }
        if v > max { max = v }
    }
    mean := sum / float64(len(vals))
    variance := 0.0
    for _, v := range vals {
        d := v - mean
        variance += d * d
    }
    stddev := math.Sqrt(variance / float64(len(vals)))

    format := fmt.Sprintf("%%.%df", precision)
    result := fmt.Sprintf(
        "n=%d  min="+format+"  max="+format+"  mean="+format+"  stddev="+format,
        len(vals), min, max, mean, stddev,
    )

    return tools.Result{
        Content: []ai.ContentBlock{ai.TextContent{Type: "text", Text: result}},
        Details: map[string]any{
            "n": len(vals), "min": min, "max": max, "mean": mean, "stddev": stddev,
        },
    }, nil
}
```

---

## Part 2 — Plugin Tools (Any Language)

A plugin is an executable that speaks a simple JSON-over-stdin/stdout protocol.
The agent starts it once at startup and keeps it alive for the session.

### Protocol Specification

All messages are **newline-delimited JSON** (one JSON object per line).
The plugin reads from **stdin**, writes to **stdout**.
Nothing should be printed to stdout except protocol messages (use stderr for
logging).

#### Step 1 — Describe

After the agent starts the plugin, it sends a describe request:

```json
{"type":"describe"}
```

The plugin must respond with its tool definition:

```json
{
  "name": "my_tool",
  "description": "What this tool does. Be specific — the LLM reads this.",
  "parameters": {
    "type": "object",
    "properties": {
      "input": { "type": "string", "description": "The input value" },
      "count": { "type": "integer", "description": "How many results" }
    },
    "required": ["input"]
  }
}
```

#### Step 2 — Execute

For each tool call the agent sends:

```json
{"type":"call","call_id":"call_abc123","params":{"input":"hello","count":3}}
```

The plugin executes and responds:

```json
{
  "content": [{"type":"text","text":"Result: ..."}],
  "error": false
}
```

On error:

```json
{
  "content": [{"type":"text","text":"Error: something went wrong"}],
  "error": true
}
```

#### Key Rules

- The plugin process **must not exit** between calls
- **One JSON object per line** — no pretty-printing in responses
- **stdout** is protocol only — use **stderr** for debug logs
- The `content` array may contain multiple text blocks; the agent concatenates them
- `call_id` in the request can be ignored (it's for the agent's internal tracking)

### Registering Plugins

```yaml
tools:
  preset: coding        # or readonly, none, etc.
  plugins:
    - path: ./plugins/my_tool.py
      args: []
    - path: ./plugins/my_tool
      args: ["--verbose"]
```

Or in Go:

```go
tool, err := tools.LoadPlugin("./plugins/my_tool.py")
if err != nil {
    log.Fatal(err)
}
reg.Register(tool)
```

---

## Part 3 — Plugin Examples by Language

Full, runnable examples are in `examples/tools/`:

| Directory | Language | Tool | Description |
|-----------|----------|------|-------------|
| `examples/tools/go/` | Go | `bash_plugin` | Shell command executor |
| `examples/tools/python/` | Python 3 | `stats` | Descriptive statistics |
| `examples/tools/typescript/` | TypeScript/Deno | `json_query` | JSON dot-path extraction |
| `examples/tools/rust/` | Rust | `file_info` | File metadata and line counts |
| `examples/tools/bash/` | Bash | `sys_info` | System info (OS, disk, memory) |
| `examples/tools/ruby/` | Ruby | `template` | String template rendering |

### Running the Examples

Each directory has a `README.md`. Quick start:

```bash
# Python (no deps)
python3 examples/tools/python/tool.py

# TypeScript (requires Deno — https://deno.com)
deno run examples/tools/typescript/tool.ts

# Rust (requires cargo)
cd examples/tools/rust && cargo build --release
./target/release/file_info

# Bash (any POSIX shell)
bash examples/tools/bash/tool.sh

# Ruby (no gems)
ruby examples/tools/ruby/tool.rb
```

Wire them into your agent config:

```yaml
tools:
  preset: coding
  plugins:
    - path: python3
      args: ["./examples/tools/python/tool.py"]
    - path: deno
      args: ["run", "./examples/tools/typescript/tool.ts"]
    - path: ./examples/tools/rust/target/release/file_info
    - path: bash
      args: ["./examples/tools/bash/tool.sh"]
    - path: ruby
      args: ["./examples/tools/ruby/tool.rb"]
```

---

## Part 4 — Advanced Patterns

### Multiple Tools from One Plugin

Return an array of definitions from `describe`:

```json
{
  "tools": [
    { "name": "tool_a", "description": "...", "parameters": {...} },
    { "name": "tool_b", "description": "...", "parameters": {...} }
  ]
}
```

> **Note:** This requires a custom `LoadPlugin` variant — the default protocol
> handles one tool per process. File an issue if you need this and it will be
> prioritised.

### Plugin with Configuration

Pass config via command-line arguments (`args` in the YAML):

```yaml
plugins:
  - path: ./my_plugin
    args: ["--api-key", "${MY_API_KEY}", "--endpoint", "https://api.example.com"]
```

Or via environment variables (the plugin inherits the agent process's env).

### Timeout and Error Handling

The agent's `Config.ToolTimeout` wraps the entire `Execute` call including the
JSON round-trip to the plugin. If the plugin hangs, the context is cancelled
and the agent logs a warning — the plugin process is **not** killed
automatically, so implement a timeout inside the plugin if needed.

### Testing Your Plugin

You can test the protocol by hand using `echo` and your plugin:

```bash
# Test describe
echo '{"type":"describe"}' | python3 examples/tools/python/tool.py

# Test a call
printf '{"type":"describe"}\n{"type":"call","call_id":"c1","params":{"numbers":[1,2,3,4,5]}}\n' \
  | python3 examples/tools/python/tool.py
```

You should see the definition followed by the result, one JSON line each.

---

## Part 5 — Compiled-In vs Plugin: Choosing

| Concern | Compiled-in | Plugin |
|---------|-------------|--------|
| Performance | Best (in-process) | ~5ms per call overhead |
| Language | Go only | Any |
| Distribution | Single binary | Binary + plugin files |
| Dependencies | Go modules | Language runtime + packages |
| Hot reload | No (needs rebuild) | Restart plugin process |
| Streaming | Yes (UpdateFn) | No (result only) |
| Image output | Yes | Yes |
| Debugging | Go debugger | Language's native debugger |
| Panic isolation | Agent catches panics | Plugin crash → error result |

**Use compiled-in when:** performance matters, you're already writing Go, or
you need streaming progress updates.

**Use plugins when:** you want Python/TypeScript/Rust libraries, you're
wrapping an existing CLI tool, or you want to update the tool without
recompiling the agent.
