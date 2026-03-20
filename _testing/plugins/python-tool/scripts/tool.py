#!/usr/bin/env python3
"""Example command-runtime plugin script using JSON-stdin/stdout protocol.

Reads a JSON request from stdin:
  {"plugin": "...", "tool": "...", "operation": "...", "arguments": {...}, "config": {...}}

Writes a JSON response to stdout:
  {"output": "...", "data": {...}}
  or {"error": "..."}
"""

import json
import sys


def word_count(arguments):
    text = arguments.get("text", "")
    words = text.split()
    count = len(words)
    return {
        "output": f"{count} words",
        "data": {"count": count, "words": words},
    }


def main():
    raw = sys.stdin.read()
    try:
        request = json.loads(raw)
    except json.JSONDecodeError as e:
        json.dump({"error": f"invalid JSON input: {e}"}, sys.stdout)
        return

    operation = request.get("operation", "")
    arguments = request.get("arguments", {})

    handlers = {
        "word-count": word_count,
    }

    handler = handlers.get(operation)
    if handler is None:
        json.dump({"error": f"unknown operation: {operation}"}, sys.stdout)
        return

    try:
        result = handler(arguments)
        json.dump(result, sys.stdout)
    except Exception as e:
        json.dump({"error": str(e)}, sys.stdout)


if __name__ == "__main__":
    main()
