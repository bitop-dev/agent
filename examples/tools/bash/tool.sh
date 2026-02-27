#!/usr/bin/env bash
# sys_info — Bash plugin tool
#
# Collects system information: OS details, CPU, memory, disk usage,
# top processes, and environment variables.
#
# No external tools beyond standard Unix utilities (uname, df, ps, etc.)
# Works on macOS and Linux.
#
# Protocol:
#   in:  {"type":"describe"}
#   out: {"name":"sys_info","description":"...","parameters":{...}}
#
#   in:  {"type":"call","call_id":"c1","params":{"query":"disk"}}
#   out: {"content":[{"type":"text","text":"..."}],"error":false}
#
# Usage:
#   bash tool.sh
#   echo '{"type":"describe"}' | bash tool.sh

set -euo pipefail

# ── JSON helpers ────────────────────────────────────────────────────────────

# Escape a string for JSON embedding.
json_escape() {
    local s="$1"
    # Escape backslash, double-quote, newline, carriage return, tab
    s="${s//\\/\\\\}"
    s="${s//\"/\\\"}"
    s="${s//$'\n'/\\n}"
    s="${s//$'\r'/\\r}"
    s="${s//$'\t'/\\t}"
    printf '%s' "$s"
}

ok_result() {
    local text
    text="$(json_escape "$1")"
    printf '{"content":[{"type":"text","text":"%s"}],"error":false}\n' "$text"
}

err_result() {
    local text
    text="$(json_escape "$1")"
    printf '{"content":[{"type":"text","text":"%s"}],"error":true}\n' "$text"
}

# ── Tool definition ─────────────────────────────────────────────────────────

DEFINITION='{
  "name": "sys_info",
  "description": "Collect system information about the host machine. Queries include: os (OS name and version), cpu (processor info), memory (RAM usage), disk (disk usage), processes (top CPU/memory consumers), env (environment variables matching a pattern), hostname, and uptime. Use this to understand the environment the agent is running in.",
  "parameters": {
    "type": "object",
    "properties": {
      "query": {
        "type": "string",
        "description": "What to query: os | cpu | memory | disk | processes | env | hostname | uptime | all"
      },
      "env_pattern": {
        "type": "string",
        "description": "For query=env: glob pattern to filter variable names, e.g. PATH, GO*, *KEY*"
      },
      "disk_path": {
        "type": "string",
        "description": "For query=disk: path to check (default: /)"
      }
    },
    "required": ["query"]
  }
}'

# ── Query implementations ───────────────────────────────────────────────────

query_os() {
    local info=""
    if [[ "$(uname)" == "Darwin" ]]; then
        local version; version="$(sw_vers -productVersion 2>/dev/null || echo unknown)"
        local build; build="$(sw_vers -buildVersion 2>/dev/null || echo unknown)"
        info="OS: macOS ${version} (build ${build})\nKernel: $(uname -r)\nArch: $(uname -m)"
    else
        if [[ -f /etc/os-release ]]; then
            local name; name="$(. /etc/os-release && echo "$PRETTY_NAME")"
            info="OS: ${name}\nKernel: $(uname -r)\nArch: $(uname -m)"
        else
            info="OS: $(uname -s) $(uname -r)\nArch: $(uname -m)"
        fi
    fi
    printf '%b' "$info"
}

query_cpu() {
    if [[ "$(uname)" == "Darwin" ]]; then
        local brand; brand="$(sysctl -n machdep.cpu.brand_string 2>/dev/null || echo unknown)"
        local cores; cores="$(sysctl -n hw.physicalcpu 2>/dev/null || echo ?)"
        local threads; threads="$(sysctl -n hw.logicalcpu 2>/dev/null || echo ?)"
        printf 'CPU:     %s\nCores:   %s physical, %s logical' "$brand" "$cores" "$threads"
    else
        local model; model="$(grep 'model name' /proc/cpuinfo | head -1 | cut -d: -f2 | xargs || echo unknown)"
        local cores; cores="$(grep -c '^processor' /proc/cpuinfo 2>/dev/null || echo ?)"
        printf 'CPU:     %s\nThreads: %s' "$model" "$cores"
    fi
}

query_memory() {
    if [[ "$(uname)" == "Darwin" ]]; then
        local total_bytes; total_bytes="$(sysctl -n hw.memsize 2>/dev/null || echo 0)"
        local total_gb; total_gb=$(echo "scale=1; $total_bytes / 1073741824" | bc 2>/dev/null || echo "?")
        local vm_stat; vm_stat="$(vm_stat 2>/dev/null | head -10)"
        printf 'Total RAM: %s GB\n\n%s' "$total_gb" "$vm_stat"
    else
        cat /proc/meminfo 2>/dev/null | grep -E '^(MemTotal|MemFree|MemAvailable|Buffers|Cached|SwapTotal|SwapFree):' \
            || echo "Memory info unavailable"
    fi
}

query_disk() {
    local path="${1:-/}"
    df -h "$path" 2>/dev/null || echo "Disk info unavailable"
}

query_processes() {
    if [[ "$(uname)" == "Darwin" ]]; then
        ps aux | sort -k3 -rn | head -10
    else
        ps aux --sort=-%cpu | head -11
    fi
}

query_env() {
    local pattern="${1:-*}"
    env | grep -i "$pattern" 2>/dev/null | sort || echo "No variables matched: $pattern"
}

query_hostname() {
    printf 'Hostname: %s\nFQDN:     %s' "$(hostname -s 2>/dev/null || hostname)" "$(hostname 2>/dev/null)"
}

query_uptime() {
    uptime 2>/dev/null || echo "Uptime unavailable"
}

query_all() {
    printf '=== OS ===\n%s\n\n' "$(query_os)"
    printf '=== CPU ===\n%s\n\n' "$(query_cpu)"
    printf '=== HOSTNAME ===\n%s\n\n' "$(query_hostname)"
    printf '=== UPTIME ===\n%s\n\n' "$(query_uptime)"
    printf '=== MEMORY ===\n%s\n\n' "$(query_memory)"
    printf '=== DISK (/) ===\n%s\n' "$(query_disk /)"
}

# ── JSON parser (minimal, POSIX-compatible) ──────────────────────────────────

# Extract a string value from a flat JSON object.
# Uses sed with basic regex — works on macOS and Linux.
# Usage: json_get <json> <key>
json_get() {
    local json="$1" key="$2"
    # Strip to the value after "key": "..."
    printf '%s' "$json" \
        | sed 's/.*"'"$key"'"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/' \
        | grep -v '^{' 2>/dev/null \
        || true
}

# ── Main loop ────────────────────────────────────────────────────────────────

while IFS= read -r line; do
    line="${line%%$'\r'}"  # strip CR
    [[ -z "${line// }" ]] && continue

    msg_type="$(json_get "$line" "type")"

    case "$msg_type" in
        describe)
            # Compact the definition to one line
            printf '%s\n' "$(echo "$DEFINITION" | tr -d '\n' | tr -s ' ')"
            ;;

        call)
            query="$(json_get "$line" "query")"
            env_pattern="$(json_get "$line" "env_pattern")"
            disk_path="$(json_get "$line" "disk_path")"

            [[ -z "$disk_path" ]] && disk_path="/"
            [[ -z "$env_pattern" ]] && env_pattern="*"

            if [[ -z "$query" ]]; then
                err_result "Missing required parameter: query"
                continue
            fi

            result=""
            is_error=false

            case "$query" in
                os)         result="$(query_os)" ;;
                cpu)        result="$(query_cpu)" ;;
                memory|mem) result="$(query_memory)" ;;
                disk)       result="$(query_disk "$disk_path")" ;;
                processes|procs|ps)  result="$(query_processes)" ;;
                env)        result="$(query_env "$env_pattern")" ;;
                hostname)   result="$(query_hostname)" ;;
                uptime)     result="$(query_uptime)" ;;
                all)        result="$(query_all)" ;;
                *)
                    result="Unknown query: '$query'. Valid: os, cpu, memory, disk, processes, env, hostname, uptime, all"
                    is_error=true
                    ;;
            esac

            if $is_error; then
                err_result "$result"
            else
                ok_result "$result"
            fi
            ;;

        *)
            err_result "Unknown message type: $msg_type"
            ;;
    esac
done
