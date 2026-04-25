ALTER TABLE games
  DROP COLUMN IF EXISTS protocol_version,
  DROP COLUMN IF EXISTS min_referrer_payout_value;
