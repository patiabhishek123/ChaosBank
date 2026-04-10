-- +migrate Up
CREATE TABLE IF NOT EXISTS idempotency_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key VARCHAR(255) NOT NULL UNIQUE,
    request_hash VARCHAR(64) NOT NULL,
    response TEXT,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT status_check CHECK (status IN ('pending', 'completed', 'failed'))
);

CREATE INDEX idx_idempotency_key ON idempotency_records(key);
CREATE INDEX idx_idempotency_status ON idempotency_records(status);

-- Trigger to update updated_at automatically
CREATE OR REPLACE FUNCTION update_idempotency_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_idempotency_updated_at_trigger
BEFORE UPDATE ON idempotency_records
FOR EACH ROW
EXECUTE FUNCTION update_idempotency_updated_at();

-- +migrate Down
DROP TRIGGER IF EXISTS update_idempotency_updated_at_trigger ON idempotency_records;
DROP FUNCTION IF EXISTS update_idempotency_updated_at();
DROP TABLE IF EXISTS idempotency_records;
