-- Rollback: Remove composite indexes for player address queries

DROP INDEX IF EXISTS idx_games_player_one_created;
DROP INDEX IF EXISTS idx_games_player_two_created;
