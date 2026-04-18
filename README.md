# OdooDevTools — Backend API

> A Go-based SaaS backend for real-time Odoo development intelligence. Collects ORM traces, SQL patterns, schema snapshots, error events, and live server logs from a lightweight agent — then exposes them via REST + WebSocket to the dashboard.

---

## Table of Contents

- [Architecture Overview](#architecture-overview)
- [Tech Stack](#tech-stack)
- [Prerequisites](#prerequisites)
- [Getting Started](#getting-started)
- [Configuration](#configuration)
- [API Reference](#api-reference)
  - [Public Endpoints](#public-endpoints)
  - [Auth Endpoints](#auth-endpoints)
  - [Environment Endpoints](#environment-endpoints)
  - [Schema Endpoints](#schema-endpoints)
  - [Error Group Endpoints](#error-group-endpoints)
  - [Profiler Endpoints](#profiler-endpoints)
  - [N+1 Detection Endpoints](#n1-detection-endpoints)
  - [Compute Chain Endpoints](#compute-chain-endpoints)
  - [Performance Budget Endpoints](#performance-budget-endpoints)
  - [Alert Endpoints](#alert-endpoints)
  - [Migration Endpoints](#migration-endpoints)
  - [ACL Debugger Endpoints](#acl-debugger-endpoints)
  - [Server Log Endpoints](#server-log-endpoints)
  - [Agent Endpoints](#agent-endpoints)
- [Authentication](#authentication)
- [Middleware](#middleware)
- [Database](#database)
- [Background Workers](#background-workers)
- [Development](#development)

---

## Architecture Overview

```
+---------------------------------------------------------------------------+
|                            CLOUD INFRASTRUCTURE                           |
|                                                                           |
|  +----------+    +---------------+    +--------------------------------+  |
|  |  Web UI  |--->|  API Gateway  |--->|          Core Service          |  |
|  | (Next.js)|    |  Chi Router   |    |                                |  |
|  +----------+    |  + Middleware  |    |  Auth · Tenant · Schema · ACL  |  |
|                  +-------+-------+    |  Error · Budget · ServerLogs   |  |
|                          |            +----------+---------------------+  |
|                          v                       v                        |
|          +------------+  +---------+  +--------------------------------+  |
|          | PostgreSQL |  |  Redis  |  |     Background Workers         |  |
|          | (metadata) |  | (cache/ |  |  Ingest · Aggregator · Retain  |  |
|          +------------+  |  pub-sub|  +--------------------------------+  |
+---------------------------------------------------------------------------+
                               | WebSocket (pooled, persistent)
          +--------------------+--------------------+
          |                    |                     |
   +------v------+     +------v------+      +------v------+
   |  Agent #1   |     |  Agent #2   |      |  Agent #N   |
   |  (prod)     |     |  (staging)  |      |  (client X) |
   | Odoo + PG   |     | Odoo + PG   |      | Odoo + PG   |
   +-------------+     +-------------+      +-------------+
```

Each agent is a lightweight Go binary installed on the customer's Odoo server. It samples ORM calls, tails the Odoo log file, aggregates data locally, and streams it to the cloud over a persistent WebSocket.

---

## Tech Stack

| Layer | Choice | Reason |
|---|---|---|
| Language | Go 1.21+ | Single binary deployment, low memory, fast startup |
| Router | [chi v5](https://github.com/go-chi/chi) | `net/http` native, composable middleware |
| Database | PostgreSQL | JSONB for schema snapshots, multi-tenant writes |
| Cache / Pub-Sub | Redis | Rate limiting, server log storage and streaming |
| Auth | JWT (access + refresh tokens) | Stateless, tenant-scoped claims |
| Logging | zerolog | Structured JSON in production, human-readable in dev |

---

## Prerequisites

- Go 1.21 or later
- PostgreSQL 14+
- Redis 6+

---

## Getting Started

```bash
# Clone the repository
git clone https://github.com/your-org/odoo-dev-tools-backend.git
cd odoo-dev-tools-backend

# Copy and configure environment variables
cp .env.example .env

# Run database migrations
go run ./cmd/migrate up

# Start the server
go run ./cmd/server
```

The server starts on `http://localhost:8869` by default.

---

## Configuration

| Variable | Description | Default |
|---|---|---|
| `APP_ENV` | `development` or `production` | `development` |
| `DATABASE_URL` | PostgreSQL connection string | — |
| `REDIS_URL` | Redis connection string | — |
| `JWT_SECRET` | Secret key for signing access tokens | — |
| `JWT_REFRESH_SECRET` | Secret key for refresh tokens | — |
| `ALLOWED_ORIGINS` | Comma-separated CORS origins | — |
| `PORT` | HTTP listen port | `8869` |

In `development`, CORS is open and logs are human-readable. In `production`, CORS is restricted and logs are structured JSON.

---

## API Reference

All endpoints are prefixed with `/api/v1`. Authenticated routes require `Authorization: Bearer <token>`. Agent routes use `X-Agent-Key`.

### Public Endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/` | Service info |
| `GET` | `/api/v1/health` | Returns `{ "status": "healthy" }` |
| `GET` | `/api/v1/ready` | Readiness probe — checks PostgreSQL, Redis, agent cloud |
| `GET` | `/api/v1/version` | API and Go version info |

---

### Auth Endpoints

#### Public

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/v1/auth/register` | Create account |
| `POST` | `/api/v1/auth/login` | Login → `access_token` + `refresh_token` |
| `POST` | `/api/v1/auth/refresh` | Exchange refresh token for new access token |
| `POST` | `/api/v1/auth/forgot-password` | Send password reset email |
| `POST` | `/api/v1/auth/reset-password` | Complete password reset |
| `POST` | `/api/v1/auth/verify-email` | Verify email address |

#### Protected (JWT required)

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/v1/auth/logout` | Revoke session |
| `GET` | `/api/v1/auth/me` | Current user |
| `PATCH` | `/api/v1/auth/me` | Update profile |
| `POST` | `/api/v1/auth/change-password` | Change password |
| `GET` | `/api/v1/auth/sessions` | List active sessions |
| `DELETE` | `/api/v1/auth/sessions/{session_id}` | Revoke session |

---

### Environment Endpoints

Routes under `/api/v1/environments`. JWT required.

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/v1/environments` | Create environment |
| `GET` | `/api/v1/environments` | List environments |
| `GET` | `/api/v1/environments/{env_id}` | Get environment + agent status |
| `PATCH` | `/api/v1/environments/{env_id}` | Update name or settings |
| `DELETE` | `/api/v1/environments/{env_id}` | Delete environment |
| `PUT` | `/api/v1/environments/{env_id}/flags` | Update feature flags (pushed to agent over WS) |
| `POST` | `/api/v1/environments/{env_id}/agent` | Register agent |
| `GET` | `/api/v1/environments/{env_id}/heartbeats/latest` | Latest agent heartbeat |
| `GET` | `/api/v1/environments/{env_id}/overview` | Dashboard summary |
| `POST` | `/api/v1/environments/{env_id}/api-keys` | Create API key |
| `GET` | `/api/v1/environments/{env_id}/api-keys` | List API keys |
| `DELETE` | `/api/v1/environments/{env_id}/api-keys/{key_id}` | Revoke API key |

---

### Schema Endpoints

Routes under `/api/v1/environments/{env_id}/schema`. JWT required.

| Method | Path | Description |
|---|---|---|
| `GET` | `/schema` | List schema snapshots |
| `GET` | `/schema/latest` | Most recent snapshot |
| `GET` | `/schema/models` | Search models in latest snapshot |
| `GET` | `/schema/{snapshot_id}` | Specific snapshot (fields, ACL, record rules per model) |

---

### Error Group Endpoints

Routes under `/api/v1/environments/{env_id}/errors`. JWT required.

| Method | Path | Description |
|---|---|---|
| `GET` | `/errors` | List error groups sorted by frequency |
| `GET` | `/errors/{error_id}` | Full traceback and metadata |
| `PATCH` | `/errors/{error_id}` | Update status (`resolve`, `acknowledge`, `reopen`) |

---

### Profiler Endpoints

Routes under `/api/v1/environments/{env_id}/profiler`. JWT required.

| Method | Path | Description |
|---|---|---|
| `GET` | `/profiler/recordings` | List recordings (with N+1 flag and duration) |
| `GET` | `/profiler/recordings/slow` | Slow recordings only |
| `GET` | `/profiler/recordings/{recording_id}` | Full waterfall — ORM spans, SQL, N+1 markers |

---

### N+1 Detection Endpoints

Routes under `/api/v1/environments/{env_id}/n1`. JWT required.

| Method | Path | Description |
|---|---|---|
| `GET` | `/n1/detect` | Detected ORM-level N+1 patterns |
| `GET` | `/n1/timeline` | 7-day occurrence time series |

> **What this detects:** ORM/SQL-level N+1 — Python code calling individual `browse()` or `read()` in a loop, causing many separate SQL queries. This is detected by analysing query patterns from the agent's SQL trace. It does **not** detect JS/RPC-level N+1 (multiple XHR calls from the browser).

Patterns are scored `CRITICAL`, `HIGH`, or `MEDIUM` based on wasted time and call frequency. Each includes sample SQL and a concrete fix suggestion.

---

### Compute Chain Endpoints

Routes under `/api/v1/environments/{env_id}/profiler/chain`. JWT required.

| Method | Path | Description |
|---|---|---|
| `GET` | `/profiler/chain` | List compute chain recordings |
| `GET` | `/profiler/chain/{recording_id}` | Dependency tree, per-node duration, bottleneck |

---

### Performance Budget Endpoints

Routes under `/api/v1/environments/{env_id}/budgets`. JWT required.

| Method | Path | Description |
|---|---|---|
| `POST` | `/budgets` | Create budget (endpoint + overhead threshold %) |
| `GET` | `/budgets` | List budgets with breach status |
| `GET` | `/budgets/{budget_id}` | Single budget |
| `PATCH` | `/budgets/{budget_id}` | Update threshold or activate/deactivate |
| `DELETE` | `/budgets/{budget_id}` | Delete budget |
| `GET` | `/budgets/{budget_id}/samples` | Overhead samples |
| `GET` | `/budgets/{budget_id}/trend` | 7-day trend |

A budget is breached when `(module_ms / total_ms) * 100 > threshold`. Alerts are published automatically on breach.

---

### Alert Endpoints

Routes under `/api/v1/environments/{env_id}/alerts`. JWT required.

| Method | Path | Description |
|---|---|---|
| `GET` | `/alerts` | List alerts (filter by `critical`, `warning`, `info`) |
| `GET` | `/alerts/count` | Unacknowledged counts by severity |
| `POST` | `/alerts/acknowledge-all` | Acknowledge all |
| `GET` | `/alerts/{alert_id}` | Single alert |
| `POST` | `/alerts/{alert_id}/acknowledge` | Acknowledge |

---

### Migration Endpoints

#### Standalone (JWT, no `env_id`)

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/v1/migration/scan/source` | Scan uploaded Python/XML files (up to 2 MB) |
| `GET` | `/api/v1/migration/transitions` | Supported version transitions |

#### Per-environment (JWT, under `/{env_id}/migration`)

| Method | Path | Description |
|---|---|---|
| `POST` | `/migration/scan` | Scan live schema snapshot |
| `GET` | `/migration/scans` | Past scan results |
| `GET` | `/migration/scans/latest` | Most recent scan |
| `GET` | `/migration/scans/{scan_id}` | Specific scan (BREAKING / WARNING / MINOR issues) |
| `DELETE` | `/migration/scans/{scan_id}` | Delete scan |

---

### ACL Debugger Endpoints

Route under `/api/v1/environments/{env_id}/acl`. JWT required.

| Method | Path | Description |
|---|---|---|
| `POST` | `/acl/trace` | 5-stage ACL trace for user + model + operation |

**Trace stages:** User Resolution → Group Expansion → Model ACL Check → Record Rule Finder → Domain Evaluator

Returns `ALLOWED` or `DENIED` with the failing stage and a fix suggestion.

---

### Server Log Endpoints

Routes under `/api/v1/environments/{env_id}`. JWT required.

| Method | Path | Description |
|---|---|---|
| `GET` | `/server-logs` | Recent log lines from Redis (last 1000, filterable by `level`) |
| `GET` | `/server-logs/stream` | SSE stream — sends history first, then live lines as they arrive |

Log lines are collected by the agent tailing the Odoo log file (`AGENT_LOG_FILE` in agent config, defaults to `/var/log/odoo/odoo-server.log`). The agent sends them over WebSocket → backend stores in a Redis capped list (TTL 24h) and publishes to a Redis channel.

Each log line:
```json
{
  "timestamp": "2026-04-16 10:23:11,452",
  "level": "WARNING",
  "logger": "odoo.sql_db",
  "message": "...",
  "pid": 1234
}
```

---

### Agent Endpoints

Used exclusively by the agent binary. Auth via `X-Agent-Key`.

#### Self-Registration (no auth)

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/v1/agent/register` | Self-register with a one-time registration token |

#### Authenticated

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/agent/ws` | Persistent WebSocket — heartbeat, feature flag sync, log streaming |
| `POST` | `/api/v1/agent/schema` | Push schema snapshot (up to 10 MB) |
| `POST` | `/api/v1/agent/errors` | Ingest error batch from `ir.logging` |
| `POST` | `/api/v1/agent/batch` | Ingest profiler/ORM/SQL events |

#### Agent Distribution

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/agent/version` | Latest and supported agent versions |
| `GET` | `/api/v1/agent/download` | Download agent binary (`?version=&platform=`) |
| `GET` | `/api/v1/agent/checksums` | SHA256 checksums for a release |
| `GET` | `/install` | Serve `install-agent.sh` |
| `GET` | `/install.ps1` | Serve `install-agent.ps1` (Windows) |

---

## Authentication

```
Authorization: Bearer <access_token>
```

Access tokens are short-lived. Use `POST /api/v1/auth/refresh` to rotate. Refresh tokens are rotated on each use.

Agent routes:

```
X-Agent-Key: <api_key>
```

---

## Middleware

Applied globally:

- **RealIP** — extracts real client IP from `X-Forwarded-For`
- **RequestID** — injects `X-Request-ID` on every request
- **Recoverer** — catches panics, returns structured 500
- **CORS** — open in development; restricted to `ALLOWED_ORIGINS` in production
- **SecurityHeaders** — `X-Content-Type-Options`, `X-Frame-Options`, etc.

Applied per route group:

- **Timeout** — 30s on standard routes; disabled on WebSocket and SSE
- **MaxBodySize** — 1 MB default; 2 MB for source uploads; 10 MB for schema pushes
- **JWTAuth** — validates and parses JWT claims
- **TenantResolver** — attaches tenant ID to request context
- **TieredRateLimit** — per-plan limits (Starter / Pro / Agency) via Redis
- **AgentAPIKeyAuth** — validates agent API keys

---

## Database

PostgreSQL with migrations in `db/migrations/`:

| Migration | Purpose |
|---|---|
| 000001–002 | Extensions, multi-tenant foundation (tenants, members) |
| 000003 | Users, sessions, email verification tokens |
| 000004 | Billing events |
| 000005 | Environments |
| 000006, 000019 | Schema snapshots (JSONB) |
| 000007 | Error groups |
| 000008 | Profiler recordings |
| 000009 | Performance budgets and samples |
| 000010 | ORM stats |
| 000011 | Migration scans |
| 000012 | Anonymization profiles |
| 000013 | Alerts, notification channels, deliveries |
| 000014 | Audit logs |
| 000015 | Auto-update triggers |
| 000016–017 | API key descriptions, environment scoping |
| 000018 | Agent registration tokens |

---

## Background Workers

- **Ingest Worker** — consumes events from the Redis stream, normalises, writes to PostgreSQL
- **Aggregation Worker** — rolls up profiler spans into N+1 patterns and compute chain records
- **Retention Worker** — deletes expired records per tenant retention settings (7d / 30d / 90d / unlimited)
- **Alert Worker** — monitors budget overhead and publishes breach events to the alert feed

---

## Development

```bash
# Run with hot-reload (requires air)
air

# Run tests
go test ./...

# Run linter
golangci-lint run --timeout 5m

# Apply migrations
go run ./cmd/migrate up

# Roll back last migration
go run ./cmd/migrate down
```
