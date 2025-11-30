package repository

import (
	"context"

	"pod-backend/internal/entity"
)

// BlockchainSyncStateRepository defines the interface for blockchain sync state persistence.
// This is a singleton repository (only one row exists in the database).
// All operations must be atomic to prevent race conditions during polling.
type BlockchainSyncStateRepository interface {
	// Get retrieves the current blockchain sync state.
	// Returns the singleton state record.
	Get(ctx context.Context) (*entity.BlockchainSyncState, error)

	// UpdateLastProcessedBlock atomically updates the last processed block number.
	// This operation must be atomic to prevent event reprocessing (FR-023).
	// Also updates last_poll_timestamp to NOW().
	UpdateLastProcessedBlock(ctx context.Context, blockNumber int64) error

	// Initialize sets up the blockchain sync state for a contract address.
	// Should be called once during application startup.
	// If already initialized, this operation should be idempotent.
	Initialize(ctx context.Context, contractAddress string, startingBlock int64) error

	// GetLastProcessedBlock returns just the last processed block number.
	// Convenience method for quick checks without loading full entity.
	GetLastProcessedBlock(ctx context.Context) (int64, error)
}
