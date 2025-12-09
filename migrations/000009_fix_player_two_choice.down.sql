-- Migration 000009 down: Revert to original constraint (1-3 only)
--
-- WARNING: This may fail if any game records have player_two_choice = 0.
-- Manual data cleanup may be required before running this down migration.

ALTER TABLE games DROP CONSTRAINT IF EXISTS games_player_two_choice_check;

ALTER TABLE games ADD CONSTRAINT games_player_two_choice_check
    CHECK (player_two_choice >= 1 AND player_two_choice <= 3);
