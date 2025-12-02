-- Migration 000005: Add table partitioning for scalability (T125)
-- Partitions game_events table by month for better performance with large datasets

-- Note: This migration is optional and should only be applied if:
-- 1. You expect >10M game events
-- 2. You have PostgreSQL 10+ with native partitioning support
-- 3. You want to implement time-based data retention with DROP PARTITION

-- Convert game_events to partitioned table
-- WARNING: This requires downtime and data migration

-- Step 1: Rename existing table
ALTER TABLE game_events RENAME TO game_events_old;

-- Step 2: Create partitioned table
CREATE TABLE game_events (
    id BIGSERIAL,
    game_id BIGINT NOT NULL,
    event_type VARCHAR(50) NOT NULL,
    transaction_hash VARCHAR(100) NOT NULL,
    block_number BIGINT NOT NULL,
    payload JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id, created_at)  -- Include partition key in PK
) PARTITION BY RANGE (created_at);

-- Step 3: Create partitions for past, current, and future months
-- Past year (example for 2024)
CREATE TABLE game_events_2024_01 PARTITION OF game_events
    FOR VALUES FROM ('2024-01-01') TO ('2024-02-01');
CREATE TABLE game_events_2024_02 PARTITION OF game_events
    FOR VALUES FROM ('2024-02-01') TO ('2024-03-01');
CREATE TABLE game_events_2024_03 PARTITION OF game_events
    FOR VALUES FROM ('2024-03-01') TO ('2024-04-01');
CREATE TABLE game_events_2024_04 PARTITION OF game_events
    FOR VALUES FROM ('2024-04-01') TO ('2024-05-01');
CREATE TABLE game_events_2024_05 PARTITION OF game_events
    FOR VALUES FROM ('2024-05-01') TO ('2024-06-01');
CREATE TABLE game_events_2024_06 PARTITION OF game_events
    FOR VALUES FROM ('2024-06-01') TO ('2024-07-01');
CREATE TABLE game_events_2024_07 PARTITION OF game_events
    FOR VALUES FROM ('2024-07-01') TO ('2024-08-01');
CREATE TABLE game_events_2024_08 PARTITION OF game_events
    FOR VALUES FROM ('2024-08-01') TO ('2024-09-01');
CREATE TABLE game_events_2024_09 PARTITION OF game_events
    FOR VALUES FROM ('2024-09-01') TO ('2024-10-01');
CREATE TABLE game_events_2024_10 PARTITION OF game_events
    FOR VALUES FROM ('2024-10-01') TO ('2024-11-01');
CREATE TABLE game_events_2024_11 PARTITION OF game_events
    FOR VALUES FROM ('2024-11-01') TO ('2024-12-01');
CREATE TABLE game_events_2024_12 PARTITION OF game_events
    FOR VALUES FROM ('2024-12-01') TO ('2025-01-01');

-- Current year (2025)
CREATE TABLE game_events_2025_01 PARTITION OF game_events
    FOR VALUES FROM ('2025-01-01') TO ('2025-02-01');
CREATE TABLE game_events_2025_02 PARTITION OF game_events
    FOR VALUES FROM ('2025-02-01') TO ('2025-03-01');
CREATE TABLE game_events_2025_03 PARTITION OF game_events
    FOR VALUES FROM ('2025-03-01') TO ('2025-04-01');
CREATE TABLE game_events_2025_04 PARTITION OF game_events
    FOR VALUES FROM ('2025-04-01') TO ('2025-05-01');
CREATE TABLE game_events_2025_05 PARTITION OF game_events
    FOR VALUES FROM ('2025-05-01') TO ('2025-06-01');
CREATE TABLE game_events_2025_06 PARTITION OF game_events
    FOR VALUES FROM ('2025-06-01') TO ('2025-07-01');
CREATE TABLE game_events_2025_07 PARTITION OF game_events
    FOR VALUES FROM ('2025-07-01') TO ('2025-08-01');
CREATE TABLE game_events_2025_08 PARTITION OF game_events
    FOR VALUES FROM ('2025-08-01') TO ('2025-09-01');
CREATE TABLE game_events_2025_09 PARTITION OF game_events
    FOR VALUES FROM ('2025-09-01') TO ('2025-10-01');
CREATE TABLE game_events_2025_10 PARTITION OF game_events
    FOR VALUES FROM ('2025-10-01') TO ('2025-11-01');
CREATE TABLE game_events_2025_11 PARTITION OF game_events
    FOR VALUES FROM ('2025-11-01') TO ('2025-12-01');
CREATE TABLE game_events_2025_12 PARTITION OF game_events
    FOR VALUES FROM ('2025-12-01') TO ('2026-01-01');

-- Future months (next 6 months of 2026)
CREATE TABLE game_events_2026_01 PARTITION OF game_events
    FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');
CREATE TABLE game_events_2026_02 PARTITION OF game_events
    FOR VALUES FROM ('2026-02-01') TO ('2026-03-01');
CREATE TABLE game_events_2026_03 PARTITION OF game_events
    FOR VALUES FROM ('2026-03-01') TO ('2026-04-01');
CREATE TABLE game_events_2026_04 PARTITION OF game_events
    FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');
CREATE TABLE game_events_2026_05 PARTITION OF game_events
    FOR VALUES FROM ('2026-05-01') TO ('2026-06-01');
CREATE TABLE game_events_2026_06 PARTITION OF game_events
    FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');

-- Step 4: Create indexes on partitioned table
CREATE INDEX idx_game_events_game_id ON game_events(game_id);
CREATE INDEX idx_game_events_tx_hash ON game_events(transaction_hash);
CREATE INDEX idx_game_events_type ON game_events(event_type);
CREATE INDEX idx_game_events_block ON game_events(block_number);

-- Step 5: Migrate data from old table (run this carefully!)
-- INSERT INTO game_events SELECT * FROM game_events_old;

-- Step 6: Drop old table after verification
-- DROP TABLE game_events_old;

-- Note: To drop old partitions for data retention:
-- DROP TABLE game_events_2024_01;  -- Removes all Jan 2024 data instantly
