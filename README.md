# POD Game Backend

Go backend for the POD Game contract-led TON coin flip platform.

## Overview

`pod-backend` is responsible for exposing REST and WebSocket interfaces,
subscribing to TON blockchain events, persisting indexed state in PostgreSQL,
and serving operational endpoints such as health checks, metrics, and Swagger.

## Source of Truth and References

- The rewritten TON contracts in `../pod-contract/` are the **source of truth**
  for gameplay semantics, statuses, payout logic, and message flow.
- The original projects at
  `/home/jackkru69/Documents/profiterole-flipcoin-frontend` and
  `/home/jackkru69/Documents/flipcoin-backend` are **behavioral/product
  references** only.
- Use the reference repos to understand flows and expected behavior, but do not
  port their React/Redux or FastAPI/SQLAlchemy/Celery implementation patterns
  into this Go backend.
- If a reference conflicts with the current TON contracts, the TON contracts win
  and this backend must adapt explicitly.

## Backend Responsibilities

- Serve REST API for games, reservations, user profiles, history, and referrals
- Serve WebSocket updates for global and per-game subscriptions
- Subscribe to TON Center events and persist blockchain-derived state
- Expose health, metrics, and Swagger/OpenAPI surfaces
- Enforce validation, rate limiting, and Telegram Mini App integration rules

## Stack

- Go 1.25
- Fiber v2
- PostgreSQL + `pgx`
- `squirrel` query builder (no ORM)
- `go-playground/validator/v10`
- TON Center integration
- Prometheus metrics + Swagger generation

## Quick Start

### 1. Configure environment

Copy `.env.example` to `.env` and adjust the placeholders before running the
service.

```bash
cp .env.example .env
```

Important: `.env.example` is already tuned for the local quickstart flow. Make
sure at least the following are correct for your local environment:

- `HTTP_PORT`
- `PG_URL`
- `RMQ_URL`
- `NATS_URL`
- `BLOCKCHAIN_EVENT_SOURCE`
- `BLOCKCHAIN_RESUME_FROM_CHECKPOINT`
- `BLOCKCHAIN_RESUME_EVENT_SOURCE`
- `TON_CENTER_V2_URL`
- `TON_CENTER_V3_WS_URL`
- `TON_GAME_CONTRACT_ADDRESS`
- `TELEGRAM_BOT_TOKEN`

### 2. Start local infrastructure

```bash
make compose-up
```

This starts Postgres, RabbitMQ, and NATS, then tails logs.

For host-run development, the compose stack exposes:

- Postgres on `localhost:5433`
- RabbitMQ on `localhost:5672`
- NATS on `localhost:4223`

### 3. Run the backend

```bash
go mod download
go run cmd/app/main.go
```

For a fuller dev flow that also regenerates Swagger/proto artifacts, you can
use:

```bash
make run
```

Database migrations run automatically on startup when the app is started with
the migrate-enabled flow.

The backend will listen on the port configured by `HTTP_PORT`.

### 4. Quick operator validation

Once the app is running, verify the completion-critical interfaces before moving
to frontend checks:

```bash
curl http://localhost:8090/api/v1/health
curl http://localhost:8090/health
```

Use `/api/v1/health` as the authoritative operator/frontend surface. Confirm the
response includes:

- top-level `status`, `database`, `event_source_status`, and `event_source_type`
- nested `parser.status`, `parser.recovery_status`, `parser.last_processed_lt`,
  `parser.checkpoint_updated_at`, and fallback telemetry

Keep `/health` and `/healthz` for lightweight liveness probes only.

## Core Environment Variables

Use `.env.example` as the canonical reference. Core settings include:

```env
# App / HTTP
APP_NAME=Pod Game Backend
APP_VERSION=1.0.0
HTTP_PORT=8090
LOG_LEVEL=info

# PostgreSQL
PG_POOL_MAX=2
PG_URL=postgres://user:myAwEsOm3pa55%40w0rd@localhost:5433/db?sslmode=disable

# Messaging / infrastructure
GRPC_PORT=8081
RMQ_URL=amqp://guest:guest@localhost:5672/
NATS_URL=nats://guest:guest@localhost:4223/

# TON / gameplay
TON_CENTER_V2_URL=http://localhost:8082
TON_CENTER_V3_WS_URL=ws://localhost:8081/api/v3/websocket
TON_GAME_CONTRACT_ADDRESS=EQD...replace_with_same_address_as_frontend
TELEGRAM_BOT_TOKEN=your_bot_token_here

# API behavior
CORS_ALLOWED_ORIGINS=https://localhost:5173,http://localhost:5173,https://t.me
RATE_LIMIT_REQUESTS=100
RATE_LIMIT_WINDOW=1m

# Event source mode
BLOCKCHAIN_EVENT_SOURCE=http
ENABLE_WEBSOCKET=false
WS_RECONNECT_MAX_ATTEMPTS=10
WS_PING_INTERVAL=30s
BLOCKCHAIN_RESUME_FROM_CHECKPOINT=true
BLOCKCHAIN_RESUME_EVENT_SOURCE=true
```

If you are running `pod-tma` locally, keep these aligned:

- backend `HTTP_PORT`
- frontend `VITE_API_URL`
- frontend `VITE_BACKEND_WS_URL`
- backend `TON_GAME_CONTRACT_ADDRESS`
- frontend `VITE_POD_FACTORY_ADDRESS`

## Public Interfaces

### REST API

- `GET /api/v1/games`
- `GET /api/v1/games/:gameId`
- `POST /api/v1/games/:gameId/reserve`
- `GET /api/v1/games/:gameId/reservation`
- `DELETE /api/v1/games/:gameId/reserve`
- `GET /api/v1/reservations`
- `GET /api/v1/users/:address`
- `GET /api/v1/users/:address/history`
- `GET /api/v1/users/:address/referrals`
- `GET /api/v1/health` — overall service health plus parser recovery snapshot

### WebSocket

- `WS /ws/games` — broadcast-only global game and reservation updates
- `WS /ws/games/:gameId` — per-game updates plus reconnect reconciliation via `sync_request`

Server message families:

- `game_state_update` — emitted by global and per-game subscriptions with a top-level RFC3339 `timestamp`
- `reservation_created` — emitted when a reservation is acquired
- `reservation_released` — emitted when a reservation expires, is cancelled, or is consumed by a join
- `sync_response` — returned only by `/ws/games/:gameId` after a client `sync_request`
- `error` — returned for invalid JSON, unsupported client frames, or temporarily unavailable sync state

Client-authored frames:

- `/ws/games` is broadcast-only and rejects client payloads with an `error`
- `/ws/games/:gameId` accepts `{"type":"sync_request","last_event_timestamp":"<optional RFC3339 timestamp>"}` and responds with `sync_response`

If you change either WebSocket interface, update the frontend, docs, and any
consumers explicitly.

### Operational Endpoints

- `GET /health`
- `GET /healthz`
- `GET /metrics`
- `GET /swagger/*`
- `GET /api/v1/health`

`GET /api/v1/health` is the parser/sync recovery surface used by the frontend
and operators. In addition to top-level service fields, it returns a nested
`parser` object with:

- `status` — parser-specific health after merging live connection state and the
  persisted checkpoint snapshot
- `recovery_status` — one of `live`, `fallback_active`, `recovering`,
  `stalled`, or `not_configured`
- `last_processed_lt` — most recent successfully handled TON logical time
- `checkpoint_updated_at` — last persisted checkpoint write timestamp
- `fallback_count` and `last_fallback_at` — restart-safe fallback telemetry
- `current_source_type` — source currently reflected in parser health

## Architecture and Structure

The backend follows clean architecture with inward-only dependency flow:

`controller -> usecase -> repository -> entity`

```text
pod-backend/
├── cmd/
│   └── app/               # Application entry point
├── config/                # Environment-backed configuration
├── docs/                  # Generated Swagger/proto artifacts
├── internal/
│   ├── app/               # App wiring and bootstrap
│   ├── controller/
│   │   ├── blockchain/    # Blockchain event handling
│   │   ├── http/          # Router + middleware
│   │   ├── rest/          # REST handlers
│   │   └── websocket/     # WebSocket handlers
│   ├── entity/            # Domain models and domain errors
│   ├── infrastructure/    # External integrations
│   ├── repository/        # Persistence and query layer
│   └── usecase/           # Business logic
├── migrations/            # SQL migrations
├── pkg/                   # Shared infra packages
└── tests/                 # Unit and integration tests
```

## Validation and Development Commands

```bash
# Format
make format

# Lint
make linter-golangci

# Unit tests
go test ./tests/unit/... -v

# Integration tests
go test ./tests/integration/... -v

# Full test run
go test ./... -v -race -cover

# Swagger generation
make swag-v1

# Mock generation
make mock

# Dependency audit
make deps-audit
```

Integration tests require a working Postgres database, typically via
`TEST_PG_URL` or the integration Docker setup.

## Migrations

```bash
# Create a new migration
make migrate-create your_migration_name

# Apply migrations manually
make migrate-up
```

Migration rules:

- Keep migrations idempotent (`IF NOT EXISTS` / `IF EXISTS`).
- Do not rewrite already-deployed migrations unless the task explicitly calls
  for it.

## Blockchain Event Streaming

The backend can ingest blockchain events through either HTTP polling or
WebSocket streaming.

Key controls:

- `BLOCKCHAIN_EVENT_SOURCE=http|websocket`
- `ENABLE_WEBSOCKET=true|false`
- `BLOCKCHAIN_RESUME_FROM_CHECKPOINT=true|false`
- `BLOCKCHAIN_RESUME_EVENT_SOURCE=true|false`
- `TON_CENTER_V3_WS_URL`
- `WS_RECONNECT_MAX_ATTEMPTS`
- `WS_PING_INTERVAL`

Use `GET /api/v1/health` to confirm which event source is active, whether the
parser is live or recovering, and which LT/checkpoint the backend would resume
from after a restart. The plain `/health` and `/healthz` endpoints remain
lightweight process probes, while `/metrics` exposes longer-lived counters.

### Foundational parser and sync contract

The parser/sync pipeline has a few behavior guarantees that completion work
must preserve:

- Runtime ingestion uses the authoritative `NewRuntimeMessageParser()` path,
  which is backed by the tonutils-go cell parser in
  `internal/infrastructure/toncenter/message_parser_v2.go`. The legacy parser in
  `message_parser.go` remains for compatibility/regression coverage only.
- The runtime parser accepts both TON Center `message` payloads and
  `msg_data.body` payloads, and it rejects overflowing `uint256` game IDs
  instead of silently truncating them into `int64`.
- LT checkpoints advance only after successful transaction handling. Both the
  HTTP poller and the WebSocket subscriber keep checkpoint writes monotonic so a
  failed transaction is retried on the next pass instead of being skipped.
- Duplicate/older transactions are ignored once their LT is at or behind the
  current checkpoint, which keeps fallback/restart handoff idempotent across
  `http` and `websocket` event sources.
- Parser outcomes are intentionally split into:
  1. non-game / empty-`in_msg` traffic, which stays debug-only and does not hit
     DLQ handling,
  2. malformed or validation-failed game traffic, which is recorded as parser or
     validation failure and can be stored in the DLQ for operator follow-up.

## Operational Notes and Pitfalls

- Backend behavior must stay aligned with the rewritten TON contracts.
- Do not introduce an ORM; this codebase uses `pgx` + `squirrel`.
- Preserve compatibility with both current WebSocket interfaces unless the task
  explicitly changes the public API.
- Update Swagger comments when adding or changing endpoints.
- Do not port FastAPI/SQLAlchemy/Celery patterns from the backend reference
  repo into this service.

## Related Documentation

- [Main project README](../README.md)
- [POD smart contracts](../pod-contract/README.md)
- [POD TMA frontend](../pod-tma/README.md)
- [Repository constitution](../.specify/memory/constitution.md)

## License

This software is proprietary and confidential. Unauthorized copying,
modification, distribution, or use is strictly prohibited.
