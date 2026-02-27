#!/usr/bin/env -S deno run --allow-read --allow-net
/**
 * json_query — TypeScript plugin tool (Deno)
 *
 * Extracts values from JSON using dot-notation paths and runs simple
 * transformations. No npm packages required.
 *
 * Protocol:
 *   in:  {"type":"describe"}
 *   out: {"name":"json_query","description":"...","parameters":{...}}
 *
 *   in:  {"type":"call","call_id":"c1","params":{"json":"...","path":"user.name"}}
 *   out: {"content":[{"type":"text","text":"Alice"}],"error":false}
 *
 * Usage:
 *   deno run tool.ts                        # run as plugin
 *   echo '{"type":"describe"}' | deno run tool.ts
 *
 * Alternative (Node.js + tsx):
 *   npx tsx tool.ts
 */

const DEFINITION = {
  name: "json_query",
  description:
    "Query or transform JSON data using dot-notation paths. " +
    "Useful for extracting values from API responses, config files, or structured data. " +
    "Supports dot-path extraction, key listing, array indexing, and pretty-printing.",
  parameters: {
    type: "object",
    properties: {
      json: {
        type: "string",
        description: "JSON string to query. Must be valid JSON.",
      },
      path: {
        type: "string",
        description:
          "Dot-notation path to extract, e.g. 'user.address.city' or 'items[0].name'. " +
          "Use '.' for the root object. Use '*' to list all keys at the current level.",
      },
      transform: {
        type: "string",
        description:
          "Optional transformation to apply after extraction: " +
          "'keys' (list object keys), 'values' (list values), " +
          "'length' (count array/string length), 'type' (get JSON type), " +
          "'pretty' (pretty-print JSON), 'compact' (compact JSON).",
      },
    },
    required: ["json", "path"],
  },
};

type JsonValue =
  | string
  | number
  | boolean
  | null
  | JsonValue[]
  | { [key: string]: JsonValue };

/** Extract a value from a nested object using a dot-notation path. */
function getPath(obj: JsonValue, path: string): JsonValue {
  if (path === "." || path === "") return obj;

  // Split on dots, handling array index notation like items[0]
  const parts: (string | number)[] = [];
  for (const segment of path.split(".")) {
    if (segment === "") continue; // leading/trailing dot
    const arrMatch = segment.match(/^(.*?)\[(\d+)\]$/);
    if (arrMatch) {
      if (arrMatch[1]) parts.push(arrMatch[1]);
      parts.push(parseInt(arrMatch[2], 10));
    } else {
      parts.push(segment);
    }
  }

  let current: JsonValue = obj;
  for (const part of parts) {
    if (current === null || current === undefined) {
      throw new Error(`Path segment "${part}" not found — parent is null`);
    }
    if (typeof part === "number") {
      if (!Array.isArray(current)) {
        throw new Error(`Cannot index non-array with [${part}]`);
      }
      current = current[part];
    } else if (part === "*") {
      if (typeof current !== "object" || Array.isArray(current)) {
        throw new Error(`"*" requires an object, got ${Array.isArray(current) ? "array" : typeof current}`);
      }
      return Object.keys(current);
    } else {
      if (typeof current !== "object" || Array.isArray(current)) {
        throw new Error(`Cannot get key "${part}" on ${Array.isArray(current) ? "array" : typeof current}`);
      }
      if (!(part in current)) {
        throw new Error(`Key "${part}" not found. Available: ${Object.keys(current).join(", ")}`);
      }
      current = (current as { [k: string]: JsonValue })[part];
    }
  }
  return current;
}

function applyTransform(value: JsonValue, transform: string): string {
  switch (transform) {
    case "keys":
      if (typeof value !== "object" || value === null || Array.isArray(value)) {
        throw new Error("'keys' requires an object");
      }
      return Object.keys(value).join("\n");

    case "values":
      if (typeof value !== "object" || value === null || Array.isArray(value)) {
        throw new Error("'values' requires an object");
      }
      return Object.values(value).map((v) => JSON.stringify(v)).join("\n");

    case "length":
      if (typeof value === "string" || Array.isArray(value)) {
        return String(value.length);
      }
      if (typeof value === "object" && value !== null) {
        return String(Object.keys(value).length);
      }
      throw new Error("'length' requires a string, array, or object");

    case "type":
      if (value === null) return "null";
      if (Array.isArray(value)) return "array";
      return typeof value;

    case "pretty":
      return JSON.stringify(value, null, 2);

    case "compact":
      return JSON.stringify(value);

    default:
      throw new Error(`Unknown transform "${transform}". Use: keys, values, length, type, pretty, compact`);
  }
}

function handleCall(params: Record<string, JsonValue>): [string, boolean] {
  const jsonStr = params["json"];
  if (typeof jsonStr !== "string") {
    return ["Error: 'json' must be a string", true];
  }

  let parsed: JsonValue;
  try {
    parsed = JSON.parse(jsonStr);
  } catch (e) {
    return [`Error: invalid JSON — ${(e as Error).message}`, true];
  }

  const path = (params["path"] as string) ?? ".";
  let value: JsonValue;
  try {
    value = getPath(parsed, path);
  } catch (e) {
    return [`Error: ${(e as Error).message}`, true];
  }

  const transform = params["transform"] as string | undefined;
  let result: string;
  if (transform) {
    try {
      result = applyTransform(value, transform);
    } catch (e) {
      return [`Error: ${(e as Error).message}`, true];
    }
  } else {
    // Auto-format: primitives as-is, objects/arrays as pretty JSON
    if (typeof value === "string") {
      result = value;
    } else if (value === null) {
      result = "null";
    } else if (typeof value === "object" || Array.isArray(value)) {
      result = JSON.stringify(value, null, 2);
    } else {
      result = String(value);
    }
  }

  return [result, false];
}

async function main(): Promise<void> {
  const decoder = new TextDecoder();
  const encoder = new TextEncoder();
  const nl = encoder.encode("\n");

  let buffer = "";

  for await (const chunk of Deno.stdin.readable) {
    buffer += decoder.decode(chunk);
    const lines = buffer.split("\n");
    buffer = lines.pop() ?? ""; // keep incomplete last line

    for (const line of lines) {
      const trimmed = line.trim();
      if (!trimmed) continue;

      let msg: Record<string, JsonValue>;
      try {
        msg = JSON.parse(trimmed);
      } catch {
        const resp = { content: [{ type: "text", text: "JSON parse error" }], error: true };
        await Deno.stdout.write(encoder.encode(JSON.stringify(resp)));
        await Deno.stdout.write(nl);
        continue;
      }

      let resp: unknown;
      if (msg["type"] === "describe") {
        resp = DEFINITION;
      } else if (msg["type"] === "call") {
        const params = (msg["params"] as Record<string, JsonValue>) ?? {};
        const [text, isError] = handleCall(params);
        resp = { content: [{ type: "text", text }], error: isError };
      } else {
        resp = {
          content: [{ type: "text", text: `Unknown message type: ${msg["type"]}` }],
          error: true,
        };
      }

      await Deno.stdout.write(encoder.encode(JSON.stringify(resp)));
      await Deno.stdout.write(nl);
    }
  }
}

main().catch((e) => {
  console.error(e);
  Deno.exit(1);
});
