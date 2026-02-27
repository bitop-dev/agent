# Proxy Provider

The proxy system lets you run an agent server that proxies requests to any
upstream LLM provider, and connect to it from another agent instance (or any
HTTP client that speaks the wire format).

**Use cases:**
- Centralise API keys in a trusted server; clients hold no keys
- Add rate limiting, logging, or cost tracking at the proxy layer
- Use a powerful model on a remote GPU server; connect from a thin client

---

## Wire Protocol

The proxy uses HTTP POST with Server-Sent Events (SSE) for streaming.

### Request

`POST /stream`  
`Content-Type: application/json`  
`Authorization: Bearer <token>` (if auth is enabled)

```json
{
  "model": "claude-sonnet-4-5",
  "context": {
    "system_prompt": "You are a helpful assistant.",
    "messages": [
      {
        "role": "user",
        "content": [{"type": "text", "text": "Hello!"}]
      }
    ]
  },
  "options": {
    "max_tokens": 1024,
    "temperature": 0.7,
    "thinking_level": "medium",
    "cache_retention": "short"
  }
}
```

### Response (SSE stream)

Each SSE event has `data: <json>`:

```
data: {"type":"start","partial":{...}}

data: {"type":"text_delta","delta":"Hello","partial":{...}}

data: {"type":"text_delta","delta":", world!","partial":{...}}

data: {"type":"tool_call_start","delta":"bash","partial":{...}}

data: {"type":"tool_call_delta","delta":"{\"command\":\"ls\"}","partial":{...}}

data: {"type":"tool_call_end","partial":{...}}

data: {"type":"done","partial":{...}}
```

The `partial` field is the full `AssistantMessage` snapshot at that point.

---

## Serving a Proxy

```go
package main

import (
    "fmt"
    "net/http"
    "os"

    "github.com/bitop-dev/agent/pkg/ai/providers/anthropic"
    "github.com/bitop-dev/agent/pkg/ai/providers/proxy"
)

func main() {
    // Any provider as the upstream
    upstream := anthropic.New(os.Getenv("ANTHROPIC_API_KEY"))

    // Optional bearer token for auth (empty = no auth)
    token := os.Getenv("PROXY_TOKEN")

    handler := proxy.NewHandler(upstream, token)

    http.Handle("/stream", handler)
    fmt.Println("Proxy listening on :8080")
    http.ListenAndServe(":8080", nil)
}
```

See `examples/proxy-server/` for a complete runnable example with TLS and
multiple upstream providers.

---

## Connecting as a Client

### Config file

```yaml
provider: proxy
base_url: http://localhost:8080
api_key: ${PROXY_TOKEN}    # optional bearer token
model: claude-sonnet-4-5   # forwarded to upstream
max_tokens: 4096

tools:
  preset: coding
```

### Go code

```go
import "github.com/bitop-dev/agent/pkg/ai/providers/proxy"

client := proxy.New("http://localhost:8080", os.Getenv("PROXY_TOKEN"))
// use client as ai.Provider
```

---

## Authentication

When a non-empty token is passed to `NewHandler`, the server validates every
request's `Authorization: Bearer <token>` header. Requests without the token
receive `401 Unauthorized`.

The token is a simple shared secret. For production, use TLS and a strong
random token.

---

## CORS and Multiple Upstreams

The `Handler` is a standard `http.Handler`. Wrap it with your own middleware:

```go
upstream := anthropic.New(apiKey)
handler  := proxy.NewHandler(upstream, token)

// Add CORS
mux := http.NewServeMux()
mux.Handle("/stream", corsMiddleware(handler))

// Multiple upstreams
mux.Handle("/stream/anthropic", proxy.NewHandler(anthropic.New(key1), token))
mux.Handle("/stream/openai",    proxy.NewHandler(openai.New(key2), token))
```
