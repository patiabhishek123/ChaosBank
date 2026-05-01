# ChaosBank

A production-grade Go-based distributed system for banking operations.

## Architecture

This monorepo contains:

- **api-gateway**: Entry point for all API requests
- **transaction-service**: Handles transaction processing
- **worker-service**: Background worker for processing messages
- **pkg/**: Shared packages

## Services

### API Gateway
- Port: 8080
- Routes: /health, etc.

### Transaction Service
- Port: 8081
- Handles transaction creation and management

### Worker Service
- Consumes Kafka messages for background processing

## Infrastructure

- **Kafka**: Message broker
- **Zookeeper**: Kafka coordination
- **PostgreSQL**: Database

## Running

1. Start infrastructure:
   ```bash
   docker-compose up -d
   ```

2. Build and run services:
   ```bash
   # For each service
   cd services/api-gateway
   go run cmd/main.go
   ```

## Environment Variables

Each service uses environment variables for configuration. See config/ directories for details.

### Kafka replay controls (worker-service)

- `KAFKA_GROUP_ID` (default: `worker-group`)
- `KAFKA_REPLAY_FROM_BEGINNING` (default: `false`)
- `REPLAY_ENABLED` (default: `false`)
- `REPLAY_CONFIRM_TOKEN` (default: `REPLAY_ALL_EVENTS`)

When `KAFKA_REPLAY_FROM_BEGINNING=true`, the worker starts with a fresh replay group and consumes from offset `0` for topic `transactions`.

### Event retention

Kafka broker is configured with:

- `KAFKA_LOG_RETENTION_MS=-1`
- `KAFKA_LOG_RETENTION_BYTES=-1`

This keeps events from being deleted, enabling replay.

### Replay endpoint

- Endpoint: `POST /replay` (worker-service)
- Required header: `X-Replay-Confirm: <REPLAY_CONFIRM_TOKEN>`
- Safety checks:
   - replay must be enabled (`REPLAY_ENABLED=true`)
   - `CHAOS_MODE` must be disabled
   - only one replay can run at a time

Behavior:

- Clears replay tables (`accounts`, `transactions`, `ledger_entries`, `processed_events`, `account_locks`)
- Reprocesses all Kafka events from offset `0` across all `transactions` topic partitions
- Rebuilds balances by replaying transfer events
- Emits progress logs (`worker.replay.*`)