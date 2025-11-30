package repository

import (
	"context"

	"pod-backend/internal/entity"
)

// GameRepository defines the interface for game data persistence.
// Implementations must handle database operations for game instances.
type GameRepository interface {
	// Create creates a new game record.
	// Returns error if game_id already exists.
	Create(ctx context.Context, game *entity.Game) error

	// GetByID retrieves a game by its ID.
	// Returns nil if not found.
	GetByID(ctx context.Context, gameID int64) (*entity.Game, error)

	// GetByStatus retrieves all games with a specific status.
	// Status must be one of: 0 (UNINITIALIZED), 1 (WAITING_FOR_OPPONENT),
	// 2 (WAITING_FOR_OPEN_BIDS), 3 (ENDED), 4 (PAID).
	// Returns empty slice if no games found.
	GetByStatus(ctx context.Context, status int) ([]*entity.Game, error)

	// GetAvailableGames retrieves all games waiting for an opponent (status = 1).
	// Equivalent to GetByStatus(ctx, entity.GameStatusWaitingForOpponent).
	// Returns empty slice if no games found.
	GetAvailableGames(ctx context.Context) ([]*entity.Game, error)

	// GetByPlayerAddress retrieves all games where the player participated.
	// Searches both player_one_address and player_two_address.
	// Returns empty slice if no games found.
	GetByPlayerAddress(ctx context.Context, walletAddress string) ([]*entity.Game, error)

	// Update updates an existing game record.
	// Returns error if game doesn't exist.
	Update(ctx context.Context, game *entity.Game) error

	// UpdateStatus updates only the game status and completed_at timestamp.
	// Used for atomic status transitions.
	// Returns error if game doesn't exist or transition is invalid.
	UpdateStatus(ctx context.Context, gameID int64, newStatus int) error

	// JoinGame updates a game when player 2 joins.
	// Sets player_two_address, joined_at, join_tx_hash, and updates status.
	// Returns error if game doesn't exist or already has player 2.
	JoinGame(ctx context.Context, gameID int64, playerTwoAddress string, joinTxHash string) error

	// RevealChoice updates a game when a player reveals their choice.
	// Sets revealed_at, reveal_tx_hash, and the player's choice.
	// Returns error if game doesn't exist.
	RevealChoice(ctx context.Context, gameID int64, playerAddress string, choice int, revealTxHash string) error

	// CompleteGame updates a game when it's finished.
	// Sets winner_address, payout_amount, completed_at, complete_tx_hash, and status.
	// Returns error if game doesn't exist.
	CompleteGame(ctx context.Context, gameID int64, winnerAddress string, payoutAmount int64, completeTxHash string) error

	// CancelGame marks a game as cancelled.
	// Updates status to ENDED with no winner.
	// Returns error if game doesn't exist.
	CancelGame(ctx context.Context, gameID int64, cancelTxHash string) error

	// DeleteOlderThan deletes games whose completed_at is older than the specified date.
	// Used for data retention compliance (FR-017: 1 year retention).
	// Returns number of games deleted.
	DeleteOlderThan(ctx context.Context, olderThanDate string) (int64, error)

	// Exists checks if a game with the given ID exists.
	// Used for duplicate detection.
	Exists(ctx context.Context, gameID int64) (bool, error)
}
