# OdooDevTools — Backend API

> A Go-based SaaS backend for real-time Odoo development intelligence. Monitor performance, debug ACL issues, detect N+1 queries, track schema changes, and scan for migration breaking changes — all from a connected agent on your Odoo instance.

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
|  | (React)  |    |  Chi Router   |    |                                |  |
|  +----------+    |  + Middleware  |    |  Auth · Tenant · Schema · ACL  |  |
|                  +-------+-------+    |  Error · Budget · Anonymizer   |  |
|                          |            +----------+---------------------+  |
|                          v                       v                        |
|          +------------+  +---------+  +--------------------------------+  |
|          | PostgreSQL |  |  Redis  |  |     Background Workers         |  |
|          | (metadata) |  | (queue/ |  |  Ingest · Aggregator · Retain  |  |
|          +------------+  |  cache) |  +--------------------------------+  |
|          +------------+  +---------+                                      |
|          |    S3      |                                                    |
|          | (raw logs) |                                                    |
|          +------------+                                                    |
+---------------------------------------------------------------------------+
                               | WebSocket (pooled)
          +--------------------+--------------------+
          |                    |                     |
   +------v------+     +------v------+      +------v------+
   |  Agent #1   |     |  Agent #2   |      |  Agent #N   |
   |  (prod)     |     |  (staging)  |      |  (client X) |
   | Odoo + PG   |     | Odoo + PG   |      | Odoo + PG   |
   +-------------+     +-------------+      +-------------+
```

Each agent is a lightweight Go binary installed on the customer's Odoo server. It samples ORM calls, aggregates data locally, and streams it to the cloud over a persistent WebSocket connection.

---

## Tech Stack

| Layer | Choice | Reason |
|---|---|---|
| Language | Go 1.21+ | Single binary deployment, low memory, fast startup |
| Router | [chi v5](https://github.com/go-chi/chi) | `net/http` native, composable middleware, no vendor lock-in |
| Database | PostgreSQL | JSONB for schema snapshots, concurrent multi-tenant writes |
| Cache / Queue | Redis | Rate limiting, background job queues, pub/sub for alerts |
| Object Storage | S3 / Cloudflare R2 | Raw log retention, zero egress fees on R2 |
| Auth | JWT (access + refresh tokens) | Stateless, tenant-scoped claims |
| API Docs | Swagger / OpenAPI | Auto-served at `/docs` |
| Logging | zerolog | Structured JSON in production, human-readable in development |

---

## Prerequisites

- Go 1.21 or later
- PostgreSQL 14+
- Redis 6+
- (Optional) S3-compatible storage for raw log retention

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

The server will start on `http://localhost:8869` by default. Visit `/docs` for the interactive Swagger UI.

---

## Configuration

The server is configured via environment variables:

| Variable | Description | Default |
|---|---|---|
| `APP_ENV` | `development` or `production` | `development` |
| `DATABASE_URL` | PostgreSQL connection string | — |
| `REDIS_URL` | Redis connection string | — |
| `JWT_SECRET` | Secret key for signing JWTs | — |
| `JWT_REFRESH_SECRET` | Secret key for refresh tokens | — |
| `ALLOWED_ORIGINS` | Comma-separated CORS origins (production) | — |
| `AGENT_CLOUD_URL` | WebSocket URL agents connect to | — |
| `S3_BUCKET` | Bucket name for raw log storage | — |
| `S3_ENDPOINT` | Custom S3 endpoint (for R2 or MinIO) | — |
| `PORT` | HTTP listen port | `8869` |

In `development`, CORS is open and logs are printed in human-readable format. In `production`, CORS is restricted to `ALLOWED_ORIGINS` and logs are emitted as structured JSON.

---

## API Reference

All endpoints are prefixed with `/api/v1`. Authenticated routes require a `Bearer` token in the `Authorization` header. Agent routes use an API key passed as `X-Agent-Key`.

### Public Endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/` | Service info (name, version, status) |
| `GET` | `/api/v1/health` | Returns `{ "status": "healthy" }` |
| `GET` | `/api/v1/ready` | Deep readiness probe — checks database, Redis, and agent cloud connectivity |
| `GET` | `/api/v1/version` | Returns API and Go version information |

**Readiness probe checks:**
- `database` — PostgreSQL ping
- `cache` — Redis ping (if configured)
- `agent_cloud` — HTTP/WS reachability of the agent endpoint

---

### Auth Endpoints

#### Public (no token required)

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/v1/auth/register` | Create a new account |
| `POST` | `/api/v1/auth/login` | Log in and receive `access_token` + `refresh_token` |
| `POST` | `/api/v1/auth/refresh` | Exchange a refresh token for a new access token |
| `POST` | `/api/v1/auth/forgot-password` | Send a password reset email (silent on unknown address) |
| `POST` | `/api/v1/auth/reset-password` | Complete a password reset using a token from email |
| `POST` | `/api/v1/auth/verify-email` | Verify email address using a token from email |

#### Protected (JWT required)

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/v1/auth/logout` | Revoke the current session (or all sessions) |
| `GET` | `/api/v1/auth/me` | Get the current authenticated user |
| `PATCH` | `/api/v1/auth/me` | Update the current user's profile |
| `POST` | `/api/v1/auth/change-password` | Change password (requires current password) |
| `GET` | `/api/v1/auth/sessions` | List all active sessions for the current user |
| `DELETE` | `/api/v1/auth/sessions/{session_id}` | Revoke a specific session |
| `POST` | `/api/v1/auth/resend-verification` | Resend the email verification link |

---

### Environment Endpoints

All routes below are under `/api/v1/environments`. JWT required.

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/v1/environments` | Create a new environment (dev / staging / prod) |
| `GET` | `/api/v1/environments` | List all environments for the current tenant |
| `GET` | `/api/v1/environments/{env_id}` | Get environment details including agent connection status |
| `PATCH` | `/api/v1/environments/{env_id}` | Update environment name or settings |
| `DELETE` | `/api/v1/environments/{env_id}` | Delete an environment |
| `PUT` | `/api/v1/environments/{env_id}/flags` | Update feature flags (ORM Collector, SQL Collector, Profiler, PII Stripping, Field Redaction) |
| `POST` | `/api/v1/environments/{env_id}/agent` | Register an agent to an environment |
| `GET` | `/api/v1/environments/{env_id}/heartbeats` | List agent heartbeat history |
| `GET` | `/api/v1/environments/{env_id}/heartbeats/latest` | Get the most recent heartbeat |
| `GET` | `/api/v1/environments/{env_id}/overview` | Dashboard summary (errors, profiler stats, N+1 counts, alert counts) |
| `POST` | `/api/v1/environments/{env_id}/api-keys` | Create an API key scoped to this environment |
| `GET` | `/api/v1/environments/{env_id}/api-keys` | List API keys |
| `DELETE` | `/api/v1/environments/{env_id}/api-keys/{key_id}` | Revoke an API key |

---

### Schema Endpoints

Routes are under `/api/v1/environments/{env_id}/schema`. JWT required.

| Method | Path | Description |
|---|---|---|
| `GET` | `/schema` | List all schema snapshots for the environment |
| `GET` | `/schema/latest` | Get the most recent schema snapshot |
| `GET` | `/schema/models` | Search/filter models within the latest snapshot |
| `GET` | `/schema/{snapshot_id}` | Get a specific snapshot (fields, access rules, record rules per model) |

Schema snapshots are pushed by the agent and stored as JSONB. Each model includes its fields, `ir.model.access` entries, and `ir.rule` record rules.

---

### Error Group Endpoints

Routes are under `/api/v1/environments/{env_id}/errors`. JWT required.

| Method | Path | Description |
|---|---|---|
| `GET` | `/errors` | List error groups (grouped by signature/traceback hash), sorted by frequency |
| `GET` | `/errors/{error_id}` | Get a specific error group with full traceback and metadata |
| `PATCH` | `/errors/{error_id}` | Update error group status (`resolve`, `acknowledge`, `reopen`) |

Errors are collected by the agent from Odoo's `ir.logging` and ingested via `/api/v1/agent/errors`.

---

### Profiler Endpoints

Routes are under `/api/v1/environments/{env_id}/profiler`. JWT required.

| Method | Path | Description |
|---|---|---|
| `GET` | `/profiler/recordings` | List all profiler recordings (batch IDs with duration and N+1 flag) |
| `GET` | `/profiler/recordings/slow` | List only slow recordings (above threshold) |
| `GET` | `/profiler/recordings/{recording_id}` | Get a recording's full waterfall view — ORM spans, SQL queries, N+1 markers |

Each recording represents one request batch. The waterfall shows all ORM method calls as spans, color-coded as Normal / Slow / N+1.

---

### N+1 Detection Endpoints

Routes are under `/api/v1/environments/{env_id}/n1`. JWT required.

| Method | Path | Description |
|---|---|---|
| `GET` | `/n1/detect` | List all detected N+1 patterns (model, method, call count, wasted time, sample SQL, fix suggestion) |
| `GET` | `/n1/timeline` | Time-series view of N+1 occurrences over the last 7 days |

N+1 patterns are scored as `CRITICAL`, `HIGH`, or `MEDIUM` based on wasted time and call frequency.

---

### Compute Chain Endpoints

Routes are under `/api/v1/environments/{env_id}/profiler/chain`. JWT required.

| Method | Path | Description |
|---|---|---|
| `GET` | `/profiler/chain` | List recorded compute chain analyses |
| `GET` | `/profiler/chain/{recording_id}` | Get a specific chain — dependency tree, per-node duration, bottleneck detection |

---

### Performance Budget Endpoints

Routes are under `/api/v1/environments/{env_id}/budgets`. JWT required.

| Method | Path | Description |
|---|---|---|
| `POST` | `/budgets` | Create a performance budget (endpoint + overhead threshold %) |
| `GET` | `/budgets` | List all budgets with current breach status |
| `GET` | `/budgets/{budget_id}` | Get a specific budget |
| `PATCH` | `/budgets/{budget_id}` | Update threshold or activate/deactivate |
| `DELETE` | `/budgets/{budget_id}` | Delete a budget |
| `GET` | `/budgets/{budget_id}/samples` | List recorded overhead samples |
| `GET` | `/budgets/{budget_id}/samples/{sample_id}/breakdown` | Per-function overhead breakdown for a sample |
| `GET` | `/budgets/{budget_id}/trend` | 7-day overhead trend data |

A budget is breached when `(module_ms / total_ms) * 100 > threshold`. Alerts are published automatically when a breach is detected.

---

### Alert Endpoints

Routes are under `/api/v1/environments/{env_id}/alerts`. JWT required.

| Method | Path | Description |
|---|---|---|
| `GET` | `/alerts` | List alerts (filterable by severity: `critical`, `warning`, `info`) |
| `GET` | `/alerts/count` | Get unacknowledged alert counts by severity |
| `POST` | `/alerts/acknowledge-all` | Acknowledge all pending alerts |
| `GET` | `/alerts/{alert_id}` | Get a specific alert |
| `POST` | `/alerts/{alert_id}/acknowledge` | Acknowledge a specific alert |

Alerts are generated by the background alert worker when performance budgets are breached or tracked errors are re-opened.

---

### Migration Endpoints

#### Standalone (JWT required, no `env_id`)

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/v1/migration/scan/source` | Upload Python/XML source files and scan for breaking changes (up to 2 MB) |
| `GET` | `/api/v1/migration/transitions` | List all supported version transitions (e.g. `v14→v15`, `v17→v18`) |

#### Per-environment (JWT required, under `/{env_id}/migration`)

| Method | Path | Description |
|---|---|---|
| `POST` | `/migration/scan` | Run a migration scan against the live schema snapshot |
| `POST` | `/migration/scan/source` | Scan uploaded source files scoped to this environment |
| `GET` | `/migration/transitions` | Supported transitions for this environment's Odoo version |
| `GET` | `/migration/scans` | List past scan results |
| `GET` | `/migration/scans/latest` | Get the most recent scan |
| `GET` | `/migration/scans/{scan_id}` | Get a specific scan result (breaking changes, warnings, minor issues) |
| `DELETE` | `/migration/scans/{scan_id}` | Delete a scan result |

Issues are categorized as `BREAKING`, `WARNING`, or `MINOR` and include the affected model, method, file, and line number where applicable.

---

### ACL Debugger Endpoints

Route is under `/api/v1/environments/{env_id}/acl`. JWT required.

| Method | Path | Description |
|---|---|---|
| `POST` | `/acl/trace` | Run a 5-stage ACL trace for a given user, model, operation, and optional record ID |

**Trace stages:**

1. **User Resolution** — resolve user ID to name, email, and active status
2. **Group Expansion** — expand direct groups to full implied group set
3. **Model ACL Check** — evaluate `ir.model.access` entries for the model
4. **Record Rule Finder** — match applicable `ir.rule` record rules to the user's groups
5. **Domain Evaluator** — evaluate rule domains against actual record data (requires agent)

Returns `ALLOWED` or `DENIED` with the failing stage and a fix suggestion.

---

### Agent Endpoints

These endpoints are used exclusively by the Go agent binary. They authenticate with a per-environment API key (`X-Agent-Key` header).

#### Self-Registration (no auth)

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/v1/agent/register` | Agent self-registers using a one-time registration token |

#### Authenticated Agent Routes

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/agent/ws` | Persistent WebSocket connection (heartbeat every 30s, bidirectional commands) |
| `POST` | `/api/v1/agent/schema` | Push a full schema snapshot (up to 10 MB) |
| `POST` | `/api/v1/agent/errors` | Ingest a batch of error events from `ir.logging` |
| `POST` | `/api/v1/agent/batch` | Ingest a batch of profiler/ORM/SQL events (sampled and aggregated by the agent) |

---

## Authentication

The API uses JWT bearer tokens.

```
Authorization: Bearer <access_token>
```

Access tokens are short-lived. Use `POST /api/v1/auth/refresh` with a valid refresh token to obtain a new access token without re-authenticating. Refresh tokens are rotated on each use.

Agent routes use API keys scoped to a specific environment:

```
X-Agent-Key: <api_key>
```

---

## Middleware

Applied globally (all routes):

- **RealIP** — extracts the real client IP from `X-Forwarded-For`
- **RequestID** — injects a unique `X-Request-ID` into every request
- **Recoverer** — catches panics and returns a structured 500 response
- **CORS** — open in development; restricted to `ALLOWED_ORIGINS` in production
- **SecurityHeaders** — sets `X-Content-Type-Options`, `X-Frame-Options`, etc.

Applied per route group:

- **Timeout** — 30s on all standard routes; none on the WebSocket route
- **MaxBodySize** — 1 MB default; 2 MB for source scan uploads; 10 MB for schema pushes
- **JWTAuth** — validates and parses JWT claims on all protected routes
- **TenantResolver** — looks up the tenant ID from the JWT and attaches it to the request context
- **TieredRateLimit** — per-plan rate limits (Starter / Pro / Agency) enforced via Redis
- **AgentAPIKeyAuth** — validates agent API keys for agent-specific routes

---

## Database

PostgreSQL with 19 migrations covering:

| Migration | Purpose |
|---|---|
| 000001–002 | Extensions, multi-tenant foundation (tenants, tenant members) |
| 000003 | Users, sessions, email verification tokens |
| 000004 | Billing events |
| 000005 | Environments |
| 000006, 000019 | Schema snapshots (JSONB — models with embedded accesses and rules) |
| 000007 | Error groups |
| 000008 | Profiler recordings |
| 000009 | Performance budgets and overhead samples |
| 000010 | ORM stats |
| 000011 | Migration scans |
| 000012 | Anonymization profiles |
| 000013 | Alerts, notification channels, alert deliveries |
| 000014 | Audit logs |
| 000015 | Auto-update triggers |
| 000016–017 | API key descriptions, environment scoping |
| 000018 | Agent registration tokens |

---

## Background Workers

Three background workers run alongside the HTTP server:

- **Ingest Worker** — consumes events from the Redis stream pushed by agents, normalises them, and writes to PostgreSQL
- **Aggregation Worker** — rolls up profiler spans into N+1 patterns and compute chain records on a periodic schedule
- **Retention Worker** — deletes expired records from PostgreSQL and S3 according to per-tenant retention settings (7d / 30d / 90d / unlimited)

An additional **Alert Worker** monitors performance budget overhead calculations and publishes threshold breach events to the alert feed.

---

## Development

```bash
# Run with hot-reload (requires air)
air

# Run tests
go test ./...

# Run linter
golangci-lint run

# Generate Swagger docs (requires swag)
swag init -g cmd/server/main.go

# Apply migrations
go run ./cmd/migrate up

# Roll back last migration
go run ./cmd/migrate down
```

API documentation is available at `http://localhost:8869/docs` when the server is running. The Swagger UI auto-injects the Bearer token after a successful login or register call.
