package integration_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pod-backend/internal/entity"
	"pod-backend/internal/infrastructure/toncenter"
	repopg "pod-backend/internal/repository/postgres"
	"pod-backend/internal/usecase"
	"pod-backend/pkg/logger"
)

// mockEventSource is a test double for EventSource that allows direct transaction injection.
// Implements toncenter.EventSource for integration testing (T085).
type mockEventSource struct {
	mu              sync.RWMutex
	handler         toncenter.EventHandler
	lastProcessedLt string
	connected       bool
	sourceType      string
}

func newMockEventSource() *mockEventSource {
	return &mockEventSource{
		sourceType: "mock",
		connected:  true,
	}
}

func (m *mockEventSource) Start(ctx context.Context) {
	m.mu.Lock()
	m.connected = true
	m.mu.Unlock()
}

func (m *mockEventSource) Stop() {
	m.mu.Lock()
	m.connected = false
	m.mu.Unlock()
}

func (m *mockEventSource) Subscribe(handler toncenter.EventHandler) {
	m.mu.Lock()
	m.handler = handler
	m.mu.Unlock()
}

func (m *mockEventSource) GetLastProcessedLt() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastProcessedLt
}

func (m *mockEventSource) SetLastProcessedLt(lt string) {
	m.mu.Lock()
	m.lastProcessedLt = lt
	m.mu.Unlock()
}

func (m *mockEventSource) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connected
}

func (m *mockEventSource) GetSourceType() string {
	return m.sourceType
}

// InjectTransaction simulates receiving a transaction from the blockchain.
// This directly calls the registered handler to test transaction processing.
func (m *mockEventSource) InjectTransaction(ctx context.Context, tx toncenter.Transaction) error {
	m.mu.RLock()
	handler := m.handler
	m.mu.RUnlock()

	if handler == nil {
		return nil
	}

	return handler.HandleTransaction(ctx, tx)
}

// createTransactionWithBOC creates a test transaction with BOC-encoded message.
// Uses TestMessageBuilder to create properly formatted messages.
func createTransactionWithBOC(lt, hash string, utime int64, inMsgJSON string) toncenter.Transaction {
	return toncenter.Transaction{
		Type: "raw.transaction",
		TransactionID: struct {
			Type string `json:"@type"`
			Lt   string `json:"lt"`
			Hash string `json:"hash"`
		}{
			Type: "internal.transactionId",
			Lt:   lt,
			Hash: hash,
		},
		Utime: utime,
		InMsg: json.RawMessage(inMsgJSON),
	}
}

// createTransaction creates a test transaction with proper TON Center API v2 format.
// DEPRECATED: Use createTransactionWithBOC for proper BOC-encoded messages.
// This is kept for backward compatibility with tests that don't need full parsing.
func createTransaction(lt, hash string, utime int64, eventData map[string]interface{}) toncenter.Transaction {
	inMsgData, _ := json.Marshal(eventData)
	return toncenter.Transaction{
		Type: "raw.transaction",
		TransactionID: struct {
			Type string `json:"@type"`
			Lt   string `json:"lt"`
			Hash string `json:"hash"`
		}{
			Type: "internal.transactionId",
			Lt:   lt,
			Hash: hash,
		},
		Utime: utime,
		InMsg: inMsgData, // Put event data in InMsg for parseTransaction to find
	}
}

// Integration test for blockchain event processing (T085)
func TestBlockchainEventProcessing_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup test helper
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	// Initialize logger
	l := logger.New("debug")

	// Initialize repositories using Postgres wrapper
	pg := helper.Postgres()
	gameRepo := repopg.NewGameRepository(pg)
	userRepo := repopg.NewUserRepository(pg)
	eventRepo := repopg.NewGameEventRepository(pg)

	// Initialize game persistence use case (correct arg order: gameRepo, eventRepo, userRepo)
	gamePersistenceUC := usecase.NewGamePersistenceUseCase(gameRepo, eventRepo, userRepo)

	// Create mock event source for direct transaction injection
	mockSource := newMockEventSource()

	// Initialize blockchain subscriber with EventSource abstraction (T152)
	blockchainUC := usecase.NewBlockchainSubscriberUseCase(
		mockSource,
		gamePersistenceUC,
		l,
	)
	_ = blockchainUC // Used indirectly via mockSource.handler

	ctx := context.Background()

	// Test 1: GameInitialized Event
	t.Run("GameInitialized event creates game", func(t *testing.T) {
		helper.CleanDatabase(t)

		// Create user first (FK constraint: games.player_one_address -> users.wallet_address)
		testUser := &entity.User{
			TelegramUserID:   Int64Ptr(123456789),
			TelegramUsername: "player_one",
			WalletAddress:    "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2",
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		}
		err := userRepo.CreateOrUpdate(ctx, testUser)
		require.NoError(t, err)

		eventData := map[string]interface{}{
			"event_type":        entity.EventTypeGameInitialized,
			"game_id":           float64(1),
			"player_one":        "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2",
			"bet_amount":        "1000000000",
			"player_one_choice": float64(1),
			"secret_hash":       "secret_hash_123",
		}

		tx := createTransaction("100", "init_tx_hash_1", time.Now().Unix(), eventData)

		// Process transaction via mock event source
		err = mockSource.InjectTransaction(ctx, tx)
		require.NoError(t, err)

		// Verify game was created
		game, err := gameRepo.GetByID(ctx, 1)
		require.NoError(t, err)
		assert.Equal(t, int64(1), game.GameID)
		assert.Equal(t, entity.GameStatusWaitingForOpponent, game.Status)
		assert.Equal(t, "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2", game.PlayerOneAddress)
		assert.Equal(t, int64(1000000000), game.BetAmount)

		// Verify event was stored
		events, err := eventRepo.GetByGameID(ctx, 1)
		require.NoError(t, err)
		assert.Len(t, events, 1)
		assert.Equal(t, entity.EventTypeGameInitialized, events[0].EventType)
	})

	// Test 2: GameStarted Event
	t.Run("GameStarted event updates game status", func(t *testing.T) {
		// Create player_two user (FK constraint)
		playerTwo := &entity.User{
			TelegramUserID:   Int64Ptr(987654321),
			TelegramUsername: "player_two",
			WalletAddress:    "EQBvW8Z5huBkMJYdnfAEM5JqTNLuuU3FYxrVjxFBzXn3r95X",
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		}
		err := userRepo.CreateOrUpdate(ctx, playerTwo)
		require.NoError(t, err)

		// Use same game from Test 1
		eventData := map[string]interface{}{
			"event_type":        entity.EventTypeGameStarted,
			"game_id":           float64(1),
			"player_two":        "EQBvW8Z5huBkMJYdnfAEM5JqTNLuuU3FYxrVjxFBzXn3r95X",
			"player_two_choice": float64(2),
		}

		tx := createTransaction("101", "start_tx_hash_1", time.Now().Unix(), eventData)

		err = mockSource.InjectTransaction(ctx, tx)
		require.NoError(t, err)

		// Verify game was updated
		game, err := gameRepo.GetByID(ctx, 1)
		require.NoError(t, err)
		assert.Equal(t, entity.GameStatusWaitingForOpenBids, game.Status)
		assert.NotNil(t, game.PlayerTwoAddress)
		assert.Equal(t, "EQBvW8Z5huBkMJYdnfAEM5JqTNLuuU3FYxrVjxFBzXn3r95X", *game.PlayerTwoAddress)
	})

	// Test 3: GameFinished Event
	t.Run("GameFinished event completes game", func(t *testing.T) {
		eventData := map[string]interface{}{
			"event_type":      entity.EventTypeGameFinished,
			"game_id":         float64(1),
			"winner":          "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2",
			"revealed_choice": float64(1),
			"secret":          "secret_123",
		}

		tx := createTransaction("102", "finish_tx_hash_1", time.Now().Unix(), eventData)

		err := mockSource.InjectTransaction(ctx, tx)
		require.NoError(t, err)

		// Verify game was completed
		game, err := gameRepo.GetByID(ctx, 1)
		require.NoError(t, err)
		assert.Equal(t, entity.GameStatusPaid, game.Status)
		assert.NotNil(t, game.WinnerAddress)
		assert.Equal(t, "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2", *game.WinnerAddress)
	})

	// Test 4: Duplicate Event Rejection
	t.Run("Duplicate events are rejected", func(t *testing.T) {
		// Try to process the same finish event again (same tx hash)
		eventData := map[string]interface{}{
			"event_type":      entity.EventTypeGameFinished,
			"game_id":         float64(1),
			"winner":          "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2",
			"revealed_choice": float64(1),
			"secret":          "secret_123",
		}

		tx := createTransaction("102", "finish_tx_hash_1", time.Now().Unix(), eventData)

		// Should not error, but should not create duplicate
		err := mockSource.InjectTransaction(ctx, tx)
		require.NoError(t, err)

		// Verify only one finish event exists
		events, err := eventRepo.GetByGameID(ctx, 1)
		require.NoError(t, err)

		finishEvents := 0
		for _, e := range events {
			if e.EventType == entity.EventTypeGameFinished {
				finishEvents++
			}
		}
		assert.Equal(t, 1, finishEvents, "Should have only one finish event")
	})
}

// Integration test for event source connectivity (T150)
func TestEventSourceConnectivity_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Run("Mock event source reports connected status", func(t *testing.T) {
		mockSource := newMockEventSource()

		assert.True(t, mockSource.IsConnected())
		assert.Equal(t, "mock", mockSource.GetSourceType())
	})

	t.Run("Event source tracks last processed lt", func(t *testing.T) {
		mockSource := newMockEventSource()

		mockSource.SetLastProcessedLt("12345")
		assert.Equal(t, "12345", mockSource.GetLastProcessedLt())

		mockSource.SetLastProcessedLt("67890")
		assert.Equal(t, "67890", mockSource.GetLastProcessedLt())
	})

	t.Run("Stop disconnects event source", func(t *testing.T) {
		mockSource := newMockEventSource()
		assert.True(t, mockSource.IsConnected())

		mockSource.Stop()
		assert.False(t, mockSource.IsConnected())
	})
}

// Test full game lifecycle with proper status transitions
func TestFullGameLifecycle_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	helper := NewTestHelper(t)
	defer helper.Cleanup()

	l := logger.New("debug")

	// Initialize all components
	pg := helper.Postgres()
	gameRepo := repopg.NewGameRepository(pg)
	userRepo := repopg.NewUserRepository(pg)
	eventRepo := repopg.NewGameEventRepository(pg)

	gamePersistenceUC := usecase.NewGamePersistenceUseCase(gameRepo, eventRepo, userRepo)

	mockSource := newMockEventSource()
	blockchainUC := usecase.NewBlockchainSubscriberUseCase(mockSource, gamePersistenceUC, l)
	_ = blockchainUC

	ctx := context.Background()

	// Create users first
	user1 := &entity.User{
		WalletAddress:    "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2",
		TelegramUserID:   Int64Ptr(123456),
		TelegramUsername: "player1",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	user2 := &entity.User{
		WalletAddress:    "EQBvW8Z5huBkMJYdnfAEM5JqTNLuuU3FYxrVjxFBzXn3r95X",
		TelegramUserID:   Int64Ptr(789012),
		TelegramUsername: "player2",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	require.NoError(t, userRepo.CreateOrUpdate(ctx, user1))
	require.NoError(t, userRepo.CreateOrUpdate(ctx, user2))

	gameID := int64(100)

	// Step 1: Initialize game
	t.Run("Full lifecycle - Initialize", func(t *testing.T) {
		eventData := map[string]interface{}{
			"event_type":        entity.EventTypeGameInitialized,
			"game_id":           float64(gameID),
			"player_one":        user1.WalletAddress,
			"bet_amount":        "2000000000",
			"player_one_choice": float64(1),
			"secret_hash":       "hash123",
		}

		tx := createTransaction("200", "lifecycle_init_hash", time.Now().Unix(), eventData)

		err := mockSource.InjectTransaction(ctx, tx)
		require.NoError(t, err)

		game, err := gameRepo.GetByID(ctx, gameID)
		require.NoError(t, err)
		assert.Equal(t, entity.GameStatusWaitingForOpponent, game.Status)
	})

	// Step 2: Player 2 joins
	t.Run("Full lifecycle - Start", func(t *testing.T) {
		eventData := map[string]interface{}{
			"event_type":        entity.EventTypeGameStarted,
			"game_id":           float64(gameID),
			"player_two":        user2.WalletAddress,
			"player_two_choice": float64(2),
		}

		tx := createTransaction("201", "lifecycle_start_hash", time.Now().Unix(), eventData)

		err := mockSource.InjectTransaction(ctx, tx)
		require.NoError(t, err)

		game, err := gameRepo.GetByID(ctx, gameID)
		require.NoError(t, err)
		assert.Equal(t, entity.GameStatusWaitingForOpenBids, game.Status)
	})

	// Step 3: Game finishes
	t.Run("Full lifecycle - Finish", func(t *testing.T) {
		eventData := map[string]interface{}{
			"event_type":      entity.EventTypeGameFinished,
			"game_id":         float64(gameID),
			"winner":          user1.WalletAddress,
			"revealed_choice": float64(1),
			"secret":          "secret123",
		}

		tx := createTransaction("202", "lifecycle_finish_hash", time.Now().Unix(), eventData)

		err := mockSource.InjectTransaction(ctx, tx)
		require.NoError(t, err)

		game, err := gameRepo.GetByID(ctx, gameID)
		require.NoError(t, err)
		assert.Equal(t, entity.GameStatusPaid, game.Status)
		assert.NotNil(t, game.WinnerAddress)
		assert.Equal(t, user1.WalletAddress, *game.WinnerAddress)

		// Verify all 3 events were stored
		events, err := eventRepo.GetByGameID(ctx, gameID)
		require.NoError(t, err)
		assert.Len(t, events, 3)
	})
}

// Test transaction format handling
func TestTransactionFormat_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Run("Transaction has correct field accessors", func(t *testing.T) {
		eventData := map[string]interface{}{
			"event_type": "test",
		}

		tx := createTransaction("12345", "test_hash_base64", 1704067200, eventData)

		// Test the accessor methods from client.go
		assert.Equal(t, "test_hash_base64", tx.Hash())
		assert.Equal(t, "12345", tx.Lt())
		assert.Equal(t, int64(1704067200), tx.Utime)
	})
}
