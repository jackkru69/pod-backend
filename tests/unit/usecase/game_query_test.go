package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"pod-backend/internal/entity"
	"pod-backend/internal/usecase"
)

// MockGameRepository is a mock implementation of repository.GameRepository
type MockGameRepository struct {
	mock.Mock
}

func (m *MockGameRepository) Create(ctx context.Context, game *entity.Game) error {
	args := m.Called(ctx, game)
	return args.Error(0)
}

func (m *MockGameRepository) GetByID(ctx context.Context, gameID int64) (*entity.Game, error) {
	args := m.Called(ctx, gameID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.Game), args.Error(1)
}

func (m *MockGameRepository) GetByStatus(ctx context.Context, status int) ([]*entity.Game, error) {
	args := m.Called(ctx, status)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*entity.Game), args.Error(1)
}

func (m *MockGameRepository) GetAvailableGames(ctx context.Context) ([]*entity.Game, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*entity.Game), args.Error(1)
}

func (m *MockGameRepository) GetByPlayerAddress(ctx context.Context, walletAddress string) ([]*entity.Game, error) {
	args := m.Called(ctx, walletAddress)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*entity.Game), args.Error(1)
}

func (m *MockGameRepository) Update(ctx context.Context, game *entity.Game) error {
	args := m.Called(ctx, game)
	return args.Error(0)
}

func (m *MockGameRepository) UpdateStatus(ctx context.Context, gameID int64, newStatus int) error {
	args := m.Called(ctx, gameID, newStatus)
	return args.Error(0)
}

func (m *MockGameRepository) JoinGame(ctx context.Context, gameID int64, playerTwoAddress string, joinTxHash string) error {
	args := m.Called(ctx, gameID, playerTwoAddress, joinTxHash)
	return args.Error(0)
}

func (m *MockGameRepository) RevealChoice(ctx context.Context, gameID int64, playerAddress string, choice int, revealTxHash string) error {
	args := m.Called(ctx, gameID, playerAddress, choice, revealTxHash)
	return args.Error(0)
}

func (m *MockGameRepository) CompleteGame(ctx context.Context, gameID int64, winnerAddress string, payoutAmount int64, completeTxHash string) error {
	args := m.Called(ctx, gameID, winnerAddress, payoutAmount, completeTxHash)
	return args.Error(0)
}

func (m *MockGameRepository) CancelGame(ctx context.Context, gameID int64, cancelTxHash string) error {
	args := m.Called(ctx, gameID, cancelTxHash)
	return args.Error(0)
}

func (m *MockGameRepository) DeleteOlderThan(ctx context.Context, olderThanDate string) (int64, error) {
	args := m.Called(ctx, olderThanDate)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockGameRepository) Exists(ctx context.Context, gameID int64) (bool, error) {
	args := m.Called(ctx, gameID)
	return args.Bool(0), args.Error(1)
}

// TestGameQueryUseCase_ListGames tests the ListGames method
func TestGameQueryUseCase_ListGames(t *testing.T) {
	t.Run("should return games filtered by status with pagination", func(t *testing.T) {
		// Arrange
		mockRepo := new(MockGameRepository)
		ctx := context.Background()

		expectedGames := []*entity.Game{
			{
				GameID:           1,
				Status:           entity.GameStatusWaitingForOpponent,
				PlayerOneAddress: "EQAbcdefghijklmnopqrstuvwxyz0123456789ABCDEFGH",
				PlayerOneChoice:  entity.CoinSideHeads,
				BetAmount:        1000000000,
				CreatedAt:        time.Now(),
				InitTxHash:       "abc123def456",
			},
			{
				GameID:           2,
				Status:           entity.GameStatusWaitingForOpponent,
				PlayerOneAddress: "EQXyzabcdefghijklmnopqrstuvwxyz0123456789ABCDE",
				PlayerOneChoice:  entity.CoinSideTails,
				BetAmount:        2000000000,
				CreatedAt:        time.Now(),
				InitTxHash:       "def456ghi789",
			},
		}

		mockRepo.On("GetByStatus", ctx, entity.GameStatusWaitingForOpponent).
			Return(expectedGames, nil)

		uc := usecase.NewGameQueryUseCase(mockRepo)

		// Act
		games, err := uc.ListGames(ctx, entity.GameStatusWaitingForOpponent, 20, 0)

		// Assert
		assert.NoError(t, err)
		assert.Len(t, games, 2)
		assert.Equal(t, expectedGames, games)
		mockRepo.AssertExpectations(t)
	})

	t.Run("should return empty slice when no games found", func(t *testing.T) {
		// Arrange
		mockRepo := new(MockGameRepository)
		ctx := context.Background()

		mockRepo.On("GetByStatus", ctx, entity.GameStatusWaitingForOpponent).
			Return([]*entity.Game{}, nil)

		uc := usecase.NewGameQueryUseCase(mockRepo)

		// Act
		games, err := uc.ListGames(ctx, entity.GameStatusWaitingForOpponent, 20, 0)

		// Assert
		assert.NoError(t, err)
		assert.Empty(t, games)
		mockRepo.AssertExpectations(t)
	})

	t.Run("should handle repository error", func(t *testing.T) {
		// Arrange
		mockRepo := new(MockGameRepository)
		ctx := context.Background()

		mockRepo.On("GetByStatus", ctx, entity.GameStatusWaitingForOpponent).
			Return(nil, errors.New("database connection failed"))

		uc := usecase.NewGameQueryUseCase(mockRepo)

		// Act
		games, err := uc.ListGames(ctx, entity.GameStatusWaitingForOpponent, 20, 0)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, games)
		assert.Contains(t, err.Error(), "database connection failed")
		mockRepo.AssertExpectations(t)
	})

	t.Run("should validate status parameter", func(t *testing.T) {
		// Arrange
		mockRepo := new(MockGameRepository)
		ctx := context.Background()
		uc := usecase.NewGameQueryUseCase(mockRepo)

		// Act
		games, err := uc.ListGames(ctx, 99, 20, 0) // Invalid status

		// Assert
		assert.Error(t, err)
		assert.Nil(t, games)
		assert.Contains(t, err.Error(), "invalid status")
	})

	t.Run("should validate limit parameter", func(t *testing.T) {
		// Arrange
		mockRepo := new(MockGameRepository)
		ctx := context.Background()
		uc := usecase.NewGameQueryUseCase(mockRepo)

		// Act
		games, err := uc.ListGames(ctx, entity.GameStatusWaitingForOpponent, 0, 0) // Invalid limit

		// Assert
		assert.Error(t, err)
		assert.Nil(t, games)
		assert.Contains(t, err.Error(), "limit must be positive")
	})

	t.Run("should validate offset parameter", func(t *testing.T) {
		// Arrange
		mockRepo := new(MockGameRepository)
		ctx := context.Background()
		uc := usecase.NewGameQueryUseCase(mockRepo)

		// Act
		games, err := uc.ListGames(ctx, entity.GameStatusWaitingForOpponent, 20, -1) // Invalid offset

		// Assert
		assert.Error(t, err)
		assert.Nil(t, games)
		assert.Contains(t, err.Error(), "offset cannot be negative")
	})

	t.Run("should apply pagination correctly", func(t *testing.T) {
		// Arrange
		mockRepo := new(MockGameRepository)
		ctx := context.Background()

		allGames := []*entity.Game{
			{GameID: 1, Status: entity.GameStatusWaitingForOpponent, PlayerOneAddress: "EQAbcdefghijklmnopqrstuvwxyz0123456789ABCDEFGH", BetAmount: 1000000000, InitTxHash: "abc1"},
			{GameID: 2, Status: entity.GameStatusWaitingForOpponent, PlayerOneAddress: "EQAbcdefghijklmnopqrstuvwxyz0123456789ABCDEFGH", BetAmount: 1000000000, InitTxHash: "abc2"},
			{GameID: 3, Status: entity.GameStatusWaitingForOpponent, PlayerOneAddress: "EQAbcdefghijklmnopqrstuvwxyz0123456789ABCDEFGH", BetAmount: 1000000000, InitTxHash: "abc3"},
		}

		mockRepo.On("GetByStatus", ctx, entity.GameStatusWaitingForOpponent).
			Return(allGames, nil)

		uc := usecase.NewGameQueryUseCase(mockRepo)

		// Act
		games, err := uc.ListGames(ctx, entity.GameStatusWaitingForOpponent, 2, 1)

		// Assert
		assert.NoError(t, err)
		assert.Len(t, games, 2) // Should return 2 games (limit=2)
		// After offset of 1, should get games[1] and games[2]
		assert.Equal(t, int64(2), games[0].GameID)
		assert.Equal(t, int64(3), games[1].GameID)
		mockRepo.AssertExpectations(t)
	})
}

// TestGameQueryUseCase_GetGameByID tests the GetGameByID method
func TestGameQueryUseCase_GetGameByID(t *testing.T) {
	t.Run("should return game when found", func(t *testing.T) {
		// Arrange
		mockRepo := new(MockGameRepository)
		ctx := context.Background()

		expectedGame := &entity.Game{
			GameID:           123,
			Status:           entity.GameStatusWaitingForOpponent,
			PlayerOneAddress: "EQAbcdefghijklmnopqrstuvwxyz0123456789ABCDEFGH",
			PlayerOneChoice:  entity.CoinSideHeads,
			BetAmount:        1000000000,
			CreatedAt:        time.Now(),
			InitTxHash:       "abc123def456",
		}

		mockRepo.On("GetByID", ctx, int64(123)).
			Return(expectedGame, nil)

		uc := usecase.NewGameQueryUseCase(mockRepo)

		// Act
		game, err := uc.GetGameByID(ctx, 123)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, game)
		assert.Equal(t, int64(123), game.GameID)
		assert.Equal(t, expectedGame, game)
		mockRepo.AssertExpectations(t)
	})

	t.Run("should return error when game not found", func(t *testing.T) {
		// Arrange
		mockRepo := new(MockGameRepository)
		ctx := context.Background()

		mockRepo.On("GetByID", ctx, int64(999)).
			Return(nil, errors.New("game not found"))

		uc := usecase.NewGameQueryUseCase(mockRepo)

		// Act
		game, err := uc.GetGameByID(ctx, 999)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, game)
		assert.Contains(t, err.Error(), "game not found")
		mockRepo.AssertExpectations(t)
	})

	t.Run("should handle repository error", func(t *testing.T) {
		// Arrange
		mockRepo := new(MockGameRepository)
		ctx := context.Background()

		mockRepo.On("GetByID", ctx, int64(123)).
			Return(nil, errors.New("database connection failed"))

		uc := usecase.NewGameQueryUseCase(mockRepo)

		// Act
		game, err := uc.GetGameByID(ctx, 123)

		// Assert
		assert.Error(t, err)
		assert.Nil(t, game)
		assert.Contains(t, err.Error(), "database connection failed")
		mockRepo.AssertExpectations(t)
	})

	t.Run("should validate gameID parameter", func(t *testing.T) {
		// Arrange
		mockRepo := new(MockGameRepository)
		ctx := context.Background()
		uc := usecase.NewGameQueryUseCase(mockRepo)

		// Act
		game, err := uc.GetGameByID(ctx, 0) // Invalid gameID

		// Assert
		assert.Error(t, err)
		assert.Nil(t, game)
		assert.Contains(t, err.Error(), "gameID must be positive")
	})

	t.Run("should validate negative gameID parameter", func(t *testing.T) {
		// Arrange
		mockRepo := new(MockGameRepository)
		ctx := context.Background()
		uc := usecase.NewGameQueryUseCase(mockRepo)

		// Act
		game, err := uc.GetGameByID(ctx, -5) // Invalid gameID

		// Assert
		assert.Error(t, err)
		assert.Nil(t, game)
		assert.Contains(t, err.Error(), "gameID must be positive")
	})
}
