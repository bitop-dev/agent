#!/usr/bin/env bash
# nightly-check.sh — run repo health checks and open a GitHub issue on failure.
#
# Crontab (runs at 02:00 every day):
#   0 2 * * * /path/to/nightly-check.sh >> /var/log/agent/nightly-check.log 2>&1
#
# Required env vars:
#   ANTHROPIC_API_KEY   — LLM provider key
#   PROJECT_DIR         — absolute path to the repo to check
#
# Optional env vars:
#   AGENT_BIN           — path to agent binary (default: agent on PATH)
#   AGENT_CONFIG        — path to config file (default: same dir as script)
#   GITHUB_TOKEN        — if set, opens a GH issue on failure (requires gh CLI)
#   GITHUB_REPO         — owner/repo, e.g. bitop-dev/agent (required if using GH)
#   NOTIFY_EMAIL        — email address to notify on failure
#   CHECK_COMMANDS      — space-separated commands to run (default: see below)

set -euo pipefail

# ── Config ────────────────────────────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AGENT_BIN="${AGENT_BIN:-agent}"
AGENT_CONFIG="${AGENT_CONFIG:-${SCRIPT_DIR}/agent-scheduled.yaml}"
PROJECT_DIR="${PROJECT_DIR:-$(pwd)}"
LOG_DIR="/var/log/agent"
DATE="$(date +%Y-%m-%d)"
TIMESTAMP="$(date -Iseconds)"

# ── Validate ──────────────────────────────────────────────────────────────────
if [[ -z "${ANTHROPIC_API_KEY:-}" ]]; then
  echo "[ERROR] ANTHROPIC_API_KEY is not set" >&2
  exit 1
fi

if [[ ! -d "$PROJECT_DIR" ]]; then
  echo "[ERROR] PROJECT_DIR does not exist: $PROJECT_DIR" >&2
  exit 1
fi

mkdir -p "$LOG_DIR"
RAW_OUTPUT="${LOG_DIR}/nightly-raw-${DATE}.txt"
REPORT="${LOG_DIR}/nightly-report-${DATE}.txt"

echo "[$TIMESTAMP] Starting nightly check on $PROJECT_DIR"

# ── Gather raw check output ───────────────────────────────────────────────────
cd "$PROJECT_DIR"
{
  echo "=== go vet ==="
  go vet ./... 2>&1 || true

  echo ""
  echo "=== go test ==="
  go test -timeout 120s ./... 2>&1 || true

  echo ""
  echo "=== go mod tidy check ==="
  cp go.sum go.sum.bak
  go mod tidy 2>&1 || true
  if ! diff -q go.sum go.sum.bak &>/dev/null; then
    echo "WARN: go.sum changed after go mod tidy — run 'go mod tidy' and commit"
  else
    echo "go.sum is clean"
  fi
  mv go.sum.bak go.sum

} > "$RAW_OUTPUT" 2>&1

echo "[$TIMESTAMP] Raw output written to $RAW_OUTPUT"

# ── Let the agent summarise and assess ───────────────────────────────────────
"$AGENT_BIN" -config "$AGENT_CONFIG" -prompt "
Analyse this CI output from a nightly health check of a Go repository.

$(cat "$RAW_OUTPUT")

Respond with exactly one of:
  STATUS: HEALTHY
  STATUS: FAILED

Then a blank line, then a concise summary (bullet points) of:
- What passed
- What failed (with file/line if available)
- Recommended fix for each failure

Be brief — this goes into a GitHub issue or notification email.
" > "$REPORT" 2>&1

echo "[$TIMESTAMP] Report written to $REPORT"

# ── Check status ──────────────────────────────────────────────────────────────
STATUS="HEALTHY"
if grep -q "STATUS: FAILED" "$REPORT"; then
  STATUS="FAILED"
fi

echo "[$TIMESTAMP] Status: $STATUS"

if [[ "$STATUS" == "HEALTHY" ]]; then
  echo "[$TIMESTAMP] All checks passed — nothing to report"
  exit 0
fi

# ── Notify on failure ─────────────────────────────────────────────────────────
REPORT_BODY="$(cat "$REPORT")"

# GitHub issue
if [[ -n "${GITHUB_TOKEN:-}" && -n "${GITHUB_REPO:-}" ]]; then
  if command -v gh &>/dev/null; then
    gh issue create \
      --repo "$GITHUB_REPO" \
      --title "Nightly check failed — ${DATE}" \
      --body "$REPORT_BODY" \
      --label "automated,bug" \
      2>&1 && echo "[$TIMESTAMP] GitHub issue created"
  else
    echo "[WARN] gh CLI not found — skipping GitHub issue" >&2
  fi
fi

# Email
if [[ -n "${NOTIFY_EMAIL:-}" ]]; then
  if command -v mail &>/dev/null; then
    echo "$REPORT_BODY" | mail -s "Nightly check FAILED — ${DATE}" "$NOTIFY_EMAIL"
    echo "[$TIMESTAMP] Failure email sent to $NOTIFY_EMAIL"
  fi
fi

echo "[$TIMESTAMP] Done (with failures)"
exit 1
