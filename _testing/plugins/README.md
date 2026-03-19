# _testing/plugins/

The `_testing/plugins/` directory contains local plugin bundles used by this repository for development, testing, and examples.

Important distinction:

- this directory is part of the repository
- but the bundles inside it are not core agent runtime code
- they are plugin manifests and assets that the agent can discover locally

So in practice:

- `cmd/agent` and `internal/` are the core framework and host
- `_testing/plugins/` contains example or first-party plugin bundles
- plugin runtime executables live separately under `_testing/runtimes/`, for example:
  - `_testing/runtimes/send-email-plugin`
  - `_testing/runtimes/web-research-plugin`

This keeps the core agent small while still letting the repo include working plugin examples.

If we want even stricter separation later, these bundles can move to:

- `docs/examples/plugins/`
- `first_party_plugins/`
- or separate repositories entirely

The repository also keeps testing profiles under `_testing/profiles/`.
