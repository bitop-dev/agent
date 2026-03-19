# _testing/

The `_testing/` directory contains repository-local fixtures and examples used while developing the framework.

Contents:

- `_testing/plugins/` - example plugin bundles
- `_testing/profiles/` - example profiles for local testing
- `_testing/runtimes/` - example plugin runtime binaries

These are not part of the core `agent` runtime.

The core code lives in:

- `cmd/agent`
- `internal/`
- `pkg/`

Normal runtime discovery no longer points at `_testing/` by default. Use these assets explicitly for local development, testing, or docs examples.
