-- Migration 000005: Partitioning removed per design decision
--
-- RATIONALE: Feature spec 001-fix-migrations determined that table
-- partitioning adds unnecessary complexity. Data retention will be
-- managed through application-level cleanup.
--
-- This file is kept as a no-op to maintain migration sequence numbering.
-- Existing databases with partitions should manually consolidate data
-- before applying this change, or skip to migration 000006.

SELECT 1 AS migration_005_removed;
