# COMPACTION

## Repo

`agent` is the core framework and CLI.

It owns:

- runtime loop
- providers
- policy and approvals
- sessions
- built-in tools
- plugin loading and local install/config flows

It does not own:

- plugin package bundles (`../agent-plugins`)
- registry server (`../agent-registry`)

## Status

- `v0.1.0` is released
- Phase 0 is done
- active work is Phase 1 plugin/package workflow

## Current focus

We just added local plugin source support:

- `plugins sources add/list/remove`
- `plugins search [query]` for filesystem sources
- install by plugin name from configured filesystem sources
- registry sources can be configured but remote search/install is not done yet

## Current next step

Integrate remote registry search/install against `../agent-registry`.

Priority:

1. remote `plugins search`
2. remote `plugins install <name>`
3. store source/version metadata locally

## Important docs

- `WHERE-WE-ARE.md`
- `docs/plugins.md`
- `docs/architecture/plans/go-agent-framework-roadmap.md`
- `docs/architecture/plans/go-agent-plugin-package-model.md`
- `../agent-registry/docs/plugin-registry-contract.md`

## Quick commands

```bash
go test ./...
go run ./cmd/agent plugins sources list
go run ./cmd/agent plugins search
go run ./cmd/agent plugins install ../agent-plugins/send-email --link
```

Goal
Continue Phase 1 after v0.1.0 by evolving the plugin ecosystem toward a real package/registry model:
- keep local plugin installs always supported
- support configurable plugin sources
- build a separate plugin package repo (agent-plugins)
- build a separate registry server repo (agent-registry)
- next major implementation goal is remote plugins search / later plugins install from the registry server
Also make repo handoff/context-switching easier with WHERE-WE-ARE.md and COMPACTION.md files in each repo.
Instructions
- Local install should always be supported.
- The user wants a future plugin ecosystem similar to npm/Homebrew/Terraform Registry.
- The user wants the registry server code, docs, roadmap, etc. in /Users/nickcecere/Projects/GOLANG/agent-registry, not in agent or agent-plugins.
- The user explicitly wanted detailed design/build docs so other coding agents can work in parallel.
- The user asked for WHERE-WE-ARE.md and compact summary docs in all three repos so they can switch directories without losing context.
Discoveries
- v0.1.0 was successfully tagged and pushed from agent.
- Secrets were rotated externally per user, so Phase 0 release blockers were treated as closed.
- agent-plugins and agent-registry are currently just directories, not git repos.
- The core repo agent is a git repo and has recent uncommitted/then committed Phase 1 work.
- Plugin bundles were successfully moved out of agent/_testing/plugins into /Users/nickcecere/Projects/GOLANG/agent-plugins.
- Plugin-owned example profiles were moved out of agent/_testing/profiles into matching packages under agent-plugins/.../examples/profiles/.
- Framework-owned profiles remain in agent/_testing/profiles/.
- Framework test runtimes remain in agent/_testing/runtimes/ for now.
- MCP support was expanded earlier to cover stdio, remote HTTP, and SSE; this is already committed in agent.
- The user has an existing Context7 MCP setup, which led to remote MCP support being added earlier.
- A practical first registry abstraction already exists in agent:
  - pluginSources in config
  - plugins sources add/list/remove
  - plugins search for filesystem sources
  - install by plugin name from configured filesystem sources
  - registry sources can be configured, but remote search/install is not implemented yet
- agent-registry now contains a first working registry server scaffold:
  - scans ../agent-plugins
  - serves /healthz
  - serves /v1/index.json
  - serves /v1/packages/{name}.json
  - serves /v1/packages/{name}/{version}.json
  - generates and serves .tar.gz artifacts
- The registry server was run and validated against those endpoints locally.
- The user asked to pause and move registry work from agent-plugins/registry into a dedicated sibling repo agent-registry; that move was performed.
Accomplished
Completed earlier in this conversation
- Implemented command runtime in agent:
  - argv-template mode for existing CLIs like gh
  - JSON stdin/stdout mode for binaries/scripts
- Added example command plugins:
  - github-cli
  - json-tool
  - python-tool
- Added command runtime docs and tests.
- Added missing docs:
  - docs/examples/build-a-send-email-plugin.md
  - docs/plugin-author-checklist.md
- Wrote roadmap:
  - docs/architecture/plans/go-agent-framework-roadmap.md
- Ran Phase 0 validation and fixed:
  - plugin/profile directory loading
  - MCP dynamic tool auto-registration
- Implemented remote MCP transport support:
  - HTTP
  - SSE
  - spec-level runtime.headers
  - spec-level runtime.env
- Added MCP docs/examples:
  - docs/examples/build-an-mcp-plugin.md
- Released v0.1.0:
  - commit/tag pushed
Completed in Phase 1/package work
- Moved plugin bundles from agent/_testing/plugins to /Users/nickcecere/Projects/GOLANG/agent-plugins.
- Updated docs/tests in agent to reference ../agent-plugins/....
- Moved plugin-owned profiles into agent-plugins:
  - send-email/examples/profiles/...
  - web-research/examples/profiles/...
  - spawn-sub-agent/examples/profiles/...
- Kept framework-owned profiles in agent/_testing/profiles/.
- Added package model planning doc:
  - agent/docs/architecture/plans/go-agent-plugin-package-model.md
Completed in agent for plugin source groundwork
- Added pluginSources to config.
- Added CLI support:
  - plugins sources list
  - plugins sources add
  - plugins sources remove
- Added plugins search [query] across configured filesystem sources.
- Added install-by-name from configured filesystem sources.
- Added acceptance of registry plugin sources in config/CLI with clear “not implemented yet” behavior for remote search/install.
- Added tests for source resolution and search.
- Committed these changes in agent:
  - commit 4c53a87
  - message: feat(plugin): add source search and handoff docs
Completed in registry planning/docs
- Wrote registry contract doc (now moved to agent-registry):
  - agent-registry/docs/plugin-registry-contract.md
- Wrote registry server implementation plan:
  - agent-registry/docs/plugin-registry-server-plan.md
- Wrote registry build guide:
  - agent-registry/docs/registry-server-build-guide.md
Completed in agent-registry
- Created Go module:
  - agent-registry/go.mod
- Added README:
  - agent-registry/README.md
- Added scanner:
  - internal/source/source.go
  - internal/source/source_test.go
- Added artifact builder:
  - internal/archive/archive.go
- Added index shaper:
  - internal/index/index.go
- Added HTTP server:
  - internal/httpapi/server.go
  - cmd/registry-server/main.go
- Added top-level test:
  - registry-server_test.go
- Ran:
  - go mod tidy
  - go test ./...
  - all passed in agent-registry
- Ran server and validated:
  - /healthz
  - /v1/index.json
  - /v1/packages/send-email.json
  - /v1/packages/send-email/0.1.0.json
Completed for handoff/context switching
Added detailed and compact status docs in all three repos:
- agent/WHERE-WE-ARE.md
- agent/COMPACTION.md
- agent-plugins/WHERE-WE-ARE.md
- agent-plugins/COMPACTION.md
- agent-registry/WHERE-WE-ARE.md
- agent-registry/COMPACTION.md
Current status / in progress
- agent-registry has a working first scaffold, but needs hardening and then agent needs to consume it.
- The next logical implementation step is in agent:
  1. implement remote plugins search against GET /v1/index.json
  2. then remote plugins install <name> using package metadata + artifact download
Left to do next
- In agent:
  - implement remote plugins search for configured registry sources
  - later implement remote install and version/source recording
- In agent-registry:
  - add stronger tests, especially artifact download coverage
  - improve metadata richness
  - possibly add generated/cached index/artifact support under data/
- Optionally initialize agent-plugins and agent-registry as git repos if desired; they are not git repos yet.
Relevant files / directories
Core repo: agent
- WHERE-WE-ARE.md
- COMPACTION.md
- README.md
- docs/plugins.md
- docs/building-plugins.md
- docs/plugin-runtime-choices.md
- docs/plugin-http-example.md
- docs/mcp-bridge.md
- docs/examples/build-a-web-research-plugin.md
- docs/examples/build-a-send-email-plugin.md
- docs/examples/build-an-mcp-plugin.md
- docs/plugin-author-checklist.md
- docs/release-checklist-v0.1.md
- docs/architecture/plans/go-agent-framework-roadmap.md
- docs/architecture/plans/go-agent-plugin-package-model.md
- internal/cli/run.go
- internal/plugin/manage.go
- internal/plugin/manage_test.go
- internal/plugin/register.go
- internal/plugin/register_test.go
- internal/plugin/loader.go
- internal/plugin/loader_test.go
- internal/profile/loader.go
- internal/profile/loader_test.go
- internal/mcp/client.go
- internal/mcp/client_test.go
- internal/mcp/manager.go
- pkg/config/config.go
- pkg/plugin/plugin.go
- _testing/README.md
- _testing/profiles/
- _testing/profiles/README.md
- _testing/runtimes/
Plugin package repo: agent-plugins
- /Users/nickcecere/Projects/GOLANG/agent-plugins/README.md
- /Users/nickcecere/Projects/GOLANG/agent-plugins/WHERE-WE-ARE.md
- /Users/nickcecere/Projects/GOLANG/agent-plugins/COMPACTION.md
- /Users/nickcecere/Projects/GOLANG/agent-plugins/core-tools/
- /Users/nickcecere/Projects/GOLANG/agent-plugins/github-cli/
- /Users/nickcecere/Projects/GOLANG/agent-plugins/json-tool/
- /Users/nickcecere/Projects/GOLANG/agent-plugins/mcp-filesystem/
- /Users/nickcecere/Projects/GOLANG/agent-plugins/python-tool/
- /Users/nickcecere/Projects/GOLANG/agent-plugins/send-email/
- /Users/nickcecere/Projects/GOLANG/agent-plugins/send-email/examples/profiles/
- /Users/nickcecere/Projects/GOLANG/agent-plugins/spawn-sub-agent/
- /Users/nickcecere/Projects/GOLANG/agent-plugins/spawn-sub-agent/examples/profiles/
- /Users/nickcecere/Projects/GOLANG/agent-plugins/web-research/
- /Users/nickcecere/Projects/GOLANG/agent-plugins/web-research/examples/profiles/
Registry repo: agent-registry
- /Users/nickcecere/Projects/GOLANG/agent-registry/README.md
- /Users/nickcecere/Projects/GOLANG/agent-registry/WHERE-WE-ARE.md
- /Users/nickcecere/Projects/GOLANG/agent-registry/COMPACTION.md
- /Users/nickcecere/Projects/GOLANG/agent-registry/go.mod
- /Users/nickcecere/Projects/GOLANG/agent-registry/go.sum
- /Users/nickcecere/Projects/GOLANG/agent-registry/cmd/registry-server/main.go
- /Users/nickcecere/Projects/GOLANG/agent-registry/internal/source/source.go
- /Users/nickcecere/Projects/GOLANG/agent-registry/internal/source/source_test.go
- /Users/nickcecere/Projects/GOLANG/agent-registry/internal/index/index.go
- /Users/nickcecere/Projects/GOLANG/agent-registry/internal/archive/archive.go
- /Users/nickcecere/Projects/GOLANG/agent-registry/internal/httpapi/server.go
- /Users/nickcecere/Projects/GOLANG/agent-registry/registry-server_test.go
- /Users/nickcecere/Projects/GOLANG/agent-registry/data/
- /Users/nickcecere/Projects/GOLANG/agent-registry/docs/plugin-registry-contract.md
- /Users/nickcecere/Projects/GOLANG/agent-registry/docs/plugin-registry-server-plan.md
- /Users/nickcecere/Projects/GOLANG/agent-registry/docs/registry-server-build-guide.md