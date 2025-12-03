-- Allow telegram_user_id to be nullable for blockchain-only users
-- When a user is first discovered via blockchain events (GameInitializedNotify),
-- we only have their wallet_address. The telegram_user_id will be populated
-- when the user connects via Telegram later.
ALTER TABLE users ALTER COLUMN telegram_user_id DROP NOT NULL;

-- Add default value of NULL for blockchain-only users
ALTER TABLE users ALTER COLUMN telegram_user_id SET DEFAULT NULL;
