# ChaosBank

<p align="center">
  <img src="public/Logo.png" alt="ChaosBank logo" width="1000">
</p>

ChaosBank is a distributed banking simulation built to demonstrate production-minded backend design under failure, retries, replay, and concurrency pressure.

It is intentionally designed like a system design interview project:

- synchronous API for client-facing requests
- asynchronous processing via Kafka
- transactional balance updates in PostgreSQL
- idempotency at the edge
- deduplication in the worker
- replayable event log
- chaos injection for resilience testing
- operational endpoints for replay, chaos control, logs, stats, and metrics

## Why this project is interesting

ChaosBank is not just a CRUD app. It models the kinds of failure modes and guarantees interviewers care about:

- What happens if a client retries the same request?
- What happens if Kafka redelivers the same message?
- What happens if the worker crashes after commit but before acknowledgement?
- How do you replay events from the beginning and rebuild state?
- How do you inject failure safely to prove the system is robust?

This repository answers those questions with concrete code.

## Architecture

### High-level flow

```mermaid
flowchart LR
      U[Client / Frontend] --> AG[API Gateway :8080]
      AG --> TS[Transaction Service :8081]
      TS --> IDEMP[(Idempotency Records)]
      TS --> K[(Kafka Topic: transactions)]
      WS[Worker Service :8082] --> K
      WS --> PG[(PostgreSQL)]
      WS --> METRICS[/metrics]
      WS --> ADMIN[/chaos /replay /stats /transactions/log]
      FE[React Frontend :5173] --> WS

      subgraph State in PostgreSQL
         PG --> A[accounts]
         PG --> T[transactions]
         PG --> L[ledger_entries]
         PG --> P[processed_events]
      end
```

### Request lifecycle

1. Client sends `POST /transfer` with an `Idempotency-Key`.
2. Transaction service validates the request and checks/creates an idempotency record.
3. Transaction service publishes a transfer event to Kafka.
4. Worker consumes the event.
5. Worker deduplicates on `eventId`.
6. Worker applies debit/credit in a single DB transaction.
7. Worker records immutable transaction + ledger entries.
8. Metrics, logs, replay, and chaos endpoints make the system observable and testable.

## Monorepo layout

```text
services/
   api-gateway/
   transaction-service/
   worker-service/
   frontend/
pkg/
   chaos/
   metrics/
   service/
   util/
```

## Core design decisions

### 1) API edge is idempotent

The transaction service treats the `Idempotency-Key` as the client contract.

- same key + same request body → cached accepted response
- same key + different request body → conflict
- no key → reject

This ensures safe client retries even when the caller times out or loses the response.

### 2) Processing is asynchronous

The API does not directly mutate balances.

Instead, it:

- validates input
- writes/checks idempotency state
- publishes an event to Kafka
- returns immediately with `accepted`

This decouples request acceptance from execution and makes replay possible.

### 3) Worker is the source of state mutation

The worker is the only component that actually mutates balances.

That simplifies invariants:

- one place owns balance transitions
- one place owns dedupe logic
- one place owns replay semantics

### 4) Postgres is used transactionally

Transfers are applied with:

- a SQL transaction
- row-level locking (`FOR UPDATE`)
- guarded debit (`balance >= amount`)
- rollback on any failure

This prevents negative balances and protects against race conditions during concurrent transfers.

## How idempotency works

There are two layers of protection.

### Layer 1: API idempotency

At the HTTP boundary, the transaction service stores:

- `key`
- request hash
- response payload
- status

Flow:

1. Hash incoming request.
2. `CheckAndCreateIfNotExists(idempotencyKey, requestHash)`.
3. If record exists with different hash → return `409 Conflict`.
4. If record exists with completed response → return cached response.
5. If new → publish Kafka event and store accepted response.

This solves duplicate submissions from the client side.

### Layer 2: Worker deduplication

At the event-processing layer, the worker uses `processed_events`.

Flow:

1. Start SQL transaction.
2. Check whether `eventId` already exists in `processed_events`.
3. If already processed → skip safely.
4. If not processed → insert `eventId` and process transfer.
5. Commit transaction.

This solves duplicate Kafka delivery, replay, and retry-after-crash scenarios.

### Why both layers matter

- API idempotency protects clients.
- Worker dedupe protects balances.

Together they provide end-to-end safety.

## How money transfer works

Transfers are applied in the worker inside one DB transaction.

### Invariants

- sender must exist and be active
- receiver must exist and be active
- sender and receiver must differ
- amount must be positive
- sender balance must not go negative

### Execution steps

1. Lock both account rows in deterministic order.
2. Validate available balance.
3. Debit sender.
4. Credit receiver.
5. Insert a `transactions` record.
6. Insert two `ledger_entries` rows:
    - debit entry
    - credit entry
7. Commit.

If anything fails, the transaction rolls back.

### Why this is safe under concurrency

- row locks serialize conflicting updates
- debit uses `balance >= amount`
- dedupe prevents repeated application
- replay uses the same logic as live processing

## Failure injection / chaos mode

ChaosBank includes a shared failure injection system in `pkg/chaos`.

It can be enabled with:

```bash
CHAOS_MODE=true
```

### What chaos mode injects

- random delay: `0–3s`
- random failure: `10–30%`
- duplicate request simulation
- DB failure simulation
- network timeout simulation
- partial failure simulation

### Where it is applied

- HTTP middleware for API paths
- Kafka producer writes
- Kafka consumer fetch/commit path
- worker DB operations

### Why this is useful

It lets you prove that:

- retries do not double-charge
- duplicate events do not double-update balances
- replay still converges to correct state
- partial failures are survivable because state mutation is transactional and deduplicated

### Runtime chaos controls

The worker exposes:

- `GET /chaos`
- `POST /chaos`

So chaos mode can be toggled from the frontend without restarting the service.

## Replay mode

Replay is a first-class operational feature.

### Why replay exists

If the system state becomes suspect, or if you want to demonstrate event sourcing-style rebuild behavior, you can wipe mutable state and reconstruct it by replaying the Kafka log from offset `0`.

### Replay guarantees

- Kafka retention is configured to keep events indefinitely.
- Replay runs against the same worker transfer logic used in live processing.
- Replay disables overlap with normal processing via a replay guard.
- `processed_events` is rebuilt during replay.

### Safety checks

Replay requires:

- `REPLAY_ENABLED=true`
- valid `X-Replay-Confirm` header
- `CHAOS_MODE=false`
- no other replay currently running

### Replay behavior

`POST /replay` will:

1. verify safety conditions
2. clear mutable state tables:
    - `accounts`
    - `transactions`
    - `ledger_entries`
    - `processed_events`
    - `account_locks`
3. scan the Kafka `transactions` topic from offset `0`
4. process every event partition-by-partition
5. rebuild balances and ledger state
6. emit progress logs during execution

### Replay-related environment variables

- `REPLAY_ENABLED=false`
- `REPLAY_CONFIRM_TOKEN=REPLAY_ALL_EVENTS`
- `KAFKA_REPLAY_FROM_BEGINNING=false`
- `KAFKA_GROUP_ID=worker-group`

## Metrics and observability

The worker exposes Prometheus-style metrics at:

- `GET /metrics`

Tracked metrics:

- total transactions
- failed transactions
- retries
- processing latency

Examples:

- `chaosbank_total_transactions_total`
- `chaosbank_failed_transactions_total`
- `chaosbank_transaction_retries_total`
- `chaosbank_processing_latency_seconds_avg`

The worker also exposes operational JSON endpoints:

- `GET /transactions/log`
- `GET /stats`
- `GET /chaos`
- `POST /chaos`
- `POST /replay`

## Frontend

The frontend is intentionally minimalist and terminal-inspired.

### Design language

- black and white palette
- sharp edges
- heavy bottom-right shadows
- monospace typography

### Pages

- live transaction log
- chaos toggle
- replay trigger
- system stats

This turns ChaosBank into a usable demo instead of just a code sample.

## Running the system

### 1. One-command startup (recommended)

Use the root launcher script to:

- clear stale ports/processes
- start Docker dependencies
- run DB migrations
- seed demo accounts (`acc-alice`, `acc-bob`)
- start all Go services

```bash
bash start.sh
```

### 2. Run frontend

```bash
npm --prefix services/frontend install
npm --prefix services/frontend run dev -- --port 5174
```

Open:

- frontend: `http://localhost:5174`
- api-gateway: `http://localhost:8080`
- transaction-service: `http://localhost:8081`
- worker-service: `http://localhost:8082`

### 3. Manual startup (optional)

If you prefer separate terminals instead of `start.sh`:

### 3.1 Start infrastructure

```bash
docker-compose up -d
```

### 3.2 Run services

Terminal 1:

```bash
cd services/api-gateway
go run cmd/main.go
```

Terminal 2:

```bash
cd services/transaction-service
go run cmd/main.go
```

Terminal 3:

```bash
cd services/worker-service
go run cmd/main.go
```

### 3.3 Run frontend

```bash
cd services/frontend
npm install
npm run dev
```

Then open:

- frontend: `http://localhost:5173`
- api-gateway: `http://localhost:8080`
- transaction-service: `http://localhost:8081`
- worker-service: `http://localhost:8082`

## Demo script

This sequence works well in interviews or demos.

### Demo 1: Basic transfer acceptance

```bash
curl -X POST http://localhost:8081/transfer \
   -H 'Content-Type: application/json' \
   -H 'Idempotency-Key: 11111111-1111-1111-1111-111111111111' \
   -d '{"from":"acc-alice","to":"acc-bob","amount":50.00}'
```

What to explain:

- API validates + stores idempotency state
- event is published to Kafka
- response is accepted immediately
- worker does the actual debit/credit later

### Demo 2: Idempotency

Repeat the exact same request with the same key.

Expected behavior:

- same accepted response returned
- no duplicate transfer applied

Then send the same key with a different payload.

Expected behavior:

- `409 Conflict`

### Demo 3: Chaos mode

Enable chaos mode from the UI or via worker endpoint:

```bash
curl -X POST http://localhost:8082/chaos \
   -H 'Content-Type: application/json' \
   -d '{"enabled":true}'
```

What to explain:

- delays, failures, duplicates, DB/network issues can now be injected
- the system remains safe because of idempotency + dedupe + DB transactions

### Demo 4: Replay from the event log

Disable chaos first, then trigger replay:

```bash
curl -X POST http://localhost:8082/replay \
   -H 'X-Replay-Confirm: REPLAY_ALL_EVENTS'
```

> Note: replay is disabled by default unless `REPLAY_ENABLED=true` is set for worker-service.

What to explain:

- DB state is cleared
- Kafka log is consumed from offset `0`
- balances are rebuilt from historical events
- this demonstrates recoverability and event-log-driven reconstruction

### Demo 5: Metrics

```bash
curl http://localhost:8082/metrics
```

What to explain:

- throughput
- failures
- retries
- latency

## Frontend walkthrough script (ready to speak)

### 60-second version

1. "This UI has four operational pages: live transaction log, chaos toggle, replay, and system stats."
2. "Transaction Log auto-refreshes every 2 seconds and shows completed processing, not just request acceptance."
3. "Chaos Toggle enables runtime fault injection to test retries and resilience without restart."
4. "Replay rebuilds state from Kafka history with safety guards and confirmation token."
5. "System Stats gives live health and counts for accounts, transactions, and processed events."

### 2–3 minute version

- **Transaction Log**
   - shows near-real-time worker outcomes
   - confirms async pipeline completion
   - useful for observing lag/failures quickly

- **Chaos Toggle**
   - switches failure injection ON/OFF live
   - demonstrates robustness of idempotency + dedupe + transactional updates

- **Replay**
   - clears mutable state and reprocesses topic history
   - demonstrates recoverability and deterministic rebuild
   - protected by `REPLAY_ENABLED` and `X-Replay-Confirm`

- **System Stats**
   - summarizes system state in one view
   - correlates user actions with backend effects

### Optional live traffic generator for frontend demo

```bash
for i in 1 2 3; do
   KEY=$(cat /proc/sys/kernel/random/uuid)
   curl -s -X POST http://localhost:8081/transfer \
      -H "Content-Type: application/json" \
      -H "Idempotency-Key: $KEY" \
      -d "{\"from\":\"acc-alice\",\"to\":\"acc-bob\",\"amount\":$((i*50))}"
   echo
done
```

## What interviewers usually ask about this design

### Why not write balances directly in the API?

Because decoupling acceptance from execution makes retries, replay, and failure testing easier. It also reflects real-world event-driven architectures.

### Why not rely only on Kafka exactly-once semantics?

Because application-level dedupe is still needed in distributed systems. The worker protects the business invariant directly with `processed_events`.

### Why is replay safe?

Because:

- events are retained
- offsets can be read from the beginning
- state mutation is deterministic and transactional
- duplicate application is blocked by dedupe

### What would you do next in production?

- proper account creation and bootstrap flow
- authentication and authorization
- schema migrations at startup or CI/CD
- distributed tracing
- dead-letter queue for poison events
- Prometheus + Grafana integration
- partitioning strategy and load testing
- multi-broker Kafka and replicated PostgreSQL

## Tech stack

- Go 1.21
- PostgreSQL 15
- Kafka + Zookeeper
- Docker Compose
- React + Vite frontend

## Project goals

This repository is optimized to demonstrate:

- backend engineering judgment
- resilience thinking
- concurrency safety
- replayability
- observability
- interview-grade system design storytelling

If you want one sentence for interviews:

> ChaosBank is a fault-injected, replayable, idempotent event-driven banking system that demonstrates how to safely process money movements under retries, crashes, duplicates, and concurrent load.
