# Build an MCP Plugin

This walkthrough shows how to set up MCP plugins for the three transport styles the framework now supports:

- stdio MCP
- remote HTTP MCP
- remote SSE MCP

The plugin shape is almost the same in all three cases. What changes is how the framework reaches the MCP server.

## Goal

We want the agent to use tools exposed by an MCP server without re-implementing those tools inside the core agent.

Examples:

- filesystem tools
- documentation lookup tools
- database tools
- browser or search tools already available through MCP

## Mental model

For `runtime.type: mcp`, the plugin bundle usually does **not** declare static tool descriptors.

Instead, the flow is:

1. install the plugin bundle
2. enable the plugin
3. the framework connects to the MCP server
4. it calls `initialize`
5. it calls `tools/list`
6. it registers the discovered tools into the tool registry
7. when the agent calls one of those tools, the framework forwards it with `tools/call`

That means the MCP server remains the source of truth for tool names, descriptions, and input schemas.

## Shared plugin shape

This is the minimal plugin structure for an MCP bridge:

```text
my-mcp-plugin/
  plugin.yaml
  profiles/
    researcher.yaml
  prompts/
    system.md
```

Minimal shared `plugin.yaml`:

```yaml
apiVersion: agent/v1
kind: Plugin
metadata:
  name: my-mcp-plugin
  version: 0.1.0
  description: MCP bridge plugin

spec:
  category: bridge
  runtime:
    type: mcp

  contributes:
    tools: []
    prompts: []
    profileTemplates:
      - id: my-mcp/researcher
        path: profiles/researcher.yaml
    policies: []

  configSchema:
    type: object
    properties:
      command:
        type: array
      endpoint:
        type: string
      headers:
        type: object
      env:
        type: object
    required: []

  permissions:
    network:
      outbound:
        - https
    sensitiveActions: []

  requires:
    framework: ">=0.1.0"
    plugins: []
```

Notes:

- `contributes.tools` is empty because MCP tools are discovered dynamically
- `runtime.command` is used for stdio MCP
- `runtime.endpoint` is used for remote HTTP or SSE MCP
- `runtime.headers` can provide default request headers for remote MCP
- `runtime.env` can provide default environment variables for stdio MCP
- plugin config can override either `command` or `endpoint`
- plugin config can also override `headers` and `env`

That gives you two layers:

- plugin spec for shipped defaults
- plugin config for machine-specific overrides

Example:

```yaml
spec:
  runtime:
    type: mcp
    endpoint: https://mcp.example.com/mcp
    headers:
      Authorization: Bearer default-token
```

Then a local install can override the header without editing the bundle:

```bash
go run ./cmd/agent plugins config set my-mcp-plugin headers '{"Authorization":"Bearer local-token"}'
```

## Example 1: stdio MCP server

This is the classic local MCP setup.

The framework starts the server process and talks over stdin/stdout.

### Example plugin

```yaml
apiVersion: agent/v1
kind: Plugin
metadata:
  name: mcp-filesystem
  version: 0.1.0
  description: MCP bridge plugin for a local filesystem server

spec:
  category: bridge
  runtime:
    type: mcp
    command:
      - npx
      - -y
      - "@modelcontextprotocol/server-filesystem"
      - /tmp
    env:
      LOG_LEVEL: info

  contributes:
    tools: []
    prompts: []
    profileTemplates:
      - id: mcp/filesystem-researcher
        path: profiles/researcher.yaml
    policies: []

  configSchema:
    type: object
    properties:
      command:
        type: array
      env:
        type: object
    required: []

  permissions:
    network:
      outbound: []
    sensitiveActions:
      - write_file
      - create_directory
      - move_file

  requires:
    framework: ">=0.1.0"
    plugins: []
```

### Install and enable

```bash
go run ./cmd/agent plugins install ../agent-plugins/mcp-filesystem --link
go run ./cmd/agent plugins enable mcp-filesystem
```

### Override the command if needed

```bash
go run ./cmd/agent plugins config set mcp-filesystem command '["npx","-y","@modelcontextprotocol/server-filesystem","/private/tmp"]'
go run ./cmd/agent plugins validate-config mcp-filesystem
```

### Example profile

```yaml
apiVersion: agent/v1
kind: Profile
metadata:
  name: mcp-filesystem-researcher
  version: 0.1.0
spec:
  instructions:
    system:
      - ./prompts/system.md
  provider:
    default: openai
    model: nemotron-3-super-120b-a12b
  tools:
    enabled:
      - read_file
      - list_directory
      - search_files
  approval:
    mode: on-request
    requireFor:
      - write_file
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
go run ./cmd/agent run --profile ../agent-plugins/mcp-filesystem/profiles/researcher.yaml "List the entries in /tmp"
```

Use stdio MCP when:

- the MCP server already runs as a local command
- you want the agent to own process startup
- the tool is local-first and command-driven

## Example 2: remote HTTP MCP server

This is for MCP servers that accept JSON-RPC requests over HTTP and return JSON responses.

### Example plugin

```yaml
apiVersion: agent/v1
kind: Plugin
metadata:
  name: remote-docs-mcp
  version: 0.1.0
  description: Remote MCP bridge for documentation tools

spec:
  category: bridge
  runtime:
    type: mcp
    endpoint: https://mcp.example.com/mcp
    headers:
      Authorization: Bearer default-token

  contributes:
    tools: []
    prompts: []
    profileTemplates: []
    policies: []

  configSchema:
    type: object
    properties:
      endpoint:
        type: string
      headers:
        type: object
    required: []

  permissions:
    network:
      outbound:
        - https
    sensitiveActions: []

  requires:
    framework: ">=0.1.0"
    plugins: []
```

### Configure auth headers

```bash
go run ./cmd/agent plugins install ./remote-docs-mcp --link
go run ./cmd/agent plugins config set remote-docs-mcp headers '{"Authorization":"Bearer your-token"}'
go run ./cmd/agent plugins enable remote-docs-mcp
```

You can also override the endpoint at config time:

```bash
go run ./cmd/agent plugins config set remote-docs-mcp endpoint https://mcp.example.com/mcp
```

### Expected transport behavior

The framework will:

- `POST` JSON-RPC requests to the endpoint
- send `Accept: application/json, text/event-stream`
- include configured headers
- decode normal JSON responses directly

Use remote HTTP MCP when:

- the MCP server is already hosted remotely
- authentication is header-based
- you do not want the agent to manage a local subprocess

## Example 3: remote SSE MCP server

Some remote MCP servers return `text/event-stream` instead of a plain JSON body.

The plugin manifest looks the same as remote HTTP MCP.
The difference is the server response format.

### Example plugin

```yaml
apiVersion: agent/v1
kind: Plugin
metadata:
  name: context7-mcp
  version: 0.1.0
  description: Remote Context7 MCP bridge

spec:
  category: bridge
  runtime:
    type: mcp
    endpoint: https://mcp.context7.com/mcp
    headers:
      CONTEXT7_API_KEY: your-default-key

  contributes:
    tools: []
    prompts: []
    profileTemplates: []
    policies: []

  configSchema:
    type: object
    properties:
      endpoint:
        type: string
      headers:
        type: object
    required: []

  permissions:
    network:
      outbound:
        - https
    sensitiveActions: []

  requires:
    framework: ">=0.1.0"
    plugins: []
```

### Configure headers

```bash
go run ./cmd/agent plugins install ./context7-mcp --link
go run ./cmd/agent plugins config set context7-mcp headers '{"CONTEXT7_API_KEY":"your-key"}'
go run ./cmd/agent plugins enable context7-mcp
```

### Example profile

```yaml
apiVersion: agent/v1
kind: Profile
metadata:
  name: context7-openai
  version: 0.1.0
spec:
  instructions:
    system:
      - ./prompts/system.md
  provider:
    default: openai
    model: nemotron-3-super-120b-a12b
  tools:
    enabled:
      - resolve-library-id
      - query-docs
  approval:
    mode: never
    requireFor: []
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
go run ./cmd/agent run --profile ./context7-openai.yaml "How do I install and get started with zod?"
```

The framework will:

- `POST` JSON-RPC requests to the endpoint
- accept SSE responses
- parse `data:` event payloads into JSON-RPC responses
- register the discovered MCP tools automatically

Use remote SSE MCP when:

- the provider already exposes an MCP endpoint over SSE
- tool discovery is remote and dynamic
- you want shared, hosted tool infrastructure

## Which transport should you choose?

Use this rule of thumb:

- choose `command` + stdio MCP when the server is local and process-based
- choose `endpoint` + remote HTTP when the server returns JSON over HTTP
- choose `endpoint` + remote SSE when the server streams JSON-RPC responses as `text/event-stream`

In all three cases, the plugin runtime type is still `mcp`.
Only the transport changes.

## Common config patterns

### Override endpoint from config

```bash
go run ./cmd/agent plugins config set my-mcp-plugin endpoint https://mcp.example.com/mcp
```

### Set headers

```bash
go run ./cmd/agent plugins config set my-mcp-plugin headers '{"Authorization":"Bearer token"}'
```

### Set default headers in the plugin spec

```yaml
runtime:
  type: mcp
  endpoint: https://mcp.example.com/mcp
  headers:
    Authorization: Bearer default-token
```

### Set stdio command

```bash
go run ./cmd/agent plugins config set my-mcp-plugin command '["npx","-y","@my-org/mcp-server"]'
```

### Set environment variables for stdio MCP

```bash
go run ./cmd/agent plugins config set my-mcp-plugin env '{"API_KEY":"secret"}'
```

### Set default env vars in the plugin spec

```yaml
runtime:
  type: mcp
  command:
    - my-mcp-server
  env:
    API_KEY: secret
    LOG_LEVEL: debug
```

## Troubleshooting

### No tools are registered

Check:

- the plugin is enabled
- the MCP server supports `tools/list`
- the profile enables the discovered tool IDs
- auth headers are correct for remote MCP

### The endpoint works manually but not in the agent

Check:

- `endpoint` is correct
- headers are present in plugin config
- the server accepts JSON-RPC `POST` requests
- the server returns either JSON or `text/event-stream`

### A stdio MCP server starts but tools fail

Check:

- the command is correct
- required env vars are configured
- the process can run outside your interactive shell
- path arguments match your platform (`/tmp` vs `/private/tmp` can matter on macOS)

## Key takeaway

The framework now supports one MCP runtime with three transport styles:

- local stdio MCP
- remote HTTP MCP
- remote SSE MCP

That lets you treat MCP as a single plugin category while still matching how real servers are deployed.
