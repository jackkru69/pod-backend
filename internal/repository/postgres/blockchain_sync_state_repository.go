package postgres

import (
	"context"
	"fmt"
	"time"

	"pod-backend/internal/entity"
	"pod-backend/internal/repository"
	"pod-backend/pkg/postgres"
)

// Ensure BlockchainSyncStateRepository implements repository.BlockchainSyncStateRepository interface
var _ repository.BlockchainSyncStateRepository = (*BlockchainSyncStateRepository)(nil)

// BlockchainSyncStateRepository implements repository.BlockchainSyncStateRepository using PostgreSQL.
// This is a singleton repository (only one row exists in the database with id=1).
type BlockchainSyncStateRepository struct {
	pg *postgres.Postgres
}

// NewBlockchainSyncStateRepository creates a new PostgreSQL-backed blockchain sync state repository.
func NewBlockchainSyncStateRepository(pg *postgres.Postgres) *BlockchainSyncStateRepository {
	return &BlockchainSyncStateRepository{pg: pg}
}

// Get retrieves the current blockchain sync state.
func (r *BlockchainSyncStateRepository) Get(ctx context.Context) (*entity.BlockchainSyncState, error) {
	sql, args, err := r.pg.Builder.
		Select(
			"id",
			"contract_address",
			"last_processed_block",
			"last_poll_timestamp",
			"updated_at",
		).
		From("blockchain_sync_state").
		Where("id = ?", 1).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	state := &entity.BlockchainSyncState{}
	err = r.pg.Pool.QueryRow(ctx, sql, args...).Scan(
		&state.ID,
		&state.ContractAddress,
		&state.LastProcessedBlock,
		&state.LastPollTimestamp,
		&state.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("execute query: %w", err)
	}

	return state, nil
}

// UpdateLastProcessedBlock atomically updates the last processed block number.
func (r *BlockchainSyncStateRepository) UpdateLastProcessedBlock(ctx context.Context, blockNumber int64) error {
	now := time.Now()
	sql, args, err := r.pg.Builder.
		Update("blockchain_sync_state").
		Set("last_processed_block", blockNumber).
		Set("last_poll_timestamp", now).
		Where("id = ?", 1).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = r.pg.Pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("execute query: %w", err)
	}

	return nil
}

// Initialize sets up the blockchain sync state for a contract address.
func (r *BlockchainSyncStateRepository) Initialize(ctx context.Context, contractAddress string, startingBlock int64) error {
	// Try to update first (idempotent operation)
	sql, args, err := r.pg.Builder.
		Update("blockchain_sync_state").
		Set("contract_address", contractAddress).
		Set("last_processed_block", startingBlock).
		Where("id = ?", 1).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	result, err := r.pg.Pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("execute query: %w", err)
	}

	// If no rows updated, the singleton row doesn't exist (shouldn't happen with migration)
	if result.RowsAffected() == 0 {
		return fmt.Errorf("blockchain_sync_state singleton row not found (id=1)")
	}

	return nil
}

// GetLastProcessedBlock returns just the last processed block number.
func (r *BlockchainSyncStateRepository) GetLastProcessedBlock(ctx context.Context) (int64, error) {
	sql, args, err := r.pg.Builder.
		Select("last_processed_block").
		From("blockchain_sync_state").
		Where("id = ?", 1).
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("build query: %w", err)
	}

	var blockNumber int64
	err = r.pg.Pool.QueryRow(ctx, sql, args...).Scan(&blockNumber)
	if err != nil {
		return 0, fmt.Errorf("execute query: %w", err)
	}

	return blockNumber, nil
}
