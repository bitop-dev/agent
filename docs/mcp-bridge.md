# MCP Runtime Bridge

## What MCP is

Model Context Protocol (MCP) is an open standard for connecting AI models to external tools and data sources.

An MCP server exposes a set of callable tools over a simple JSON-RPC-like protocol on stdin/stdout or HTTP.

## What the MCP bridge does

The MCP bridge runtime type (`runtime.type: mcp`) lets a plugin connect to an external MCP server.

When a profile enables an `mcp`-runtime plugin tool, the framework:

1. starts the MCP server process (if command-based) or connects to an endpoint
2. sends a `tools/list` request to discover available tools
3. registers discovered tools into the tool registry
4. when a tool is called, forwards the call to the MCP server
5. returns the result back to the runtime

## Plugin manifest for MCP

```yaml
apiVersion: agent/v1
kind: Plugin
metadata:
  name: my-mcp-server
  version: 0.1.0
  description: Example MCP bridge plugin

spec:
  category: bridge
  runtime:
    type: mcp
    command:
      - my-mcp-server
      - --config
      - /path/to/config.json
    # or use endpoint for HTTP-based MCP:
    # endpoint: http://localhost:3000/mcp

  contributes:
    tools: []  # populated from MCP server discovery

  configSchema:
    type: object
    properties:
      endpoint:
        type: string
  requires:
    framework: ">=0.1.0"
    plugins: []
```

## MCP transport modes

### stdio

The most common MCP mode.

The framework spawns the MCP server as a subprocess and communicates over stdin/stdout.

```yaml
runtime:
  type: mcp
  command:
    - npx
    - -y
    - "@my-org/mcp-server"
```

### HTTP/SSE

Some MCP servers expose an HTTP endpoint.

```yaml
runtime:
  type: mcp
  endpoint: http://localhost:3000/mcp
```

## MCP protocol overview

The framework speaks the MCP JSON-RPC protocol.

Key operations:

- `initialize` - handshake with protocol version
- `tools/list` - discover available tools
- `tools/call` - invoke a tool by name

Request format (stdio):

```json
{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}
```

Response format:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "tools": [
      {
        "name": "my_tool",
        "description": "Does something useful",
        "inputSchema": {
          "type": "object",
          "properties": {
            "input": {"type": "string"}
          }
        }
      }
    ]
  }
}
```

Tool call:

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "tools/call",
  "params": {
    "name": "my_tool",
    "arguments": {"input": "hello"}
  }
}
```

Tool result:

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "result": {
    "content": [
      {"type": "text", "text": "Tool output here"}
    ]
  }
}
```

## v0.1 scope for MCP bridge

Phase 1 (current):

- plugin manifest parsing with `runtime.type: mcp`
- stdio transport: spawn command, communicate via stdin/stdout
- `initialize` handshake
- `tools/list` for auto-discovery
- `tools/call` for tool execution
- tool registration into the tool registry at bootstrap

Phase 2 (later):

- HTTP/SSE transport
- resource listing and reading
- prompt listing and reading
- sampling support
- reconnection and health checks

## Security model

MCP servers run as separate processes and are isolated from the core agent runtime.

Policy and approval still apply to tool calls before they are forwarded to the MCP server.

## Example real-world usage

Installing a filesystem MCP server:

```bash
go run ./cmd/agent plugins install ./_testing/plugins/mcp-filesystem --link
go run ./cmd/agent plugins config set mcp-filesystem command '["npx", "-y", "@modelcontextprotocol/server-filesystem", "/tmp"]'
go run ./cmd/agent plugins enable mcp-filesystem
```

Running with an MCP-enabled profile:

```bash
go run ./cmd/agent run --profile ./_testing/plugins/mcp-filesystem/profiles/researcher.yaml "Summarize all markdown files in /tmp"
```
