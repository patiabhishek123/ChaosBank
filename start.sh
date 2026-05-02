#!/usr/bin/env bash
set -e

ROOT="$(cd "$(dirname "$0")" && pwd)"
DB_URL="postgres://chaosbank:chaosbank123@localhost:5432/chaosbank?sslmode=disable"
KAFKA="localhost:9092"

# ── Kill stale processes ──────────────────────────────────────────────────────
echo "→ Clearing ports 8080/8081/8082..."
fuser -k 8080/tcp 8081/tcp 8082/tcp 2>/dev/null || true
pkill -f "go run cmd/main.go" 2>/dev/null || true
sleep 1

# ── Infrastructure ────────────────────────────────────────────────────────────
echo "→ Starting Postgres / Zookeeper / Kafka..."
docker-compose -f "$ROOT/docker-compose.yml" up -d postgres zookeeper kafka

echo "→ Waiting for Postgres..."
until docker exec chaosbank_postgres_1 pg_isready -U chaosbank -d chaosbank -q 2>/dev/null; do sleep 1; done
echo "   Postgres ready."

echo "→ Waiting for Kafka..."
until docker exec chaosbank_kafka_1 kafka-broker-api-versions --bootstrap-server localhost:9092 &>/dev/null; do sleep 2; done
echo "   Kafka ready."

# ── Services ──────────────────────────────────────────────────────────────────
echo "→ Starting transaction-service :8081..."
cd "$ROOT/services/transaction-service"
PORT=8081 DATABASE_URL="$DB_URL" KAFKA_BROKERS="$KAFKA" go run cmd/main.go >/tmp/transaction-svc.log 2>&1 &

echo "→ Starting worker-service :8082..."
cd "$ROOT/services/worker-service"
PORT=8082 DATABASE_URL="$DB_URL" KAFKA_BROKERS="$KAFKA" go run cmd/main.go >/tmp/worker-svc.log 2>&1 &

echo "→ Starting api-gateway :8080..."
cd "$ROOT/services/api-gateway"
PORT=8080 TRANSACTION_SVC_URL="http://localhost:8081" WORKER_SVC_URL="http://localhost:8082" \
  go run cmd/main.go >/tmp/api-gw.log 2>&1 &

# ── Health checks ─────────────────────────────────────────────────────────────
echo "→ Waiting for services..."
for p in 8081 8082 8080; do
  for i in $(seq 1 30); do
    curl -sf "http://localhost:$p/health" >/dev/null 2>&1 && echo "   :$p ✓" && break
    sleep 1
  done
done

echo ""
echo "✅  All services running:"
echo "   API Gateway   → http://localhost:8080/health"
echo "   Transaction   → http://localhost:8081/health"
echo "   Worker        → http://localhost:8082/health"
echo "   Metrics       → http://localhost:8082/metrics"
echo "   Frontend      → http://localhost:5173  (cd services/frontend && npm run dev)"
echo ""
echo "Logs: /tmp/api-gw.log  /tmp/transaction-svc.log  /tmp/worker-svc.log"
