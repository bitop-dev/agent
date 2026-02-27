#!/usr/bin/env python3
"""stats — descriptive statistics plugin tool.

Implements the agent plugin protocol over stdin/stdout.
No third-party dependencies — uses Python stdlib only.

Protocol:
  in:  {"type":"describe"}
  out: {"name":"stats","description":"...","parameters":{...}}

  in:  {"type":"call","call_id":"c1","params":{"numbers":[1,2,3,4,5]}}
  out: {"content":[{"type":"text","text":"..."}],"error":false}

Usage:
  python3 tool.py           # run as plugin (reads from stdin)
  echo '{"type":"describe"}' | python3 tool.py
"""

import json
import math
import sys


DEFINITION = {
    "name": "stats",
    "description": (
        "Compute descriptive statistics on a list of numbers. "
        "Returns count, min, max, mean, median, and standard deviation. "
        "Use this whenever you need to summarise or analyse numeric data."
    ),
    "parameters": {
        "type": "object",
        "properties": {
            "numbers": {
                "type": "array",
                "description": "List of numeric values to analyse",
                "items": {"type": "number"},
            },
            "precision": {
                "type": "integer",
                "description": "Decimal places in output (default: 4)",
            },
            "percentiles": {
                "type": "array",
                "description": "Percentile values to compute, e.g. [25, 75, 95]",
                "items": {"type": "number"},
            },
        },
        "required": ["numbers"],
    },
}


def compute_stats(numbers: list[float], precision: int, percentiles: list[float]) -> str:
    n = len(numbers)
    if n == 0:
        return "Error: empty list"

    sorted_nums = sorted(numbers)
    total = sum(numbers)
    mean = total / n

    # Median
    mid = n // 2
    if n % 2 == 0:
        median = (sorted_nums[mid - 1] + sorted_nums[mid]) / 2.0
    else:
        median = sorted_nums[mid]

    # Standard deviation (population)
    variance = sum((x - mean) ** 2 for x in numbers) / n
    stddev = math.sqrt(variance)

    fmt = f".{precision}f"

    lines = [
        f"n        = {n}",
        f"min      = {sorted_nums[0]:{fmt}}",
        f"max      = {sorted_nums[-1]:{fmt}}",
        f"mean     = {mean:{fmt}}",
        f"median   = {median:{fmt}}",
        f"std_dev  = {stddev:{fmt}}",
    ]

    # Percentiles
    for p in sorted(percentiles):
        if not (0 <= p <= 100):
            continue
        idx = (p / 100) * (n - 1)
        lo = int(idx)
        hi = min(lo + 1, n - 1)
        frac = idx - lo
        val = sorted_nums[lo] * (1 - frac) + sorted_nums[hi] * frac
        lines.append(f"p{int(p):<6}  = {val:{fmt}}")

    return "\n".join(lines)


def handle_call(params: dict) -> tuple[str, bool]:
    """Returns (text_result, is_error)."""
    raw = params.get("numbers")
    if not isinstance(raw, list) or len(raw) == 0:
        return "Error: 'numbers' must be a non-empty array", True

    try:
        numbers = [float(x) for x in raw]
    except (TypeError, ValueError) as e:
        return f"Error: non-numeric value in numbers: {e}", True

    precision = int(params.get("precision", 4))
    precision = max(0, min(precision, 10))

    raw_pcts = params.get("percentiles", [])
    try:
        percentiles = [float(p) for p in raw_pcts]
    except (TypeError, ValueError):
        percentiles = []

    result = compute_stats(numbers, precision, percentiles)
    return result, False


def main() -> None:
    # Use unbuffered output so JSON lines flush immediately.
    out = sys.stdout

    for raw_line in sys.stdin:
        raw_line = raw_line.strip()
        if not raw_line:
            continue

        try:
            msg = json.loads(raw_line)
        except json.JSONDecodeError as e:
            # Write an error result and continue.
            resp = {"content": [{"type": "text", "text": f"JSON parse error: {e}"}], "error": True}
            print(json.dumps(resp), file=out, flush=True)
            continue

        msg_type = msg.get("type")

        if msg_type == "describe":
            print(json.dumps(DEFINITION), file=out, flush=True)

        elif msg_type == "call":
            params = msg.get("params", {})
            text, is_error = handle_call(params)
            resp = {
                "content": [{"type": "text", "text": text}],
                "error": is_error,
            }
            print(json.dumps(resp), file=out, flush=True)

        else:
            resp = {
                "content": [{"type": "text", "text": f"Unknown message type: {msg_type!r}"}],
                "error": True,
            }
            print(json.dumps(resp), file=out, flush=True)


if __name__ == "__main__":
    main()
