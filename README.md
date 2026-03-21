# agent

Go framework and CLI for building and running AI agents powered by large language models.

## What it is

Define agents via profile YAML — tools, plugins, policies — then run from the CLI,
as an HTTP worker, or as an MCP server. Profiles are model-optional: the model
is resolved from config, not hardcoded.

```bash
agent run --profile researcher "Summarize today's AI news"
agent run --profile researcher --model gpt-4o-mini "Quick summary"
agent chat --profile code-reviewer
agent serve --addr :9898                    # HTTP worker
agent serve --profile researcher            # MCP server
```

## Features

- **Model resolution** — config default + per-profile overrides + CLI `--model` flag
- **Responses API** — OpenAI Responses API with multi-turn tool calls and token extraction
- **Plugin system** — install, upgrade, publish, on-demand install, env-based config
- **9 plugins** — ddg-research, github, http-request, csv-tool, docker, kubectl, send-email, slack, spawn-sub-agent
- **6 profiles** — researcher, orchestrator, security-researcher, code-reviewer, devops, writer
- **Profile inheritance** — `extends: researcher` merges tools and instructions
- **Agent memory** — `agent/remember` + `agent/recall` across tasks
- **Model fallback** — `provider.fallback: [model1, model2]`
- **Sub-agent orchestration** — discover, spawn, parallel, pipeline
- **MCP server** — expose agents to opencode/Claude Desktop
- **Session compaction** — structured summarization for long conversations

## Configuration

```yaml
# ~/.agent/config.yaml
providers:
  openai:
    baseURL: https://api.openai.com/v1
    apiKey: sk-...
    model: gpt-4o              # default for all profiles
    apiMode: responses
    models:                    # per-profile overrides
      code-reviewer: claude-sonnet-4-5
      writer: gpt-4o-mini
```

## Related repos

| Repo | Purpose |
|---|---|
| [agent-gateway](https://github.com/bitop-dev/agent-gateway) | Task routing, auth, dashboard, costs |
| [agent-registry](https://github.com/bitop-dev/agent-registry) | Marketplace + package server |
| [agent-plugins](https://github.com/bitop-dev/agent-plugins) | 9 plugin packages |
| [agent-profiles](https://github.com/bitop-dev/agent-profiles) | 6 agent profiles |
| [agent-docs](https://github.com/bitop-dev/agent-docs) | Documentation |
