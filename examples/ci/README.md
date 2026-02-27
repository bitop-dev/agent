# CI/CD Examples

AI-powered pipeline examples for GitHub Actions and GitLab CI/CD.

---

## Platforms

| Directory | Platform | Jobs |
|-----------|----------|------|
| [`github-actions/`](github-actions/) | GitHub Actions | PR review, release notes, doc auto-update |
| [`gitlab/`](gitlab/) | GitLab CI/CD | MR review, release notes, nightly health check |

---

## Quick Start

### GitHub Actions

```bash
mkdir -p .github/workflows

# Copy whichever workflows you need
cp github-actions/pr-review.yml      .github/workflows/
cp github-actions/release-notes.yml  .github/workflows/
cp github-actions/doc-update.yml     .github/workflows/
```

Add `ANTHROPIC_API_KEY` in **Settings → Secrets and variables → Actions**.

### GitLab CI/CD

```bash
cp gitlab/.gitlab-ci.yml ./.gitlab-ci.yml
```

Add `ANTHROPIC_API_KEY` and `GITLAB_API_TOKEN` in **Settings → CI/CD → Variables**.

---

## Feature Comparison

| Feature | GitHub Actions | GitLab CI/CD |
|---------|---------------|-------------|
| PR/MR review | ✅ `pr-review.yml` | ✅ `ai:mr-review` |
| Release notes | ✅ `release-notes.yml` | ✅ `ai:release-notes` |
| Doc auto-update | ✅ `doc-update.yml` | — |
| Nightly health check | — | ✅ `ai:nightly-check` |
| Docker image support | ✅ | ✅ |

---

## Common Config Options

Both platforms use the same agent YAML config. The key options for CI:

```yaml
max_turns: 15       # hard stop — prevents runaway loops
auto_approve: true  # no interactive prompts in CI

tools:
  preset: readonly  # safe default for review/analysis jobs
  # preset: coding  # only when the job needs to write files
```

See [`docs/config.md`](../../docs/config.md) for the full reference.
