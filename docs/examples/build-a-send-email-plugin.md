# Build a Send Email Plugin

This walkthrough shows the intended way to build a plugin like `email/draft` and `email/send`.

## Goal

We want an agent to be able to:

- draft an email
- send an email

without putting SMTP or provider-specific logic into the core agent binary.

## Why this is a plugin

Email is exactly the kind of capability that should stay outside core:

- it needs credentials
- it performs real side effects
- it may talk to SMTP or third-party APIs
- it should usually be approval-gated before sending

The framework should load the tool definitions, enforce policy, and ask for approval.
The plugin runtime should handle the actual email delivery.

## Architecture

You will create two things:

1. a plugin bundle
2. a runtime service or binary

The bundle declares the tools, config, prompts, profile templates, and policies.
The runtime actually drafts or sends email.

For this example, we will use the `http` runtime because it is the clearest fit for a side-effecting integration.

## Step 1: Create the plugin bundle

Directory layout:

```text
send-email/
  plugin.yaml
  tools/
    draft.yaml
    send.yaml
  prompts/
    style-default.md
  profiles/
    email-assistant.yaml
  policies/
    default.yaml
```

Minimal `plugin.yaml`:

```yaml
apiVersion: agent/v1
kind: Plugin
metadata:
  name: send-email
  version: 0.1.0
  description: Email drafting and sending plugin

spec:
  category: integration
  runtime:
    type: http

  contributes:
    tools:
      - id: email/draft
        path: tools/draft.yaml
      - id: email/send
        path: tools/send.yaml
    prompts:
      - id: email/style-default
        path: prompts/style-default.md
    profileTemplates:
      - id: email/assistant
        path: profiles/email-assistant.yaml
    policies:
      - id: email/default
        path: policies/default.yaml

  configSchema:
    type: object
    properties:
      baseURL:
        type: string
      provider:
        type: string
        enum: [smtp, resend, sendgrid]
      smtpHost:
        type: string
      smtpPort:
        type: integer
      username:
        type: string
      password:
        type: string
        secret: true
      from:
        type: string
    required:
      - baseURL
      - provider

  permissions:
    network:
      outbound:
        - smtp
        - https
    sensitiveActions:
      - email/send

  requires:
    framework: ">=0.1.0"
    plugins: []
```

## Step 2: Add tool descriptors

`tools/draft.yaml`

```yaml
id: email/draft
kind: tool
description: Draft an email without sending it
inputSchema:
  type: object
  properties:
    to:
      type: string
    subject:
      type: string
    body:
      type: string
  required:
    - to
    - subject
execution:
  mode: http
  operation: draft-email
risk:
  level: low
```

`tools/send.yaml`

```yaml
id: email/send
kind: tool
description: Send an email through a configured provider
inputSchema:
  type: object
  properties:
    to:
      type: string
    subject:
      type: string
    body:
      type: string
  required:
    - to
    - subject
    - body
execution:
  mode: http
  operation: send-email
risk:
  level: high
```

These two tools are intentionally separate:

- `email/draft` is cheap and low risk
- `email/send` performs a real side effect and should usually require approval

That distinction matters for both policy and UX.

## Step 3: Add the default policy

`policies/default.yaml`

```yaml
version: 1
rules:
  - id: require-approval-email-send
    action: tool
    tool: email/send
    decision: require_approval
```

This keeps the safer default behavior:

- drafting is allowed normally
- sending requires approval unless the user deliberately overrides policy

## Step 4: Add a prompt and profile template

`prompts/style-default.md`

```md
Write concise, professional emails.

Prefer drafts first unless explicitly asked to send.
```

`profiles/email-assistant.yaml`

```yaml
apiVersion: agent/v1
kind: Profile
metadata:
  name: email-assistant-template
  version: 0.1.0
  description: Example profile template for email tasks
spec:
  instructions:
    system:
      - ./prompts/system.md
  provider:
    default: openai
    model: nemotron-3-super-120b-a12b
  tools:
    enabled:
      - email/draft
      - email/send
  approval:
    mode: on-request
    requireFor:
      - email/send
  workspace:
    required: false
    writeScope: read-only
  session:
    persistence: sqlite
    compaction: auto
  policy:
    overlays:
      - ./policies/default.yaml
```

The profile template is not required for the plugin to work, but it makes adoption much easier.

## Step 5: Build the runtime service

Because this plugin uses `runtime.type: http`, you need a separate service or binary.

The framework will call endpoints like:

- `POST /draft-email`
- `POST /send-email`

The request shape is:

```json
{
  "plugin": "send-email",
  "tool": "email/send",
  "operation": "send-email",
  "arguments": {
    "to": "alice@example.com",
    "subject": "Meeting follow-up",
    "body": "Thanks again for your time today."
  },
  "config": {
    "provider": "smtp",
    "baseURL": "http://127.0.0.1:8091",
    "smtpHost": "mail.privateemail.com",
    "smtpPort": 465,
    "username": "support@example.com",
    "password": "super-secret",
    "from": "support@example.com"
  }
}
```

Your service returns JSON like:

```json
{
  "output": "email sent to alice@example.com",
  "data": {
    "provider": "smtp",
    "messageId": "msg_123"
  }
}
```

Or for drafting:

```json
{
  "output": "draft prepared for alice@example.com",
  "data": {
    "to": "alice@example.com",
    "subject": "Meeting follow-up",
    "body": "Thanks again for your time today."
  }
}
```

Or an error:

```json
{
  "error": "SMTP authentication failed"
}
```

That request/response contract is the important part.
The runtime can be written in Go, Python, Rust, or anything else that can speak HTTP.

## Step 6: Install the bundle

While developing, use `--link` so the installed plugin points at your local checkout:

```bash
./bin/agent plugins install ./send-email --link
```

## Step 7: Configure the plugin

Example SMTP configuration:

```bash
./bin/agent plugins config set send-email provider smtp
./bin/agent plugins config set send-email baseURL http://127.0.0.1:8091
./bin/agent plugins config set send-email smtpHost mail.privateemail.com
./bin/agent plugins config set send-email smtpPort 465
./bin/agent plugins config set send-email username support@example.com
./bin/agent plugins config set send-email password 'super-secret'
./bin/agent plugins config set send-email from support@example.com
```

Then validate and enable it:

```bash
./bin/agent plugins validate-config send-email
./bin/agent plugins enable send-email
```

## Step 8: Use a profile that enables the tools

Example profile tool section:

```yaml
tools:
  enabled:
    - email/draft
    - email/send
approval:
  mode: on-request
  requireFor:
    - email/send
```

Then run:

```bash
./bin/agent run --profile ./my-email-profile.yaml "draft a follow-up email to alice@example.com about tomorrow's meeting"
```

Or:

```bash
./bin/agent run --profile ./my-email-profile.yaml "send a short thank-you email to alice@example.com for today's meeting"
```

In the second case, the framework should hit the approval path before the actual send happens.

## Recommended runtime behavior

For email plugins specifically, these are good defaults:

- prefer `email/draft` when the user is ambiguous
- require approval for `email/send`
- validate required config before enabling
- return clear provider errors instead of generic failures
- include useful metadata in `data`, such as `messageId`, provider, or normalized recipient

## Troubleshooting

### The plugin validates but sending fails

Usually this means runtime config is structurally valid, but the runtime cannot authenticate or connect.

Check:

- `baseURL` points at the correct runtime
- SMTP host and port are correct
- username/password are correct
- the runtime can reach the mail server or provider API

### The plugin is enabled but the agent never uses it

Check:

- the selected profile enables `email/draft` or `email/send`
- the plugin is actually enabled
- the run is not blocked by policy or approval mode

### The agent sends when it should draft first

Check:

- the system prompt guidance
- the profile approval settings
- the default policy overlay for `email/send`

## Key takeaway

An email plugin is a good example of the intended architecture:

- the core framework stays small
- the plugin bundle declares tools and safety defaults
- the runtime holds credentials and performs the real side effect
- policy and approval still apply before the send happens

That is the separation we want for integrations with real-world consequences.
