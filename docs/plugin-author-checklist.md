# Plugin Author Checklist

Use this checklist before you hand a plugin to another user, rely on it in a profile, or treat it as release-ready.

This is meant to be practical. It is not a style guide. It is the minimum set of things a plugin author should verify so the plugin is understandable, safe, and operable.

## 1. Runtime choice

- [ ] I chose the runtime type intentionally.
- [ ] I can explain why this plugin is `asset`, `http`, `command`, `mcp`, or `host`.
- [ ] I did not choose `host` for an ordinary external integration.
- [ ] I did not choose `http` when `command` or `mcp` would have been simpler.

Quick guidance:

- use `asset` for prompts, policies, and profile templates only
- use `http` for custom networked integrations or side-effecting service runtimes
- use `command` for local CLIs, binaries, and scripts
- use `mcp` when an MCP server already exists
- use `host` only for bounded privileged runtime behavior

## 2. Plugin bundle structure

- [ ] The plugin has a `plugin.yaml` file.
- [ ] Tool descriptors live under `tools/`.
- [ ] Optional prompts, profiles, and policies live in clear directories.
- [ ] Paths in `plugin.yaml` point at real files.
- [ ] Contribution IDs are stable and namespaced.

Typical layout:

```text
my-plugin/
  plugin.yaml
  tools/
  prompts/
  profiles/
  policies/
```

## 3. Manifest sanity

- [ ] `apiVersion` is correct.
- [ ] `kind` is `Plugin`.
- [ ] `metadata.name` is unique and stable.
- [ ] `metadata.version` is present.
- [ ] `metadata.description` tells a user what the plugin actually does.
- [ ] `spec.category` is reasonable for the plugin.
- [ ] `spec.runtime.type` matches the intended runtime.
- [ ] `spec.requires.framework` is set to a sensible minimum version.

## 4. Tool definitions

For each tool descriptor:

- [ ] `id` is stable and namespaced, like `email/send` or `gh/pr-list`.
- [ ] `description` is written for the model and the human reading docs.
- [ ] `inputSchema` is present and only includes arguments the tool really supports.
- [ ] required fields are marked as required.
- [ ] optional fields are actually optional.
- [ ] `execution.mode` matches the runtime contract.
- [ ] `execution.operation` or `execution.argv` is correct.
- [ ] `risk.level` reflects real-world impact.

Good signs:

- low-risk read-like tools are separate from high-risk side-effecting tools
- destructive or external actions are not hidden behind vague tool names
- the schema is small and explicit instead of "accepts arbitrary JSON"

## 5. Runtime contract

### If using `http`

- [ ] The runtime exposes the expected endpoint for each tool operation.
- [ ] The runtime accepts the framework request shape:

```json
{
  "plugin": "...",
  "tool": "...",
  "operation": "...",
  "arguments": {"...": "..."},
  "config": {"...": "..."}
}
```

- [ ] The runtime returns either:

```json
{"output":"...","data":{"...":"..."}}
```

or:

```json
{"error":"..."}
```

- [ ] Runtime errors are readable and actionable.

### If using `command`

- [ ] I chose the correct command mode.
- [ ] For argv-template mode, `execution.argv` expands correctly.
- [ ] Optional flag/value pairs behave correctly when values are absent.
- [ ] For JSON mode, the executable reads JSON from stdin and writes JSON to stdout.
- [ ] Non-zero exits produce meaningful stderr.
- [ ] Relative command paths are intentional and documented.
- [ ] The runtime works in the expected working directory.

For argv-template mode, verify placeholders like:

- `{{repo}}`
- `{{query}}`
- `{{config.apiKey}}`

### If using `mcp`

- [ ] The MCP server starts reliably.
- [ ] The expected tools appear in `tools/list`.
- [ ] Tool names and schemas match what the agent should expose.
- [ ] Authentication or environment setup is documented.

### If using `host`

- [ ] The plugin truly needs bounded host behavior.
- [ ] The host operation is narrow and auditable.
- [ ] The plugin does not smuggle unrelated privileged behavior through `host`.

## 6. Config schema

- [ ] The plugin declares all expected config keys in `configSchema`.
- [ ] Required keys are really required.
- [ ] Secret values are marked with `secret: true`.
- [ ] Enum values are used where appropriate.
- [ ] Config names are clear and predictable.
- [ ] The plugin can be validated from the CLI.

Questions to ask yourself:

- if a user runs `plugins config set`, will the expected types parse cleanly?
- if a key is wrong or missing, will the error explain what to fix?
- are secrets kept out of normal CLI output?

## 7. Policy and approval

- [ ] I identified which tools are low risk and which are sensitive.
- [ ] Sensitive tools are listed under `permissions.sensitiveActions` when appropriate.
- [ ] A policy overlay exists for actions that should require approval by default.
- [ ] The profile guidance and policy overlay agree with each other.
- [ ] The plugin does not rely on human caution alone for risky actions.

Typical approval-worthy actions:

- sending email
- mutating remote state
- executing destructive CLI operations
- changing infrastructure
- actions with external side effects or billing impact

## 8. Profile and prompt support

- [ ] If the plugin is meant to be used directly, it provides a helpful profile template.
- [ ] Prompt assets describe the right default behavior.
- [ ] High-risk tools are not encouraged casually in the prompt.
- [ ] The profile enables the correct tool IDs.
- [ ] Approval mode in the profile matches the risk level of the tools.

## 9. Local install workflow

- [ ] The plugin can be installed locally with `--link` during development.
- [ ] `plugins validate` succeeds.
- [ ] `plugins validate-config` succeeds after configuration.
- [ ] `plugins enable` succeeds.
- [ ] `plugins show` and `plugins config` output is understandable.

Recommended verification flow:

```bash
go run ./cmd/agent plugins validate ./path/to/my-plugin
go run ./cmd/agent plugins install ./path/to/my-plugin --link
go run ./cmd/agent plugins config set my-plugin ...
go run ./cmd/agent plugins validate-config my-plugin
go run ./cmd/agent plugins enable my-plugin
```

## 10. End-to-end behavior

- [ ] I tested the plugin with a real profile that enables the tool IDs.
- [ ] I tested at least one successful call.
- [ ] I tested at least one failure path.
- [ ] I tested at least one policy or approval path for risky tools.
- [ ] Tool output is readable by both humans and models.
- [ ] The runtime does not return giant blobs when a concise summary would do.

For example:

- `email/draft` should return a draft, not just `ok`
- `gh/pr-list` should return useful PR information, not raw noise if that can be avoided
- errors should say what failed, not just `request failed`

## 11. Testing expectations

- [ ] There is at least one automated test covering the runtime contract.
- [ ] There is at least one integration-style check or scripted verification for the plugin flow.
- [ ] High-risk tools have policy or approval coverage.
- [ ] Example plugins are maintained as compatibility checks when possible.

Minimum bar:

- one success case
- one failure case
- one config validation case
- one approval or policy case for risky behavior

## 12. Documentation

- [ ] The plugin has a short README or walkthrough.
- [ ] The docs explain what runtime was chosen and why.
- [ ] The docs explain required config.
- [ ] The docs explain how to install and enable the plugin.
- [ ] The docs explain how to verify it works.
- [ ] The docs explain common failure modes.

If another developer asks any of these, the docs should answer clearly:

- what files do I create?
- where does the real runtime code live?
- how does the agent call it?
- how do I test it locally?
- what should require approval?

## 13. Release readiness

Before treating the plugin as stable:

- [ ] tool IDs and config keys are unlikely to churn immediately
- [ ] docs match current behavior
- [ ] known limitations are documented honestly
- [ ] secrets are not committed
- [ ] the plugin works in a clean checkout, not only on the author's machine

## Common failure patterns

Watch for these before you call the plugin done:

- a tool schema that requires fields the runtime ignores
- a runtime that returns unreadable errors
- a high-risk action marked as low risk
- a profile that enables the tool but forgets approval requirements
- a command plugin that only works because of the author's shell environment
- docs that explain the concept but not the actual install/config/run flow

## Definition of done

A plugin is in good shape when all of these are true:

- the runtime choice is easy to justify
- the manifest and tool descriptors are clear
- config validation catches the obvious mistakes
- risky actions are policy-aware and approval-aware
- a user can install, configure, enable, and run the plugin locally
- the docs explain the real workflow without sending people into the source tree

If any of those are missing, the plugin is still in development, even if the happy path works on the author's machine.
