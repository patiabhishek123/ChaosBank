-- +migrate Up

-- Accounts table with balance and version for optimistic locking
CREATE TABLE IF NOT EXISTS accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_number VARCHAR(20) NOT NULL UNIQUE,
    owner_name VARCHAR(255) NOT NULL,
    balance NUMERIC(19, 2) NOT NULL DEFAULT 0.00,
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT balance_non_negative CHECK (balance >= 0),
    CONSTRAINT status_check CHECK (status IN ('active', 'suspended', 'closed')),
    CONSTRAINT currency_length CHECK (CHAR_LENGTH(currency) = 3)
);

-- Indexes on accounts for performance
CREATE INDEX idx_account_number ON accounts(account_number);
CREATE INDEX idx_account_status ON accounts(status);
CREATE INDEX idx_account_created_at ON accounts(created_at);

-- Transactions table with comprehensive tracking
CREATE TABLE IF NOT EXISTS transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    from_account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    to_account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    amount NUMERIC(19, 2) NOT NULL,
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    description TEXT,
    reference_code VARCHAR(100),
    failure_reason TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP,
    CONSTRAINT amount_positive CHECK (amount > 0),
    CONSTRAINT from_to_different CHECK (from_account_id != to_account_id),
    CONSTRAINT status_check CHECK (status IN ('pending', 'processing', 'completed', 'failed', 'reversed')),
    CONSTRAINT currency_length CHECK (CHAR_LENGTH(currency) = 3)
);

-- Indexes on transactions for performance and queries
CREATE INDEX idx_transaction_from_account ON transactions(from_account_id);
CREATE INDEX idx_transaction_to_account ON transactions(to_account_id);
CREATE INDEX idx_transaction_status ON transactions(status);
CREATE INDEX idx_transaction_created_at ON transactions(created_at);
CREATE INDEX idx_transaction_completed_at ON transactions(completed_at);
-- Composite index for common queries: find all transactions for an account
CREATE INDEX idx_transaction_account_status ON transactions(from_account_id, status);
CREATE INDEX idx_transaction_account_status_to ON transactions(to_account_id, status);

-- Ledger table for immutable transaction log (append-only)
CREATE TABLE IF NOT EXISTS ledger_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_id UUID NOT NULL REFERENCES transactions(id) ON DELETE RESTRICT,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    entry_type VARCHAR(10) NOT NULL,
    amount NUMERIC(19, 2) NOT NULL,
    balance_before NUMERIC(19, 2) NOT NULL,
    balance_after NUMERIC(19, 2) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT entry_type_check CHECK (entry_type IN ('debit', 'credit')),
    CONSTRAINT amount_positive CHECK (amount > 0)
);

-- Indexes on ledger for audit and reconciliation
CREATE INDEX idx_ledger_transaction ON ledger_entries(transaction_id);
CREATE INDEX idx_ledger_account ON ledger_entries(account_id);
CREATE INDEX idx_ledger_created_at ON ledger_entries(created_at);
CREATE INDEX idx_ledger_account_date ON ledger_entries(account_id, created_at DESC);

-- Idempotency keys table (link to both transactions and requests)
CREATE TABLE IF NOT EXISTS idempotency_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key VARCHAR(255) NOT NULL UNIQUE,
    request_hash VARCHAR(64) NOT NULL,
    transaction_id UUID REFERENCES transactions(id) ON DELETE SET NULL,
    response_data JSON NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL DEFAULT (CURRENT_TIMESTAMP + INTERVAL '24 hours'),
    CONSTRAINT status_check CHECK (status IN ('pending', 'completed', 'failed'))
);

-- Indexes on idempotency keys
CREATE INDEX idx_idempotency_key ON idempotency_keys(key);
CREATE INDEX idx_idempotency_status ON idempotency_keys(status);
CREATE INDEX idx_idempotency_expires_at ON idempotency_keys(expires_at);
CREATE INDEX idx_idempotency_transaction ON idempotency_keys(transaction_id);

-- Account locks table for distributed locking on high-concurrency writes
CREATE TABLE IF NOT EXISTS account_locks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE UNIQUE,
    locked_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    locked_by_request_id VARCHAR(255),
    expires_at TIMESTAMP NOT NULL DEFAULT (CURRENT_TIMESTAMP + INTERVAL '30 seconds'),
    CONSTRAINT lock_duration CHECK (expires_at > locked_at)
);

-- Index for lock lookups and cleanup
CREATE INDEX idx_account_locks_account ON account_locks(account_id);
CREATE INDEX idx_account_locks_expires ON account_locks(expires_at);

-- Trigger function to update updated_at on accounts
CREATE OR REPLACE FUNCTION update_account_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger for accounts table
CREATE TRIGGER update_account_updated_at_trigger
BEFORE UPDATE ON accounts
FOR EACH ROW
EXECUTE FUNCTION update_account_updated_at();

-- Trigger function to update updated_at on transactions
CREATE OR REPLACE FUNCTION update_transaction_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger for transactions table
CREATE TRIGGER update_transaction_updated_at_trigger
BEFORE UPDATE ON transactions
FOR EACH ROW
EXECUTE FUNCTION update_transaction_updated_at();

-- Trigger function to update updated_at on idempotency keys
CREATE OR REPLACE FUNCTION update_idempotency_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger for idempotency keys table
CREATE TRIGGER update_idempotency_updated_at_trigger
BEFORE UPDATE ON idempotency_keys
FOR EACH ROW
EXECUTE FUNCTION update_idempotency_updated_at();

-- +migrate Down

DROP TRIGGER IF EXISTS update_idempotency_updated_at_trigger ON idempotency_keys;
DROP TRIGGER IF EXISTS update_transaction_updated_at_trigger ON transactions;
DROP TRIGGER IF EXISTS update_account_updated_at_trigger ON accounts;
DROP FUNCTION IF EXISTS update_idempotency_updated_at();
DROP FUNCTION IF EXISTS update_transaction_updated_at();
DROP FUNCTION IF EXISTS update_account_updated_at();
DROP TABLE IF EXISTS account_locks;
DROP TABLE IF EXISTS ledger_entries;
DROP TABLE IF EXISTS idempotency_keys;
DROP TABLE IF EXISTS transactions;
DROP TABLE IF EXISTS accounts;
