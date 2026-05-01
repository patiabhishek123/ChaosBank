-- +migrate Up

CREATE TABLE IF NOT EXISTS processed_events (
    event_id UUID PRIMARY KEY,
    topic VARCHAR(255) NOT NULL,
    partition_id INT NOT NULL,
    message_offset BIGINT NOT NULL,
    processed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_processed_events_processed_at ON processed_events(processed_at);

-- +migrate Down

DROP TABLE IF EXISTS processed_events;
