# Build a Web Research Plugin

This walkthrough shows the intended way to build a plugin like `web/search` and `web/fetch`.

## Goal

We want an agent to be able to:

- search the web
- fetch a URL

without adding that logic to the core agent binary.

## Architecture

You will create two things:

1. a plugin bundle
2. a runtime service or binary

The bundle tells the framework what exists.
The runtime does the actual work.

## Step 1: Create the plugin bundle

Directory layout:

```text
web-research/
  plugin.yaml
  tools/
    search.yaml
    fetch.yaml
  prompts/
    research-style.md
  profiles/
    researcher.yaml
  policies/
    default.yaml
```

Minimal `plugin.yaml`:

```yaml
apiVersion: agent/v1
kind: Plugin
metadata:
  name: web-research
  version: 0.1.0
  description: Web search and fetch plugin

spec:
  category: integration
  runtime:
    type: http

  contributes:
    tools:
      - id: web/search
        path: tools/search.yaml
      - id: web/fetch
        path: tools/fetch.yaml
    prompts:
      - id: web/research-style
        path: prompts/research-style.md
    profileTemplates:
      - id: web/researcher
        path: profiles/researcher.yaml
    policies:
      - id: web/default
        path: policies/default.yaml

  configSchema:
    type: object
    properties:
      provider:
        type: string
      baseURL:
        type: string
      apiKey:
        type: string
        secret: true
    required:
      - provider
      - baseURL
```

## Step 2: Add tool descriptors

`tools/search.yaml`

```yaml
id: web/search
description: Search the web using a configured provider
inputSchema:
  type: object
  properties:
    query:
      type: string
    topK:
      type: integer
  required:
    - query
execution:
  mode: http
  operation: web-search
```

`tools/fetch.yaml`

```yaml
id: web/fetch
description: Fetch a URL for analysis
inputSchema:
  type: object
  properties:
    url:
      type: string
  required:
    - url
execution:
  mode: http
  operation: web-fetch
```

## Step 3: Build the runtime service

For `http` plugins, write a separate binary or service.

For example, a Go service exposing:

- `POST /web-search`
- `POST /web-fetch`

The framework sends a request like:

```json
{
  "plugin": "web-research",
  "tool": "web/search",
  "operation": "web-search",
  "arguments": {
    "query": "golang plugin architecture"
  },
  "config": {
    "provider": "internal",
    "baseURL": "http://127.0.0.1:8092",
    "apiKey": "local-test-key"
  }
}
```

And your service returns:

```json
{
  "output": "search results for \"golang plugin architecture\": result one, result two",
  "data": {
    "results": [
      {"title": "Result One", "url": "https://example.com/one"},
      {"title": "Result Two", "url": "https://example.com/two"}
    ]
  }
}
```

That is the contract.

## Step 4: Install the bundle

```bash
./bin/agent plugins install ./web-research --link
```

## Step 5: Configure it

```bash
./bin/agent plugins config set web-research provider internal
./bin/agent plugins config set web-research baseURL http://127.0.0.1:8092
./bin/agent plugins config set web-research apiKey local-test-key
./bin/agent plugins validate-config web-research
./bin/agent plugins enable web-research
```

## Step 6: Use a profile that enables the tools

Example profile tool section:

```yaml
tools:
  enabled:
    - web/search
    - web/fetch
```

Then run:

```bash
./bin/agent run --profile ./my-research-profile.yaml "search golang plugin architecture"
```

## Key takeaway

If you want a `web-search` / `web-fetch` plugin, then yes: you are usually writing a separate runtime binary or service that the agent calls.

The agent does not import your plugin code directly.
It loads the plugin bundle, validates config, enables the tools, and then dispatches calls to your runtime.

That separation is the intended design.
