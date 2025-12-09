-- Migration 000007 down: Revert to original constraint (1-3 only)

ALTER TABLE games DROP CONSTRAINT IF EXISTS games_player_one_choice_check;

ALTER TABLE games ADD CONSTRAINT games_player_one_choice_check
    CHECK (player_one_choice >= 1 AND player_one_choice <= 3);
