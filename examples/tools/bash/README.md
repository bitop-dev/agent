# sys_info — Bash Plugin Tool

Collects system information: OS details, CPU, memory, disk usage,
top processes, and environment variables.

No dependencies beyond standard Unix utilities — works on macOS and Linux.

## Requirements

Any POSIX shell (`bash`, `sh`). No packages to install.

## Running as a Plugin

```yaml
# agent.yaml
tools:
  preset: coding
  plugins:
    - path: bash
      args: ["./examples/tools/bash/tool.sh"]
```

## Testing Manually

```bash
chmod +x tool.sh

# Describe
echo '{"type":"describe"}' | bash tool.sh

# Query OS info
printf '{"type":"describe"}\n{"type":"call","call_id":"c1","params":{"query":"os"}}\n' \
  | bash tool.sh

# Query disk usage at a specific path
printf '{"type":"describe"}\n{"type":"call","call_id":"c2","params":{"query":"disk","disk_path":"/tmp"}}\n' \
  | bash tool.sh

# List environment variables matching GO*
printf '{"type":"describe"}\n{"type":"call","call_id":"c3","params":{"query":"env","env_pattern":"GO"}}\n' \
  | bash tool.sh

# All info at once
printf '{"type":"describe"}\n{"type":"call","call_id":"c4","params":{"query":"all"}}\n' \
  | bash tool.sh
```

## Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `query` | string | ✓ | What to collect (see below) |
| `env_pattern` | string | | For `query=env`: filter pattern (default: `*`) |
| `disk_path` | string | | For `query=disk`: path to check (default: `/`) |

## Queries

| Value | Returns |
|-------|---------|
| `os` | OS name, kernel version, architecture |
| `cpu` | CPU model, core count |
| `memory` | RAM total and availability |
| `disk` | Disk usage for the given path |
| `processes` | Top 10 processes by CPU usage |
| `env` | Environment variables (filtered by `env_pattern`) |
| `hostname` | Hostname and FQDN |
| `uptime` | System uptime |
| `all` | All of the above combined |
