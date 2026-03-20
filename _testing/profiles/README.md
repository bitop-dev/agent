# _testing/profiles/

This directory now contains framework-owned profiles only.

Current framework-owned profiles:

- `readonly/` - minimal read-only profile
- `coding/` - coding profile with approvals
- `coding-no-approval/` - coding profile with shell allowed by policy override

Why these stay here:

- they test the core framework rather than a specific plugin
- they are useful as baseline profiles when validating the runtime itself

Plugin-owned example profiles now live with their plugin packages under `../agent-plugins/`.

Examples:

- `../agent-plugins/send-email/examples/profiles/`
- `../agent-plugins/web-research/examples/profiles/`
- `../agent-plugins/spawn-sub-agent/examples/profiles/`

The `openai-coding/` directory remains an incomplete experimental fixture and is not part of the current supported profile set.
