#!/usr/bin/env node
/**
 * json_query — Node.js ESM fallback (no TypeScript compilation required).
 *
 * Run with: node tool.mjs
 *
 * For the TypeScript version: deno run tool.ts
 * (or: npx tsx tool.ts)
 */

import { createInterface } from 'node:readline';

const DEFINITION = {
  name: 'json_query',
  description:
    'Query or transform JSON data using dot-notation paths. ' +
    'Useful for extracting values from API responses, config files, or structured data. ' +
    'Supports dot-path extraction, key listing, array indexing, and pretty-printing.',
  parameters: {
    type: 'object',
    properties: {
      json: {
        type: 'string',
        description: 'JSON string to query. Must be valid JSON.',
      },
      path: {
        type: 'string',
        description:
          "Dot-notation path to extract, e.g. 'user.address.city' or 'items[0].name'. " +
          "Use '.' for the root object. Use '*' to list all keys.",
      },
      transform: {
        type: 'string',
        description:
          "Optional transformation: 'keys', 'values', 'length', 'type', 'pretty', 'compact'.",
      },
    },
    required: ['json', 'path'],
  },
};

function getPath(obj, path) {
  if (path === '.' || path === '') return obj;

  const parts = [];
  for (const segment of path.split('.')) {
    if (segment === '') continue; // leading/trailing dot
    const arrMatch = segment.match(/^(.*?)\[(\d+)\]$/);
    if (arrMatch) {
      if (arrMatch[1]) parts.push(arrMatch[1]);
      parts.push(parseInt(arrMatch[2], 10));
    } else {
      parts.push(segment);
    }
  }

  let current = obj;
  for (const part of parts) {
    if (current === null || current === undefined) {
      throw new Error(`Path segment "${part}" not found — parent is null`);
    }
    if (typeof part === 'number') {
      if (!Array.isArray(current)) throw new Error(`Cannot index non-array with [${part}]`);
      current = current[part];
    } else if (part === '*') {
      if (typeof current !== 'object' || Array.isArray(current))
        throw new Error('"*" requires an object');
      return Object.keys(current);
    } else {
      if (typeof current !== 'object' || Array.isArray(current))
        throw new Error(`Cannot get key "${part}" on ${typeof current}`);
      if (!(part in current))
        throw new Error(`Key "${part}" not found. Available: ${Object.keys(current).join(', ')}`);
      current = current[part];
    }
  }
  return current;
}

function applyTransform(value, transform) {
  switch (transform) {
    case 'keys':
      if (typeof value !== 'object' || value === null || Array.isArray(value))
        throw new Error("'keys' requires an object");
      return Object.keys(value).join('\n');
    case 'values':
      if (typeof value !== 'object' || value === null || Array.isArray(value))
        throw new Error("'values' requires an object");
      return Object.values(value).map(v => JSON.stringify(v)).join('\n');
    case 'length':
      if (typeof value === 'string' || Array.isArray(value)) return String(value.length);
      if (typeof value === 'object' && value !== null) return String(Object.keys(value).length);
      throw new Error("'length' requires a string, array, or object");
    case 'type':
      if (value === null) return 'null';
      if (Array.isArray(value)) return 'array';
      return typeof value;
    case 'pretty':
      return JSON.stringify(value, null, 2);
    case 'compact':
      return JSON.stringify(value);
    default:
      throw new Error(`Unknown transform "${transform}". Use: keys, values, length, type, pretty, compact`);
  }
}

function handleCall(params) {
  const jsonStr = params.json;
  if (typeof jsonStr !== 'string') return ['Error: "json" must be a string', true];

  let parsed;
  try { parsed = JSON.parse(jsonStr); }
  catch (e) { return [`Error: invalid JSON — ${e.message}`, true]; }

  const path = params.path ?? '.';
  let value;
  try { value = getPath(parsed, path); }
  catch (e) { return [`Error: ${e.message}`, true]; }

  const transform = params.transform;
  let result;
  if (transform) {
    try { result = applyTransform(value, transform); }
    catch (e) { return [`Error: ${e.message}`, true]; }
  } else {
    result = typeof value === 'string' ? value
           : value === null ? 'null'
           : typeof value === 'object' ? JSON.stringify(value, null, 2)
           : String(value);
  }

  return [result, false];
}

const rl = createInterface({ input: process.stdin, crlfDelay: Infinity });

rl.on('line', (line) => {
  const trimmed = line.trim();
  if (!trimmed) return;

  let msg;
  try { msg = JSON.parse(trimmed); }
  catch {
    process.stdout.write(JSON.stringify({ content: [{ type: 'text', text: 'JSON parse error' }], error: true }) + '\n');
    return;
  }

  let resp;
  if (msg.type === 'describe') {
    resp = DEFINITION;
  } else if (msg.type === 'call') {
    const [text, isError] = handleCall(msg.params ?? {});
    resp = { content: [{ type: 'text', text }], error: isError };
  } else {
    resp = { content: [{ type: 'text', text: `Unknown type: ${msg.type}` }], error: true };
  }

  process.stdout.write(JSON.stringify(resp) + '\n');
});
