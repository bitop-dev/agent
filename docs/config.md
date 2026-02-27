# Configuration Reference

The agent is configured via a YAML file. All `${ENV_VAR}` references are
expanded from the environment before parsing, so secrets can live safely in
environment variables or a `.env` file.

Default config path is `agent.yaml` in the current directory. Override with
`-config <path>`.

---

## Full Schema

```yaml
# ── Provider ────────────────────────────────────────────────────────────────

# Required. Selects the LLM provider.
# Values: "anthropic" | "openai-responses" | "openai-completions" |
#         "google" | "azure" | "bedrock" | "proxy"
provider: anthropic

# Required. Model identifier string.
model: claude-sonnet-4-5

# For OpenAI-compatible providers (openai-completions, azure, proxy, etc.)
# Overrides the provider's default API endpoint.
base_url: https://api.openai.com/v1

# API key. Can be a literal, or ${ENV_VAR} to read from environment.
api_key: ${ANTHROPIC_API_KEY}

# ── Generation ──────────────────────────────────────────────────────────────

# System/instructions message sent with every call.
system_prompt: |
  You are a helpful coding assistant. Be concise and accurate.

# Maximum output tokens (0 = provider default).
max_tokens: 4096

# Maximum LLM turns per prompt (0 = unlimited).
# Each turn = one assistant response + all its tool calls.
# Prevents runaway loops where the model keeps calling tools indefinitely.
# Recommended: 50 for general use, 200 for long agentic/research tasks.
max_turns: 50

# Sampling temperature (omit for provider default).
temperature: 0.7

# Extended reasoning level. Values: "off" | "minimal" | "low" | "medium" |
# "high" | "xhigh". Requires a model that supports thinking/reasoning.
thinking_level: medium

# Prompt caching aggressiveness for providers that support it (Anthropic).
# Values: "none" | "short" | "long". Default: "short" (caching enabled).
cache_retention: short

# ── Provider-specific ───────────────────────────────────────────────────────

# Azure OpenAI API version (e.g. "2024-12-01-preview").
api_version: "2024-12-01-preview"

# Amazon Bedrock region (defaults to AWS_DEFAULT_REGION or us-east-1).
region: us-east-1

# AWS profile name for Bedrock authentication.
profile: my-aws-profile

# ── Context window ──────────────────────────────────────────────────────────

# Context window in tokens. Used for overflow detection and compaction.
# Auto-filled from the model registry if the model is known; only set this
# manually for custom/unknown models.
context_window: 200000

# ── Compaction ──────────────────────────────────────────────────────────────

compaction:
  # Enable automatic context compaction when context exceeds the threshold.
  enabled: true

  # Override the context window used for compaction decisions.
  # Defaults to context_window (above) or the model registry value.
  context_window: 200000

  # Tokens to reserve for the LLM response. Compaction triggers when:
  #   context_tokens > context_window - reserve_tokens
  reserve_tokens: 16384

  # How many recent tokens to keep verbatim (not summarized).
  keep_recent_tokens: 20000

# ── Tools ───────────────────────────────────────────────────────────────────

tools:
  # Built-in tool preset.
  # Values:
  #   "coding"   — read, bash, edit, write            (default)
  #   "readonly" — read, grep, find, ls
  #   "web"      — web_search, web_fetch
  #   "all"      — all built-in tools
  #   "none"     — no built-in tools
  preset: coding

  # Working directory for file tools (default: process working directory).
  work_dir: /path/to/project

  # External subprocess plugin tools.
  plugins:
    - path: ./plugins/my-tool        # path to executable
      args: ["--mode", "production"] # optional extra arguments
    - path: ./plugins/another-tool
```

---

## Field Reference

### `provider`

| Value | Description |
|-------|-------------|
| `anthropic` | Anthropic Messages API (Claude) |
| `openai-responses` | OpenAI Responses API (GPT-4o, o1, etc.) |
| `openai-completions` | OpenAI Chat Completions (legacy; also for OpenRouter, Ollama, etc.) |
| `google` | Google Generative AI (Gemini) |
| `azure` | Azure OpenAI Service |
| `bedrock` | Amazon Bedrock |
| `proxy` | Remote agent proxy server |

### `model`

Any model ID accepted by the provider. Common values:

| Provider | Model IDs |
|----------|-----------|
| anthropic | `claude-opus-4-5`, `claude-sonnet-4-5`, `claude-haiku-4-5` |
| openai-responses | `gpt-4o`, `gpt-4o-mini`, `o1`, `o3-mini` |
| google | `gemini-2.0-flash`, `gemini-2.5-pro`, `gemini-1.5-pro` |
| bedrock | `us.anthropic.claude-sonnet-4-20250514-v1:0` |

See [providers.md](providers.md) and [models.md](models.md) for the full list.

### `thinking_level`

Controls extended reasoning. Only meaningful for models that support it:

- Anthropic: Claude 3.5+ (budget-based)
- OpenAI: o1, o3, o3-mini (effort-based)
- Google: Gemini 2.5+ (budget-based)

| Value | Description |
|-------|-------------|
| `off` | Disable thinking entirely |
| `minimal` | Shortest possible trace (~1k tokens) |
| `low` | Light reasoning (~2k tokens) |
| `medium` | Balanced (~8k tokens) |
| `high` | Thorough reasoning (~16k tokens) |
| `xhigh` | Maximum (OpenAI only) |

### `cache_retention`

Controls Anthropic prompt cache headers:

| Value | Description |
|-------|-------------|
| `none` | No cache headers (caching disabled) |
| `short` | Cache system prompt + last user message (default) |
| `long` | Same as short (reserved for future TTL controls) |

### `max_turns`

Caps the number of LLM calls the agent loop will make in response to a single prompt. `0` means unlimited.

Each **turn** is one assistant response plus all the tool calls it triggers. A complex agentic task (e.g. a research agent fetching 30 pages) can easily run 50+ turns. Without a cap, a misbehaving model or a broken tool feedback loop can spin forever.

| Value | Behaviour |
|-------|-----------|
| `0` | Unlimited (default) |
| `50` | Recommended for general interactive use |
| `200` | Suitable for long research or agentic tasks |

When the limit is hit:
- The loop stops cleanly (no error returned)
- An `EventTurnLimitReached` event is broadcast to all subscribers
- The CLI prints `[agent] turn limit reached — stopping loop`

---

### `tools.preset`

| Preset | Tools Included |
|--------|----------------|
| `coding` | `read`, `bash`, `edit`, `write` |
| `readonly` | `read`, `grep`, `find`, `ls` |
| `web` | `web_search`, `web_fetch` |
| `all` | All of the above |
| `none` | No built-in tools (only plugins) |

---

## Environment Variable Expansion

Every string value in the YAML is subject to `${VAR}` expansion before
parsing. This applies to nested fields too:

```yaml
api_key: ${MY_API_KEY}
base_url: https://${COMPANY}.openai.azure.com
system_prompt: |
  You work in the ${ENVIRONMENT} environment.
```

There is no fallback syntax — if the variable is unset the literal
`${VAR}` string is used.

---

## Multiple Configs

You can maintain multiple config files and select one at runtime:

```bash
./agent -config configs/openai.yaml   -prompt "..."
./agent -config configs/anthropic.yaml -prompt "..."
./agent -config configs/local.yaml    -prompt "..."  # Ollama
```

---

## Minimal Examples

### Code assistant (Anthropic)

```yaml
provider: anthropic
model: claude-sonnet-4-5
api_key: ${ANTHROPIC_API_KEY}
max_tokens: 8192
thinking_level: low
cache_retention: short

system_prompt: |
  You are an expert Go programmer.
  Always write idiomatic, well-tested code.

tools:
  preset: coding
  work_dir: .

compaction:
  enabled: true
  reserve_tokens: 16384
  keep_recent_tokens: 20000
```

### Research agent (web tools only)

```yaml
provider: openai-responses
model: gpt-4o
api_key: ${OPENAI_API_KEY}
max_tokens: 4096

system_prompt: |
  You are a research assistant. Search the web to answer questions accurately.
  Always cite your sources.

tools:
  preset: web
```

### Read-only code reviewer

```yaml
provider: google
model: gemini-2.0-flash
api_key: ${GEMINI_API_KEY}
max_tokens: 2048

system_prompt: |
  You review code for correctness, style, and security. Never modify files.

tools:
  preset: readonly
  work_dir: /path/to/repo
```
