-- Migration: Add composite index for player address queries
-- This optimizes GetByPlayerAddress query which uses OR condition
-- 
-- Current query: WHERE player_one_address = ? OR player_two_address = ?
-- Problem: OR conditions don't efficiently use single-column indexes
-- Solution: PostgreSQL can use bitmap index scan with multiple single-column indexes

-- Create additional index on player_one_address with created_at for sorting
-- (existing idx_games_player_one_address only covers player_one_address)
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_games_player_one_created 
    ON games(player_one_address, created_at DESC);

-- Create additional index on player_two_address with created_at for sorting  
-- (existing idx_games_player_two_address only covers player_two_address)
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_games_player_two_created 
    ON games(player_two_address, created_at DESC) 
    WHERE player_two_address IS NOT NULL;

-- Note: PostgreSQL optimizer will use BitmapOr with these two indexes
-- for the OR query pattern, which is more efficient than sequential scan.
--
-- Alternative considered: UNION query instead of OR
-- Pros: Can be more efficient in some cases
-- Cons: Requires code changes, more complex query
-- Decision: Keep OR query with better indexes for now
