package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pod-backend/internal/entity"
	"pod-backend/internal/usecase"
)

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

	t.Run("should include paid games when filtering ended for compatibility", func(t *testing.T) {
		mockRepo := new(MockGameRepository)
		ctx := context.Background()

		endedGames := []*entity.Game{
			{GameID: 1, Status: entity.GameStatusEnded, CreatedAt: time.Unix(100, 0)},
		}
		paidGames := []*entity.Game{
			{GameID: 2, Status: entity.GameStatusPaid, CreatedAt: time.Unix(200, 0)},
		}

		mockRepo.On("GetByStatus", ctx, entity.GameStatusEnded).Return(endedGames, nil).Once()
		mockRepo.On("GetByStatus", ctx, entity.GameStatusPaid).Return(paidGames, nil).Once()

		uc := usecase.NewGameQueryUseCase(mockRepo)

		games, err := uc.ListGames(ctx, entity.GameStatusEnded, 20, 0)

		assert.NoError(t, err)
		assert.Len(t, games, 2)
		assert.Equal(t, int64(2), games[0].GameID)
		assert.Equal(t, int64(1), games[1].GameID)
		mockRepo.AssertExpectations(t)
	})
}

func TestGameQueryUseCase_GetGamesByPlayerPage(t *testing.T) {
	t.Run("should return paginated player history with the full total", func(t *testing.T) {
		mockRepo := new(MockGameRepository)
		ctx := context.Background()

		allGames := []*entity.Game{
			{GameID: 5, PlayerOneAddress: "EQAbcdefghijklmnopqrstuvwxyz0123456789ABCDEFGH"},
			{GameID: 4, PlayerOneAddress: "EQAbcdefghijklmnopqrstuvwxyz0123456789ABCDEFGH"},
			{GameID: 3, PlayerOneAddress: "EQAbcdefghijklmnopqrstuvwxyz0123456789ABCDEFGH"},
			{GameID: 2, PlayerOneAddress: "EQAbcdefghijklmnopqrstuvwxyz0123456789ABCDEFGH"},
			{GameID: 1, PlayerOneAddress: "EQAbcdefghijklmnopqrstuvwxyz0123456789ABCDEFGH"},
		}
		walletAddress := "EQAbcdefghijklmnopqrstuvwxyz0123456789ABCDEFGH"

		mockRepo.On("GetByPlayerAddress", ctx, walletAddress).Return(allGames, nil).Once()

		uc := usecase.NewGameQueryUseCase(mockRepo)

		games, total, err := uc.GetGamesByPlayerPage(ctx, walletAddress, 2, 1)

		assert.NoError(t, err)
		assert.Len(t, games, 2)
		assert.Equal(t, 5, total)
		assert.Equal(t, int64(4), games[0].GameID)
		assert.Equal(t, int64(3), games[1].GameID)
		mockRepo.AssertExpectations(t)
	})

	t.Run("should preserve full total when page is beyond the available history", func(t *testing.T) {
		mockRepo := new(MockGameRepository)
		ctx := context.Background()

		allGames := []*entity.Game{
			{GameID: 2, PlayerOneAddress: "EQAbcdefghijklmnopqrstuvwxyz0123456789ABCDEFGH"},
			{GameID: 1, PlayerOneAddress: "EQAbcdefghijklmnopqrstuvwxyz0123456789ABCDEFGH"},
		}
		walletAddress := "EQAbcdefghijklmnopqrstuvwxyz0123456789ABCDEFGH"

		mockRepo.On("GetByPlayerAddress", ctx, walletAddress).Return(allGames, nil).Once()

		uc := usecase.NewGameQueryUseCase(mockRepo)

		games, total, err := uc.GetGamesByPlayerPage(ctx, walletAddress, 20, 5)

		assert.NoError(t, err)
		assert.Empty(t, games)
		assert.Equal(t, 2, total)
		mockRepo.AssertExpectations(t)
	})

	t.Run("should order history by the most recent lifecycle timestamp", func(t *testing.T) {
		mockRepo := new(MockGameRepository)
		ctx := context.Background()
		walletAddress := "EQAbcdefghijklmnopqrstuvwxyz0123456789ABCDEFGH"

		createdFirst := time.Date(2026, 4, 4, 10, 0, 0, 0, time.UTC)
		joinedRecently := createdFirst.Add(4 * time.Hour)
		completedLatest := createdFirst.Add(6 * time.Hour)

		gamesForWallet := []*entity.Game{
			{
				GameID:           1,
				PlayerOneAddress: walletAddress,
				CreatedAt:        createdFirst,
			},
			{
				GameID:           2,
				PlayerOneAddress: walletAddress,
				CreatedAt:        createdFirst.Add(30 * time.Minute),
				JoinedAt:         &joinedRecently,
			},
			{
				GameID:           3,
				PlayerOneAddress: walletAddress,
				CreatedAt:        createdFirst.Add(time.Hour),
				CompletedAt:      &completedLatest,
			},
		}

		mockRepo.On("GetByPlayerAddress", ctx, walletAddress).Return(gamesForWallet, nil).Once()

		uc := usecase.NewGameQueryUseCase(mockRepo)

		games, total, err := uc.GetGamesByPlayerPage(ctx, walletAddress, 10, 0)

		assert.NoError(t, err)
		assert.Equal(t, 3, total)
		require.Len(t, games, 3)
		assert.Equal(t, int64(3), games[0].GameID)
		assert.Equal(t, int64(2), games[1].GameID)
		assert.Equal(t, int64(1), games[2].GameID)
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
