# json_query — TypeScript Plugin Tool

Extracts values from JSON using dot-notation paths and applies simple
transformations. No npm packages required.

## Requirements

**Deno** (recommended): https://deno.com — single binary, no `npm install`

```bash
curl -fsSL https://deno.land/install.sh | sh
```

**Or Node.js + tsx:**

```bash
npm install -g tsx
```

## Running as a Plugin

```yaml
# agent.yaml — Deno (recommended)
tools:
  preset: coding
  plugins:
    - path: deno
      args: ["run", "--allow-read", "./examples/tools/typescript/tool.ts"]

# agent.yaml — Node.js ESM (no compilation, no deps)
tools:
  preset: coding
  plugins:
    - path: node
      args: ["./examples/tools/typescript/tool.mjs"]

# agent.yaml — Node.js with tsx (TypeScript source)
tools:
  preset: coding
  plugins:
    - path: npx
      args: ["tsx", "./examples/tools/typescript/tool.ts"]
```

## Testing Manually

```bash
# With Deno
echo '{"type":"describe"}' | deno run tool.ts

# With Node.js (no build step)
echo '{"type":"describe"}' | node tool.mjs

# Extract a nested value
echo '{"type":"call","call_id":"c1","params":{"json":"{\"user\":{\"name\":\"Alice\"}}","path":"user.name"}}' \
  | node tool.mjs

# Array indexing
echo '{"type":"call","call_id":"c2","params":{"json":"[10,20,30]","path":".[1]"}}' \
  | node tool.mjs

# List all keys
echo '{"type":"call","call_id":"c3","params":{"json":"{\"a\":1,\"b\":2,\"c\":3}","path":".","transform":"keys"}}' \
  | node tool.mjs
```

## Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `json` | string | ✓ | Valid JSON string to query |
| `path` | string | ✓ | Dot-notation path: `user.name`, `items[0].id`, `.` for root, `*` for all keys |
| `transform` | string | | `keys`, `values`, `length`, `type`, `pretty`, `compact` |

## Examples

| Input | Path | Transform | Result |
|-------|------|-----------|--------|
| `{"a": {"b": 42}}` | `a.b` | — | `42` |
| `{"items": [1,2,3]}` | `items[1]` | — | `2` |
| `{"x": 1, "y": 2}` | `.` | `keys` | `x\ny` |
| `[1,2,3,4,5]` | `.` | `length` | `5` |
| `{"nested": {...}}` | `nested` | `pretty` | Pretty-printed JSON |
