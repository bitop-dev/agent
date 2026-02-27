# stats — Python Plugin Tool

Computes descriptive statistics on a list of numbers.

## Requirements

Python 3.9+. No third-party packages needed.

## Running as a Plugin

```yaml
# agent.yaml
tools:
  preset: coding
  plugins:
    - path: python3
      args: ["./examples/tools/python/tool.py"]
```

## Testing Manually

```bash
# Test describe
echo '{"type":"describe"}' | python3 tool.py

# Test a call
printf '{"type":"describe"}\n{"type":"call","call_id":"c1","params":{"numbers":[2,4,4,4,5,5,7,9],"percentiles":[25,75]}}\n' \
  | python3 tool.py
```

Expected output:

```
{"name": "stats", "description": "...", "parameters": {...}}
{"content": [{"type": "text", "text": "n        = 8\nmin      = 2.0000\n..."}], "error": false}
```

## What the LLM Sees

When the agent calls `stats`, the tool returns:

```
n        = 8
min      = 2.0000
max      = 9.0000
mean     = 5.0000
median   = 4.5000
std_dev  = 2.0000
p25      = 4.0000
p75      = 5.5000
```

## Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `numbers` | array | ✓ | List of numeric values |
| `precision` | integer | | Decimal places (default: 4) |
| `percentiles` | array | | Percentiles to compute, e.g. `[25, 50, 75, 95]` |
