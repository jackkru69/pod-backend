-- Revert telegram_user_id to NOT NULL
-- First set all NULL values to 0 (or some default)
UPDATE users SET telegram_user_id = 0 WHERE telegram_user_id IS NULL;

-- Remove default value
ALTER TABLE users ALTER COLUMN telegram_user_id DROP DEFAULT;

-- Add NOT NULL constraint back
ALTER TABLE users ALTER COLUMN telegram_user_id SET NOT NULL;
