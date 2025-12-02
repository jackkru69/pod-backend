-- Rollback event_source_type migration (T145)

-- Remove columns from blockchain_sync_state
ALTER TABLE blockchain_sync_state
DROP COLUMN IF EXISTS event_source_type,
DROP COLUMN IF EXISTS last_processed_lt,
DROP COLUMN IF EXISTS websocket_connected,
DROP COLUMN IF EXISTS fallback_count,
DROP COLUMN IF EXISTS last_fallback_at;

-- Drop the event_source_type enum
DROP TYPE IF EXISTS event_source_type;
