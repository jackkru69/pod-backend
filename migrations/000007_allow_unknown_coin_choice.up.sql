-- Migration 000007: Allow player_one_choice to be 0 (unknown) during initialization
-- (Idempotent version with IF EXISTS clause)

ALTER TABLE games DROP CONSTRAINT IF EXISTS games_player_one_choice_check;

ALTER TABLE games ADD CONSTRAINT games_player_one_choice_check
    CHECK (player_one_choice >= 0 AND player_one_choice <= 3);
