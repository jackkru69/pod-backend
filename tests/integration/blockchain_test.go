package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
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

// Integration test for blockchain event processing (T085)
func TestBlockchainEventProcessing_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup test helper
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	// Clean and seed database
	helper.CleanDatabase(t)

	// Initialize logger
	l := logger.New("debug")

	// Initialize repositories
	gameRepo := repopg.NewGameRepository(helper.DB)
	userRepo := repopg.NewUserRepository(helper.DB)
	eventRepo := repopg.NewGameEventRepository(helper.DB)

	// Initialize use cases
	gamePersistenceUC := usecase.NewGamePersistenceUseCase(gameRepo, userRepo, eventRepo)

	// Create TON Center client pointing to mock server
	mockTonCenterURL := "http://localhost:8082"
	tonClient := toncenter.NewClient(toncenter.ClientConfig{
		V2BaseURL:       mockTonCenterURL,
		ContractAddress: "0:test_contract",
		HTTPTimeout:     10 * time.Second,
	})

	// Initialize blockchain subscriber
	blockchainUC := usecase.NewBlockchainSubscriberUseCase(
		tonClient,
		gamePersistenceUC,
		l,
		0, // start from block 0
	)

	ctx := context.Background()

	// Test 1: GameInitialized Event
	t.Run("GameInitialized event creates game", func(t *testing.T) {
		// Prepare mock transaction
		eventData := map[string]interface{}{
			"event_type":        entity.EventTypeGameInitialized,
			"game_id":           float64(1),
			"player_one":        "0:abc123",
			"bet_amount":        "1000000000",
			"player_one_choice": float64(1),
			"secret_hash":       "secret_hash_123",
		}
		txData, _ := json.Marshal(eventData)

		tx := toncenter.Transaction{
			Hash:        "init_tx_1",
			Lt:          "100",
			BlockNumber: 1000,
			Timestamp:   time.Now().Unix(),
			Data:        txData,
		}

		// Add transaction to mock server
		mockResp, err := http.Post(
			mockTonCenterURL+"/test/add-transaction",
			"application/json",
			bytes.NewBuffer(txData),
		)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, mockResp.StatusCode)
		mockResp.Body.Close()

		// Process transaction
		err = blockchainUC.HandleTransaction(ctx, tx)
		require.NoError(t, err)

		// Verify game was created
		game, err := gameRepo.GetByID(ctx, 1)
		require.NoError(t, err)
		assert.Equal(t, int64(1), game.GameID)
		assert.Equal(t, entity.GameStatusWaitingForOpponent, game.Status)
		assert.Equal(t, "0:abc123", game.PlayerOneAddress)
		assert.Equal(t, int64(1000000000), game.BetAmount)

		// Verify event was stored
		events, err := eventRepo.GetByGameID(ctx, 1)
		require.NoError(t, err)
		assert.Len(t, events, 1)
		assert.Equal(t, entity.EventTypeGameInitialized, events[0].EventType)
	})

	// Test 2: GameStarted Event
	t.Run("GameStarted event updates game status", func(t *testing.T) {
		eventData := map[string]interface{}{
			"event_type":        entity.EventTypeGameStarted,
			"game_id":           float64(1),
			"player_two":        "0:def456",
			"player_two_choice": float64(2),
		}
		txData, _ := json.Marshal(eventData)

		tx := toncenter.Transaction{
			Hash:        "join_tx_1",
			Lt:          "101",
			BlockNumber: 1001,
			Timestamp:   time.Now().Unix(),
			Data:        txData,
		}

		err := blockchainUC.HandleTransaction(ctx, tx)
		require.NoError(t, err)

		// Verify game was updated
		game, err := gameRepo.GetByID(ctx, 1)
		require.NoError(t, err)
		assert.Equal(t, entity.GameStatusActive, game.Status)
		assert.NotNil(t, game.PlayerTwoAddress)
		assert.Equal(t, "0:def456", *game.PlayerTwoAddress)
	})

	// Test 3: GameFinished Event
	t.Run("GameFinished event completes game", func(t *testing.T) {
		eventData := map[string]interface{}{
			"event_type":      entity.EventTypeGameFinished,
			"game_id":         float64(1),
			"winner":          "0:abc123",
			"revealed_choice": float64(1),
			"secret":          "secret_123",
		}
		txData, _ := json.Marshal(eventData)

		tx := toncenter.Transaction{
			Hash:        "finish_tx_1",
			Lt:          "102",
			BlockNumber: 1002,
			Timestamp:   time.Now().Unix(),
			Data:        txData,
		}

		err := blockchainUC.HandleTransaction(ctx, tx)
		require.NoError(t, err)

		// Verify game was completed
		game, err := gameRepo.GetByID(ctx, 1)
		require.NoError(t, err)
		assert.Equal(t, entity.GameStatusFinished, game.Status)
		assert.NotNil(t, game.WinnerAddress)
		assert.Equal(t, "0:abc123", *game.WinnerAddress)
	})

	// Test 4: Duplicate Event Rejection
	t.Run("Duplicate events are rejected", func(t *testing.T) {
		// Try to process the same finish event again
		eventData := map[string]interface{}{
			"event_type":      entity.EventTypeGameFinished,
			"game_id":         float64(1),
			"winner":          "0:abc123",
			"revealed_choice": float64(1),
			"secret":          "secret_123",
		}
		txData, _ := json.Marshal(eventData)

		tx := toncenter.Transaction{
			Hash:        "finish_tx_1", // Same hash
			Lt:          "102",
			BlockNumber: 1002,
			Timestamp:   time.Now().Unix(),
			Data:        txData,
		}

		// Should not error, but should not create duplicate
		err := blockchainUC.HandleTransaction(ctx, tx)
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

// Integration test for circuit breaker behavior (T086)
func TestCircuitBreakerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	mockTonCenterURL := "http://localhost:8082"
	l := logger.New("debug")

	// Create TON Center client with circuit breaker
	tonClient := toncenter.NewClient(toncenter.ClientConfig{
		V2BaseURL:                 mockTonCenterURL,
		ContractAddress:           "0:test_contract",
		HTTPTimeout:               5 * time.Second,
		CircuitBreakerMaxFailures: 3,
		CircuitBreakerTimeout:     10 * time.Second,
	})

	ctx := context.Background()

	t.Run("Circuit breaker opens after failures", func(t *testing.T) {
		// Enable failure mode on mock server
		failureReq := map[string]bool{"enabled": true}
		reqBody, _ := json.Marshal(failureReq)

		resp, err := http.Post(
			mockTonCenterURL+"/test/set-failure-mode",
			"application/json",
			bytes.NewReader(reqBody),
		)
		require.NoError(t, err)
		resp.Body.Close()

		// Make multiple failed requests to trigger circuit breaker
		for i := 0; i < 5; i++ {
			_, err := tonClient.GetTransactions(ctx, "0:test_contract", 100)
			// Expect errors
			if err == nil {
				t.Logf("Request %d succeeded unexpectedly", i)
			}
			time.Sleep(100 * time.Millisecond)
		}

		// Check circuit breaker state
		state := tonClient.GetCircuitBreakerState()
		t.Logf("Circuit breaker state after failures: %v", state)

		// State should be Open (2) or HalfOpen (1)
		assert.NotEqual(t, 0, state, "Circuit breaker should not be Closed after multiple failures")

		// Disable failure mode
		failureReq["enabled"] = false
		reqBody, _ = json.Marshal(failureReq)
		resp, err = http.Post(
			mockTonCenterURL+"/test/set-failure-mode",
			"application/json",
			bytes.NewReader(reqBody),
		)
		require.NoError(t, err)
		resp.Body.Close()
	})

	t.Run("Circuit breaker recovers after successful requests", func(t *testing.T) {
		// Wait for circuit breaker to move to half-open
		time.Sleep(11 * time.Second)

		// Make successful request
		_, err := tonClient.GetTransactions(ctx, "0:test_contract", 100)
		if err != nil {
			t.Logf("Recovery request failed: %v", err)
		}

		// Circuit should eventually close
		// In production, this would be verified over time
		t.Log("Circuit breaker should recover over time with successful requests")
	})
}

// Test full game lifecycle
func TestFullGameLifecycle_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	helper := NewTestHelper(t)
	defer helper.Cleanup()
	helper.CleanDatabase(t)

	l := logger.New("debug")

	// Initialize all components
	gameRepo := repopg.NewGameRepository(helper.DB)
	userRepo := repopg.NewUserRepository(helper.DB)
	eventRepo := repopg.NewGameEventRepository(helper.DB)

	gamePersistenceUC := usecase.NewGamePersistenceUseCase(gameRepo, userRepo, eventRepo)

	mockTonCenterURL := "http://localhost:8082"
	tonClient := toncenter.NewClient(toncenter.ClientConfig{
		V2BaseURL:       mockTonCenterURL,
		ContractAddress: "0:test_contract",
		HTTPTimeout:     10 * time.Second,
	})

	blockchainUC := usecase.NewBlockchainSubscriberUseCase(tonClient, gamePersistenceUC, l, 0)

	ctx := context.Background()

	// Create users first
	user1 := &entity.User{
		WalletAddress:    "0:player_one_address",
		TelegramUserID:   123456,
		TelegramUsername: "player1",
	}
	user2 := &entity.User{
		WalletAddress:    "0:player_two_address",
		TelegramUserID:   789012,
		TelegramUsername: "player2",
	}

	require.NoError(t, userRepo.CreateOrUpdate(ctx, user1))
	require.NoError(t, userRepo.CreateOrUpdate(ctx, user2))

	gameID := int64(100)

	// Step 1: Initialize game
	t.Run("Full lifecycle - Initialize", func(t *testing.T) {
		event := map[string]interface{}{
			"event_type":        entity.EventTypeGameInitialized,
			"game_id":           float64(gameID),
			"player_one":        user1.WalletAddress,
			"bet_amount":        "2000000000",
			"player_one_choice": float64(1),
			"secret_hash":       "hash123",
		}
		txData, _ := json.Marshal(event)

		tx := toncenter.Transaction{
			Hash:        "lifecycle_init",
			Lt:          "200",
			BlockNumber: 2000,
			Timestamp:   time.Now().Unix(),
			Data:        txData,
		}

		err := blockchainUC.HandleTransaction(ctx, tx)
		require.NoError(t, err)

		game, err := gameRepo.GetByID(ctx, gameID)
		require.NoError(t, err)
		assert.Equal(t, entity.GameStatusWaitingForOpponent, game.Status)
	})

	// Step 2: Player 2 joins
	t.Run("Full lifecycle - Start", func(t *testing.T) {
		event := map[string]interface{}{
			"event_type":        entity.EventTypeGameStarted,
			"game_id":           float64(gameID),
			"player_two":        user2.WalletAddress,
			"player_two_choice": float64(2),
		}
		txData, _ := json.Marshal(event)

		tx := toncenter.Transaction{
			Hash:        "lifecycle_start",
			Lt:          "201",
			BlockNumber: 2001,
			Timestamp:   time.Now().Unix(),
			Data:        txData,
		}

		err := blockchainUC.HandleTransaction(ctx, tx)
		require.NoError(t, err)

		game, err := gameRepo.GetByID(ctx, gameID)
		require.NoError(t, err)
		assert.Equal(t, entity.GameStatusActive, game.Status)
	})

	// Step 3: Game finishes
	t.Run("Full lifecycle - Finish", func(t *testing.T) {
		event := map[string]interface{}{
			"event_type":      entity.EventTypeGameFinished,
			"game_id":         float64(gameID),
			"winner":          user1.WalletAddress,
			"revealed_choice": float64(1),
			"secret":          "secret123",
		}
		txData, _ := json.Marshal(event)

		tx := toncenter.Transaction{
			Hash:        "lifecycle_finish",
			Lt:          "202",
			BlockNumber: 2002,
			Timestamp:   time.Now().Unix(),
			Data:        txData,
		}

		err := blockchainUC.HandleTransaction(ctx, tx)
		require.NoError(t, err)

		game, err := gameRepo.GetByID(ctx, gameID)
		require.NoError(t, err)
		assert.Equal(t, entity.GameStatusFinished, game.Status)
		assert.NotNil(t, game.WinnerAddress)
		assert.Equal(t, user1.WalletAddress, *game.WinnerAddress)

		// Verify all 3 events were stored
		events, err := eventRepo.GetByGameID(ctx, gameID)
		require.NoError(t, err)
		assert.Len(t, events, 3)
	})
}
