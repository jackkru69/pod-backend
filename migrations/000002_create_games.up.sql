CREATE TABLE games (
    game_id                 BIGINT PRIMARY KEY,
    status                  INTEGER NOT NULL CHECK (status >= 0 AND status <= 4),
    player_one_address      VARCHAR(66) NOT NULL,
    player_two_address      VARCHAR(66),
    player_one_choice       INTEGER NOT NULL CHECK (player_one_choice >= 1 AND player_one_choice <= 3),
    player_two_choice       INTEGER CHECK (player_two_choice >= 1 AND player_two_choice <= 3),
    player_one_referrer     VARCHAR(66),
    player_two_referrer     VARCHAR(66),
    bet_amount              BIGINT NOT NULL CHECK (bet_amount > 0),
    winner_address          VARCHAR(66),
    payout_amount           BIGINT,
    service_fee_numerator   BIGINT NOT NULL,
    referrer_fee_numerator  BIGINT NOT NULL,
    waiting_timeout_seconds BIGINT NOT NULL,
    lowest_bid_allowed      BIGINT NOT NULL,
    highest_bid_allowed     BIGINT NOT NULL,
    fee_receiver_address    VARCHAR(66) NOT NULL,
    created_at              TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    joined_at               TIMESTAMPTZ,
    revealed_at             TIMESTAMPTZ,
    completed_at            TIMESTAMPTZ,
    init_tx_hash            VARCHAR(66) NOT NULL,
    join_tx_hash            VARCHAR(66),
    reveal_tx_hash          TEXT,
    complete_tx_hash        VARCHAR(66)
);

-- Indexes for common queries
CREATE INDEX idx_games_status ON games(status);
CREATE INDEX idx_games_player_one_address ON games(player_one_address);
CREATE INDEX idx_games_player_two_address ON games(player_two_address) WHERE player_two_address IS NOT NULL;
CREATE INDEX idx_games_created_at ON games(created_at);
CREATE INDEX idx_games_completed_at ON games(completed_at) WHERE completed_at IS NOT NULL;

-- Foreign key constraints
ALTER TABLE games
    ADD CONSTRAINT fk_games_player_one
    FOREIGN KEY (player_one_address)
    REFERENCES users(wallet_address)
    ON DELETE RESTRICT;

ALTER TABLE games
    ADD CONSTRAINT fk_games_player_two
    FOREIGN KEY (player_two_address)
    REFERENCES users(wallet_address)
    ON DELETE RESTRICT;
