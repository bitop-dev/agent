# file_info — Rust Plugin Tool

Detailed metadata and statistics about files and directories:
file size, MIME type guess, line/word/character counts; directory
listing with sizes.

Zero external crates — pure Rust standard library.

## Requirements

Rust toolchain: https://rustup.rs

```bash
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
```

## Build

```bash
cd examples/tools/rust
cargo build --release
# Binary: ./target/release/file_info
```

## Running as a Plugin

```yaml
# agent.yaml
tools:
  preset: coding
  plugins:
    - path: ./examples/tools/rust/target/release/file_info
```

## Testing Manually

```bash
# Describe
echo '{"type":"describe"}' | ./target/release/file_info

# File info
printf '{"type":"describe"}\n{"type":"call","call_id":"c1","params":{"path":"src/main.rs"}}\n' \
  | ./target/release/file_info

# Directory listing
printf '{"type":"describe"}\n{"type":"call","call_id":"c2","params":{"path":"src","max_entries":10}}\n' \
  | ./target/release/file_info

# During development (no build step)
echo '{"type":"describe"}' | cargo run -q
```

## What the LLM Sees

For a file:

```
path    : src/main.rs
type    : file
size    : 12401 (12.1 KB)
mime    : text/x-rust
modified: 1700000000s since epoch
lines   : 312
words   : 1024
chars   : 12401
```

For a directory:

```
path     : src
type     : directory
entries  : 3
total    : 37.2 KB
modified : 1700000000s since epoch

  main.rs                                          12.1 KB
  lib.rs                                           25.0 KB
  tests/                                              0 B
```

## Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `path` | string | ✓ | File or directory to inspect |
| `max_entries` | integer | | Max directory entries shown (default: 50) |
