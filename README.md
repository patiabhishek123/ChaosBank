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