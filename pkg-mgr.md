# Agent Package Manager — Planning Doc

> Status: **Planning** — not yet built  
> Inspiration: npm, Go modules, Homebrew

---

## Vision

A package manager for the agent ecosystem. Developers publish tools, skills,
prompt templates, and config presets to a central registry. Users install them
with a single command and they are immediately available to any agent session.

```bash
agent install @nickcecere/web-tools
agent install @company/internal-tools --registry https://pkg.company.com
agent search "github"
agent publish
```

---

## Core Concepts

### Package Types

| Type | Description | Example |
|------|-------------|---------|
| `tool` | Subprocess plugin (any language) | Python web scraper, Rust formatter |
| `skill` | SKILL.md injected into system prompt | go-expert, sql-expert |
| `prompt` | Reusable prompt template | review.md, explain.md |
| `config` | Shareable agent.yaml preset | coding-agent, web-researcher |
| `bundle` | Named collection of any of the above | full-stack-dev (tools + skills + prompts) |

A single package can provide any mix of these types.

---

## Package Manifest — `agent-package.yaml`

Analogous to `package.json`.

```yaml
name: "@nickcecere/web-tools"
version: "1.2.0"
description: "Web search, fetch, and scraping tools for agent"
author: "Nick Cecere <nick@example.com>"
license: MIT
homepage: "https://github.com/nickcecere/agent-web-tools"
repository: "github.com/nickcecere/agent-web-tools"
keywords: [web, search, scraping, fetch]

# Runtime requirements (agent enforces these at install time)
requires:
  agent: ">=0.1.0"
  python: ">=3.9"      # if any tool uses python
  node: ">=18"         # if any tool uses node

# What this package provides
provides:
  tools:
    - name: web_search
      type: python
      path: tools/web_search.py
      description: "Search the web using DuckDuckGo"

    - name: web_fetch
      type: bash
      path: tools/web_fetch.sh
      description: "Fetch and clean a web page"

    - name: web_screenshot
      type: node
      path: tools/screenshot.mjs
      description: "Take a screenshot of a URL (requires puppeteer)"
      requires:
        node: ">=18"
        npm_packages: [puppeteer]   # installed automatically

  skills:
    - name: web-researcher
      path: skills/web-researcher/SKILL.md

  prompts:
    - name: summarize-page
      path: prompts/summarize-page.md

# Package-level dependencies (other agent packages)
dependencies:
  "@agent/http-utils": "^1.0.0"

# Lifecycle hooks (optional scripts run by the package manager)
hooks:
  post_install: scripts/post-install.sh    # e.g. pip install -r requirements.txt
  pre_remove: scripts/pre-remove.sh
```

---

## Lock File — `agent.lock`

Analogous to `package-lock.json`. Pinned versions and checksums.
Committed to version control for reproducibility.

```yaml
lockfile_version: 1
generated: "2026-02-27T18:00:00Z"

packages:
  "@nickcecere/web-tools@1.2.0":
    resolved: "https://registry.agent.dev/@nickcecere/web-tools/-/web-tools-1.2.0.tar.gz"
    integrity: "sha256-abc123..."
    requires:
      python: ">=3.9"

  "@agent/http-utils@1.0.3":
    resolved: "https://registry.agent.dev/@agent/http-utils/-/http-utils-1.0.3.tar.gz"
    integrity: "sha256-def456..."
```

---

## Install Location

### Global (user-wide, like `npm install -g`)

```
~/.config/agent/
  packages/
    @nickcecere/
      web-tools/
        agent-package.yaml
        tools/
          web_search.py
          web_fetch.sh
        skills/
          web-researcher/
            SKILL.md
        prompts/
          summarize-page.md
  agent.lock
  installed.yaml       ← index of globally installed packages
```

### Project-local (like `node_modules/`)

```
<project>/
  .agent/
    packages/
      @nickcecere/
        web-tools/
          ...
  agent.lock
```

Project-local packages take precedence over global. Config `agent.yaml`
can reference both:

```yaml
packages:
  - "@nickcecere/web-tools"        # installed locally or globally
  - "@company/internal-tools"      # from private registry

# Or auto-load everything installed
packages: auto
```

---

## CLI Interface

Modelled on npm. New `agent` subcommands:

```bash
# Install
agent install @scope/package              # latest from registry
agent install @scope/package@1.2.0        # pinned version
agent install @scope/package@^1.0.0       # semver range
agent install -g @scope/package           # global install
agent install ./path/to/local-package     # local (dev/testing)
agent install                             # install from agent.lock

# Remove
agent remove @scope/package
agent remove -g @scope/package

# Update
agent update                              # update all to latest compatible
agent update @scope/package              # update one package

# Info
agent list                                # list installed packages
agent list -g                             # list globally installed
agent search "web scraping"              # search registry
agent info @scope/package                # show package details
agent outdated                            # show packages with newer versions

# Publishing
agent pkg init                            # scaffold a new package (interactive)
agent pkg validate                        # validate agent-package.yaml
agent pkg pack                            # create a .tar.gz tarball locally
agent pkg publish                         # publish to registry
agent pkg publish --tag beta             # publish under a dist-tag
agent pkg unpublish @scope/package@1.2.0  # remove a version (time-limited)

# Auth
agent login                               # authenticate with registry
agent logout
agent whoami

# Dev workflow
agent link                                # link current dir as a global package (like npm link)
agent link @scope/package                 # use linked version in current project
agent unlink @scope/package
```

---

## Registry

### Architecture

A lightweight HTTPS registry — not a CDN-scale npm clone, but a solid
foundation that can grow.

```
registry.agent.dev
  ├── GET  /@scope/package                  → package metadata (all versions)
  ├── GET  /@scope/package/1.2.0            → specific version metadata
  ├── GET  /@scope/package/-/pkg-1.2.0.tar.gz → tarball download
  ├── PUT  /@scope/package                  → publish (authenticated)
  ├── DELETE /@scope/package/1.2.0          → unpublish (time-limited)
  ├── GET  /-/search?q=web&size=20          → search
  ├── GET  /-/ping                          → health check
  └── POST /-/user/login                    → token auth
```

### Storage

- **Metadata**: Postgres (package index, versions, authors, download counts)
- **Tarballs**: S3-compatible object storage (Cloudflare R2, AWS S3, MinIO)
- **Auth**: Token-based (scoped tokens like npm, no OAuth in v1)
- **CDN**: Cloudflare in front of R2 for tarball delivery

### Private Registries

Organizations can run their own registry (self-hosted):

```yaml
# agent.yaml — point to a private registry
registry:
  default: https://registry.agent.dev
  scopes:
    "@company": https://pkg.company.internal
```

```bash
agent install --registry https://pkg.company.internal @company/tools
```

The self-hosted registry just needs to implement the same HTTP API surface.
A minimal Go implementation would be straightforward.

---

## Scoped Packages

Following npm's `@scope/name` convention:

| Scope | Who | Example |
|-------|-----|---------|
| `@agent` | Official/verified packages | `@agent/coding-tools` |
| `@username` | Individual developers | `@nickcecere/web-tools` |
| `@orgname` | Teams/companies | `@company/internal-tools` |

Unscoped names (`web-tools`) are reserved for `@agent` verified packages
(similar to npm's unscoped namespace being open but scoped being cleaner).

---

## Security Model

This is the hardest part — packages execute arbitrary code.

### Measures

**1. Checksum verification**  
Every install verifies SHA-256 of the tarball against the registry manifest.
Tampering is detected before any code runs.

**2. Capability declarations**  
Packages declare what they need in `agent-package.yaml`:

```yaml
capabilities:
  network: true       # makes outbound HTTP requests
  filesystem: true    # reads/writes files
  exec: false         # spawns subprocesses
  env: [HOME, PATH]  # environment variables it reads
```

The package manager warns (or blocks) if capabilities seem inconsistent
with what the tool actually does.

**3. First-run confirmation**  
The first time a tool from a package is invoked by the agent, the user
sees a prompt:

```
Tool web_search from @nickcecere/web-tools wants to run.
Capabilities: network=true, filesystem=false
[y/N/always/never]
```

**4. Verified publishers**  
The `@agent` scope requires manual review (like npm's security review program).
Community packages under `@username` are use-at-your-own-risk with a clear warning.

**5. Unpublish window**  
Packages can be unpublished within 72 hours of publish (like npm's policy).
After that, versions are immutable.

**6. Audit command**

```bash
agent audit                # check installed packages against known-bad list
agent audit --fix          # remove or update flagged packages
```

---

## Dependency Resolution

npm-style semver ranges. Simple algorithm for v1 (no hoisting):

```
@nickcecere/web-tools@1.2.0
  └── @agent/http-utils@^1.0.0   → resolves to 1.0.3
```

For v1, keep it simple: tools and skills have no shared runtime state, so
dependency conflicts are unlikely. No need for complex hoisting like npm.

**agent.lock** pins everything so installs are reproducible across machines
and CI environments.

---

## Publishing Flow

Modelled on `npm publish`:

```bash
cd my-agent-package
agent pkg init              # creates agent-package.yaml interactively
# ... develop tools and skills ...
agent pkg validate          # check manifest, run basic lint
agent pkg pack              # creates nickcecere-web-tools-1.2.0.tar.gz locally
agent login                 # authenticate (one-time)
agent pkg publish           # upload to registry
```

**What `publish` does:**
1. Validate `agent-package.yaml`
2. Run `hooks.pre_publish` if defined
3. Pack into tarball (respects `.agentignore`, like `.npmignore`)
4. Compute SHA-256
5. POST tarball + metadata to registry
6. Registry stores tarball in object storage, updates index

---

## `.agentignore`

Like `.npmignore` — files excluded from the published tarball:

```
# .agentignore
*.test.sh
tests/
.env
secrets/
node_modules/
__pycache__/
*.pyc
.git/
```

---

## `agent pkg init` — Interactive Scaffolding

```
$ agent pkg init

  Package name (@scope/name): @nickcecere/web-tools
  Version (1.0.0):
  Description: Web search and fetch tools
  Author: Nick Cecere <nick@example.com>
  License (MIT):
  Homepage: https://github.com/nickcecere/agent-web-tools

  Add a tool? (y/N): y
    Tool name: web_search
    Type (python/bash/node/ruby/rust/go): python
    Path: tools/web_search.py
    Description: Search the web

  Add another tool? (y/N): n
  Add a skill? (y/N): n

  ✅ Created agent-package.yaml
  ✅ Created tools/ directory
  ✅ Created .agentignore

  Next steps:
    1. Add your tool scripts to tools/
    2. agent pkg validate
    3. agent pkg publish
```

---

## Integration with `agent.yaml`

Packages declared in config are installed automatically on first run
(like `npm install` with a committed `package.json`):

```yaml
packages:
  - "@nickcecere/web-tools"
  - "@agent/github-tools@^1.0.0"
  - "@company/internal-tools"       # from private registry scope
```

The agent checks `agent.lock` on startup. If packages are missing or
versions don't match, it prompts (or auto-installs if `auto_install: true`).

---

## Phased Build Plan

### Phase 1 — Local packaging (no registry)

- `agent-package.yaml` manifest format
- `agent pkg init` scaffolding
- `agent pkg validate` linting
- `agent pkg pack` tarball creation
- `agent install ./path/to/local-package`
- Agent auto-discovers and loads from `~/.config/agent/packages/`
- `agent list`, `agent remove`

### Phase 2 — GitHub as registry (no infra)

- Treat GitHub release tarballs as packages
- `agent install github.com/nickcecere/agent-web-tools@v1.2.0`
- Fetches the `agent-package.yaml` from the repo/release
- Downloads and verifies the tarball
- `agent.lock` with checksums
- `agent update`

### Phase 3 — Central registry (registry.agent.dev)

- Registry server (Go, Postgres, R2)
- `agent login` / `agent publish`
- `agent search`
- Scoped packages (`@scope/name`)
- Download counts, package pages on the web
- `agent audit`

### Phase 4 — Ecosystem

- Private registry support (`--registry` flag, scope mapping)
- `agent link` for local development
- Capability declarations and first-run confirmation
- Verified publisher program for `@agent` scope
- Web UI at registry.agent.dev (package search, docs, stats)

---

## Open Questions

1. **Name**: `agent install` vs `agent pkg install` — should pkg management be
   a subcommand or top-level? npm uses top-level (`npm install`). Leaning
   toward top-level for ergonomics.

2. **Unscoped names**: Reserve them for `@agent` official packages, or allow
   community unscoped names (higher collision risk)?

3. **Go tools that need compilation**: If someone writes a tool in Go that
   needs to be compiled, do we ship pre-built binaries per platform (like
   Homebrew bottles) or require `go build` at install time?

4. **Compiled Go plugins vs subprocess plugins**: The current plugin protocol
   is subprocess-based. Should packages be able to ship compiled Go plugins
   that are loaded in-process? More powerful but harder to sandbox.

5. **Registry hosting**: Self-host from day one (full control, costs money)
   vs start with GitHub Releases as the "registry" (zero infra, less
   discoverability)?

6. **Monetization / sustainability**: Free forever? Paid private registries?
   Sponsorship model? Relevant if infra costs grow.

7. **Package signing**: GPG/sigstore signing of tarballs (like npm provenance)?
   Nice-to-have for v1, important for v2.

---

## Comparison to npm

| Feature | npm | Agent PM |
|---------|-----|----------|
| Central registry | npmjs.com | registry.agent.dev |
| Scoped packages | `@scope/pkg` | `@scope/pkg` |
| Manifest | `package.json` | `agent-package.yaml` |
| Lock file | `package-lock.json` | `agent.lock` |
| Local install | `node_modules/` | `.agent/packages/` |
| Global install | `/usr/lib/node_modules/` | `~/.config/agent/packages/` |
| Publish | `npm publish` | `agent publish` |
| Auth | `npm login` | `agent login` |
| Private registry | `.npmrc` scope mapping | `agent.yaml` scope mapping |
| Link (dev) | `npm link` | `agent link` |
| Audit | `npm audit` | `agent audit` |
| Lifecycle hooks | `scripts` in package.json | `hooks` in agent-package.yaml |
| Peer deps | ✅ | Phase 2+ |
| Workspaces/monorepo | ✅ | Not planned |
