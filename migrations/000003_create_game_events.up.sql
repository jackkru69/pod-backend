CREATE TABLE game_events (
    id                BIGSERIAL PRIMARY KEY,
    game_id           BIGINT NOT NULL,
    event_type        VARCHAR(50) NOT NULL CHECK (
        event_type IN (
            'GameInitializedNotify',
            'GameStartedNotify',
            'GameFinishedNotify',
            'GameCancelledNotify',
            'DrawNotify',
            'SecretOpenedNotify',
            'InsufficientBalanceNotify'
        )
    ),
    transaction_hash  VARCHAR(66) NOT NULL,
    block_number      BIGINT NOT NULL CHECK (block_number >= 0),
    timestamp         TIMESTAMPTZ NOT NULL,
    payload           TEXT NOT NULL,
    created_at        TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    CONSTRAINT unique_event_per_tx UNIQUE (game_id, transaction_hash, event_type)
);

-- Indexes for queries and duplicate detection
CREATE INDEX idx_game_events_game_id ON game_events(game_id);
CREATE INDEX idx_game_events_transaction_hash ON game_events(transaction_hash);
CREATE INDEX idx_game_events_block_number ON game_events(block_number);
CREATE INDEX idx_game_events_timestamp ON game_events(timestamp);
CREATE INDEX idx_game_events_event_type ON game_events(event_type);

-- Foreign key to games table
ALTER TABLE game_events
    ADD CONSTRAINT fk_game_events_game
    FOREIGN KEY (game_id)
    REFERENCES games(game_id)
    ON DELETE CASCADE;
