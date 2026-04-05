package repository

import (
	"context"

	"pod-backend/internal/entity"
)

// EventSourceType represents the type of blockchain event source.
type EventSourceType string

const (
	EventSourceWebSocket EventSourceType = "websocket"
	EventSourceHTTP      EventSourceType = "http"
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

	// GetEventSourceType returns the current event source type (websocket or http) (T146).
	GetEventSourceType(ctx context.Context) (EventSourceType, error)

	// SetEventSourceType updates the event source type and related status (T146).
	// Also updates websocket_connected status for WebSocket sources.
	SetEventSourceType(ctx context.Context, sourceType EventSourceType, connected bool) error

	// UpdateLastProcessedLt atomically updates the last processed logical time (lt) (T146).
	// This operation must be atomic for TON blockchain event ordering.
	UpdateLastProcessedLt(ctx context.Context, lt string) error

	// PersistCheckpoint stores the resumable TON checkpoint and current source status.
	// This keeps restart recovery and health reporting aligned to the same state snapshot.
	PersistCheckpoint(ctx context.Context, lt string, sourceType EventSourceType, connected bool) error

	// GetLastProcessedLt returns the last processed logical time (lt) (T146).
	GetLastProcessedLt(ctx context.Context) (string, error)

	// RecordFallback increments the fallback counter and sets fallback timestamp (T146).
	// Called when WebSocket falls back to HTTP polling.
	RecordFallback(ctx context.Context) error
}
