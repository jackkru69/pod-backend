-- Revert to require player_one_choice to be 1-3
ALTER TABLE games DROP CONSTRAINT games_player_one_choice_check;
ALTER TABLE games ADD CONSTRAINT games_player_one_choice_check CHECK (player_one_choice >= 1 AND player_one_choice <= 3);
