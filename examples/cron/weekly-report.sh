#!/usr/bin/env bash
# weekly-report.sh — generate a weekly code quality and activity report.
#
# Crontab (runs at 09:00 every Monday):
#   0 9 * * MON /path/to/weekly-report.sh >> /var/log/agent/weekly-report.log 2>&1
#
# Required env vars:
#   ANTHROPIC_API_KEY
#   PROJECT_DIR         — absolute path to the repo
#
# Optional env vars:
#   AGENT_BIN, AGENT_CONFIG, NOTIFY_EMAIL, GITHUB_TOKEN, GITHUB_REPO
#   REPORT_OUTPUT       — directory to write HTML exports (default: /var/log/agent)

set -euo pipefail

# ── Config ────────────────────────────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AGENT_BIN="${AGENT_BIN:-agent}"
AGENT_CONFIG="${AGENT_CONFIG:-${SCRIPT_DIR}/agent-scheduled.yaml}"
PROJECT_DIR="${PROJECT_DIR:-$(pwd)}"
REPORT_OUTPUT="${REPORT_OUTPUT:-/var/log/agent}"
WEEK="week-$(date +%Y-%W)"
DATE="$(date +%Y-%m-%d)"

mkdir -p "$REPORT_OUTPUT"
REPORT="${REPORT_OUTPUT}/weekly-${WEEK}.txt"

echo "[$(date -Iseconds)] Starting weekly report for $WEEK"

cd "$PROJECT_DIR"

# ── Gather context ────────────────────────────────────────────────────────────
GIT_LOG=$(git log --since="7 days ago" --oneline --no-merges 2>/dev/null || echo "no git history available")
GIT_AUTHORS=$(git log --since="7 days ago" --format="%an" 2>/dev/null | sort | uniq -c | sort -rn | head -10 || echo "")
FILE_COUNT=$(find . -name "*.go" -not -path "*/vendor/*" | wc -l | tr -d ' ')
TEST_COUNT=$(grep -r "^func Test" --include="*.go" . | wc -l | tr -d ' ' || echo "0")

# ── Run agent ─────────────────────────────────────────────────────────────────
"$AGENT_BIN" -config "$AGENT_CONFIG" -cwd "$PROJECT_DIR" -prompt "
Generate a weekly engineering report for the repository at $(pwd).

Context gathered automatically:
--- Git log (last 7 days) ---
${GIT_LOG}

--- Top contributors (last 7 days) ---
${GIT_AUTHORS}

--- Repo stats ---
Go source files: ${FILE_COUNT}
Test functions:  ${TEST_COUNT}

Using the read, grep, and find tools, also:
1. Identify the 3 most recently modified packages and summarise their changes
2. Check for any TODO or FIXME comments added in the last week
3. Note any files with more than 300 lines (potential complexity)

Format the final report as:
## Weekly Engineering Report — ${DATE}

### Summary
(2-3 sentence overview)

### Commits This Week
(bullet list)

### Notable Changes
(by package)

### Items to Watch
(TODOs, large files, concerns)

### Metrics
(file counts, test counts, etc.)
" > "$REPORT" 2>&1

echo "[$(date -Iseconds)] Report written to $REPORT"

# ── Email ─────────────────────────────────────────────────────────────────────
if [[ -n "${NOTIFY_EMAIL:-}" ]] && command -v mail &>/dev/null; then
  mail -s "Weekly engineering report — ${WEEK}" "$NOTIFY_EMAIL" < "$REPORT"
  echo "[$(date -Iseconds)] Report emailed to $NOTIFY_EMAIL"
fi

# ── GitHub discussion / issue ─────────────────────────────────────────────────
if [[ -n "${GITHUB_TOKEN:-}" && -n "${GITHUB_REPO:-}" ]] && command -v gh &>/dev/null; then
  gh issue create \
    --repo "$GITHUB_REPO" \
    --title "Weekly report — ${WEEK}" \
    --body "$(cat "$REPORT")" \
    --label "automated,report" \
    2>&1 && echo "[$(date -Iseconds)] GitHub issue created"
fi

echo "[$(date -Iseconds)] Done"
