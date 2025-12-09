-- Migration 000006 down: Remove WebSocket event streaming fields
--
-- WARNING: This down migration will NOT recreate the enum type that was
-- in the original migration, since it was incorrect (didn't match entity code).
-- Best-effort reversal - simply drops the added columns.

ALTER TABLE blockchain_sync_state
    DROP COLUMN IF EXISTS event_source_type,
    DROP COLUMN IF EXISTS last_processed_lt,
    DROP COLUMN IF EXISTS last_processed_hash,
    DROP COLUMN IF EXISTS websocket_connected,
    DROP COLUMN IF EXISTS fallback_count,
    DROP COLUMN IF EXISTS last_fallback_at;
