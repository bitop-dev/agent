# WHERE-WE-ARE

This file is a fast handoff for work in the `agent` repository.

## What this repo owns

- core framework and CLI
- runtime loop, providers, policy, approvals, sessions
- built-in core tools
- plugin loading, install/config/enable flows
- framework-owned test profiles and runtime fixtures

This repo does **not** own:

- plugin package bundles as the long-term package unit
- the plugin registry server

Those now live in sibling directories:

- `../agent-plugins`
- `../agent-registry`

## Release status

- `v0.1.0` is tagged and pushed
- Phase 0 is complete
- we are now in Phase 1: plugin author experience and package/registry workflow

## Current Phase 1 work in this repo

The active focus here is plugin source management in the CLI.

Implemented locally but not committed yet:

- configurable `pluginSources` in config
- `plugins sources list`
- `plugins sources add <name> <path-or-url> [--type ...]`
- `plugins sources remove <name>`
- `plugins search [query]` for configured filesystem sources
- install by plugin name from configured filesystem sources
- acceptance of `registry` sources in config, with clear "not implemented yet" behavior for remote search/install

## Current uncommitted files

As of the latest handoff, these files are modified or new:

- `docs/architecture/plans/go-agent-plugin-package-model.md`
- `docs/plugins.md`
- `internal/cli/run.go`
- `internal/plugin/manage.go`
- `internal/plugin/manage_test.go`
- `pkg/config/config.go`

These are the Phase 1 CLI/source-management changes.

## What already works here

- local path installs still work:

```bash
go run ./cmd/agent plugins install ../agent-plugins/send-email --link
```

- local filesystem source config works:

```bash
go run ./cmd/agent plugins sources add local-dev ../agent-plugins
go run ./cmd/agent plugins sources list
go run ./cmd/agent plugins search email
go run ./cmd/agent plugins install send-email --link
```

- registry sources can be configured, but remote search/install is not implemented yet:

```bash
go run ./cmd/agent plugins sources add official https://plugins.example.com --type registry
```

## What this repo should do next

The next major step here is to consume the registry server from `../agent-registry`.

Priority order:

1. implement remote `plugins search` against `GET /v1/index.json`
2. implement remote `plugins install <name>` against package metadata + artifact download
3. record installed source and version metadata locally
4. add source filtering such as `--source official`
5. add version selection such as `send-email@0.1.0`

## Important docs to read before changing Phase 1 behavior

- `docs/plugins.md`
- `docs/architecture/plans/go-agent-framework-roadmap.md`
- `docs/architecture/plans/go-agent-plugin-package-model.md`
- `../agent-registry/docs/plugin-registry-contract.md`
- `../agent-registry/docs/plugin-registry-server-plan.md`

## Repo boundary reminders

- framework-owned profiles stay here under `_testing/profiles/`
- plugin-owned example profiles moved to `../agent-plugins/.../examples/profiles/`
- framework test runtimes still live here under `_testing/runtimes/`
- plugin registry server code should not be added here; it belongs in `../agent-registry`

## Quick validation commands

```bash
go test ./...
go run ./cmd/agent plugins sources list
go run ./cmd/agent plugins search
go run ./cmd/agent plugins install ../agent-plugins/send-email --link
```

## Short summary

This repo is in the middle of Phase 1 CLI/package-source work.
The main unfinished task here is remote registry integration against the new server in `../agent-registry`.
