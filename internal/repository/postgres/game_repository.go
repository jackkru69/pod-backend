package postgres

import (
	"context"
	"fmt"
	"time"

	"pod-backend/internal/entity"
	"pod-backend/internal/repository"
	"pod-backend/pkg/postgres"
)

// Ensure GameRepository implements repository.GameRepository interface
var _ repository.GameRepository = (*GameRepository)(nil)

// GameRepository implements repository.GameRepository using PostgreSQL.
type GameRepository struct {
	pg *postgres.Postgres
}

// NewGameRepository creates a new PostgreSQL-backed game repository.
func NewGameRepository(pg *postgres.Postgres) *GameRepository {
	return &GameRepository{pg: pg}
}

// Create creates a new game record.
func (r *GameRepository) Create(ctx context.Context, game *entity.Game) error {
	if err := game.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	sql, args, err := r.pg.Builder.
		Insert("games").
		Columns(
			"game_id",
			"status",
			"player_one_address",
			"player_two_address",
			"player_one_choice",
			"player_two_choice",
			"player_one_referrer",
			"player_two_referrer",
			"bet_amount",
			"winner_address",
			"payout_amount",
			"service_fee_numerator",
			"referrer_fee_numerator",
			"waiting_timeout_seconds",
			"lowest_bid_allowed",
			"highest_bid_allowed",
			"fee_receiver_address",
			"joined_at",
			"revealed_at",
			"completed_at",
			"init_tx_hash",
			"join_tx_hash",
			"reveal_tx_hash",
			"complete_tx_hash",
		).
		Values(
			game.GameID,
			game.Status,
			game.PlayerOneAddress,
			game.PlayerTwoAddress,
			game.PlayerOneChoice,
			game.PlayerTwoChoice,
			game.PlayerOneReferrer,
			game.PlayerTwoReferrer,
			game.BetAmount,
			game.WinnerAddress,
			game.PayoutAmount,
			game.ServiceFeeNumerator,
			game.ReferrerFeeNumerator,
			game.WaitingTimeoutSeconds,
			game.LowestBidAllowed,
			game.HighestBidAllowed,
			game.FeeReceiverAddress,
			game.JoinedAt,
			game.RevealedAt,
			game.CompletedAt,
			game.InitTxHash,
			game.JoinTxHash,
			game.RevealTxHash,
			game.CompleteTxHash,
		).
		Suffix("RETURNING created_at").
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	err = r.pg.Pool.QueryRow(ctx, sql, args...).Scan(&game.CreatedAt)
	if err != nil {
		return fmt.Errorf("execute query: %w", err)
	}

	return nil
}

// CreateOrIgnore creates a new game record or ignores if already exists.
// Returns true if a new game was created, false if it already existed.
// Used for idempotent processing of blockchain events.
func (r *GameRepository) CreateOrIgnore(ctx context.Context, game *entity.Game) (bool, error) {
	if err := game.Validate(); err != nil {
		return false, fmt.Errorf("validation failed: %w", err)
	}

	sql, args, err := r.pg.Builder.
		Insert("games").
		Columns(
			"game_id",
			"status",
			"player_one_address",
			"player_two_address",
			"player_one_choice",
			"player_two_choice",
			"player_one_referrer",
			"player_two_referrer",
			"bet_amount",
			"winner_address",
			"payout_amount",
			"service_fee_numerator",
			"referrer_fee_numerator",
			"waiting_timeout_seconds",
			"lowest_bid_allowed",
			"highest_bid_allowed",
			"fee_receiver_address",
			"joined_at",
			"revealed_at",
			"completed_at",
			"init_tx_hash",
			"join_tx_hash",
			"reveal_tx_hash",
			"complete_tx_hash",
		).
		Values(
			game.GameID,
			game.Status,
			game.PlayerOneAddress,
			game.PlayerTwoAddress,
			game.PlayerOneChoice,
			game.PlayerTwoChoice,
			game.PlayerOneReferrer,
			game.PlayerTwoReferrer,
			game.BetAmount,
			game.WinnerAddress,
			game.PayoutAmount,
			game.ServiceFeeNumerator,
			game.ReferrerFeeNumerator,
			game.WaitingTimeoutSeconds,
			game.LowestBidAllowed,
			game.HighestBidAllowed,
			game.FeeReceiverAddress,
			game.JoinedAt,
			game.RevealedAt,
			game.CompletedAt,
			game.InitTxHash,
			game.JoinTxHash,
			game.RevealTxHash,
			game.CompleteTxHash,
		).
		Suffix("ON CONFLICT (game_id) DO NOTHING").
		ToSql()
	if err != nil {
		return false, fmt.Errorf("build query: %w", err)
	}

	result, err := r.pg.Pool.Exec(ctx, sql, args...)
	if err != nil {
		return false, fmt.Errorf("execute query: %w", err)
	}

	// RowsAffected() returns 1 if inserted, 0 if conflict (already exists)
	return result.RowsAffected() > 0, nil
}

// GetByID retrieves a game by its ID.
func (r *GameRepository) GetByID(ctx context.Context, gameID int64) (*entity.Game, error) {
	sql, args, err := r.pg.Builder.
		Select(
			"game_id", "status", "player_one_address", "player_two_address",
			"player_one_choice", "player_two_choice", "player_one_referrer", "player_two_referrer",
			"bet_amount", "winner_address", "payout_amount",
			"service_fee_numerator", "referrer_fee_numerator", "waiting_timeout_seconds",
			"lowest_bid_allowed", "highest_bid_allowed", "fee_receiver_address",
			"created_at", "joined_at", "revealed_at", "completed_at",
			"init_tx_hash", "join_tx_hash", "reveal_tx_hash", "complete_tx_hash",
		).
		From("games").
		Where("game_id = ?", gameID).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	game := &entity.Game{}
	err = r.pg.Pool.QueryRow(ctx, sql, args...).Scan(
		&game.GameID, &game.Status, &game.PlayerOneAddress, &game.PlayerTwoAddress,
		&game.PlayerOneChoice, &game.PlayerTwoChoice, &game.PlayerOneReferrer, &game.PlayerTwoReferrer,
		&game.BetAmount, &game.WinnerAddress, &game.PayoutAmount,
		&game.ServiceFeeNumerator, &game.ReferrerFeeNumerator, &game.WaitingTimeoutSeconds,
		&game.LowestBidAllowed, &game.HighestBidAllowed, &game.FeeReceiverAddress,
		&game.CreatedAt, &game.JoinedAt, &game.RevealedAt, &game.CompletedAt,
		&game.InitTxHash, &game.JoinTxHash, &game.RevealTxHash, &game.CompleteTxHash,
	)
	if err != nil {
		return nil, fmt.Errorf("execute query: %w", err)
	}

	return game, nil
}

// GetByStatus retrieves all games with a specific status.
func (r *GameRepository) GetByStatus(ctx context.Context, status int) ([]*entity.Game, error) {
	sql, args, err := r.pg.Builder.
		Select(
			"game_id", "status", "player_one_address", "player_two_address",
			"player_one_choice", "player_two_choice", "player_one_referrer", "player_two_referrer",
			"bet_amount", "winner_address", "payout_amount",
			"service_fee_numerator", "referrer_fee_numerator", "waiting_timeout_seconds",
			"lowest_bid_allowed", "highest_bid_allowed", "fee_receiver_address",
			"created_at", "joined_at", "revealed_at", "completed_at",
			"init_tx_hash", "join_tx_hash", "reveal_tx_hash", "complete_tx_hash",
		).
		From("games").
		Where("status = ?", status).
		OrderBy("created_at DESC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	return r.queryGames(ctx, sql, args...)
}

// GetAvailableGames retrieves all games waiting for an opponent.
func (r *GameRepository) GetAvailableGames(ctx context.Context) ([]*entity.Game, error) {
	return r.GetByStatus(ctx, entity.GameStatusWaitingForOpponent)
}

// GetByPlayerAddress retrieves all games where the player participated.
func (r *GameRepository) GetByPlayerAddress(ctx context.Context, walletAddress string) ([]*entity.Game, error) {
	sql, args, err := r.pg.Builder.
		Select(
			"game_id", "status", "player_one_address", "player_two_address",
			"player_one_choice", "player_two_choice", "player_one_referrer", "player_two_referrer",
			"bet_amount", "winner_address", "payout_amount",
			"service_fee_numerator", "referrer_fee_numerator", "waiting_timeout_seconds",
			"lowest_bid_allowed", "highest_bid_allowed", "fee_receiver_address",
			"created_at", "joined_at", "revealed_at", "completed_at",
			"init_tx_hash", "join_tx_hash", "reveal_tx_hash", "complete_tx_hash",
		).
		From("games").
		Where("player_one_address = ? OR player_two_address = ?", walletAddress, walletAddress).
		OrderBy("created_at DESC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	return r.queryGames(ctx, sql, args...)
}

// GetByPlayer is an alias for GetByPlayerAddress for cleaner use case code.
func (r *GameRepository) GetByPlayer(ctx context.Context, walletAddress string) ([]*entity.Game, error) {
	return r.GetByPlayerAddress(ctx, walletAddress)
}

// queryGames is a helper to execute queries returning multiple games.
func (r *GameRepository) queryGames(ctx context.Context, sql string, args ...interface{}) ([]*entity.Game, error) {
	rows, err := r.pg.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("execute query: %w", err)
	}
	defer rows.Close()

	var games []*entity.Game
	for rows.Next() {
		game := &entity.Game{}
		err = rows.Scan(
			&game.GameID, &game.Status, &game.PlayerOneAddress, &game.PlayerTwoAddress,
			&game.PlayerOneChoice, &game.PlayerTwoChoice, &game.PlayerOneReferrer, &game.PlayerTwoReferrer,
			&game.BetAmount, &game.WinnerAddress, &game.PayoutAmount,
			&game.ServiceFeeNumerator, &game.ReferrerFeeNumerator, &game.WaitingTimeoutSeconds,
			&game.LowestBidAllowed, &game.HighestBidAllowed, &game.FeeReceiverAddress,
			&game.CreatedAt, &game.JoinedAt, &game.RevealedAt, &game.CompletedAt,
			&game.InitTxHash, &game.JoinTxHash, &game.RevealTxHash, &game.CompleteTxHash,
		)
		if err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		games = append(games, game)
	}

	return games, nil
}

// Update updates an existing game record.
func (r *GameRepository) Update(ctx context.Context, game *entity.Game) error {
	if err := game.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	sql, args, err := r.pg.Builder.
		Update("games").
		Set("status", game.Status).
		Set("player_two_address", game.PlayerTwoAddress).
		Set("player_one_choice", game.PlayerOneChoice).
		Set("player_two_choice", game.PlayerTwoChoice).
		Set("player_two_referrer", game.PlayerTwoReferrer).
		Set("winner_address", game.WinnerAddress).
		Set("payout_amount", game.PayoutAmount).
		Set("joined_at", game.JoinedAt).
		Set("revealed_at", game.RevealedAt).
		Set("completed_at", game.CompletedAt).
		Set("join_tx_hash", game.JoinTxHash).
		Set("reveal_tx_hash", game.RevealTxHash).
		Set("complete_tx_hash", game.CompleteTxHash).
		Where("game_id = ?", game.GameID).
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

// UpdateStatus updates only the game status and completed_at timestamp.
func (r *GameRepository) UpdateStatus(ctx context.Context, gameID int64, newStatus int) error {
	now := time.Now()
	sql, args, err := r.pg.Builder.
		Update("games").
		Set("status", newStatus).
		Set("completed_at", now).
		Where("game_id = ?", gameID).
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

// JoinGame updates a game when player 2 joins.
func (r *GameRepository) JoinGame(ctx context.Context, gameID int64, playerTwoAddress string, joinTxHash string, joinedAt time.Time) error {
	sql, args, err := r.pg.Builder.
		Update("games").
		Set("player_two_address", playerTwoAddress).
		Set("joined_at", joinedAt).
		Set("join_tx_hash", joinTxHash).
		Set("status", entity.GameStatusWaitingForOpenBids).
		Set("player_one_choice", entity.CoinSideClosed). // Set to CLOSED (1) when game starts
		Set("player_two_choice", entity.CoinSideClosed). // Both players have unrevealed choices
		Where("game_id = ?", gameID).
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

// RevealChoice updates a game when a player reveals their choice.
func (r *GameRepository) RevealChoice(ctx context.Context, gameID int64, playerAddress string, choice int, revealTxHash string, revealedAt time.Time) error {
	// First, get the game to determine which player is revealing
	game, err := r.GetByID(ctx, gameID)
	if err != nil {
		return fmt.Errorf("get game: %w", err)
	}

	updateBuilder := r.pg.Builder.Update("games").
		Set("revealed_at", revealedAt).
		Set("reveal_tx_hash", revealTxHash)

	if game.PlayerOneAddress == playerAddress {
		updateBuilder = updateBuilder.Set("player_one_choice", choice)
	} else if game.PlayerTwoAddress != nil && *game.PlayerTwoAddress == playerAddress {
		updateBuilder = updateBuilder.Set("player_two_choice", choice)
	} else {
		return fmt.Errorf("player %s not found in game %d", playerAddress, gameID)
	}

	sql, args, err := updateBuilder.Where("game_id = ?", gameID).ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = r.pg.Pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("execute query: %w", err)
	}

	return nil
}

// CompleteGame updates a game when it's finished.
func (r *GameRepository) CompleteGame(ctx context.Context, gameID int64, winnerAddress string, payoutAmount int64, completeTxHash string, completedAt time.Time) error {
	return r.CompleteGameWithQuerier(ctx, r.pg.Pool, gameID, winnerAddress, payoutAmount, completeTxHash, completedAt)
}

// CompleteGameWithQuerier performs CompleteGame using the provided Querier (for transaction support).
func (r *GameRepository) CompleteGameWithQuerier(ctx context.Context, q repository.Querier, gameID int64, winnerAddress string, payoutAmount int64, completeTxHash string, completedAt time.Time) error {
	var winnerValue interface{}
	if winnerAddress != "" {
		winnerValue = winnerAddress
	}

	var payoutValue interface{}
	if payoutAmount > 0 {
		payoutValue = payoutAmount
	}

	sql, args, err := r.pg.Builder.
		Update("games").
		Set("winner_address", winnerValue).
		Set("payout_amount", payoutValue).
		Set("completed_at", completedAt).
		Set("complete_tx_hash", completeTxHash).
		Set("status", entity.GameStatusPaid).
		Where("game_id = ?", gameID).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = q.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("execute query: %w", err)
	}

	return nil
}

// CancelGame marks a game as cancelled.
func (r *GameRepository) CancelGame(ctx context.Context, gameID int64, cancelTxHash string, completedAt time.Time) error {
	sql, args, err := r.pg.Builder.
		Update("games").
		Set("status", entity.GameStatusPaid).
		Set("completed_at", completedAt).
		Set("winner_address", nil).
		Set("payout_amount", nil).
		Set("complete_tx_hash", cancelTxHash).
		Where("game_id = ?", gameID).
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

// DeleteOlderThan deletes games whose completed_at is older than the specified date.
func (r *GameRepository) DeleteOlderThan(ctx context.Context, olderThanDate string) (int64, error) {
	sql, args, err := r.pg.Builder.
		Delete("games").
		Where("completed_at < ?", olderThanDate).
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("build query: %w", err)
	}

	result, err := r.pg.Pool.Exec(ctx, sql, args...)
	if err != nil {
		return 0, fmt.Errorf("execute query: %w", err)
	}

	return result.RowsAffected(), nil
}

// Exists checks if a game with the given ID exists.
func (r *GameRepository) Exists(ctx context.Context, gameID int64) (bool, error) {
	sql, args, err := r.pg.Builder.
		Select("COUNT(*)").
		From("games").
		Where("game_id = ?", gameID).
		ToSql()
	if err != nil {
		return false, fmt.Errorf("build query: %w", err)
	}

	var count int
	err = r.pg.Pool.QueryRow(ctx, sql, args...).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("execute query: %w", err)
	}

	return count > 0, nil
}
