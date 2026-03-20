# Plugin CLI Workflow

This project supports installing local plugin bundles, mutating plugin config from the CLI, and validating config against each plugin's schema.

Plugin config is stored in `~/.agent/config.yaml` under `plugins.<name>.config`.

If you want the conceptual overview first, start with:

- `docs/building-plugins.md`
- `docs/plugin-runtime-choices.md`
- `docs/examples/build-a-web-research-plugin.md`
- `docs/examples/build-a-send-email-plugin.md`
- `docs/plugin-author-checklist.md`

## Common commands

```bash
go run ./cmd/agent plugins list
go run ./cmd/agent plugins show send-email
go run ./cmd/agent plugins validate ./_testing/plugins/send-email
go run ./cmd/agent plugins config send-email
go run ./cmd/agent plugins validate-config send-email
```

## Install a local plugin

Use `--link` while developing so the installed plugin points at your local checkout.

```bash
go run ./cmd/agent plugins install ./_testing/plugins/send-email --link
go run ./cmd/agent plugins install ./_testing/plugins/web-research --link
```

## Set plugin config from the CLI

Use `plugins config set` to write top-level config keys without hand-editing YAML.

```bash
go run ./cmd/agent plugins config set send-email provider smtp
go run ./cmd/agent plugins config set send-email baseURL http://127.0.0.1:8091
go run ./cmd/agent plugins config set send-email smtpHost mail.privateemail.com
go run ./cmd/agent plugins config set send-email smtpPort 465
go run ./cmd/agent plugins config set send-email username support@example.com
go run ./cmd/agent plugins config set send-email password 'super-secret'
go run ./cmd/agent plugins config set send-email from support@example.com
```

Inspect the resolved config at any time:

```bash
go run ./cmd/agent plugins config send-email
```

Secret fields declared in the plugin schema are masked in command output.

Setting config does not enable the plugin. Enable it explicitly after validation.

## Unset plugin config keys

```bash
go run ./cmd/agent plugins config unset send-email baseURL
```

If the plugin is enabled and the removed key is required by the schema, the command fails instead of saving an invalid config.

## How values are parsed

When a plugin declares a config schema, `plugins config set` parses values using the property type before saving them.

- `string`: stored as-is
- `integer`: parsed with decimal input such as `465`
- `boolean`: accepts Go-style booleans such as `true` and `false`
- `array`: accepts a JSON array like `'["a","b"]'` or a comma-separated string like `a,b`
- `object`: accepts a JSON object like `'{"mode":"strict"}'`

If a key is not declared in the schema, the CLI stores the raw string value.

## Validate before enabling

Use schema validation to confirm required config exists and types match.

```bash
go run ./cmd/agent plugins validate-config send-email
go run ./cmd/agent plugins enable send-email
```

Enabling a plugin also validates its config, so required keys must already be present.

## Example: local HTTP plugins

For the repository examples under `_testing/plugins/`, see `docs/plugin-http-example.md`.
