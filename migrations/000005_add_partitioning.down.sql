-- Rollback partitioning migration
-- Convert back to regular table

-- Rename partitioned table
ALTER TABLE game_events RENAME TO game_events_partitioned;

-- Recreate original table structure
CREATE TABLE game_events (
    id BIGSERIAL PRIMARY KEY,
    game_id BIGINT NOT NULL,
    event_type VARCHAR(50) NOT NULL,
    transaction_hash VARCHAR(100) NOT NULL UNIQUE,
    block_number BIGINT NOT NULL,
    payload JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Recreate indexes
CREATE INDEX idx_game_events_game_id ON game_events(game_id);
CREATE INDEX idx_game_events_tx_hash ON game_events(transaction_hash);
CREATE INDEX idx_game_events_type ON game_events(event_type);
CREATE INDEX idx_game_events_block ON game_events(block_number);

-- Migrate data back
INSERT INTO game_events SELECT * FROM game_events_partitioned;

-- Drop partitioned table
DROP TABLE game_events_partitioned CASCADE;
