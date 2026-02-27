# Providers

The agent supports multiple LLM providers. Select one with `provider:` in your
config and supply the required credentials.

---

## Anthropic

**Provider name:** `anthropic`  
**API:** [Anthropic Messages API](https://docs.anthropic.com/en/api/messages)  
**Features:** Streaming, tool use, extended thinking, prompt caching

```yaml
provider: anthropic
model: claude-sonnet-4-5      # or claude-opus-4-5, claude-haiku-4-5
api_key: ${ANTHROPIC_API_KEY}
max_tokens: 8192

# Extended thinking (Claude 3.5+)
thinking_level: medium        # off | minimal | low | medium | high

# Prompt caching reduces costs on long conversations
cache_retention: short        # none | short | long
```

### Anthropic Models

| Model | Context Window | Vision | Thinking |
|-------|---------------|--------|---------|
| `claude-opus-4-5` | 200k | ✓ | ✓ |
| `claude-sonnet-4-5` | 200k | ✓ | ✓ |
| `claude-haiku-4-5` | 200k | ✓ | ✗ |
| `claude-3-5-sonnet-20241022` | 200k | ✓ | ✓ |
| `claude-3-5-haiku-20241022` | 200k | ✓ | ✗ |
| `claude-3-7-sonnet-20250219` | 200k | ✓ | ✓ |

### Prompt Caching

When `cache_retention` is `short` or `long`, the agent adds
`"cache_control": {"type": "ephemeral"}` headers to:
1. The system prompt
2. The last user message (most recent turn)

This can significantly reduce costs on long conversations where the system
prompt and early context are stable.

---

## OpenAI — Responses API

**Provider name:** `openai-responses`  
**API:** [OpenAI Responses API](https://platform.openai.com/docs/api-reference/responses)  
**Features:** Streaming, tool use, reasoning models (o1, o3)

```yaml
provider: openai-responses
model: gpt-4o
api_key: ${OPENAI_API_KEY}
max_tokens: 4096

# For reasoning models (o1, o3, o3-mini)
thinking_level: medium        # maps to effort: "low" | "medium" | "high"
```

### OpenAI Models

| Model | Context Window | Notes |
|-------|---------------|-------|
| `gpt-4o` | 128k | Latest GPT-4o |
| `gpt-4o-mini` | 128k | Fast, cheap |
| `o1` | 200k | Reasoning model |
| `o3` | 200k | Reasoning model |
| `o3-mini` | 200k | Reasoning, fast |
| `o4-mini` | 200k | Reasoning, fast |

---

## OpenAI — Chat Completions

**Provider name:** `openai-completions`  
**API:** OpenAI Chat Completions (legacy; widely compatible)  
**Use for:** OpenRouter, Ollama, Groq, xAI, Mistral, custom proxies, etc.

```yaml
provider: openai-completions
model: gpt-4o
api_key: ${OPENAI_API_KEY}
```

Because this provider is broadly compatible, it works with any
OpenAI-compatible endpoint — just set `base_url`:

```yaml
# OpenRouter (access hundreds of models)
provider: openai-completions
base_url: https://openrouter.ai/api/v1
api_key: ${OPENROUTER_API_KEY}
model: anthropic/claude-opus-4-5

# Ollama (local models)
provider: openai-completions
base_url: http://localhost:11434/v1
api_key: ollama          # any non-empty string
model: llama3.2

# Groq (fast inference)
provider: openai-completions
base_url: https://api.groq.com/openai/v1
api_key: ${GROQ_API_KEY}
model: llama-3.3-70b-versatile

# xAI (Grok)
provider: openai-completions
base_url: https://api.x.ai/v1
api_key: ${XAI_API_KEY}
model: grok-3

# Mistral
provider: openai-completions
base_url: https://api.mistral.ai/v1
api_key: ${MISTRAL_API_KEY}
model: mistral-large-latest

# Cerebras
provider: openai-completions
base_url: https://api.cerebras.ai/v1
api_key: ${CEREBRAS_API_KEY}
model: llama3.1-70b

# Vercel AI Gateway
provider: openai-completions
base_url: https://ai-gateway.vercel.sh
api_key: ${AI_GATEWAY_API_KEY}
model: openai/gpt-4o

# University / enterprise proxy
provider: openai-completions
base_url: https://api.corp.example.com/v1
api_key: ${CORP_API_KEY}
model: gpt-oss-120b
```

---

## Google Gemini

**Provider name:** `google`  
**API:** Google Generative AI REST/SSE  
**Features:** Streaming, tool use, extended thinking (Gemini 2.5+)

```yaml
provider: google
model: gemini-2.0-flash
api_key: ${GEMINI_API_KEY}
max_tokens: 2048

# Extended thinking (Gemini 2.5+)
thinking_level: medium
```

### Google Models

| Model | Context Window | Vision | Thinking |
|-------|---------------|--------|---------|
| `gemini-2.5-pro` | 1M | ✓ | ✓ |
| `gemini-2.5-flash` | 1M | ✓ | ✓ |
| `gemini-2.0-flash` | 1M | ✓ | ✗ |
| `gemini-1.5-pro` | 2M | ✓ | ✗ |
| `gemini-1.5-flash` | 1M | ✓ | ✗ |

---

## Azure OpenAI

**Provider name:** `azure`  
**API:** Azure OpenAI Service (Chat Completions)

```yaml
provider: azure
model: gpt-4o                     # deployment name if different, see below
api_key: ${AZURE_OPENAI_API_KEY}
base_url: https://my-resource.openai.azure.com
api_version: "2024-12-01-preview"
max_tokens: 4096
```

You can also supply the resource name instead of the full URL via the
`AZURE_OPENAI_RESOURCE_NAME` environment variable, or set
`AZURE_OPENAI_BASE_URL` directly.

**Deployment names:** If your Azure deployment name differs from the model ID,
set `AZURE_OPENAI_DEPLOYMENT_NAME_MAP`:

```bash
export AZURE_OPENAI_DEPLOYMENT_NAME_MAP="gpt-4o=my-gpt4o,gpt-4o-mini=my-mini"
```

---

## Amazon Bedrock

**Provider name:** `bedrock`  
**API:** AWS Bedrock ConverseStream (AWS SDK v2)  
**Auth:** IAM credentials or AWS profile

```yaml
provider: bedrock
model: us.anthropic.claude-sonnet-4-20250514-v1:0
region: us-east-1
# profile: my-aws-profile   # optional AWS profile

# No api_key field — uses AWS credential chain:
# 1. AWS_ACCESS_KEY_ID + AWS_SECRET_ACCESS_KEY env vars
# 2. ~/.aws/credentials (profile or default)
# 3. ECS task role / EC2 instance profile / IRSA
```

### Environment Variables

```bash
# Option 1: IAM keys
export AWS_ACCESS_KEY_ID=AKIA...
export AWS_SECRET_ACCESS_KEY=...
export AWS_REGION=us-east-1

# Option 2: Profile
export AWS_PROFILE=my-profile

# Option 3: Bearer token
export AWS_BEARER_TOKEN_BEDROCK=...

# Optional: Bedrock proxy
export AWS_ENDPOINT_URL_BEDROCK_RUNTIME=https://my.corp.proxy/bedrock
export AWS_BEDROCK_SKIP_AUTH=1   # if proxy doesn't require AWS auth
```

### Bedrock Models

Use the full cross-region inference profile ID:

```
us.anthropic.claude-opus-4-20250514-v1:0
us.anthropic.claude-sonnet-4-20250514-v1:0
us.anthropic.claude-3-7-sonnet-20250219-v1:0
us.meta.llama3-3-70b-instruct-v1:0
```

---

## Proxy

**Provider name:** `proxy`  
**Use for:** Connecting to another agent instance serving as an API proxy.

```yaml
provider: proxy
base_url: http://localhost:8080
api_key: ${PROXY_TOKEN}      # optional bearer token
model: claude-sonnet-4-5     # passed through to the upstream
```

The proxy provider:
- POSTs to `{base_url}/stream` with a JSON wire request
- Reads SSE events in the agent wire format
- Forwards all streaming events to your local agent loop

See [proxy.md](proxy.md) for serving a proxy from your own server.

---

## Resolution Summary

When building a provider from config, the agent resolves credentials in this
order:

1. `api_key:` field in config (literal or `${ENV_VAR}`)
2. Standard provider environment variable (e.g. `ANTHROPIC_API_KEY`)
3. AWS credential chain (Bedrock only)

The `base_url` field overrides the provider's default endpoint for all
providers that accept it.
