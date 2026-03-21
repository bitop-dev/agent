# Service Architecture Plan

This document defines the service architecture for the distributed agent
platform. Three new components complement the existing registry and workers.

---

## Architecture overview

```
                 External clients
                 (API, webhooks, web UI, CLI)
                        │
                 ┌──────┴──────┐
                 │   Gateway   │  auth, routing, task queue, scheduling
                 └──────┬──────┘
                        │
           ┌────────────┼────────────┐
           │            │            │
     ┌─────┴────┐ ┌────┴─────┐ ┌────┴─────┐
     │ Worker 1  │ │ Worker 2  │ │ Worker N  │  execute tasks
     └─────┬────┘ └────┬─────┘ └────┬─────┘
           │            │            │
           └────────────┼────────────┘
                        │
           ┌────────────┼────────────┐
           │            │            │
     ┌─────┴────┐ ┌────┴─────┐ ┌────┴─────┐
     │ Registry  │ │  State   │ │   NATS   │
     │ packages  │ │  Store   │ │ event bus│
     └──────────┘ └──────────┘ └──────────┘
```

## Service responsibilities

| Service | Owns | Doesn't own |
|---|---|---|
| **Gateway** | Task routing, worker registry, auth, scheduling, webhooks, task history | Plugin storage, profile storage, task execution |
| **Registry** | Plugin packages, profile packages, artifact storage | Worker tracking, task routing, auth |
| **Workers** | Task execution, tool chains, LLM calls, on-demand installs | Routing decisions, scheduling, auth |
| **State Store** | Persistent storage (tasks, sessions, memory, audit) | Business logic |
| **NATS** | Event delivery, pub/sub | Everything else |

---

## 1. Gateway (agent-gateway)

**New Go service.** The single entry point for all external interaction with
the agent platform.

### What it does

- Accepts task submissions from external clients
- Authenticates requests (API key or bearer token)
- Finds the right worker for the task (capability-based routing)
- Queues tasks when workers are busy
- Tracks task lifecycle (pending → running → completed/failed)
- Stores results in the state store
- Handles webhook ingestion (Slack, GitHub, Grafana alerts)
- Runs scheduled tasks (cron)
- Publishes events to NATS (task.submitted, task.completed, etc.)

### API

#### Tasks

```
POST   /v1/tasks              Submit a task
GET    /v1/tasks              List recent tasks (with filters)
GET    /v1/tasks/{id}         Get task details + result
DELETE /v1/tasks/{id}         Cancel a running task
```

Task submission:
```json
POST /v1/tasks
{
  "profile": "researcher",
  "task": "Research Anthropic news this week",
  "context": {"date": "2026-03-21"},
  "priority": "normal",
  "callback": "https://hooks.slack.com/services/..."
}

Response:
{
  "id": "task-abc123",
  "status": "queued",
  "worker": null,
  "createdAt": "2026-03-21T03:00:00Z"
}
```

Task result (async — poll or use callback):
```json
GET /v1/tasks/task-abc123
{
  "id": "task-abc123",
  "status": "completed",
  "profile": "researcher",
  "worker": "http://worker-1:9898",
  "output": "**Topic:** Anthropic...",
  "duration": 12.3,
  "toolCalls": 4,
  "createdAt": "2026-03-21T03:00:00Z",
  "completedAt": "2026-03-21T03:00:12Z"
}
```

#### Workers

```
POST   /v1/workers            Register a worker (heartbeat)
GET    /v1/workers            List registered workers
DELETE /v1/workers?url=...    Deregister a worker
GET    /v1/workers/health     Aggregate health across all workers
```

Workers register at startup and heartbeat every 5 minutes.
The gateway removes stale workers after 15 minutes without heartbeat.

#### Webhooks

```
POST /v1/webhooks/slack       Slack event → task
POST /v1/webhooks/github      GitHub event → task
POST /v1/webhooks/grafana     Grafana alert → task
POST /v1/webhooks/generic     Any JSON → task (with template)
```

Webhook configuration:
```yaml
webhooks:
  - name: grafana-alerts
    path: /v1/webhooks/grafana
    profile: grafana-alert-summary
    taskTemplate: "Alert fired: {{.alertname}} on {{.host}}. Investigate and email nick@bitop.dev"
    contextTemplate:
      team: "{{.labels.team}}"
      alert: "{{.alertname}}"
```

#### Schedules

```
POST   /v1/schedules          Create a scheduled task
GET    /v1/schedules          List schedules
DELETE /v1/schedules/{id}     Delete a schedule
```

```json
POST /v1/schedules
{
  "name": "daily-ops-report",
  "cron": "0 8 * * *",
  "timezone": "America/New_York",
  "profile": "grafana-alert-summary",
  "task": "Generate daily ops report for team ict-aipe",
  "context": {"team": "ict-aipe", "recipient": "nick@bitop.dev"}
}
```

#### Auth

```
POST /v1/auth/keys            Create an API key
GET  /v1/auth/keys            List API keys
DELETE /v1/auth/keys/{id}     Revoke an API key
```

All endpoints require `Authorization: Bearer <api-key>`.
API keys are stored in the state store with scopes:
- `tasks:write` — submit tasks
- `tasks:read` — view task results
- `admin` — manage workers, schedules, webhooks

#### Agents (discovery proxy)

```
GET /v1/agents                List available agents (proxied from registry + workers)
```

The gateway aggregates agent info from the registry's profile index and
registered workers' capabilities.

### Routing logic

When a task arrives:

1. Look up the profile name in registered workers' capabilities
2. Filter to workers that have the profile installed (or can install it)
3. Prefer workers that are idle over busy ones
4. Prefer workers that have the profile cached (avoid on-demand install)
5. If no worker is available, queue the task
6. If queue is full, reject with 503

### Configuration

```yaml
# gateway.yaml
addr: ":8080"
stateStore: "postgres://user:pass@state-store:5432/agent?sslmode=disable"
natsURL: "nats://nats:4222"
registryURL: "http://agent-registry:9080"
auth:
  enabled: true
  adminKey: "your-admin-key"
routing:
  maxQueueSize: 100
  taskTimeout: 300s
  staleworkerTimeout: 15m
schedules:
  enabled: true
webhooks:
  enabled: true
```

---

## 2. State Store (PostgreSQL)

**Off-the-shelf PostgreSQL.** No custom service — just schema and migrations.

### Schema

```sql
-- Task history
CREATE TABLE tasks (
  id          TEXT PRIMARY KEY,
  profile     TEXT NOT NULL,
  task        TEXT NOT NULL,
  context     JSONB,
  status      TEXT NOT NULL DEFAULT 'queued',  -- queued, running, completed, failed, cancelled
  worker_url  TEXT,
  output      TEXT,
  error       TEXT,
  tool_calls  INTEGER DEFAULT 0,
  duration_ms INTEGER,
  created_at  TIMESTAMPTZ DEFAULT now(),
  started_at  TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  metadata    JSONB
);

-- API keys
CREATE TABLE api_keys (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL,
  key_hash    TEXT NOT NULL,
  scopes      TEXT[] NOT NULL,
  created_at  TIMESTAMPTZ DEFAULT now(),
  last_used   TIMESTAMPTZ,
  revoked     BOOLEAN DEFAULT false
);

-- Schedules
CREATE TABLE schedules (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL,
  cron        TEXT NOT NULL,
  timezone    TEXT DEFAULT 'UTC',
  profile     TEXT NOT NULL,
  task        TEXT NOT NULL,
  context     JSONB,
  enabled     BOOLEAN DEFAULT true,
  last_run    TIMESTAMPTZ,
  next_run    TIMESTAMPTZ,
  created_at  TIMESTAMPTZ DEFAULT now()
);

-- Worker registry (replaces in-memory registry)
CREATE TABLE workers (
  url           TEXT PRIMARY KEY,
  profiles      TEXT[],
  capabilities  TEXT[],
  status        TEXT DEFAULT 'active',
  registered_at TIMESTAMPTZ DEFAULT now(),
  last_heartbeat TIMESTAMPTZ DEFAULT now()
);

-- Agent memory (per-profile persistent knowledge)
CREATE TABLE agent_memory (
  id          SERIAL PRIMARY KEY,
  profile     TEXT NOT NULL,
  key         TEXT NOT NULL,
  value       TEXT NOT NULL,
  created_at  TIMESTAMPTZ DEFAULT now(),
  updated_at  TIMESTAMPTZ DEFAULT now(),
  UNIQUE(profile, key)
);

-- Webhook configs
CREATE TABLE webhooks (
  id              TEXT PRIMARY KEY,
  name            TEXT NOT NULL,
  path            TEXT NOT NULL,
  profile         TEXT NOT NULL,
  task_template   TEXT NOT NULL,
  context_template JSONB,
  enabled         BOOLEAN DEFAULT true,
  created_at      TIMESTAMPTZ DEFAULT now()
);

-- Audit log
CREATE TABLE audit_log (
  id          SERIAL PRIMARY KEY,
  action      TEXT NOT NULL,
  actor       TEXT,
  resource    TEXT,
  details     JSONB,
  created_at  TIMESTAMPTZ DEFAULT now()
);
```

### Deployment

```yaml
# k8s StatefulSet with PVC
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: state-store
spec:
  serviceName: state-store
  replicas: 1
  template:
    spec:
      containers:
        - name: postgres
          image: postgres:17
          env:
            - name: POSTGRES_DB
              value: agent
          volumeMounts:
            - name: data
              mountPath: /var/lib/postgresql/data
  volumeClaimTemplates:
    - metadata:
        name: data
      spec:
        accessModes: [ReadWriteOnce]
        resources:
          requests:
            storage: 10Gi
```

---

## 3. Event Bus (NATS)

**Off-the-shelf NATS.** Single binary, designed for cloud-native pub/sub.

### Event topics

```
agent.task.submitted     — gateway publishes when a task is queued
agent.task.started       — worker publishes when execution begins
agent.task.tool_call     — worker publishes on each tool invocation
agent.task.completed     — worker publishes on successful completion
agent.task.failed        — worker publishes on failure
agent.worker.registered  — gateway publishes when a worker registers
agent.worker.lost        — gateway publishes when a worker goes stale
agent.alert.fired        — webhook handler publishes on alert ingestion
agent.schedule.triggered — scheduler publishes when a cron fires
```

### Who publishes / subscribes

| Component | Publishes | Subscribes |
|---|---|---|
| Gateway | task.submitted, worker.registered, worker.lost, schedule.triggered | task.completed, task.failed (to update state store) |
| Workers | task.started, task.tool_call, task.completed, task.failed | task.submitted (optional: pull-based queue) |
| Dashboard | — | * (real-time display) |
| Reactive agents | task.completed, alert.fired | alert.fired, task.completed (trigger follow-up tasks) |

### Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nats
spec:
  replicas: 1
  template:
    spec:
      containers:
        - name: nats
          image: nats:2.10
          ports:
            - containerPort: 4222
```

---

## Migration plan

### What moves out of existing services

| Currently in | Moves to | What |
|---|---|---|
| Registry | Gateway | Worker registration (`POST/GET/DELETE /v1/workers`) |
| Workers | Gateway | Task submission entry point |
| Workers | State Store | Task history |
| Workers | NATS | Event publishing (currently in-memory message bus) |

### What stays

| Service | Keeps |
|---|---|
| Registry | Plugin packages, profile packages, artifact storage, publish endpoint |
| Workers | Task execution, on-demand installs, MCP serve mode, tool chains |

### Backward compatibility

- Workers continue to accept `POST /v1/task` directly (for local dev)
- Workers register with the gateway instead of the registry
- `agent serve --addr :9898` works unchanged for local use
- `agent serve --gateway http://gateway:8080` is the new k8s mode

---

## Implementation order

### Phase 1: Gateway core
- New `agent-gateway` Go project
- `POST /v1/tasks` — accept and dispatch to workers
- `GET /v1/tasks/{id}` — retrieve results
- Worker registration (moved from registry)
- Health-aware routing
- SQLite state store (upgrade to Postgres later)

### Phase 2: State persistence
- PostgreSQL schema and migrations
- Task history stored in Postgres
- API key auth

### Phase 3: Webhooks
- Grafana alert webhook → auto-create task
- Slack webhook → auto-create task
- Generic webhook with templates

### Phase 4: Scheduling
- Cron parser and scheduler
- Schedule CRUD API
- Scheduler goroutine in gateway

### Phase 5: NATS integration
- Deploy NATS
- Gateway publishes task lifecycle events
- Workers publish tool call events
- Foundation for reactive patterns and dashboard

### Phase 6: Dashboard
- Web UI (separate project)
- Reads from gateway API
- Subscribes to NATS for real-time updates

---

## Project structure

```
github.com/bitop-dev/
  agent/              — CLI, framework, worker
  agent-registry/     — plugin + profile package server
  agent-gateway/      — NEW: task routing, auth, scheduling, webhooks
  agent-plugins/      — plugin packages
  agent-profiles/     — profile packages
  agent-docs/         — documentation
  agent-dashboard/    — FUTURE: web UI
```

---

## k8s deployment (target state)

```yaml
# 5 services total
Namespace: agent-system

Deployments:
  agent-gateway:   1 replica   (entry point)
  agent-registry:  1 replica   (package server)
  agent-workers:   N replicas  (compute, auto-scale)
  nats:            1 replica   (event bus)

StatefulSets:
  state-store:     1 replica   (PostgreSQL with PVC)

Services:
  agent-gateway:   ClusterIP :8080  (+ Ingress for external access)
  agent-registry:  ClusterIP :9080
  agent-workers:   ClusterIP :9898
  state-store:     ClusterIP :5432
  nats:            ClusterIP :4222
```
