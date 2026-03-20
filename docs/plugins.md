# Plugin CLI Workflow

This project supports installing, upgrading, and publishing plugin bundles.
Plugins can be installed from local paths, filesystem sources, or remote registry servers.

Plugin config is stored in `~/.agent/config.yaml` under `plugins.<name>`.
Plugin source configuration is stored under `pluginSources` in the same config file.

If you want the conceptual overview first, start with:

- `docs/building-plugins.md` — how to write a plugin from scratch
- `docs/plugin-runtime-choices.md` — which runtime type to pick
- `docs/patterns/` — end-to-end agent patterns with plugins

---

## Common commands

```bash
agent plugins list                         # show installed plugins with version and source
agent plugins search email                 # search configured sources
agent plugins show send-email              # show plugin details
agent plugins config send-email            # show plugin config
agent plugins validate-config send-email   # validate config against schema
```

---

## Install

### From a local path

Use `--link` while developing so the installed plugin points at your local checkout.

```bash
agent plugins install ../agent-plugins/send-email --link
agent plugins install ../agent-plugins/web-research --link
```

Local path installs are always supported, even if no plugin sources are configured.

### From a configured source by name

```bash
agent plugins install send-email
agent plugins install ddg-research --source official
```

### With version pinning

```bash
agent plugins install send-email@0.2.0
agent plugins install grafana-alerts@0.1.0 --source official
```

If the exact version is not found in any configured registry, the install fails.
When no version is specified, the latest available version is installed.

### What happens during install

When installing from a registry source, the framework:
1. Fetches the package metadata from `/v1/packages/<name>.json`
2. Selects the matching version (or latest if no `@version` specified)
3. Downloads the artifact tarball
4. Verifies the SHA256 checksum
5. Extracts the tarball to `~/.agent/plugins/<name>/`
6. Preserves file permissions — executables (`chmod +x`) remain executable
7. Records the installed version and source name in config

### Version and source tracking

After install, `plugins list` shows the installed version and source:

```
ddg-research  0.1.0  integration/command  enabled=true  source=official  ~/.agent/plugins/ddg-research/plugin.yaml
```

This information is stored in config:

```yaml
plugins:
  ddg-research:
    enabled: true
    installedVersion: "0.1.0"
    installedSource: "official"
    config: {}
```

---

## Upgrade

Upgrade an installed plugin to the latest version from its original source:

```bash
agent plugins upgrade ddg-research
```

This:
1. Checks the registry for the latest version
2. If the latest is newer than the installed version, removes the current install
3. Downloads and installs the new version
4. Updates the version record in config
5. Preserves existing config values (enabled state, config keys)

Locally-linked plugins cannot be upgraded — use `plugins install <path>` instead.

---

## Publish

Publish a plugin to a configured registry source:

```bash
agent plugins publish ../agent-plugins/grafana-alerts --registry official
```

This:
1. Reads `plugin.yaml` from the specified directory
2. Validates the manifest
3. Packs the directory into a `.tar.gz` with deterministic timestamps
4. POSTs the tarball to the registry with the configured publish token

### Registry configuration for publishing

The source must have a `publishToken` configured:

```yaml
pluginSources:
  - name: official
    type: registry
    url: http://127.0.0.1:9080
    enabled: true
    publishToken: your-secret-token
```

The registry server must be started with a matching `--publish-token` flag:

```bash
go run ./cmd/registry-server --plugin-root ../agent-plugins --addr 127.0.0.1:9080 --publish-token your-secret-token
```

---

## Configure plugin sources

Plugin sources tell the CLI where to search for and install plugins by name.
Two source types are supported: filesystem directories and registry servers.

### Filesystem source

Points at a local directory of plugin bundles. Good for local development or
a shared team directory.

```bash
agent plugins sources add local-dev ../agent-plugins
agent plugins sources list
agent plugins search web
agent plugins install web-research --link
```

### Registry source

Points at a running registry server that serves a plugin index and downloadable tarballs.

```bash
agent plugins sources add official http://127.0.0.1:9080 --type registry
agent plugins search email
agent plugins install send-email
```

### View and manage sources

```bash
agent plugins sources list
agent plugins sources add my-source /path/to/dir
agent plugins sources remove my-source
```

---

## Plugin dependencies

Plugins can declare dependencies on other plugins in `requires.plugins`:

```yaml
# plugin.yaml
spec:
  requires:
    framework: ">=0.1.0"
    plugins:
      - send-email     # this plugin requires send-email to be installed and enabled
```

Dependencies are enforced at plugin registration time — when the agent starts up.
If a required plugin is not installed or not enabled, the agent reports an error:

```
plugin my-plugin requires plugin "send-email" which is not installed or enabled
```

Fix: install and enable the dependency first.

---

## Environment variable mapping (envMapping)

Plugins that use MCP or command runtimes can map config keys to environment variables
using `envMapping` in `plugin.yaml`:

```yaml
spec:
  runtime:
    type: mcp
    command: ["mcp-grafana", "-t", "stdio"]
    envMapping:
      grafanaURL: GRAFANA_URL           # config.grafanaURL → $GRAFANA_URL
      grafanaAPIKey: GRAFANA_API_KEY    # config.grafanaAPIKey → $GRAFANA_API_KEY
```

Users configure values normally:

```bash
agent plugins config set grafana-mcp grafanaURL https://grafana.example.com
agent plugins config set grafana-mcp grafanaAPIKey glsa_abc123
```

The framework passes these as environment variables to the subprocess automatically.
Secret fields are stored in config but masked in CLI output.

---

## Set and unset plugin config

```bash
agent plugins config set send-email provider smtp
agent plugins config set send-email smtpPort 587
agent plugins config set send-email password 'super-secret'
agent plugins config unset send-email baseURL
```

### How values are parsed

When a plugin declares a config schema, `plugins config set` parses values using the property type:

- `string`: stored as-is
- `integer`: parsed with decimal input such as `587`
- `boolean`: accepts `true` and `false`
- `array`: accepts a JSON array `'["a","b"]'` or comma-separated `a,b`
- `object`: accepts a JSON object `'{"mode":"strict"}'`

If a key is not declared in the schema, the CLI stores the raw string value.

---

## Validate before enabling

```bash
agent plugins validate-config send-email
agent plugins enable send-email
```

Enabling a plugin validates its config, so required keys must already be present.
Enabling also checks that all `requires.plugins` dependencies are satisfied.
