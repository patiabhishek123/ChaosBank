# ChaosBank Database Schema Design

## Overview

The schema is designed for a distributed banking system with strong ACID guarantees, high concurrency support, and double-spending prevention.

## Tables

### 1. accounts

Stores account information with optimistic locking for concurrent updates.

**Key Features:**
- `version` field for optimistic locking (prevent lost updates)
- `balance` as NUMERIC(19,2) for precise financial calculations
- Status tracking (active, suspended, closed)
- Multi-currency support

**Concurrency Strategy:**
- Optimistic locking via version numbers
- All balance updates check the current version
- Prevents phantom reads during balance updates

**Constraints:**
- Balance must be non-negative (CHECK constraint)
- Account number is unique
- Foreign keys enforced on transactions

### 2. transactions

Immutable record of all transfer attempts with comprehensive tracking.

**Key Features:**
- Status lifecycle: pending → processing → completed/failed → reversed
- Reference codes for integration with external systems
- Comprehensive timestamp tracking (created, updated, completed)
- Failure reason tracking for debugging

**Concurrency Strategy:**
- Append-only pattern (immutable after completion)
- Minimal locking required once status is final

**Constraints:**
- Amount must be positive
- from_account_id ≠ to_account_id
- Both accounts must exist (foreign key constraints with ON DELETE RESTRICT)
- Currency must be ISO 4217 compliant

### 3. ledger_entries

Immutable, append-only audit trail of all account balance changes.

**Key Features:**
- Double-entry bookkeeping (one debit, one credit per transaction)
- Before/after balance snapshots for reconciliation
- Comprehensive audit trail
- Cannot be modified (RESTRICT deletes)

**Concurrency Strategy:**
- Append-only (no updates after creation)
- Minimal locking
- Natural ordering via created_at timestamp

**Constraints:**
- Amount must be positive
- Entry type: debit or credit

### 4. idempotency_keys

Links API requests to transactions for idempotent retry handling.

**Key Features:**
- Request hash for duplicate detection
- Automatic expiration (24 hours)
- Response stored as JSON for replay
- Transaction ID link for verification

**Concurrency Strategy:**
- Unique constraint on key prevents duplicate processing
- Expires automatically for cleanup

**Constraints:**
- Key is unique
- Expires after 24 hours (can be configured)

### 5. account_locks

Distributed lock table for high-concurrency scenarios.

**Key Features:**
- Row-level locks for individual accounts
- TTL-based auto-expiration (30 seconds)
- Optional: used for pessimistic locking on high-contention accounts

**Use Cases:**
- During settlement/batch operations
- When optimistic locking fails repeatedly

## Indexes

### Performance Indexes

```
-- Fast lookups by natural keys
idx_account_number              - UNIQUE, used for login/lookup
idx_idempotency_key            - UNIQUE, prevent duplicate processing

-- Status-based queries
idx_account_status             - Find suspended/closed accounts
idx_transaction_status         - Find pending/processing transactions
idx_idempotency_status         - Find pending idempotency records

-- Time-based queries
idx_account_created_at         - Account creation reporting
idx_transaction_created_at     - Transaction history by date
idx_transaction_completed_at   - Settlement reports
idx_idempotency_expires_at     - Cleanup of expired records

-- Account history queries (composite indexes)
idx_transaction_account_status           - FROM account + status
idx_transaction_account_status_to        - TO account + status
idx_ledger_account_date                  - Account history with date

-- Foreign key performance
idx_transaction_from_account   - Speed up joins/aggregations
idx_transaction_to_account     - Speed up joins/aggregations
idx_ledger_transaction         - Audit trail reconstruction
idx_ledger_account             - Account balance verification
```

## ACID Guarantees

### Atomicity
- Database transactions ensure all-or-nothing semantics
- Ledger entries and account balance updates are atomic
- Idempotency records prevent duplicate processing

### Consistency
- Foreign key constraints prevent orphaned records
- CHECK constraints enforce business rules
- Optimistic locking prevents dirty writes
- Double-entry bookkeeping maintains balance integrity

### Isolation
- SERIALIZABLE isolation for critical operations
- READ COMMITTED default for read-heavy operations
- Row-level locks on account updates
- Snapshot isolation for ledger queries

### Durability
- All transactions committed to disk
- WAL (Write-Ahead Logging) enabled
- Point-in-time recovery capability

## Double-Spending Prevention

### Strategy 1: Optimistic Locking
```sql
-- Account balance update with version check
UPDATE accounts 
SET balance = balance - amount, version = version + 1
WHERE id = from_account_id 
AND version = expected_version
AND balance >= amount  -- Ensure sufficient funds
```

**Benefits:**
- High concurrency (no locking)
- Detects conflicts immediately

**When Used:**
- Normal low-contention transfers

### Strategy 2: Idempotency Keys
```sql
-- First: Create idempotency record (unique key)
INSERT INTO idempotency_keys (key, request_hash) VALUES (...)
-- Prevents duplicate processing of same request

-- Second: Link to transaction when complete
UPDATE idempotency_keys 
SET transaction_id = tx_id, status = 'completed'
WHERE key = request_key
```

**Benefits:**
- Prevents duplicate processing
- Handles client retries safely
- Enables transaction replay

### Strategy 3: Ledger Double-Entry
```sql
-- For each transfer, create TWO entries:
INSERT INTO ledger_entries (transaction_id, account_id, entry_type, amount, ...)
VALUES 
  (tx_id, from_id, 'debit', 100, ...),   -- Debit from sender
  (tx_id, to_id, 'credit', 100, ...);    -- Credit to receiver

-- Sum of debits must equal sum of credits (reconciliation invariant)
```

**Benefits:**
- Immutable audit trail
- Built-in reconciliation
- Enables balance verification

## Concurrency Handling

### High-Concurrency Pattern
```go
// 1. Check idempotency (prevent duplicates)
record, err := idempotencyRepo.CheckAndCreateIfNotExists(key, hash)

// 2. Get accounts with version
from, _ := accountRepo.GetByID(fromID)
to, _ := accountRepo.GetByID(toID)

// 3. Verify funds (pessimistic check)
if from.Balance < amount { return ErrInsufficientFunds }

// 4. Update both accounts atomically
// Use transaction with serializable isolation
tx := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})

// 5. On conflict (optimistic lock failed), retry
if !accountRepo.UpdateBalance(from.ID, newBalance, from.Version) {
  // Retry with fresh version
  return ErrOptimisticLockFailed
}

// 6. Record transfer in ledger (immutable)
ledgerRepo.Create(debitEntry)
ledgerRepo.Create(creditEntry)

// 7. Update transaction status
transactionRepo.UpdateStatus(txID, "completed")

// 8. Update idempotency record
idempotencyRepo.UpdateResponse(key, jsonResponse)
```

### Race Condition Handling

**Scenario: Two concurrent requests to transfer from same account**

1. **Request 1**: Reads account (balance=100, version=1)
2. **Request 2**: Reads account (balance=100, version=1)
3. **Request 1**: Transfers 100 (balance=0, version=2) ✅
4. **Request 2**: Tries to transfer 50, but version=2 ≠ 1 → RETRY

**Outcome**: Request 2 detects the change and retries with fresh data.

## Cleanup & Maintenance

### Idempotency Key Expiration
```sql
-- Automatic cleanup of expired records
DELETE FROM idempotency_keys 
WHERE expires_at < NOW();
-- Run periodically (e.g., hourly)
```

### Account Lock Expiration
```sql
-- Release stale locks
DELETE FROM account_locks 
WHERE expires_at < NOW();
-- Run periodically (e.g., every 5 minutes)
```

### Ledger Archival (optional)
```sql
-- Archive old entries to cold storage
INSERT INTO ledger_entries_archive 
SELECT * FROM ledger_entries 
WHERE created_at < DATE_TRUNC('month', NOW() - INTERVAL '6 months');
```

## Monitoring & Observability

### Key Metrics
- Account balance reconciliation (sum of ledger entries)
- Transaction completion rate
- Idempotency key collision rate
- Optimistic lock conflict rate
- Query performance on indexes

### Reconciliation Queries
```sql
-- Verify all account balances match ledger
SELECT a.id, a.balance, 
       COALESCE(SUM(CASE WHEN l.entry_type='credit' THEN l.amount ELSE 0 END) -
                SUM(CASE WHEN l.entry_type='debit' THEN l.amount ELSE 0 END), 0) as ledger_balance
FROM accounts a
LEFT JOIN ledger_entries l ON a.id = l.account_id
GROUP BY a.id
HAVING a.balance != (ledger_balance);
```

## Migration Strategy

### Adding New Columns
- Use `ALTER TABLE` with `DEFAULT` clause
- Non-blocking operations preferred
- Test on staging first

### Backwards Compatibility
- Old code must work with new schema
- Use feature flags for new columns
- Gradual rollout pattern

## Performance Considerations

### Connection Pooling
- Min 10, Max 100 connections
- Timeout: 30 seconds
- Idle timeout: 5 minutes

### Query Optimization
- Use prepared statements
- Avoid N+1 queries
- Batch operations when possible
- Analyze slow queries with EXPLAIN

### Scaling Strategy
- Vertical: Increase hardware
- Horizontal: Sharding by account_id or country (future)
- Read replicas for reporting queries
