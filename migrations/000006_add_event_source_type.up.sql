-- Migration 000006: Add WebSocket event streaming fields (FIXED)
--
-- CHANGES FROM ORIGINAL:
-- - Removed enum type creation (use VARCHAR instead to match Go string type)
-- - Added missing last_processed_hash column
-- - Changed event_source_type to nullable VARCHAR
-- - Added IF NOT EXISTS/IF EXISTS clauses for idempotency

-- Step 1: Drop existing enum type if it was created by old version of this migration
DO $$
BEGIN
    -- Drop the enum type if it exists
    IF EXISTS (SELECT 1 FROM pg_type WHERE typname = 'event_source_type') THEN
        -- First, we need to drop the column using it, or alter its type
        -- Since we're about to recreate the column anyway, we can drop it
        IF EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_name = 'blockchain_sync_state'
            AND column_name = 'event_source_type'
        ) THEN
            ALTER TABLE blockchain_sync_state DROP COLUMN event_source_type;
        END IF;

        -- Now drop the enum type
        DROP TYPE event_source_type;
    END IF;
END $$;

-- Step 2: Add new columns to blockchain_sync_state
ALTER TABLE blockchain_sync_state
    ADD COLUMN IF NOT EXISTS event_source_type VARCHAR(20),  -- Nullable, no default
    ADD COLUMN IF NOT EXISTS last_processed_lt VARCHAR(32) DEFAULT '0',
    ADD COLUMN IF NOT EXISTS last_processed_hash VARCHAR,  -- NEW COLUMN (was missing)
    ADD COLUMN IF NOT EXISTS websocket_connected BOOLEAN DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS fallback_count INTEGER DEFAULT 0,
    ADD COLUMN IF NOT EXISTS last_fallback_at TIMESTAMPTZ;

-- Step 3: Add comments for documentation
COMMENT ON COLUMN blockchain_sync_state.event_source_type IS
    'Current event source: websocket for real-time (<2s) or http for polling (5-30s). Nullable - empty when not set.';
COMMENT ON COLUMN blockchain_sync_state.last_processed_lt IS
    'Last processed logical time (lt) for TON blockchain';
COMMENT ON COLUMN blockchain_sync_state.last_processed_hash IS
    'Last processed transaction hash (base64) - required with lt for TON API pagination';
COMMENT ON COLUMN blockchain_sync_state.websocket_connected IS
    'Whether WebSocket is currently connected';
COMMENT ON COLUMN blockchain_sync_state.fallback_count IS
    'Number of times system fell back from WebSocket to HTTP';
COMMENT ON COLUMN blockchain_sync_state.last_fallback_at IS
    'Timestamp of last fallback event';
