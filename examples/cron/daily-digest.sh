#!/usr/bin/env bash
# daily-digest.sh — fetch an AI-generated digest of today's news and email it.
#
# Crontab (runs at 08:00 every day):
#   0 8 * * * /path/to/daily-digest.sh >> /var/log/agent/daily-digest.log 2>&1
#
# Required env vars (set in /etc/environment or a secrets manager):
#   ANTHROPIC_API_KEY   — or whichever provider you configure
#   DIGEST_EMAIL        — recipient address
#
# Optional env vars:
#   AGENT_BIN           — path to agent binary (default: agent on PATH)
#   AGENT_CONFIG        — path to config file (default: same dir as script)
#   DIGEST_TOPICS       — comma-separated topics (default: see below)

set -euo pipefail

# ── Config ────────────────────────────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AGENT_BIN="${AGENT_BIN:-agent}"
AGENT_CONFIG="${AGENT_CONFIG:-${SCRIPT_DIR}/agent-scheduled.yaml}"
DIGEST_EMAIL="${DIGEST_EMAIL:-}"
DIGEST_TOPICS="${DIGEST_TOPICS:-AI and LLMs, Go programming, software engineering}"
LOG_DIR="/var/log/agent"
DATE="$(date +%Y-%m-%d)"

# ── Validate ──────────────────────────────────────────────────────────────────
if [[ -z "${ANTHROPIC_API_KEY:-}" ]]; then
  echo "[ERROR] ANTHROPIC_API_KEY is not set" >&2
  exit 1
fi

if ! command -v "$AGENT_BIN" &>/dev/null; then
  echo "[ERROR] agent binary not found: $AGENT_BIN" >&2
  exit 1
fi

mkdir -p "$LOG_DIR"
OUTFILE="${LOG_DIR}/digest-${DATE}.txt"

echo "[$(date -Iseconds)] Starting daily digest for $DATE"

# ── Run agent ─────────────────────────────────────────────────────────────────
"$AGENT_BIN" -config "$AGENT_CONFIG" -prompt "
Search the web for the top news stories from the last 24 hours on these topics:
${DIGEST_TOPICS}

For each story:
- Title
- One-sentence summary
- Source and URL

Limit to 8 stories total. Format as plain text with clear section headers per topic.
Today's date is ${DATE}.
" > "$OUTFILE" 2>&1

echo "[$(date -Iseconds)] Digest written to $OUTFILE"

# ── Email (optional) ──────────────────────────────────────────────────────────
if [[ -n "$DIGEST_EMAIL" ]]; then
  if command -v mail &>/dev/null; then
    mail -s "Daily digest — ${DATE}" "$DIGEST_EMAIL" < "$OUTFILE"
    echo "[$(date -Iseconds)] Digest emailed to $DIGEST_EMAIL"
  else
    echo "[WARN] mail command not found — skipping email" >&2
  fi
fi

echo "[$(date -Iseconds)] Done"
