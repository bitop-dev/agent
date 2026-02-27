# GitLab CI/CD Examples

A single `.gitlab-ci.yml` covering three AI-powered pipeline jobs.

---

## Jobs

| Job | Trigger | What it does |
|-----|---------|-------------|
| `ai:mr-review` | Every merge request | Posts an AI code review as an MR note |
| `ai:release-notes` | Push a `vX.X.X` tag | Generates and publishes GitLab Release notes |
| `ai:nightly-check` | Scheduled pipeline (`NIGHTLY=true`) | Runs health checks; opens an issue on failure |

---

## Setup

### 1. Add CI/CD Variables

Go to **Settings → CI/CD → Variables → Add variable**:

| Variable | Value | Options |
|----------|-------|---------|
| `ANTHROPIC_API_KEY` | `sk-ant-...` | ✅ Protected, ✅ Masked |
| `GITLAB_API_TOKEN` | project access token | ✅ Protected, ✅ Masked |

**Creating a Project Access Token** (for MR notes and release API calls):

1. Go to **Settings → Access Tokens → Add new token**
2. Name: `ci-agent`
3. Role: `Developer`
4. Scopes: `api`
5. Copy the token and save it as `GITLAB_API_TOKEN`

To use a different provider, change `ANTHROPIC_API_KEY` to e.g.
`OPENAI_API_KEY` and update the provider/model in the config block.

### 2. Copy `.gitlab-ci.yml` into your project

```bash
cp .gitlab-ci.yml /path/to/your/project/.gitlab-ci.yml
```

If your project already has a `.gitlab-ci.yml`, merge the relevant jobs into it.

### 3. Set up the nightly schedule

Go to **CI/CD → Schedules → New schedule**:

| Field | Value |
|-------|-------|
| Description | Nightly health check |
| Cron | `0 2 * * *` |
| Target branch | `main` |
| Variable key | `NIGHTLY` |
| Variable value | `true` |

### 4. Commit and push

```bash
git add .gitlab-ci.yml
git commit -m "ci: add AI-powered pipeline jobs"
git push
```

---

## Job Details

### `ai:mr-review`

- Only runs on merge request pipelines (`CI_PIPELINE_SOURCE == "merge_request_event"`)
- Diffs the MR branch against the target branch (Go, YAML, Markdown only)
- Deletes any previous bot review notes before posting a new one
- Skips if the diff is empty

**Required variables:** `ANTHROPIC_API_KEY`, `GITLAB_API_TOKEN`

### `ai:release-notes`

- Only runs when a tag matching `v*.*.*` is pushed
- Collects commits since the previous tag (falls back to last 30)
- Generates grouped notes (Features, Bug Fixes, Other)
- Creates or updates the GitLab Release description

**Required variables:** `ANTHROPIC_API_KEY`, `GITLAB_API_TOKEN`

### `ai:nightly-check`

- Only runs when `NIGHTLY=true` is set (via Pipeline Schedule)
- Runs `go vet`, `go test`, and `go mod tidy` check
- Asks the agent to summarise and classify the results
- Opens a GitLab issue if the status is `FAILED`
- Uploads raw output and report as job artifacts (retained 7 days)

**Required variables:** `ANTHROPIC_API_KEY`, `GITLAB_API_TOKEN`

---

## Using the Docker Image

For jobs that need plugin tools (Python, Node.js, Ruby), use the agent
Docker image directly as the job image:

```yaml
ai:mr-review:
  image: ghcr.io/bitop-dev/agent:latest
  # The agent binary is already at /usr/local/bin/agent
  # Plugin scripts are at /opt/agent/plugins/
  script:
    - agent -config ci/agent.yaml -prompt "Review the diff..."
```

This skips the `curl | tar` install step and also gives access to all
bundled plugin tools.

---

## Customising the Model

Edit the `cat > /tmp/agent.yaml` heredoc inside any job:

```yaml
provider: google
model: gemini-2.0-flash
api_key: ${GEMINI_API_KEY}
```

Update the CI variable name to match.

---

## Cost Considerations

| Job | Typical cost per run |
|-----|---------------------|
| MR review | ~$0.01–0.05 (depends on diff size) |
| Release notes | ~$0.01 |
| Nightly check | ~$0.02–0.05 |

`max_turns: 15` and `max_tokens: 4096` keep costs bounded.
