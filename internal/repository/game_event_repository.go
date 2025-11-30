package repository

import (
	"context"

	"pod-backend/internal/entity"
)

// GameEventRepository defines the interface for game event data persistence.
// Implementations must handle database operations for blockchain event audit trail.
type GameEventRepository interface {
	// Create creates a new game event record.
	// Returns error if duplicate event detected (same game_id, transaction_hash, event_type).
	Create(ctx context.Context, event *entity.GameEvent) error

	// GetByGameID retrieves all events for a specific game.
	// Events are returned in chronological order (by timestamp).
	// Returns empty slice if no events found.
	GetByGameID(ctx context.Context, gameID int64) ([]*entity.GameEvent, error)

	// GetByTransactionHash retrieves all events associated with a transaction.
	// Used for duplicate detection and debugging.
	// Returns empty slice if no events found.
	GetByTransactionHash(ctx context.Context, txHash string) ([]*entity.GameEvent, error)

	// GetByEventType retrieves all events of a specific type.
	// Event type must be one of: GameInitializedNotify, GameStartedNotify,
	// GameFinishedNotify, GameCancelledNotify, DrawNotify, SecretOpenedNotify,
	// InsufficientBalanceNotify.
	// Returns empty slice if no events found.
	GetByEventType(ctx context.Context, eventType string) ([]*entity.GameEvent, error)

	// GetByBlockRange retrieves all events within a block number range (inclusive).
	// Used for blockchain re-sync operations.
	// Returns empty slice if no events found.
	GetByBlockRange(ctx context.Context, startBlock, endBlock int64) ([]*entity.GameEvent, error)

	// Exists checks if an event already exists for a given game, transaction, and type.
	// Used for duplicate detection (SC-009).
	Exists(ctx context.Context, gameID int64, txHash string, eventType string) (bool, error)

	// GetLatestByGameID retrieves the most recent event for a specific game.
	// Returns nil if no events found.
	GetLatestByGameID(ctx context.Context, gameID int64) (*entity.GameEvent, error)
}
