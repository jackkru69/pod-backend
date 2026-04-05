package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"pod-backend/internal/entity"
	"pod-backend/internal/usecase"
)

// TestHandleGameInitialized_Success tests successful game creation from blockchain event
func TestHandleGameInitialized_Success(t *testing.T) {
	mockGameRepo := new(MockGameRepository)
	mockEventRepo := new(MockGameEventRepository)
	mockUserRepo := new(MockUserRepository)

	uc := usecase.NewGamePersistenceUseCase(mockGameRepo, mockEventRepo, mockUserRepo)

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

	// Expect user to be ensured for FK constraint
	mockUserRepo.On("EnsureUserByWallet", mock.Anything, "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2").Return(nil)

	// Expect game to be created with status WAITING_FOR_OPPONENT (1) - called FIRST
	mockGameRepo.On("CreateOrIgnore", mock.Anything, mock.MatchedBy(func(g *entity.Game) bool {
		return g.GameID == 123 &&
			g.Status == entity.GameStatusWaitingForOpponent &&
			g.PlayerOneAddress == "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2" &&
			g.BetAmount == 1000000000 &&
			g.PlayerOneChoice == 1
	})).Return(true, nil)

	// Expect event to be persisted - called SECOND
	mockEventRepo.On("Upsert", mock.Anything, event).Run(func(args mock.Arguments) {
		e := args.Get(1).(*entity.GameEvent)
		e.ID = 1 // Simulate successful insert
	}).Return(nil)

	err := uc.HandleGameInitialized(context.Background(), event)

	assert.NoError(t, err)
	mockEventRepo.AssertExpectations(t)
	mockGameRepo.AssertExpectations(t)
	mockUserRepo.AssertExpectations(t)
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
	mockUserRepo := new(MockUserRepository)

	uc := usecase.NewGamePersistenceUseCase(mockGameRepo, mockEventRepo, mockUserRepo)

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

	// Expect user to be ensured for FK constraint
	mockUserRepo.On("EnsureUserByWallet", mock.Anything, "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2").Return(nil)

	// Note: HandleGameInitialized creates game FIRST, then upserts event
	// So if CreateOrIgnore fails, Upsert should not be called
	mockGameRepo.On("CreateOrIgnore", mock.Anything, mock.Anything).Return(false, errors.New("database error"))

	err := uc.HandleGameInitialized(context.Background(), event)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database error")
	mockGameRepo.AssertExpectations(t)
	mockUserRepo.AssertExpectations(t)
	// eventRepo.Upsert should NOT be called since Create failed first
}

// TestHandleGameStarted_Success tests successful game update when player 2 joins
func TestHandleGameStarted_Success(t *testing.T) {
	mockGameRepo := new(MockGameRepository)
	mockEventRepo := new(MockGameEventRepository)
	mockUserRepo := new(MockUserRepository)

	uc := usecase.NewGamePersistenceUseCase(mockGameRepo, mockEventRepo, mockUserRepo)

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

	// Mock game retrieval (safety check in HandleGameStarted)
	existingGame := &entity.Game{
		GameID:           123,
		PlayerOneAddress: "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2",
		Status:           entity.GameStatusWaitingForOpponent,
	}
	mockGameRepo.On("GetByID", mock.Anything, int64(123)).Return(existingGame, nil)

	// Expect user to be ensured for FK constraint
	mockUserRepo.On("EnsureUserByWallet", mock.Anything, "EQAnotherPlayerWalletAddress123456789012345678").Return(nil)

	mockEventRepo.On("Upsert", mock.Anything, event).Run(func(args mock.Arguments) {
		e := args.Get(1).(*entity.GameEvent)
		e.ID = 1 // Simulate successful insert
	}).Return(nil)
	mockGameRepo.On("JoinGame", mock.Anything, int64(123), "EQAnotherPlayerWalletAddress123456789012345678", "tx_start_123", event.Timestamp).Return(nil)

	err := uc.HandleGameStarted(context.Background(), event)

	assert.NoError(t, err)
	mockEventRepo.AssertExpectations(t)
	mockGameRepo.AssertExpectations(t)
	mockUserRepo.AssertExpectations(t)
}

func TestHandleGameStarted_ReleasesReservation(t *testing.T) {
	ctx := context.Background()
	mockGameRepo := new(MockGameRepository)
	mockEventRepo := new(MockGameEventRepository)
	mockUserRepo := new(MockUserRepository)

	uc := usecase.NewGamePersistenceUseCase(mockGameRepo, mockEventRepo, mockUserRepo)
	reservationUC := usecase.NewReservationUseCase(mockGameRepo, nil, usecase.ReservationConfig{
		MaxPerWallet:           3,
		TimeoutSeconds:         60,
		CleanupIntervalSeconds: 5,
	})
	uc.SetReservationUseCase(reservationUC)

	existingGame := &entity.Game{
		GameID:           123,
		PlayerOneAddress: testWallet1,
		Status:           entity.GameStatusWaitingForOpponent,
	}

	mockGameRepo.On("GetByID", ctx, int64(123)).Return(existingGame, nil).Twice()
	_, err := reservationUC.Reserve(ctx, 123, testWallet2)
	require.NoError(t, err)

	event := &entity.GameEvent{
		EventType:       entity.EventTypeGameStarted,
		GameID:          123,
		TransactionHash: "tx_start_123",
		BlockNumber:     1001,
		Timestamp:       time.Now(),
		EventData: map[string]interface{}{
			"game_id":    int64(123),
			"player_two": testWallet2,
		},
	}

	mockUserRepo.On("EnsureUserByWallet", mock.Anything, testWallet2).Return(nil)
	mockEventRepo.On("Upsert", mock.Anything, event).Run(func(args mock.Arguments) {
		e := args.Get(1).(*entity.GameEvent)
		e.ID = 1
	}).Return(nil)
	mockGameRepo.On("JoinGame", mock.Anything, int64(123), testWallet2, "tx_start_123", event.Timestamp).Return(nil)

	err = uc.HandleGameStarted(ctx, event)

	assert.NoError(t, err)
	reservation, getErr := reservationUC.GetReservation(ctx, 123)
	require.NoError(t, getErr)
	assert.Nil(t, reservation)
	mockEventRepo.AssertExpectations(t)
	mockGameRepo.AssertExpectations(t)
	mockUserRepo.AssertExpectations(t)
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
		GameID:           123,
		PlayerOneAddress: winnerAddress,
		PlayerTwoAddress: &loserPtr,
		BetAmount:        1000000000,
		Status:           entity.GameStatusWaitingForOpenBids,
	}
	mockGameRepo.On("GetByID", mock.Anything, int64(123)).Return(existingGame, nil)

	event := &entity.GameEvent{
		EventType:       entity.EventTypeGameFinished,
		GameID:          123,
		TransactionHash: "tx_finish_123",
		BlockNumber:     1002,
		Timestamp:       time.Now(),
		EventData: map[string]interface{}{
			"game_id":        int64(123),
			"winner":         winnerAddress,
			"total_gainings": int64(1900000000), // Contract event payload
		},
	}

	mockEventRepo.On("Upsert", mock.Anything, event).Run(func(args mock.Arguments) {
		e := args.Get(1).(*entity.GameEvent)
		e.ID = 1 // Simulate successful insert
	}).Return(nil)
	mockGameRepo.On("CompleteGame", mock.Anything, int64(123), winnerAddress, int64(1900000000), "tx_finish_123", event.Timestamp).Return(nil)

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
		GameID:           123,
		PlayerOneAddress: player1,
		PlayerTwoAddress: &player2Ptr,
		BetAmount:        1000000000,
		Status:           entity.GameStatusWaitingForOpenBids,
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

	mockEventRepo.On("Upsert", mock.Anything, event).Run(func(args mock.Arguments) {
		e := args.Get(1).(*entity.GameEvent)
		e.ID = 1 // Simulate successful insert
	}).Return(nil)
	mockGameRepo.On("CompleteGame", mock.Anything, int64(123), "", int64(0), "tx_draw_123", event.Timestamp).Return(nil)

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
	mockUserRepo := new(MockUserRepository)

	uc := usecase.NewGamePersistenceUseCase(mockGameRepo, mockEventRepo, mockUserRepo)

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

	// Both calls need EnsureUserByWallet
	mockUserRepo.On("EnsureUserByWallet", mock.Anything, "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2").Return(nil)

	// First call - should process normally
	// Order: Create game first, then Upsert event
	mockGameRepo.On("CreateOrIgnore", mock.Anything, mock.Anything).Return(true, nil).Once()
	// Upsert sets event.ID = 1 (via Run) - indicates new event
	mockEventRepo.On("Upsert", mock.Anything, mock.MatchedBy(func(e *entity.GameEvent) bool {
		return e.TransactionHash == "tx_init_123"
	})).Run(func(args mock.Arguments) {
		e := args.Get(1).(*entity.GameEvent)
		e.ID = 1 // Simulate successful insert
	}).Return(nil).Once()

	err := uc.HandleGameInitialized(context.Background(), event)
	assert.NoError(t, err)
	assert.NotZero(t, event.ID, "Event ID should be set after first successful Upsert")

	// Reset event.ID to simulate a new call (in real code, the same event object might be reused)
	event.ID = 0
	event.Payload = "" // Reset payload to re-serialize

	// Second call with same tx hash - CreateOrIgnore returns false (game already exists)
	// Upsert is still called but should return event.ID=0 (duplicate via ON CONFLICT DO NOTHING)
	mockGameRepo.On("CreateOrIgnore", mock.Anything, mock.Anything).Return(false, nil).Once()
	// This time simulate duplicate - set event.ID back to 0 after Upsert mock returns
	mockEventRepo.On("Upsert", mock.Anything, mock.MatchedBy(func(e *entity.GameEvent) bool {
		return e.TransactionHash == "tx_init_123"
	})).Run(func(args mock.Arguments) {
		// Simulate ON CONFLICT DO NOTHING - event.ID stays 0
		e := args.Get(1).(*entity.GameEvent)
		e.ID = 0
	}).Return(nil).Once()

	err = uc.HandleGameInitialized(context.Background(), event)
	assert.NoError(t, err) // No error - idempotent processing
	assert.Zero(t, event.ID, "Event ID should be 0 for duplicate event")

	mockEventRepo.AssertExpectations(t)
	mockGameRepo.AssertExpectations(t)
	mockUserRepo.AssertExpectations(t)
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
		BetAmount:            1000000000, // 1 TON
		ReferrerFeeNumerator: 50,         // 0.5% = 50 basis points
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
			"game_id":        int64(123),
			"winner":         winnerAddress,
			"total_gainings": int64(1900000000),
		},
	}

	mockEventRepo.On("Upsert", mock.Anything, event).Run(func(args mock.Arguments) {
		e := args.Get(1).(*entity.GameEvent)
		e.ID = 1 // Simulate successful insert
	}).Return(nil)
	mockGameRepo.On("CompleteGame", mock.Anything, int64(123), winnerAddress, int64(1900000000), "tx_finish_123", event.Timestamp).Return(nil)

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
