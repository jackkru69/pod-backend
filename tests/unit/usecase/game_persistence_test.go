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

// TestHandleGameInitialized_Success tests successful game creation from blockchain event
func TestHandleGameInitialized_Success(t *testing.T) {
	mockGameRepo := new(MockGameRepository)
	mockEventRepo := new(MockGameEventRepository)

	uc := usecase.NewGamePersistenceUseCase(mockGameRepo, mockEventRepo, nil)

	event := &entity.GameEvent{
		EventType:       entity.EventTypeGameInitialized,
		GameID:          123,
		TransactionHash: "tx_init_123",
		BlockNumber:     1000,
		Timestamp:       time.Now(),
		EventData: map[string]interface{}{
			"game_id":           int64(123),
			"player_one":        "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2",
			"bet_amount":        int64(1000000000), // 1 TON
			"player_one_choice": int64(1),          // CLOSED
		},
	}

	// Expect event to be persisted
	mockEventRepo.On("Upsert", mock.Anything, event).Return(nil)

	// Expect game to be created with status WAITING_FOR_OPPONENT (1)
	mockGameRepo.On("Create", mock.Anything, mock.MatchedBy(func(g *entity.Game) bool {
		return g.GameID == 123 &&
			g.Status == entity.GameStatusWaitingForOpponent &&
			g.PlayerOneAddress == "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2" &&
			g.BetAmount == 1000000000 &&
			g.PlayerOneChoice == 1
	})).Return(nil)

	err := uc.HandleGameInitialized(context.Background(), event)

	assert.NoError(t, err)
	mockEventRepo.AssertExpectations(t)
	mockGameRepo.AssertExpectations(t)
}

// TestHandleGameInitialized_ValidationError tests validation of invalid event data
func TestHandleGameInitialized_ValidationError(t *testing.T) {
	mockGameRepo := new(MockGameRepository)
	mockEventRepo := new(MockGameEventRepository)

	uc := usecase.NewGamePersistenceUseCase(mockGameRepo, mockEventRepo, nil)

	event := &entity.GameEvent{
		EventType:       entity.EventTypeGameInitialized,
		GameID:          0, // Invalid game ID
		TransactionHash: "tx_invalid",
		BlockNumber:     1000,
		Timestamp:       time.Now(),
		EventData:       map[string]interface{}{},
	}

	err := uc.HandleGameInitialized(context.Background(), event)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "validation")
}

// TestHandleGameInitialized_RepositoryError tests database error handling
func TestHandleGameInitialized_RepositoryError(t *testing.T) {
	mockGameRepo := new(MockGameRepository)
	mockEventRepo := new(MockGameEventRepository)

	uc := usecase.NewGamePersistenceUseCase(mockGameRepo, mockEventRepo, nil)

	event := &entity.GameEvent{
		EventType:       entity.EventTypeGameInitialized,
		GameID:          123,
		TransactionHash: "tx_init_123",
		BlockNumber:     1000,
		Timestamp:       time.Now(),
		EventData: map[string]interface{}{
			"game_id":           int64(123),
			"player_one":        "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2",
			"bet_amount":        int64(1000000000),
			"player_one_choice": int64(1),
		},
	}

	mockEventRepo.On("Upsert", mock.Anything, event).Return(nil)
	mockGameRepo.On("Create", mock.Anything, mock.Anything).Return(errors.New("database error"))

	err := uc.HandleGameInitialized(context.Background(), event)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database error")
	mockEventRepo.AssertExpectations(t)
	mockGameRepo.AssertExpectations(t)
}

// TestHandleGameStarted_Success tests successful game update when player 2 joins
func TestHandleGameStarted_Success(t *testing.T) {
	mockGameRepo := new(MockGameRepository)
	mockEventRepo := new(MockGameEventRepository)

	uc := usecase.NewGamePersistenceUseCase(mockGameRepo, mockEventRepo, nil)

	event := &entity.GameEvent{
		EventType:       entity.EventTypeGameStarted,
		GameID:          123,
		TransactionHash: "tx_start_123",
		BlockNumber:     1001,
		Timestamp:       time.Now(),
		EventData: map[string]interface{}{
			"game_id":           int64(123),
			"player_two":        "EQAnotherPlayerWalletAddress123456789012345678",
			"player_two_choice": int64(1), // CLOSED
		},
	}

	mockEventRepo.On("Upsert", mock.Anything, event).Return(nil)
	mockGameRepo.On("JoinGame", mock.Anything, int64(123), "EQAnotherPlayerWalletAddress123456789012345678", "tx_start_123").Return(nil)

	err := uc.HandleGameStarted(context.Background(), event)

	assert.NoError(t, err)
	mockEventRepo.AssertExpectations(t)
	mockGameRepo.AssertExpectations(t)
}

// TestHandleGameFinished_Success tests successful game completion with winner
func TestHandleGameFinished_Success(t *testing.T) {
	mockGameRepo := new(MockGameRepository)
	mockEventRepo := new(MockGameEventRepository)
	mockUserRepo := new(MockUserRepository)

	uc := usecase.NewGamePersistenceUseCase(mockGameRepo, mockEventRepo, mockUserRepo)

	winnerAddress := "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2"
	loserAddress := "EQAnotherPlayerWalletAddress123456789012345678"
	loserPtr := loserAddress

	// Mock game retrieval to get player addresses
	existingGame := &entity.Game{
		GameID:            123,
		PlayerOneAddress:  winnerAddress,
		PlayerTwoAddress:  &loserPtr,
		BetAmount:         1000000000,
		Status:            entity.GameStatusWaitingForOpenBids,
	}
	mockGameRepo.On("GetByID", mock.Anything, int64(123)).Return(existingGame, nil)

	event := &entity.GameEvent{
		EventType:       entity.EventTypeGameFinished,
		GameID:          123,
		TransactionHash: "tx_finish_123",
		BlockNumber:     1002,
		Timestamp:       time.Now(),
		EventData: map[string]interface{}{
			"game_id":      int64(123),
			"winner":       winnerAddress,
			"payout":       int64(1900000000), // 1.9 TON (after fees)
		},
	}

	mockEventRepo.On("Upsert", mock.Anything, event).Return(nil)
	mockGameRepo.On("CompleteGame", mock.Anything, int64(123), winnerAddress, int64(1900000000), "tx_finish_123").Return(nil)

	// Expect user statistics updates
	mockUserRepo.On("IncrementGamesPlayed", mock.Anything, winnerAddress).Return(nil)
	mockUserRepo.On("IncrementGamesPlayed", mock.Anything, loserAddress).Return(nil)
	mockUserRepo.On("IncrementWins", mock.Anything, winnerAddress).Return(nil)
	mockUserRepo.On("IncrementLosses", mock.Anything, loserAddress).Return(nil)

	err := uc.HandleGameFinished(context.Background(), event)

	assert.NoError(t, err)
	mockEventRepo.AssertExpectations(t)
	mockGameRepo.AssertExpectations(t)
	mockUserRepo.AssertExpectations(t)
}

// TestHandleDraw_Success tests successful draw handling
func TestHandleDraw_Success(t *testing.T) {
	mockGameRepo := new(MockGameRepository)
	mockEventRepo := new(MockGameEventRepository)
	mockUserRepo := new(MockUserRepository)

	uc := usecase.NewGamePersistenceUseCase(mockGameRepo, mockEventRepo, mockUserRepo)

	player1 := "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2"
	player2 := "EQAnotherPlayerWalletAddress123456789012345678"
	player2Ptr := player2

	existingGame := &entity.Game{
		GameID:            123,
		PlayerOneAddress:  player1,
		PlayerTwoAddress:  &player2Ptr,
		BetAmount:         1000000000,
		Status:            entity.GameStatusWaitingForOpenBids,
	}
	mockGameRepo.On("GetByID", mock.Anything, int64(123)).Return(existingGame, nil)

	event := &entity.GameEvent{
		EventType:       entity.EventTypeDraw,
		GameID:          123,
		TransactionHash: "tx_draw_123",
		BlockNumber:     1003,
		Timestamp:       time.Now(),
		EventData: map[string]interface{}{
			"game_id": int64(123),
		},
	}

	mockEventRepo.On("Upsert", mock.Anything, event).Return(nil)
	mockGameRepo.On("CompleteGame", mock.Anything, int64(123), "", int64(0), "tx_draw_123").Return(nil)

	// Both players should have games played incremented (no winner/loser)
	mockUserRepo.On("IncrementGamesPlayed", mock.Anything, player1).Return(nil)
	mockUserRepo.On("IncrementGamesPlayed", mock.Anything, player2).Return(nil)

	err := uc.HandleDraw(context.Background(), event)

	assert.NoError(t, err)
	mockEventRepo.AssertExpectations(t)
	mockGameRepo.AssertExpectations(t)
	mockUserRepo.AssertExpectations(t)
}

// TestDuplicateEvent_Idempotent tests that duplicate events are handled idempotently at DB level
func TestDuplicateEvent_Idempotent(t *testing.T) {
	mockGameRepo := new(MockGameRepository)
	mockEventRepo := new(MockGameEventRepository)

	uc := usecase.NewGamePersistenceUseCase(mockGameRepo, mockEventRepo, nil)

	event := &entity.GameEvent{
		EventType:       entity.EventTypeGameInitialized,
		GameID:          123,
		TransactionHash: "tx_init_123",
		BlockNumber:     1000,
		Timestamp:       time.Now(),
		EventData: map[string]interface{}{
			"game_id":           int64(123),
			"player_one":        "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2",
			"bet_amount":        int64(1000000000),
			"player_one_choice": int64(1),
		},
	}

	// First call - should process normally
	mockEventRepo.On("Upsert", mock.Anything, event).Return(nil).Once()
	mockGameRepo.On("Create", mock.Anything, mock.Anything).Return(nil).Once()

	err := uc.HandleGameInitialized(context.Background(), event)
	assert.NoError(t, err)

	// Second call with same tx hash - Upsert is idempotent, but game Create will fail at DB level
	// The DB constraint (unique game_id) will prevent duplicate game creation
	mockEventRepo.On("Upsert", mock.Anything, event).Return(nil).Once()
	mockGameRepo.On("Create", mock.Anything, mock.Anything).Return(errors.New("duplicate key value violates unique constraint")).Once()

	err = uc.HandleGameInitialized(context.Background(), event)
	assert.Error(t, err) // Expected error from duplicate game creation attempt
	assert.Contains(t, err.Error(), "failed to create game")

	mockEventRepo.AssertExpectations(t)
	mockGameRepo.AssertExpectations(t)
}

// TestHandleGameFinished_WithReferrer tests referral statistics update (T091, FR-020, FR-021)
func TestHandleGameFinished_WithReferrer(t *testing.T) {
	mockGameRepo := new(MockGameRepository)
	mockEventRepo := new(MockGameEventRepository)
	mockUserRepo := new(MockUserRepository)

	uc := usecase.NewGamePersistenceUseCase(mockGameRepo, mockEventRepo, mockUserRepo)

	winnerAddress := "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2"
	loserAddress := "EQAnotherPlayerWalletAddress123456789012345678"
	referrerAddress := "EQReferrerWalletAddress123456789012345678901234"
	loserPtr := loserAddress
	referrerPtr := referrerAddress

	// Mock game with referrer for winner
	existingGame := &entity.Game{
		GameID:               123,
		PlayerOneAddress:     winnerAddress,
		PlayerOneReferrer:    &referrerPtr, // Winner has a referrer
		PlayerTwoAddress:     &loserPtr,
		BetAmount:            1000000000,   // 1 TON
		ReferrerFeeNumerator: 50,           // 0.5% = 50 basis points
		Status:               entity.GameStatusWaitingForOpenBids,
	}
	mockGameRepo.On("GetByID", mock.Anything, int64(123)).Return(existingGame, nil)

	event := &entity.GameEvent{
		EventType:       entity.EventTypeGameFinished,
		GameID:          123,
		TransactionHash: "tx_finish_123",
		BlockNumber:     1002,
		Timestamp:       time.Now(),
		EventData: map[string]interface{}{
			"game_id": int64(123),
			"winner":  winnerAddress,
			"payout":  int64(1900000000),
		},
	}

	mockEventRepo.On("Upsert", mock.Anything, event).Return(nil)
	mockGameRepo.On("CompleteGame", mock.Anything, int64(123), winnerAddress, int64(1900000000), "tx_finish_123").Return(nil)

	// Expect user statistics updates
	mockUserRepo.On("IncrementGamesPlayed", mock.Anything, winnerAddress).Return(nil)
	mockUserRepo.On("IncrementGamesPlayed", mock.Anything, loserAddress).Return(nil)
	mockUserRepo.On("IncrementWins", mock.Anything, winnerAddress).Return(nil)
	mockUserRepo.On("IncrementLosses", mock.Anything, loserAddress).Return(nil)

	// Expect referrer statistics update (T091)
	// Expected earnings: (1000000000 * 50) / 10000 = 5000000 nanotons = 0.005 TON
	expectedReferrerEarnings := int64(5000000)
	mockUserRepo.On("IncrementReferrals", mock.Anything, referrerAddress, expectedReferrerEarnings).Return(nil)

	err := uc.HandleGameFinished(context.Background(), event)

	assert.NoError(t, err)
	mockEventRepo.AssertExpectations(t)
	mockGameRepo.AssertExpectations(t)
	mockUserRepo.AssertExpectations(t)
}
