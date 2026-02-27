# Installation

Four ways to get the agent running, from fastest to most flexible.

---

## 1. Download a release binary (recommended)

Pre-built binaries for Linux, macOS, and Windows are attached to every
[GitHub release](https://github.com/bitop-dev/agent/releases).

```bash
# macOS (Apple Silicon)
curl -L https://github.com/bitop-dev/agent/releases/latest/download/agent_latest_darwin_arm64.tar.gz \
  | tar -xz && mv agent /usr/local/bin/

# macOS (Intel)
curl -L https://github.com/bitop-dev/agent/releases/latest/download/agent_latest_darwin_amd64.tar.gz \
  | tar -xz && mv agent /usr/local/bin/

# Linux (amd64)
curl -L https://github.com/bitop-dev/agent/releases/latest/download/agent_latest_linux_amd64.tar.gz \
  | tar -xz && mv agent /usr/local/bin/

# Linux (arm64)
curl -L https://github.com/bitop-dev/agent/releases/latest/download/agent_latest_linux_arm64.tar.gz \
  | tar -xz && mv agent /usr/local/bin/

# Windows — download agent_latest_windows_amd64.zip from the releases page
```

Verify:

```bash
agent -help
```

---

## 2. Install with `go install`

Requires Go 1.25+.

```bash
go install github.com/bitop-dev/agent/cmd/agent@latest
```

The binary is placed in `$(go env GOPATH)/bin`. Make sure that directory is
in your `PATH`:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

To install a specific version:

```bash
go install github.com/bitop-dev/agent/cmd/agent@v0.1.0
```

---

## 3. Build from source

```bash
git clone https://github.com/bitop-dev/agent.git
cd agent
go build -o agent ./cmd/agent

# Optional: install to PATH
sudo mv agent /usr/local/bin/
```

Run tests before using:

```bash
go test ./...
```

---

## 4. Docker

The Docker image includes the agent binary plus all runtimes needed for the
bundled plugin tools (Python 3.12, Node.js 18, Ruby 3.2).

```bash
docker pull ghcr.io/bitop-dev/agent:latest
```

The agent requires a config file. Mount one at `/etc/agent/agent.yaml`:

```bash
docker run --rm -it \
  -v $(pwd)/agent.yaml:/etc/agent/agent.yaml \
  -v $(pwd):/workspace \
  ghcr.io/bitop-dev/agent:latest
```

One-shot prompt:

```bash
docker run --rm \
  -v $(pwd)/agent.yaml:/etc/agent/agent.yaml \
  -v $(pwd):/workspace \
  ghcr.io/bitop-dev/agent:latest \
  -prompt "Summarise this repository."
```

Pass the API key as an environment variable instead of hardcoding it in the
config file:

```bash
docker run --rm -it \
  -e ANTHROPIC_API_KEY=sk-ant-... \
  -v $(pwd)/agent.yaml:/etc/agent/agent.yaml \
  -v $(pwd):/workspace \
  ghcr.io/bitop-dev/agent:latest
```

### Docker config for plugin tools

Inside the container, plugin scripts live at `/opt/agent/plugins/`. A
container-ready config looks like:

```yaml
provider: anthropic
model: claude-sonnet-4-5
api_key: ${ANTHROPIC_API_KEY}
max_tokens: 4096

tools:
  preset: coding
  work_dir: /workspace
  plugins:
    - path: python3
      args: ["/opt/agent/plugins/stats.py"]
    - path: node
      args: ["/opt/agent/plugins/json_query.mjs"]
    - path: bash
      args: ["/opt/agent/plugins/sys_info.sh"]
    - path: ruby
      args: ["/opt/agent/plugins/template.rb"]
    - path: /usr/local/bin/file_info-plugin
```

### Specific versions

```bash
docker pull ghcr.io/bitop-dev/agent:v0.1.0
docker pull ghcr.io/bitop-dev/agent:0.1      # latest 0.1.x patch
docker pull ghcr.io/bitop-dev/agent:latest
```

### Docker Compose

```yaml
services:
  agent:
    image: ghcr.io/bitop-dev/agent:latest
    volumes:
      - ./agent.yaml:/etc/agent/agent.yaml
      - .:/workspace
    environment:
      - ANTHROPIC_API_KEY=${ANTHROPIC_API_KEY}
    working_dir: /workspace
    stdin_open: true
    tty: true
```

```bash
docker compose run agent
docker compose run agent -prompt "List the Go files here."
```

---

## First config

Once installed, create a config file:

```bash
cat > agent.yaml << 'EOF'
provider: anthropic
model: claude-sonnet-4-5
api_key: ${ANTHROPIC_API_KEY}
max_tokens: 4096

tools:
  preset: coding
  work_dir: .
EOF
```

Export your API key and run:

```bash
export ANTHROPIC_API_KEY=sk-ant-...
agent -config agent.yaml
```

See [config.md](config.md) for the full YAML reference and
[providers.md](providers.md) for other provider options.

---

## Updating

**Binary:** re-download or re-run the install command with `@latest`.

**Go install:**

```bash
go install github.com/bitop-dev/agent/cmd/agent@latest
```

**Docker:**

```bash
docker pull ghcr.io/bitop-dev/agent:latest
```

**Source:**

```bash
cd agent && git pull && go build -o agent ./cmd/agent
```
