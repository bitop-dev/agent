# Distributed Agent Workers — Design Plan

This document extends the multi-agent plan with a distributed architecture
that supports running agents across multiple servers, k8s pods, and environments.

---

## Core idea

The agent binary becomes a **general-purpose worker** that can load any profile
on demand. Workers communicate over HTTP. Workers discover each other through
the registry. An orchestrator is just a worker running an orchestrator profile.

```
agent serve                    # starts a general-purpose worker
agent serve --profile researcher  # starts a fixed-profile worker (existing behavior)
```

When `agent serve` runs without `--profile`, it accepts any task with a profile
specified in the request. The worker loads the profile, runs the task, and
returns the result. The worker stays alive for the next request.

---

## Architecture

```
┌──────────────┐  ┌──────────────┐  ┌──────────────┐
│ agent-worker  │  │ agent-worker  │  │ agent-worker  │
│ (pod 1)       │  │ (pod 2)       │  │ (pod 3)       │
│               │  │               │  │               │
│ HTTP :8080    │  │ HTTP :8080    │  │ HTTP :8080    │
│ MCP  stdio    │  │ MCP  stdio    │  │ MCP  stdio    │
│               │  │               │  │               │
│ any profile   │  │ any profile   │  │ any profile   │
│ on demand     │  │ on demand     │  │ on demand     │
└──────┬───────┘  └──────┬───────┘  └──────┬───────┘
       │                 │                 │
       └────────┬────────┴────────┬────────┘
                │                 │
        ┌───────┴───────┐ ┌──────┴───────┐
        │ agent-registry │ │ config/      │
        │ (service       │ │ profiles/    │
        │  discovery)    │ │ plugins/     │
        └───────────────┘ └──────────────┘
```

### Worker modes

| Mode | Command | Behavior |
|---|---|---|
| **Fixed profile** | `agent serve --profile researcher` | Only runs the researcher profile. MCP stdio transport. |
| **Dynamic worker** | `agent serve --addr :8080` | Accepts any profile via HTTP. Loads profile per-request. |
| **Hybrid** | `agent serve --addr :8080 --profile researcher` | MCP over stdio for the fixed profile, HTTP for dynamic tasks. |

### Communication protocols

| Protocol | When used |
|---|---|
| **MCP over stdio** | External clients (opencode, Claude Desktop, Cursor) calling a fixed-profile agent |
| **HTTP** | Worker-to-worker communication, k8s service mesh, load balancers |
| **In-process (goroutines)** | Sub-agents spawned within the same worker process (existing behavior) |

---

## HTTP API for workers

### POST /v1/task

Submit a task to the worker. The worker loads the specified profile and runs it.

```json
// Request
{
  "profile": "researcher",
  "task": "Research Anthropic news this week",
  "context": {
    "date": "2026-03-20",
    "constraints": ["Only sources from March 13-20"]
  },
  "maxTurns": 8
}

// Response
{
  "id": "task-abc123",
  "status": "completed",
  "output": "**Topic:** Anthropic\n\n**Key stories:**\n- ...",
  "sessionId": "20260320T171300.000000000",
  "duration": 45.2
}
```

### POST /v1/pipeline

Submit a pipeline to the worker. Steps can reference other workers by URL
or rely on local execution.

```json
{
  "steps": [
    {
      "agent": "researcher",
      "task": "Research {{topic}}",
      "as": "research"
    },
    {
      "agent": "grafana-researcher",
      "task": "Get alerts for {{team}}",
      "as": "alerts",
      "worker": "http://grafana-worker:8080"
    },
    {
      "agent": "emailer",
      "task": "Send combined report to {{recipient}}",
      "context": {
        "research": "{{research}}",
        "alerts": "{{alerts}}"
      }
    }
  ],
  "variables": {
    "topic": "Anthropic",
    "team": "ict-aipe",
    "recipient": "nick@bitop.dev"
  }
}
```

### GET /v1/agents

List available profiles on this worker.

```json
{
  "agents": [
    {
      "name": "researcher",
      "description": "Web research agent",
      "capabilities": ["web-search", "summarization"],
      "accepts": "A topic or question",
      "returns": "Structured summary"
    }
  ]
}
```

### GET /v1/health

Health check for k8s probes.

```json
{
  "ok": true,
  "profiles": 5,
  "plugins": 8,
  "tools": 77,
  "uptime": 3600
}
```

---

## Worker registration and discovery

### Option A: Registry-based (centralised)

Workers register with the agent-registry at startup:

```
POST /v1/workers
{
  "url": "http://agent-worker-1:8080",
  "profiles": ["researcher", "grafana-researcher", "emailer"],
  "capabilities": ["web-search", "grafana-alerts", "email"]
}
```

Orchestrators query the registry to find workers:

```
GET /v1/workers?capability=web-search
→ [{"url": "http://agent-worker-1:8080", ...}, ...]
```

### Option B: k8s service discovery

Workers run as a k8s Service. The orchestrator calls the service endpoint
and k8s load-balances across pods:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: agent-workers
spec:
  selector:
    app: agent-worker
  ports:
    - port: 8080
```

The orchestrator calls `http://agent-workers:8080/v1/task` and any pod handles it.

### Option C: Hybrid

Use k8s services for basic load balancing, but also register with the
agent-registry for capability-based routing. The registry knows which
workers have which profiles and can route tasks to the right worker.

---

## Dynamic profile loading

When a worker receives a task with a profile name it hasn't loaded yet:

1. Check `~/.agent/profiles/` for the profile directory
2. If not found locally, check the registry for a profile package
3. Load the profile manifest and resolve its tools
4. Verify all required plugins are installed and enabled
5. Run the task

Profile resolution is cached — the second request for the same profile
skips steps 1-4.

Workers can be configured with a shared filesystem or profile registry
to ensure all workers have access to the same profiles:

```yaml
# k8s ConfigMap or shared volume
profiles:
  researcher/profile.yaml
  grafana-researcher/profile.yaml
  orchestrator/profile.yaml
```

---

## Message bus (for reactive patterns)

For agent-to-agent communication beyond request/response (Phase 5 of the
multi-agent plan), the worker HTTP API is extended with:

### POST /v1/messages

Send a message to a specific agent or broadcast:

```json
{
  "from": "monitor-agent",
  "to": "diagnostic-agent",
  "type": "request",
  "content": "CPU spike detected on host X, investigate",
  "data": {"host": "az1-prod-web-01", "metric": "cpu_percent", "value": 95}
}
```

### GET /v1/messages?agent=diagnostic-agent

Poll for pending messages (pull-based, simple):

```json
{
  "messages": [
    {
      "id": "msg-123",
      "from": "monitor-agent",
      "type": "request",
      "content": "CPU spike detected..."
    }
  ]
}
```

### WebSocket /v1/messages/stream

Real-time message delivery (push-based, for long-lived agents):

```
ws://agent-worker:8080/v1/messages/stream?agent=diagnostic-agent
```

---

## Deployment models

### Local development

```bash
agent serve --addr :8080
# One process, handles everything
```

### Docker Compose

```yaml
services:
  worker:
    image: agent-worker
    command: ["agent", "serve", "--addr", ":8080"]
    deploy:
      replicas: 3
  registry:
    image: agent-registry
    command: ["registry-server", "--addr", ":9080"]
```

### Kubernetes

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: agent-workers
spec:
  replicas: 5
  template:
    spec:
      containers:
        - name: worker
          image: ghcr.io/bitop-dev/agent:latest
          command: ["agent", "serve", "--addr", ":8080"]
          ports:
            - containerPort: 8080
          livenessProbe:
            httpGet:
              path: /v1/health
              port: 8080
          env:
            - name: OPENAI_BASE_URL
              valueFrom:
                secretKeyRef:
                  name: agent-secrets
                  key: openai-base-url
            - name: OPENAI_API_KEY
              valueFrom:
                secretKeyRef:
                  name: agent-secrets
                  key: openai-api-key
          volumeMounts:
            - name: profiles
              mountPath: /root/.agent/profiles
            - name: plugins
              mountPath: /root/.agent/plugins
      volumes:
        - name: profiles
          configMap:
            name: agent-profiles
        - name: plugins
          persistentVolumeClaim:
            claimName: agent-plugins
```

---

## Implementation phases

### Phase 5a: Dynamic profile loading in `agent serve`

- `agent serve` without `--profile` starts in dynamic mode
- Accepts `POST /v1/task` with profile in the request body
- Loads profile on demand, caches for reuse
- `GET /v1/agents` lists available profiles
- `GET /v1/health` for liveness/readiness probes

### Phase 5b: Worker-to-worker HTTP communication

- When a pipeline step has a `worker` URL, the framework dispatches
  the task to that remote worker via HTTP instead of running locally
- The orchestrator's `agent/pipeline` tool supports remote step execution
- Workers can delegate sub-tasks to other workers

### Phase 5c: Worker registration with registry

- Workers register at startup with their available profiles
- Registry exposes `GET /v1/workers` for capability-based routing
- Orchestrators query the registry to find workers for specific capabilities

### Phase 5d: Message bus for reactive patterns

- Add `POST /v1/messages` and `GET /v1/messages` endpoints
- Workers can send and receive messages
- Long-lived agents (service mode) poll or stream for messages
- Enable reactive patterns: monitor → detect → delegate → act

---

## What stays the same

- `agent run` and `agent chat` are unchanged — single-process, interactive
- `agent serve --profile <name>` for MCP stdio is unchanged
- Sub-agents spawned via `agent/spawn` still run as in-process goroutines
- Profiles, plugins, policies, approvals — all work the same
- The worker is just another way to run the same agent framework

---

## What this enables

| Use case | How it works |
|---|---|
| Scale web research | Deploy 10 worker pods, any handles research tasks |
| Nightly ops report | Cron job POSTs to worker: `{profile: "ops-daily", task: "..."}` |
| CI/CD integration | GitHub Action calls `POST /v1/task` after deploy |
| Reactive monitoring | Monitor agent streams Grafana, triggers diagnostic agent on alert |
| Multi-team reports | Pipeline fans out to N workers, one per team, collects results |
| Slack bot | Slack webhook → worker → research → respond in thread |
