# Pod Game Backend

Real-time blockchain game backend with WebSocket support for TON blockchain coin flip gambling application.

## Features

- ✅ **REST API** - Game listing, user profiles, game history
- ✅ **WebSocket** - Real-time game state updates
- ✅ **Blockchain Integration** - TON blockchain event subscription and persistence
- ✅ **Swagger/OpenAPI** - Auto-generated API documentation at `/swagger`
- ✅ **Prometheus Metrics** - Comprehensive monitoring at `/metrics`
- ✅ **Health Checks** - Service health endpoint at `/health`
- ✅ **Circuit Breaker** - Resilient TON Center API integration
- ✅ **Rate Limiting** - Request throttling per user
- ✅ **CORS** - Cross-origin support for Telegram Mini Apps
- ✅ **Referral System** - Track and reward user referrals

## Quick Start

### Prerequisites

- Go 1.21+
- PostgreSQL 15+
- TON Center API access (or local node)

### Installation

1. Install dependencies:
```bash
go mod download
```

2. Set up PostgreSQL:
```bash
# Start PostgreSQL (using Docker Compose)
docker-compose -f docker-compose.dev.yaml up -d

# Run migrations
migrate -path migrations -database "postgresql://postgres:postgres@localhost:5432/pod_game?sslmode=disable" up
```

3. Configure environment variables (copy from `.env.example`):
```bash
# Application
APP_NAME=Game Backend
APP_VERSION=1.0.0

# HTTP Server
HTTP_PORT=3000

# Database
PG_URL=postgresql://postgres:postgres@localhost:5432/pod_game?sslmode=disable
PG_POOL_MAX=10

# Logging
LOG_LEVEL=info

# TON Blockchain
TON_CENTER_V2_URL=https://testnet.toncenter.com/api/v2
TON_GAME_CONTRACT_ADDRESS=0:your_contract_address

# Circuit Breaker
CIRCUIT_BREAKER_MAX_FAILURES=5
CIRCUIT_BREAKER_TIMEOUT=60s

# CORS
CORS_ALLOWED_ORIGINS=http://localhost:3001,https://yourdomain.com
```

4. Run the service:
```bash
go run cmd/game-backend/main.go
```

The service will start on `http://localhost:3000`

### API Documentation

Once the service is running, visit:
- **Swagger UI**: http://localhost:3000/swagger
- **Health Check**: http://localhost:3000/health
- **Prometheus Metrics**: http://localhost:3000/metrics

## Project Structure

```
pod-backend/
├── cmd/
│   └── game-backend/      # Application entry point
├── internal/
│   ├── controller/        # HTTP/WebSocket handlers
│   │   ├── rest/         # REST API controllers
│   │   ├── websocket/    # WebSocket controllers
│   │   └── blockchain/   # Blockchain event handler
│   ├── usecase/          # Business logic
│   ├── repository/       # Data access layer
│   │   └── postgres/     # PostgreSQL implementations
│   ├── entity/           # Domain entities
│   └── infrastructure/   # External services
│       ├── toncenter/    # TON blockchain client
│       ├── metrics/      # Prometheus metrics
│       └── telegram/     # Telegram auth
├── migrations/           # Database migrations
├── tests/
│   ├── unit/            # Unit tests
│   └── integration/     # Integration tests
├── config/              # Configuration
└── docs/               # Generated Swagger docs
```

## API Endpoints

### Game Endpoints

- `GET /api/v1/games` - List available games (with status filter)
- `GET /api/v1/games/:gameId` - Get game details by ID

### User Endpoints

- `GET /api/v1/users/:address` - Get user profile and statistics
- `GET /api/v1/users/:address/history` - Get user's game history
- `GET /api/v1/users/:address/referrals` - Get referral statistics

### WebSocket

- `WS /ws/games/:gameId` - Subscribe to real-time game updates

### System Endpoints

- `GET /health` - Service health check
- `GET /metrics` - Prometheus metrics
- `GET /swagger` - Interactive API documentation

## Development

### Running Tests

```bash
# Unit tests
go test ./tests/unit/... -v

# Integration tests
go test ./tests/integration/... -v

# All tests with coverage
go test ./... -v -race -cover
```

### Code Quality

```bash
# Format code
go fmt ./...

# Lint
go vet ./...
```

### Database Migrations

```bash
# Create new migration
migrate create -ext sql -dir migrations -seq migration_name

# Apply migrations
migrate -path migrations -database "postgresql://..." up

# Rollback migration
migrate -path migrations -database "postgresql://..." down 1
```

## Monitoring

### Prometheus Metrics

The service exposes metrics at `/metrics`:

- `http_requests_total` - HTTP requests by method, path, status
- `http_request_duration_seconds` - Request duration
- `websocket_active_connections` - Active WS connections
- `blockchain_events_received_total` - Events by type
- `blockchain_events_processed_total` - Successfully processed events
- `blockchain_last_processed_block` - Block progress
- `blockchain_circuit_breaker_state` - Circuit breaker state

### Health Checks

```json
{
  "status": "healthy",
  "database": "connected",
  "ton_center": "available",
  "circuit_breaker": "closed",
  "last_processed_block": 12345678,
  "event_source_type": "websocket"
}
```

## WebSocket Event Streaming

The backend supports two modes for receiving blockchain events from TON Center:

### Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `BLOCKCHAIN_EVENT_SOURCE` | `http` | Event source type: `websocket` or `http` |
| `ENABLE_WEBSOCKET` | `false` | Enable WebSocket streaming |
| `TON_CENTER_V3_WS_URL` | - | WebSocket endpoint URL |
| `WS_RECONNECT_MAX_ATTEMPTS` | `10` | Reconnection attempts before fallback |
| `WS_PING_INTERVAL` | `30s` | Connection health check interval |

### Mode Comparison

| Feature | HTTP Polling | WebSocket |
|---------|--------------|-----------|
| Latency | 5-30 seconds | <2 seconds |
| Connection | Stateless | Persistent |
| Reliability | High | Medium |
| Resource Usage | Higher | Lower |

### Enable WebSocket (Production)

```bash
BLOCKCHAIN_EVENT_SOURCE=websocket
ENABLE_WEBSOCKET=true
TON_CENTER_V3_WS_URL=wss://api.toncenter.com/api/v3/websocket
WS_RECONNECT_MAX_ATTEMPTS=10
WS_PING_INTERVAL=30s
```

### Troubleshooting WebSocket Issues

**Connection Refused**
- Verify `TON_CENTER_V3_WS_URL` is correct
- Check if TON Center v3 API supports WebSocket
- Ensure firewall allows WebSocket connections

**Frequent Disconnections**
- Increase `WS_RECONNECT_MAX_ATTEMPTS`
- Check network stability
- Monitor `/metrics` for `websocket_reconnection_total`

**Fallback to HTTP**
- System automatically falls back after max reconnect attempts
- Check logs for "Falling back to HTTP polling" messages
- Monitor `/health` endpoint for `event_source_type` field

**High Latency with WebSocket**
- Check `blockchain_event_latency_seconds` metric
- Verify TON Center API server proximity
- Consider using geographically closer endpoint

### Monitoring WebSocket Health

Check Prometheus metrics:
```promql
# Active WebSocket connection state
blockchain_websocket_connected

# Reconnection attempts
blockchain_websocket_reconnection_total

# Message processing time
blockchain_event_processing_duration_seconds
```

## Documentation

For detailed specifications:
- `/specs/001-game-backend/spec.md` - Feature specifications
- `/specs/001-game-backend/plan.md` - Implementation plan
- `/specs/001-game-backend/data-model.md` - Database schema
- `/specs/001-game-backend/tasks.md` - Task breakdown

## License

Proprietary - All rights reserved
