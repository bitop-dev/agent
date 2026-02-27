# GitHub Actions Examples

Copy these workflows into your project's `.github/workflows/` directory.

---

## Workflows

| File | Trigger | What it does |
|------|---------|-------------|
| `pr-review.yml` | Every PR open / update | Posts an AI code review as a PR comment |
| `release-notes.yml` | Push a `vX.X.X` tag | Generates and attaches release notes to the GitHub Release |
| `doc-update.yml` | Push to `main` touching `pkg/` | Auto-updates `docs/sdk.md` to match the current API |

---

## Setup

### 1. Add your API key as a secret

Go to **Settings → Secrets and variables → Actions → New repository secret**:

| Secret | Value |
|--------|-------|
| `ANTHROPIC_API_KEY` | `sk-ant-...` |

To use a different provider, also add `OPENAI_API_KEY`, `GEMINI_API_KEY`, etc.,
and update the `provider` / `model` fields in the config block inside the workflow.

### 2. Copy the workflow files

```bash
mkdir -p .github/workflows

# PR review
cp pr-review.yml .github/workflows/

# Release notes (only if you tag releases)
cp release-notes.yml .github/workflows/

# Doc auto-update (only if you want docs kept in sync automatically)
cp doc-update.yml .github/workflows/
```

### 3. Commit and push

```bash
git add .github/workflows/
git commit -m "ci: add AI-powered workflow steps"
git push
```

---

## Workflow Details

### `pr-review.yml`

- Runs on every PR open, push, or reopen
- Diffs only Go, Markdown, and YAML files against the base branch
- Posts a structured review comment (replaces previous bot comment on re-runs)
- Skips if the diff is empty

**Permissions required:** `pull-requests: write`, `contents: read`

### `release-notes.yml`

- Triggers on any `v*.*.*` tag push
- Collects commits since the previous tag
- Generates grouped release notes (Features, Bug Fixes, Other)
- Creates or updates the GitHub Release body

**Permissions required:** `contents: write`

### `doc-update.yml`

- Triggers when Go source in `pkg/` changes on `main`
- Reads exported symbols from key agent files
- Updates `docs/sdk.md` to match the current API
- Commits with `[skip ci]` to avoid triggering itself

**Permissions required:** `contents: write`

> **Tip:** For the doc update commit to trigger other workflows (e.g. a
> deploy), create a Personal Access Token and store it as `DOCS_PAT`.
> Commits from `GITHUB_TOKEN` do not trigger new workflow runs.

---

## Customising the Model

Edit the `cat > /tmp/agent.yaml` block inside any workflow:

```yaml
provider: openai-responses   # or: anthropic, google, openai-completions
model: gpt-4o                # any model from docs/models.md
api_key: ${OPENAI_API_KEY}
```

Update the secret name to match (e.g. `OPENAI_API_KEY`).

---

## Cost Considerations

| Workflow | Typical cost per run |
|----------|---------------------|
| PR review | ~$0.01–0.05 (depends on diff size) |
| Release notes | ~$0.01 |
| Doc update | ~$0.05–0.15 (reads several source files) |

Use `max_turns` and `max_tokens` in the config to cap spending.
