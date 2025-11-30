CREATE TABLE blockchain_sync_state (
    id                     INTEGER PRIMARY KEY CHECK (id = 1), -- Singleton table
    contract_address       VARCHAR(66) NOT NULL,
    last_processed_block   BIGINT NOT NULL DEFAULT 0,
    last_poll_timestamp    TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    updated_at             TIMESTAMPTZ DEFAULT NOW() NOT NULL
);

-- Insert the single row with default values
INSERT INTO blockchain_sync_state (id, contract_address, last_processed_block)
VALUES (1, '', 0);

-- Trigger function to auto-update updated_at
CREATE TRIGGER update_blockchain_sync_state_updated_at
    BEFORE UPDATE ON blockchain_sync_state
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Index for querying (though it's a singleton, useful for consistency)
CREATE INDEX idx_blockchain_sync_state_contract ON blockchain_sync_state(contract_address);
