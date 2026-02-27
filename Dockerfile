# ── Stage 1: Go binary ───────────────────────────────────────────────────────
FROM golang:1.25-alpine AS go-builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /out/agent \
    ./cmd/agent

# ── Stage 2: Rust plugin binary ───────────────────────────────────────────────
FROM rust:alpine AS rust-builder

WORKDIR /src
COPY examples/tools/rust/ .
RUN cargo build --release --quiet

# ── Stage 3: Final image ───────────────────────────────────────────────────────
# Ubuntu 24.04 LTS — includes Python 3.12, Node.js 18, Ruby 3.2, Bash 5
FROM ubuntu:24.04

LABEL org.opencontainers.image.source="https://github.com/bitop-dev/agent"
LABEL org.opencontainers.image.description="Go LLM agent framework with polyglot plugin tools"
LABEL org.opencontainers.image.licenses="MIT"

# Install language runtimes for plugin tools
RUN apt-get update && apt-get install -y --no-install-recommends \
    # Python (stats plugin)
    python3 \
    # Node.js (json_query plugin)
    nodejs \
    # Ruby (template plugin)
    ruby \
    # Utilities
    bash \
    ca-certificates \
    curl \
    git \
    && rm -rf /var/lib/apt/lists/*

# Agent binary
COPY --from=go-builder /out/agent /usr/local/bin/agent

# Rust file_info plugin binary (pre-compiled, no Rust runtime needed)
COPY --from=rust-builder /src/target/release/file_info /usr/local/bin/file_info-plugin

# Plugin scripts — installed to /opt/agent/plugins/
RUN mkdir -p /opt/agent/plugins
COPY examples/tools/python/tool.py     /opt/agent/plugins/stats.py
COPY examples/tools/typescript/tool.mjs /opt/agent/plugins/json_query.mjs
COPY examples/tools/bash/tool.sh       /opt/agent/plugins/sys_info.sh
COPY examples/tools/ruby/tool.rb       /opt/agent/plugins/template.rb

RUN chmod +x /opt/agent/plugins/sys_info.sh

# Example config showing plugin paths as they appear inside the container
COPY agent.example.yaml /opt/agent/agent.example.yaml

# Default working directory
WORKDIR /workspace

# Config is expected at /etc/agent/agent.yaml (mount via -v or ConfigMap)
# Override with: docker run ... agent -config /path/to/config.yaml
ENTRYPOINT ["agent"]
CMD ["-config", "/etc/agent/agent.yaml"]
