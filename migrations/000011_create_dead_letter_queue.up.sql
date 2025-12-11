-- Migration: Create dead_letter_queue table for failed transaction storage
-- This enables retry and analysis of failed blockchain event parsing (Issue #6)

CREATE TABLE dead_letter_queue (
    id                BIGSERIAL PRIMARY KEY,
    transaction_hash  VARCHAR(66) NOT NULL,
    transaction_lt    VARCHAR(50) NOT NULL,
    raw_data          TEXT NOT NULL,
    error_message     TEXT NOT NULL,
    error_type        VARCHAR(50) NOT NULL DEFAULT 'unknown', -- parse_error, persistence_error, validation_error
    retry_count       INTEGER DEFAULT 0,
    max_retries       INTEGER DEFAULT 3,
    created_at        TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    last_retry_at     TIMESTAMPTZ,
    next_retry_at     TIMESTAMPTZ,
    resolved_at       TIMESTAMPTZ,
    status            VARCHAR(20) DEFAULT 'pending' NOT NULL, -- pending, retrying, resolved, failed
    resolution_notes  TEXT,
    CONSTRAINT dlq_status_check CHECK (status IN ('pending', 'retrying', 'resolved', 'failed'))
);

-- Indexes for efficient querying
CREATE INDEX idx_dlq_status ON dead_letter_queue(status);
CREATE INDEX idx_dlq_next_retry ON dead_letter_queue(next_retry_at) WHERE status IN ('pending', 'retrying');
CREATE INDEX idx_dlq_transaction_hash ON dead_letter_queue(transaction_hash);
CREATE INDEX idx_dlq_created_at ON dead_letter_queue(created_at);

-- Unique constraint to prevent duplicate entries for same transaction
CREATE UNIQUE INDEX idx_dlq_unique_tx ON dead_letter_queue(transaction_hash, transaction_lt) 
    WHERE status IN ('pending', 'retrying');

COMMENT ON TABLE dead_letter_queue IS 'Stores failed blockchain transactions for retry and analysis';
COMMENT ON COLUMN dead_letter_queue.error_type IS 'Type of error: parse_error, persistence_error, validation_error';
COMMENT ON COLUMN dead_letter_queue.status IS 'Current status: pending, retrying, resolved, failed';
