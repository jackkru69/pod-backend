package usecase

import (
	"context"
	"errors"

	"pod-backend/internal/entity"
	"pod-backend/internal/repository"
)

// GameQueryUseCase handles game query operations
type GameQueryUseCase struct {
	gameRepo repository.GameRepository
}

// NewGameQueryUseCase creates a new GameQueryUseCase
func NewGameQueryUseCase(gameRepo repository.GameRepository) *GameQueryUseCase {
	return &GameQueryUseCase{
		gameRepo: gameRepo,
	}
}

// ListGames retrieves games filtered by status with pagination
func (uc *GameQueryUseCase) ListGames(ctx context.Context, status int, limit int, offset int) ([]*entity.Game, error) {
	// Validate status
	if status < entity.GameStatusUninitialized || status > entity.GameStatusPaid {
		return nil, errors.New("invalid status: must be 0-4")
	}

	// Validate limit
	if limit <= 0 {
		return nil, errors.New("limit must be positive")
	}

	// Validate offset
	if offset < 0 {
		return nil, errors.New("offset cannot be negative")
	}

	// Get games from repository
	games, err := uc.gameRepo.GetByStatus(ctx, status)
	if err != nil {
		return nil, err
	}

	// Apply pagination
	if offset >= len(games) {
		return []*entity.Game{}, nil
	}

	end := offset + limit
	if end > len(games) {
		end = len(games)
	}

	return games[offset:end], nil
}

// GetGameByID retrieves a game by its ID
func (uc *GameQueryUseCase) GetGameByID(ctx context.Context, gameID int64) (*entity.Game, error) {
	// Validate gameID
	if gameID <= 0 {
		return nil, errors.New("gameID must be positive")
	}

	// Get game from repository
	game, err := uc.gameRepo.GetByID(ctx, gameID)
	if err != nil {
		return nil, err
	}

	return game, nil
}
