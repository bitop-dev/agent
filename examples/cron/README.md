# Cron Examples

Runnable bash scripts for scheduled / unattended agent jobs.

---

## Files

| File | What it does |
|------|-------------|
| `agent-scheduled.yaml` | Config tuned for automation (`max_turns`, `auto_approve`) |
| `daily-digest.sh` | Fetch an AI news digest and email it |
| `nightly-check.sh` | Run repo health checks; open a GitHub issue on failure |
| `weekly-report.sh` | Generate a weekly code quality and activity report |

---

## Setup

### 1. Install the agent binary

```bash
# Download latest release
curl -L https://github.com/bitop-dev/agent/releases/latest/download/agent_latest_linux_amd64.tar.gz \
  | tar -xz && sudo mv agent /usr/local/bin/

# Or build from source
go install github.com/bitop-dev/agent/cmd/agent@latest
```

### 2. Copy and edit the config

```bash
cp agent-scheduled.yaml /etc/agent/scheduled.yaml
# Edit provider, model, api_key as needed
```

### 3. Make scripts executable

```bash
chmod +x daily-digest.sh nightly-check.sh weekly-report.sh
```

### 4. Set environment variables

Add to `/etc/environment` or use a secrets manager:

```bash
ANTHROPIC_API_KEY=sk-ant-...
DIGEST_EMAIL=me@example.com
PROJECT_DIR=/path/to/your/repo
NOTIFY_EMAIL=me@example.com
GITHUB_TOKEN=ghp_...
GITHUB_REPO=owner/repo
```

### 5. Add crontab entries

```bash
crontab -e
```

```cron
# Daily digest at 08:00
0 8 * * * ANTHROPIC_API_KEY=sk-ant-... /path/to/daily-digest.sh >> /var/log/agent/daily-digest.log 2>&1

# Nightly check at 02:00
0 2 * * * ANTHROPIC_API_KEY=sk-ant-... PROJECT_DIR=/srv/myapp /path/to/nightly-check.sh >> /var/log/agent/nightly.log 2>&1

# Weekly report every Monday at 09:00
0 9 * * MON ANTHROPIC_API_KEY=sk-ant-... PROJECT_DIR=/srv/myapp /path/to/weekly-report.sh >> /var/log/agent/weekly.log 2>&1
```

---

## Environment Variables

### Shared

| Variable | Default | Description |
|----------|---------|-------------|
| `ANTHROPIC_API_KEY` | — | LLM provider key (required) |
| `AGENT_BIN` | `agent` | Path to agent binary |
| `AGENT_CONFIG` | `agent-scheduled.yaml` | Path to config file |

### `daily-digest.sh`

| Variable | Default | Description |
|----------|---------|-------------|
| `DIGEST_EMAIL` | — | Recipient for email (optional) |
| `DIGEST_TOPICS` | AI/LLMs, Go, software engineering | Comma-separated topics |

### `nightly-check.sh`

| Variable | Default | Description |
|----------|---------|-------------|
| `PROJECT_DIR` | `pwd` | Repo to check |
| `GITHUB_TOKEN` | — | Opens a GH issue on failure |
| `GITHUB_REPO` | — | `owner/repo` for the issue |
| `NOTIFY_EMAIL` | — | Email address for failure alerts |

### `weekly-report.sh`

| Variable | Default | Description |
|----------|---------|-------------|
| `PROJECT_DIR` | `pwd` | Repo to analyse |
| `REPORT_OUTPUT` | `/var/log/agent` | Directory for report files |
| `NOTIFY_EMAIL` | — | Email the report |
| `GITHUB_TOKEN` | — | Post report as GitHub issue |
| `GITHUB_REPO` | — | `owner/repo` |

---

## Key Config Options for Automation

```yaml
# agent-scheduled.yaml — critical settings for unattended runs
max_turns: 20       # hard stop — prevents infinite loops
auto_approve: true  # no interactive confirmation prompts

tools:
  preset: readonly  # safe default; switch to 'coding' only if writes are needed
```

Always set `max_turns` for automated runs. Without it a broken tool feedback
loop can spin indefinitely.

---

## Running with Docker

Instead of installing the binary you can use the Docker image:

```bash
docker run --rm \
  -e ANTHROPIC_API_KEY \
  -v /etc/agent/scheduled.yaml:/etc/agent/agent.yaml \
  -v /srv/myapp:/workspace \
  ghcr.io/bitop-dev/agent:latest \
  -prompt "Run go test ./... and report any failures."
```

Crontab entry:

```cron
0 2 * * * docker run --rm -e ANTHROPIC_API_KEY -v /etc/agent/scheduled.yaml:/etc/agent/agent.yaml -v /srv/myapp:/workspace ghcr.io/bitop-dev/agent:latest -prompt "Run go test ./... and report any failures." >> /var/log/agent/nightly.log 2>&1
```
