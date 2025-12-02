-- Allow player_one_choice to be 0 (unknown) during game initialization
-- The choice is revealed later in SecretOpenedNotify event
ALTER TABLE games DROP CONSTRAINT games_player_one_choice_check;
ALTER TABLE games ADD CONSTRAINT games_player_one_choice_check CHECK (player_one_choice >= 0 AND player_one_choice <= 3);
