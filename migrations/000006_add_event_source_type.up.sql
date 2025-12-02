-- Add event_source_type column to track WebSocket vs HTTP polling source (T144)
-- Add last_processed_lt column for TON logical time tracking

-- Add event_source_type enum
DO $$ 
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'event_source_type') THEN
        CREATE TYPE event_source_type AS ENUM ('websocket', 'http');
    END IF;
END $$;

-- Add new columns to blockchain_sync_state
ALTER TABLE blockchain_sync_state
ADD COLUMN IF NOT EXISTS event_source_type event_source_type DEFAULT 'http' NOT NULL,
ADD COLUMN IF NOT EXISTS last_processed_lt VARCHAR(32) DEFAULT '0',
ADD COLUMN IF NOT EXISTS websocket_connected BOOLEAN DEFAULT FALSE,
ADD COLUMN IF NOT EXISTS fallback_count INTEGER DEFAULT 0,
ADD COLUMN IF NOT EXISTS last_fallback_at TIMESTAMPTZ;

-- Add comment for documentation
COMMENT ON COLUMN blockchain_sync_state.event_source_type IS 'Current event source: websocket for real-time (<2s) or http for polling (5-30s)';
COMMENT ON COLUMN blockchain_sync_state.last_processed_lt IS 'Last processed logical time (lt) for TON blockchain';
COMMENT ON COLUMN blockchain_sync_state.websocket_connected IS 'Whether WebSocket is currently connected';
COMMENT ON COLUMN blockchain_sync_state.fallback_count IS 'Number of times system fell back from WebSocket to HTTP';
COMMENT ON COLUMN blockchain_sync_state.last_fallback_at IS 'Timestamp of last fallback event';
