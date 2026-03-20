# agent

Core Go agent framework and CLI runtime.

**Full documentation:** https://github.com/bitop-dev/agent-docs

## Quick start

```bash
go run ./cmd/agent --profile ./profiles/my-profile.yaml
```

## Related repos

| Repo | Purpose |
|---|---|
| [agent-docs](https://github.com/bitop-dev/agent-docs) | All documentation |
| [agent-plugins](https://github.com/bitop-dev/agent-plugins) | Plugin packages |
| [agent-registry](https://github.com/bitop-dev/agent-registry) | Plugin registry server |

## Key docs (in agent-docs)

- [plugins](https://github.com/bitop-dev/agent-docs/blob/main/core/plugins.md)
- [profiles](https://github.com/bitop-dev/agent-docs/blob/main/core/profiles.md)
- [building plugins](https://github.com/bitop-dev/agent-docs/blob/main/core/building-plugins.md)
- [plugin runtime choices](https://github.com/bitop-dev/agent-docs/blob/main/core/plugin-runtime-choices.md)
- [architecture plans](https://github.com/bitop-dev/agent-docs/tree/main/core/architecture/plans)
- [patterns](https://github.com/bitop-dev/agent-docs/tree/main/core/patterns)

## Development

```bash
go test ./...
go build ./...
```

See [WHERE-WE-ARE.md](WHERE-WE-ARE.md) for current status and next steps.
