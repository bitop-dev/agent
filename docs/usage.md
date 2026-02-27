# Usage Guide

Patterns for running the agent in different contexts — from a daily
interactive shell to automated pipelines and scheduled jobs.

---

## Local CLI

### Interactive REPL

The default mode. Start it, type prompts, get responses.

```bash
agent -config agent.yaml
```

Useful REPL tricks:

```
# Switch model mid-session without restarting
/model gpt-4o

# Fork the current session, keep only the last 10 messages
# (useful when context gets large)
/fork 10

# Export the whole conversation as a self-contained HTML file
/export

# Resume a previous session by ID prefix
# First, find it:
/sessions
# Then exit and reopen it:
agent -config agent.yaml -session a3f7c9
```

### One-Shot Prompts

Use `-prompt` to run a single prompt and exit. Output goes to stdout,
making it easy to pipe into other tools.

```bash
# Ask a question and pipe the answer somewhere
agent -config agent.yaml -prompt "What does pkg/agent/loop.go do?" > summary.txt

# Generate a file
agent -config agent.yaml -prompt "Write a Go HTTP health check handler" > health.go

# Use shell variables in your prompt
FILE="pkg/agent/loop.go"
agent -config agent.yaml -prompt "Review $FILE for bugs and style issues."
```

### Project-Scoped Config

Keep a `agent.yaml` per project so the agent always starts in the right
directory with the right tools:

```bash
# .agent.yaml in your project root
cat > .agent.yaml << 'EOF'
provider: anthropic
model: claude-sonnet-4-5
api_key: ${ANTHROPIC_API_KEY}
max_tokens: 8192

system_prompt: |
  You are an expert Go engineer working on the agent framework.
  Always write idiomatic, tested code.
  The project is at /Users/me/Projects/agent.

tools:
  preset: coding
  work_dir: .

compaction:
  enabled: true
  reserve_tokens: 16384
  keep_recent_tokens: 20000
EOF

# Run from the project root
agent -config .agent.yaml
```

### Shell Alias

Add a shortcut that always uses your preferred config:

```bash
# ~/.zshrc or ~/.bashrc
alias ai='agent -config ~/.config/agent/agent.yaml'
alias aicode='agent -config ~/.config/agent/coding.yaml -cwd .'

# Usage
ai "Explain the builder pattern in Go"
aicode "Refactor the error handling in main.go"
```

### Multiple Configs

Maintain separate configs for different use cases:

```
~/.config/agent/
  coding.yaml      # claude-sonnet, coding tools, large context
  research.yaml    # gpt-4o, web tools, web search
  quick.yaml       # gemini-flash, no tools, fast answers
  local.yaml       # ollama, no API key, offline
```

```bash
agent -config ~/.config/agent/research.yaml -prompt "Latest Go 1.26 features"
agent -config ~/.config/agent/local.yaml    -prompt "Explain goroutines"
```

---

## Cron / Scheduled Jobs

Use `-prompt` for unattended scheduled runs. A few patterns:

### Daily Digest

Generate a daily report and email or post it somewhere:

```bash
#!/usr/bin/env bash
# /usr/local/bin/daily-digest.sh
set -euo pipefail

export ANTHROPIC_API_KEY="$(cat /run/secrets/anthropic_key)"

REPORT=$(agent -config /etc/agent/agent.yaml -prompt "
  Search the web for the top 5 AI/LLM news stories from the last 24 hours.
  For each: title, one-sentence summary, URL.
  Format as plain text.
")

echo "$REPORT" | mail -s "Daily AI digest $(date +%Y-%m-%d)" me@example.com
```

```cron
# crontab -e
0 8 * * * /usr/local/bin/daily-digest.sh >> /var/log/daily-digest.log 2>&1
```

### Repo Health Check

Run every night and open a GitHub issue if problems are found:

```bash
#!/usr/bin/env bash
# scripts/nightly-check.sh
set -euo pipefail

cd /path/to/your/project
export ANTHROPIC_API_KEY="$(cat /run/secrets/anthropic_key)"

RESULT=$(agent -config .agent.yaml -prompt "
  Run go vet ./... and go test ./... and report the results.
  If everything passes say HEALTHY.
  If there are failures explain them clearly.
")

if ! echo "$RESULT" | grep -q "HEALTHY"; then
  gh issue create \
    --title "Nightly check failed $(date +%Y-%m-%d)" \
    --body "$RESULT" \
    --label "automated,bug"
fi
```

```cron
0 2 * * * /path/to/your/project/scripts/nightly-check.sh
```

### Config for Scheduled Jobs

Scheduled jobs should never run indefinitely. Always set `max_turns` and
`auto_approve`:

```yaml
# /etc/agent/scheduled.yaml
provider: anthropic
model: claude-sonnet-4-5
api_key: ${ANTHROPIC_API_KEY}
max_tokens: 4096
max_turns: 20        # hard stop — prevent runaway loops
auto_approve: true   # no human confirmation needed

tools:
  preset: all
  work_dir: /path/to/project
```

### Capturing Exit Codes

`agent` exits `0` on success and non-zero on error. Use this in scripts:

```bash
if agent -config agent.yaml -prompt "Run the test suite and fix any failures."; then
  echo "Agent completed successfully"
else
  echo "Agent failed or was aborted" >&2
  exit 1
fi
```

---

## CI / GitHub Actions

### Code Review on Pull Requests

Add an AI review step to your PR workflow:

```yaml
# .github/workflows/ai-review.yml
name: AI Code Review

on:
  pull_request:
    types: [opened, synchronize]

jobs:
  review:
    runs-on: ubuntu-latest
    permissions:
      pull-requests: write

    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Install agent
        run: |
          curl -L https://github.com/bitop-dev/agent/releases/latest/download/agent_latest_linux_amd64.tar.gz \
            | tar -xz && sudo mv agent /usr/local/bin/

      - name: Create config
        run: |
          cat > /tmp/agent.yaml << 'EOF'
          provider: anthropic
          model: claude-sonnet-4-5
          api_key: ${ANTHROPIC_API_KEY}
          max_tokens: 4096
          max_turns: 10
          auto_approve: true
          tools:
            preset: readonly
            work_dir: .
          EOF

      - name: Get diff
        id: diff
        run: |
          git diff origin/${{ github.base_ref }}...HEAD > /tmp/pr.diff
          echo "diff_size=$(wc -c < /tmp/pr.diff)" >> $GITHUB_OUTPUT

      - name: Review
        if: steps.diff.outputs.diff_size != '0'
        id: review
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
        run: |
          REVIEW=$(agent -config /tmp/agent.yaml -prompt "
            Review this pull request diff for:
            - Correctness and logic errors
            - Security issues
            - Missing error handling
            - Test coverage gaps
            Be concise. List issues as bullet points.
            If nothing significant found, say 'LGTM'.

            $(cat /tmp/pr.diff)
          ")
          echo "review<<EOF" >> $GITHUB_OUTPUT
          echo "$REVIEW" >> $GITHUB_OUTPUT
          echo "EOF" >> $GITHUB_OUTPUT

      - name: Post comment
        if: steps.diff.outputs.diff_size != '0'
        uses: actions/github-script@v7
        with:
          script: |
            github.rest.issues.createComment({
              issue_number: context.issue.number,
              owner: context.repo.owner,
              repo: context.repo.repo,
              body: `## AI Review\n\n${{ steps.review.outputs.review }}`
            })
```

### Auto-Generate Release Notes

```yaml
# .github/workflows/release-notes.yml
name: Generate Release Notes

on:
  push:
    tags: ["v*.*.*"]

jobs:
  notes:
    runs-on: ubuntu-latest
    permissions:
      contents: write

    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Install agent
        run: |
          curl -L https://github.com/bitop-dev/agent/releases/latest/download/agent_latest_linux_amd64.tar.gz \
            | tar -xz && sudo mv agent /usr/local/bin/

      - name: Generate notes
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
        run: |
          PREV_TAG=$(git describe --tags --abbrev=0 HEAD^ 2>/dev/null || echo "")
          if [ -n "$PREV_TAG" ]; then
            COMMITS=$(git log ${PREV_TAG}..HEAD --oneline)
          else
            COMMITS=$(git log --oneline | head -20)
          fi

          cat > /tmp/agent.yaml << 'EOF'
          provider: anthropic
          model: claude-sonnet-4-5
          api_key: ${ANTHROPIC_API_KEY}
          max_tokens: 2048
          max_turns: 3
          auto_approve: true
          tools:
            preset: none
          EOF

          agent -config /tmp/agent.yaml -prompt "
            Write release notes for version ${{ github.ref_name }} based on these commits:

            $COMMITS

            Format as GitHub Markdown with sections: Features, Bug Fixes, Other Changes.
            Be concise and user-focused. Skip chore/test/docs commits.
          " > /tmp/release-notes.md

          gh release edit ${{ github.ref_name }} \
            --notes-file /tmp/release-notes.md
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

### Documentation Generation

```yaml
- name: Update API docs
  env:
    OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}
  run: |
    cat > /tmp/agent.yaml << 'EOF'
    provider: openai-completions
    model: gpt-4o
    api_key: ${OPENAI_API_KEY}
    max_tokens: 4096
    max_turns: 15
    auto_approve: true
    tools:
      preset: coding
      work_dir: .
    EOF

    agent -config /tmp/agent.yaml -prompt "
      Read every exported function and type in pkg/agent/agent.go
      and update docs/sdk.md to reflect the current API.
      Only edit the file — do not explain your changes.
    "

    git config user.name "github-actions[bot]"
    git config user.email "github-actions[bot]@users.noreply.github.com"
    git add docs/sdk.md
    git diff --staged --quiet || git commit -m "docs: update SDK reference [skip ci]"
    git push
```

---

## Docker

### Interactive

```bash
docker run --rm -it \
  -e ANTHROPIC_API_KEY \
  -v $(pwd)/agent.yaml:/etc/agent/agent.yaml \
  -v $(pwd):/workspace \
  ghcr.io/bitop-dev/agent:latest
```

### One-Shot

```bash
docker run --rm \
  -e ANTHROPIC_API_KEY \
  -v $(pwd)/agent.yaml:/etc/agent/agent.yaml \
  -v $(pwd):/workspace \
  ghcr.io/bitop-dev/agent:latest \
  -prompt "Summarise the README."
```

### Docker Compose — Development

```yaml
# docker-compose.yml
services:
  agent:
    image: ghcr.io/bitop-dev/agent:latest
    environment:
      - ANTHROPIC_API_KEY
      - OPENAI_API_KEY
    volumes:
      - ./agent.yaml:/etc/agent/agent.yaml
      - .:/workspace
    working_dir: /workspace
    stdin_open: true
    tty: true
```

```bash
# Interactive
docker compose run agent

# One-shot
docker compose run agent -prompt "List all TODO comments in the codebase."
```

### Docker Compose — Scheduled Job

```yaml
# docker-compose.scheduled.yml
services:
  nightly-check:
    image: ghcr.io/bitop-dev/agent:latest
    environment:
      - ANTHROPIC_API_KEY
    volumes:
      - ./scheduled.yaml:/etc/agent/agent.yaml
      - .:/workspace
    working_dir: /workspace
    command: ["-prompt", "Run go test ./... and report any failures."]
    restart: "no"
```

```bash
# Run manually or call from cron
docker compose -f docker-compose.scheduled.yml run nightly-check
```

### Config Inside the Container

All plugin scripts are at `/opt/agent/plugins/` in the image:

```yaml
# agent.yaml (for use inside container)
provider: anthropic
model: claude-sonnet-4-5
api_key: ${ANTHROPIC_API_KEY}
max_tokens: 4096
max_turns: 50
auto_approve: true

tools:
  preset: coding
  work_dir: /workspace
  plugins:
    - path: python3
      args: ["/opt/agent/plugins/stats.py"]
    - path: node
      args: ["/opt/agent/plugins/json_query.mjs"]
    - path: bash
      args: ["/opt/agent/plugins/sys_info.sh"]
    - path: ruby
      args: ["/opt/agent/plugins/template.rb"]
    - path: /usr/local/bin/file_info-plugin
```

### Kubernetes CronJob

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: agent-nightly
spec:
  schedule: "0 2 * * *"
  jobTemplate:
    spec:
      template:
        spec:
          restartPolicy: Never
          containers:
            - name: agent
              image: ghcr.io/bitop-dev/agent:latest
              args: ["-config", "/etc/agent/agent.yaml",
                     "-prompt", "Run health checks and report results."]
              env:
                - name: ANTHROPIC_API_KEY
                  valueFrom:
                    secretKeyRef:
                      name: agent-secrets
                      key: anthropic-api-key
              volumeMounts:
                - name: config
                  mountPath: /etc/agent
                - name: workspace
                  mountPath: /workspace
          volumes:
            - name: config
              configMap:
                name: agent-config
            - name: workspace
              persistentVolumeClaim:
                claimName: agent-workspace
```

---

## Scripting Patterns

### Reading Output Programmatically

```bash
#!/usr/bin/env bash
# Parse structured output from the agent

RESULT=$(agent -config agent.yaml -prompt "
  List all TODO comments in the codebase.
  Output as JSON array: [{\"file\": \"...\", \"line\": N, \"text\": \"...\"}]
  Output JSON only — no explanation.
")

# Process with jq
echo "$RESULT" | jq '.[] | select(.file | endswith("_test.go") | not)'
```

### Chaining Prompts

```bash
#!/usr/bin/env bash
# Multi-step pipeline

# Step 1: analyse
ANALYSIS=$(agent -config agent.yaml -prompt \
  "Analyse pkg/agent/loop.go and list the top 3 complexity hotspots.")

# Step 2: act on the analysis
agent -config agent.yaml -prompt \
  "Given this analysis:

$ANALYSIS

Refactor the highest-complexity function to be more readable.
Do not change behaviour."
```

### Exit Codes and Error Handling

```bash
#!/usr/bin/env bash
set -euo pipefail

run_agent() {
  local prompt="$1"
  local output
  if output=$(agent -config agent.yaml -prompt "$prompt" 2>/tmp/agent-err); then
    echo "$output"
    return 0
  else
    echo "Agent failed: $(cat /tmp/agent-err)" >&2
    return 1
  fi
}

# Use in scripts
if run_agent "Fix the failing tests in pkg/agent/"; then
  echo "Done"
else
  notify-send "Agent failed" "Check /tmp/agent-err"
  exit 1
fi
```

---

## Best Practices

### Always set `max_turns` for automation

Unattended runs must have a turn cap. Without it a broken tool feedback
loop can run indefinitely.

```yaml
max_turns: 20      # reasonable for most tasks
max_turns: 50      # long agentic tasks
max_turns: 0       # unlimited — only for interactive use
```

### Use `auto_approve: true` for unattended runs

Interactive confirmation prompts will hang a cron job or CI step.

```yaml
auto_approve: true
```

### Set a cost cap for automated runs

```yaml
# Stop if this run costs more than $0.50
# (prevents runaway tool loops from being expensive)
# Set in agent Config via MaxCostUSD when using the Go SDK
```

Currently `max_cost_usd` is a Go SDK config option (`Config.MaxCostUSD`).
For YAML support track [the issue](https://github.com/bitop-dev/agent/issues).

### Keep API keys out of config files

Use environment variables:

```yaml
api_key: ${ANTHROPIC_API_KEY}   # expanded at runtime
```

Pass them via the environment, secrets manager, or CI secrets — never
hardcode them or commit them to git. `agent.yaml` is in `.gitignore` by
default.

### Use `readonly` preset for review/analysis tasks

```yaml
tools:
  preset: readonly   # read, grep, find, ls — no writes, no bash
```

This prevents the agent from accidentally modifying files when you only
need it to analyse them.
