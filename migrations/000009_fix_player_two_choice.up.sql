-- Migration 000009: Fix player_two_choice constraint to allow 0 (unknown)
--
-- BACKGROUND: The original games table creation (migration 000002) set
-- player_two_choice constraint to 1-3, but entity code (game.go:33) shows
-- that 0 (CoinSideUnknown) is valid during game initialization.
--
-- This migration aligns the constraint with entity validation logic.

ALTER TABLE games DROP CONSTRAINT IF EXISTS games_player_two_choice_check;

ALTER TABLE games ADD CONSTRAINT games_player_two_choice_check
    CHECK (player_two_choice >= 0 AND player_two_choice <= 3);
