CREATE TABLE users (
    id                      BIGSERIAL PRIMARY KEY,
    telegram_user_id        BIGINT NOT NULL,
    telegram_username       VARCHAR(255),
    wallet_address          VARCHAR(66) NOT NULL UNIQUE, -- TON address format: EQ... (48 chars base64 + prefix)
    total_games_played      INTEGER DEFAULT 0 NOT NULL,
    total_wins              INTEGER DEFAULT 0 NOT NULL,
    total_losses            INTEGER DEFAULT 0 NOT NULL,
    total_referrals         INTEGER DEFAULT 0 NOT NULL,
    total_referral_earnings BIGINT DEFAULT 0 NOT NULL, -- nanotons
    created_at              TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    updated_at              TIMESTAMPTZ DEFAULT NOW() NOT NULL
);

-- Indexes
CREATE INDEX idx_users_telegram_user_id ON users(telegram_user_id);
CREATE INDEX idx_users_created_at ON users(created_at); -- For data retention queries

-- Trigger function to auto-update updated_at
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger to auto-update updated_at
CREATE TRIGGER update_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
