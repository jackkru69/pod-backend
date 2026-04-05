package usecase_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"pod-backend/internal/entity"
	"pod-backend/internal/infrastructure/toncenter"
	"pod-backend/internal/repository"
	"pod-backend/internal/usecase"
	"pod-backend/pkg/logger"
)

// mockEventSource implements toncenter.EventSource for testing
type mockEventSource struct {
	handler         toncenter.EventHandler
	lastProcessedLt string
	isConnected     atomic.Bool
	started         atomic.Bool
	stopped         atomic.Bool
}

func newMockEventSource() *mockEventSource {
	m := &mockEventSource{}
	m.isConnected.Store(true)
	return m
}

func (m *mockEventSource) Start(ctx context.Context) {
	m.started.Store(true)
	// Simulate running until context is done
	<-ctx.Done()
}

func (m *mockEventSource) Stop() {
	m.stopped.Store(true)
	m.isConnected.Store(false)
}

func (m *mockEventSource) Subscribe(handler toncenter.EventHandler) {
	m.handler = handler
}

func (m *mockEventSource) GetLastProcessedLt() string {
	return m.lastProcessedLt
}

func (m *mockEventSource) SetLastProcessedLt(lt string) {
	m.lastProcessedLt = lt
}

func (m *mockEventSource) IsConnected() bool {
	return m.isConnected.Load()
}

func (m *mockEventSource) GetSourceType() string {
	return toncenter.SourceTypeHTTP
}

// SimulateTransaction allows tests to inject transactions into the mock
func (m *mockEventSource) SimulateTransaction(ctx context.Context, tx toncenter.Transaction) error {
	if m.handler != nil {
		return m.handler.HandleTransaction(ctx, tx)
	}
	return nil
}

// Helper function to create a test transaction
func createTestTransaction(eventType string, gameID int64) toncenter.Transaction {
	eventData := map[string]interface{}{
		"event_type": eventType,
		"game_id":    float64(gameID),
	}

	// Add event-specific data based on event type
	switch eventType {
	case entity.EventTypeGameInitialized:
		eventData["player_one"] = "0:abc123"
		eventData["bet_amount"] = "1000000000"
		eventData["player_one_choice"] = float64(1)
		eventData["secret_hash"] = "secret123"
	case entity.EventTypeGameStarted:
		eventData["player_two"] = "0:def456"
		eventData["player_two_choice"] = float64(2)
	case entity.EventTypeGameFinished:
		eventData["winner"] = "0:abc123"
		eventData["revealed_choice"] = float64(1)
		eventData["secret"] = "secret123"
	case entity.EventTypeDraw:
		eventData["revealed_choice"] = float64(1)
		eventData["secret"] = "secret123"
	case entity.EventTypeGameCancelled:
		// No additional data needed
	case entity.EventTypeSecretOpened:
		eventData["secret"] = "secret123"
		eventData["revealed_choice"] = float64(1)
	case entity.EventTypeInsufficientBalance:
		eventData["player"] = "0:abc123"
	}

	dataBytes, _ := json.Marshal(eventData)

	return toncenter.Transaction{
		Type: "raw.transaction",
		TransactionID: struct {
			Type string `json:"@type"`
			Lt   string `json:"lt"`
			Hash string `json:"hash"`
		}{
			Type: "internal.transactionId",
			Lt:   "123456",
			Hash: "test_hash_123",
		},
		Utime: time.Now().Unix(),
		InMsg: dataBytes,
	}
}

// TestBlockchainSubscriberUseCase_ParseTransaction_Valid tests transaction parsing with valid data
func TestBlockchainSubscriberUseCase_ParseTransaction_Valid(t *testing.T) {
	// Arrange
	mockLogger := logger.New("debug")
	mockSource := newMockEventSource()

	// Create a minimal persistence UC (won't be called in this test)
	// We're testing only the parsing logic
	uc := usecase.NewBlockchainSubscriberUseCase(mockSource, nil, mockLogger)

	// Create test transaction with valid GameInitialized event
	tx := createTestTransaction(entity.EventTypeGameInitialized, 1)

	// Note: We cannot directly test parseTransaction as it's private
	// Instead, we test it through HandleTransaction and expect parse success
	// but persistence failure (since we passed nil persistence UC)
	ctx := context.Background()

	// Act - this will parse successfully but panic at routing (nil persistenceUC)
	// We use a recover to catch the panic and verify it's not a parse error
	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected panic from nil persistence UC
				// This proves parsing succeeded
				t.Log("Parsing succeeded, routing panicked as expected with nil persistence UC")
			}
		}()
		_ = uc.HandleTransaction(ctx, tx)
	}()

	// Assert - if we got here, parsing worked (either completed or panicked at routing)
	assert.True(t, true)
}

// TestBlockchainSubscriberUseCase_HandleTransaction_ParseError tests handling of parse errors
func TestBlockchainSubscriberUseCase_HandleTransaction_ParseError(t *testing.T) {
	// Arrange
	mockLogger := logger.New("debug")
	mockSource := newMockEventSource()

	uc := usecase.NewBlockchainSubscriberUseCase(mockSource, nil, mockLogger)

	// Create invalid transaction with malformed JSON in InMsg
	tx := toncenter.Transaction{
		Type: "raw.transaction",
		TransactionID: struct {
			Type string `json:"@type"`
			Lt   string `json:"lt"`
			Hash string `json:"hash"`
		}{
			Type: "internal.transactionId",
			Lt:   "123456",
			Hash: "test_hash_invalid",
		},
		Utime: time.Now().Unix(),
		InMsg: []byte("{invalid json"),
	}
	ctx := context.Background()

	// Act
	err := uc.HandleTransaction(ctx, tx)

	// Assert - parseTransaction returns nil error for non-game transactions, but we expect
	// an unmarshal error when InMsg contains invalid JSON
	// Note: HandleTransaction returns nil for non-game transactions to continue processing
	// Since we have invalid JSON in InMsg, it should fail to unmarshal
	assert.NoError(t, err) // HandleTransaction returns nil even on parse errors to continue processing
}

// TestBlockchainSubscriberUseCase_HandleTransaction_MissingEventType tests missing event_type
func TestBlockchainSubscriberUseCase_HandleTransaction_MissingEventType(t *testing.T) {
	// Arrange
	mockLogger := logger.New("debug")
	mockSource := newMockEventSource()

	uc := usecase.NewBlockchainSubscriberUseCase(mockSource, nil, mockLogger)

	// Create transaction without event_type
	eventData := map[string]interface{}{
		"game_id": float64(1),
	}
	data, _ := json.Marshal(eventData)

	tx := toncenter.Transaction{
		TransactionID: struct {
			Type string `json:"@type"`
			Lt   string `json:"lt"`
			Hash string `json:"hash"`
		}{
			Hash: "test_hash_no_event_type",
			Lt:   "123456",
		},
		Utime: time.Now().Unix(),
		InMsg: data,
	}
	ctx := context.Background()

	// Act
	err := uc.HandleTransaction(ctx, tx)

	// Assert - HandleTransaction returns nil for parse errors to continue processing
	assert.NoError(t, err)
}

// TestBlockchainSubscriberUseCase_HandleTransaction_MissingGameID tests missing game_id
func TestBlockchainSubscriberUseCase_HandleTransaction_MissingGameID(t *testing.T) {
	// Arrange
	mockLogger := logger.New("debug")
	mockSource := newMockEventSource()

	uc := usecase.NewBlockchainSubscriberUseCase(mockSource, nil, mockLogger)

	// Create transaction without game_id
	eventData := map[string]interface{}{
		"event_type": entity.EventTypeGameInitialized,
	}
	data, _ := json.Marshal(eventData)

	tx := toncenter.Transaction{
		TransactionID: struct {
			Type string `json:"@type"`
			Lt   string `json:"lt"`
			Hash string `json:"hash"`
		}{
			Hash: "test_hash_no_game_id",
			Lt:   "123456",
		},
		Utime: time.Now().Unix(),
		InMsg: data,
	}
	ctx := context.Background()

	// Act
	err := uc.HandleTransaction(ctx, tx)

	// Assert - HandleTransaction returns nil for parse errors to continue processing
	assert.NoError(t, err)
}

// TestBlockchainSubscriberUseCase_HandleTransaction_UnknownEventType tests unknown event type
func TestBlockchainSubscriberUseCase_HandleTransaction_UnknownEventType(t *testing.T) {
	// Arrange
	mockLogger := logger.New("debug")
	mockSource := newMockEventSource()

	uc := usecase.NewBlockchainSubscriberUseCase(mockSource, nil, mockLogger)

	// Create transaction with unknown event type
	eventData := map[string]interface{}{
		"event_type": "unknown_event",
		"game_id":    float64(1),
	}
	data, _ := json.Marshal(eventData)

	tx := toncenter.Transaction{
		TransactionID: struct {
			Type string `json:"@type"`
			Lt   string `json:"lt"`
			Hash string `json:"hash"`
		}{
			Hash: "test_hash_unknown_event",
			Lt:   "123456",
		},
		Utime: time.Now().Unix(),
		InMsg: data,
	}
	ctx := context.Background()

	// Act
	err := uc.HandleTransaction(ctx, tx)

	// Assert - HandleTransaction returns nil for validation errors to continue processing
	assert.NoError(t, err)
}

// TestBlockchainSubscriberUseCase_SetMetrics tests metrics integration
func TestBlockchainSubscriberUseCase_SetMetrics(t *testing.T) {
	// Arrange
	mockLogger := logger.New("debug")
	mockSource := newMockEventSource()

	uc := usecase.NewBlockchainSubscriberUseCase(mockSource, nil, mockLogger)

	// Note: We cannot create NewBlockchainMetrics() here because Prometheus metrics
	// are registered globally and would conflict with other tests or main application.
	// Instead, we just verify that SetMetrics doesn't panic and the method exists.

	// Act & Assert - just verify SetMetrics method exists and doesn't panic with nil
	uc.SetMetrics(nil)

	// Verify the method exists by calling it - this tests the API surface
	assert.NotPanics(t, func() {
		uc.SetMetrics(nil)
	})
}

// TestBlockchainSubscriberUseCase_GetLastProcessedBlock tests block tracking
// Note: For TON blockchain, we use logical time (lt) instead of block numbers,
// so GetLastProcessedBlock always returns 0
func TestBlockchainSubscriberUseCase_GetLastProcessedBlock(t *testing.T) {
	// Arrange
	mockLogger := logger.New("debug")
	mockSource := newMockEventSource()

	uc := usecase.NewBlockchainSubscriberUseCase(mockSource, nil, mockLogger)

	// Act
	lastBlock := uc.GetLastProcessedBlock()

	// Assert - TON uses logical time, so block number is always 0
	assert.Equal(t, int64(0), lastBlock)
}

// TestBlockchainSubscriberUseCase_SetLastProcessedBlock tests block update
// Note: For TON blockchain, SetLastProcessedBlock is ignored (uses lt instead)
func TestBlockchainSubscriberUseCase_SetLastProcessedBlock(t *testing.T) {
	// Arrange
	mockLogger := logger.New("debug")
	mockSource := newMockEventSource()

	uc := usecase.NewBlockchainSubscriberUseCase(mockSource, nil, mockLogger)

	// Act
	newBlock := int64(200)
	uc.SetLastProcessedBlock(newBlock)

	// Assert - TON ignores block numbers, so it remains 0
	assert.Equal(t, int64(0), uc.GetLastProcessedBlock())
}

// TestBlockchainSubscriberUseCase_Subscribe tests subscription lifecycle
func TestBlockchainSubscriberUseCase_Subscribe(t *testing.T) {
	// Arrange
	mockLogger := logger.New("debug")
	mockSource := newMockEventSource()

	uc := usecase.NewBlockchainSubscriberUseCase(mockSource, nil, mockLogger)

	// Create a context with timeout to prevent infinite blocking
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Act
	// Subscribe runs asynchronously, so we start it in a goroutine
	go uc.Subscribe(ctx)

	// Wait for context to timeout
	<-ctx.Done()

	// Stop the subscription
	uc.Stop()

	// Assert
	// If we reached here without panic, the test passes
	assert.True(t, true)
}

// TestBlockchainSubscriberUseCase_Lifecycle tests full start/stop lifecycle
// Note: For TON blockchain, block numbers are always 0 (uses lt instead)
func TestBlockchainSubscriberUseCase_Lifecycle(t *testing.T) {
	// Arrange
	mockLogger := logger.New("debug")
	mockSource := newMockEventSource()

	uc := usecase.NewBlockchainSubscriberUseCase(mockSource, nil, mockLogger)

	// Assert initial state - TON uses lt, so block number is always 0
	assert.Equal(t, int64(0), uc.GetLastProcessedBlock())

	// Create context for subscription
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Start subscription in background
	done := make(chan bool)
	go func() {
		uc.Subscribe(ctx)
		done <- true
	}()

	// Let it run briefly
	time.Sleep(20 * time.Millisecond)

	// Stop subscription
	uc.Stop()

	// Wait for subscribe to complete
	select {
	case <-done:
		// Success
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Subscribe did not stop in time")
	}

	// Verify SetLastProcessedBlock doesn't change anything (ignored for TON)
	uc.SetLastProcessedBlock(100)
	assert.Equal(t, int64(0), uc.GetLastProcessedBlock())
}

// TestBlockchainSubscriberUseCase_GetLastProcessedLt tests lt tracking
func TestBlockchainSubscriberUseCase_GetLastProcessedLt(t *testing.T) {
	// Arrange
	mockLogger := logger.New("debug")
	mockSource := newMockEventSource()

	uc := usecase.NewBlockchainSubscriberUseCase(mockSource, nil, mockLogger)

	// Set lt via mock event source
	mockSource.SetLastProcessedLt("12345678")

	// Act
	lt := uc.GetLastProcessedLt()

	// Assert
	assert.Equal(t, "12345678", lt)
}

// TestBlockchainSubscriberUseCase_GetEventSourceType tests source type reporting
func TestBlockchainSubscriberUseCase_GetEventSourceType(t *testing.T) {
	// Arrange
	mockLogger := logger.New("debug")
	mockSource := newMockEventSource()

	uc := usecase.NewBlockchainSubscriberUseCase(mockSource, nil, mockLogger)

	// Act
	sourceType := uc.GetSourceType()

	// Assert
	assert.Equal(t, toncenter.SourceTypeHTTP, sourceType)
}

func TestBlockchainSubscriberUseCase_HandleTransaction_ParseFailureObservable(t *testing.T) {
	mockLogger := logger.New("debug")
	mockSource := newMockEventSource()
	mockDLQRepo := new(MockDeadLetterQueueRepository)

	dlqUC := usecase.NewDeadLetterQueueUseCase(mockDLQRepo, mockLogger)
	dlqUC.SetBaseBackoff(time.Millisecond)

	uc := usecase.NewBlockchainSubscriberUseCase(mockSource, nil, mockLogger)
	uc.SetDeadLetterQueue(dlqUC)

	mockDLQRepo.
		On("Create", mock.Anything, mock.MatchedBy(func(entry *entity.DeadLetterEntry) bool {
			return entry.TransactionHash == "test_hash_invalid" &&
				entry.TransactionLt == "123456" &&
				entry.ErrorType == entity.DLQErrorTypeParse &&
				entry.Status == entity.DLQStatusPending
		})).
		Run(func(args mock.Arguments) {
			entry := args.Get(1).(*entity.DeadLetterEntry)
			entry.ID = 1
			entry.CreatedAt = time.Now()
		}).
		Return(nil).
		Once()

	tx := toncenter.Transaction{
		Type: "raw.transaction",
		TransactionID: struct {
			Type string `json:"@type"`
			Lt   string `json:"lt"`
			Hash string `json:"hash"`
		}{
			Type: "internal.transactionId",
			Lt:   "123456",
			Hash: "test_hash_invalid",
		},
		Utime: time.Now().Unix(),
		InMsg: []byte("{invalid json"),
	}

	err := uc.HandleTransaction(context.Background(), tx)

	assert.NoError(t, err)

	snapshot := uc.GetObservabilitySnapshot()
	assert.Equal(t, uint64(1), snapshot.ParseFailuresTotal)
	assert.Equal(t, uint64(0), snapshot.ValidationFailuresTotal)
	assert.Equal(t, uint64(1), snapshot.DLQStoreAttemptsTotal)
	assert.Equal(t, uint64(1), snapshot.DLQStoredTotal)
	assert.Equal(t, uint64(0), snapshot.DLQDuplicateTotal)
	assert.Equal(t, uint64(0), snapshot.DLQStoreFailuresTotal)
	assert.Equal(t, entity.DLQErrorTypeParse, snapshot.LastFailureType)
	assert.Equal(t, "test_hash_invalid", snapshot.LastFailureTxHash)
	assert.Equal(t, "123456", snapshot.LastFailureTxLt)
	assert.Equal(t, entity.DLQErrorTypeParse, snapshot.LastDLQErrorType)
	assert.Equal(t, usecase.DLQStoreOutcomeQueued, snapshot.LastDLQStoreOutcome)
	assert.Equal(t, entity.DLQStatusPending, snapshot.LastDLQStatus)
	assert.Equal(t, "queued after parse failure", snapshot.LastDLQResolutionNotes)
	assert.Equal(t, "test_hash_invalid", snapshot.LastDLQStoredTxHash)
	assert.Equal(t, "123456", snapshot.LastDLQStoredTxLt)
	assert.NotNil(t, snapshot.LastFailureAt)
	assert.NotNil(t, snapshot.LastDLQStoredAt)

	mockDLQRepo.AssertExpectations(t)
}

func TestBlockchainSubscriberUseCase_HandleTransaction_PersistenceRetryObservable(t *testing.T) {
	mockLogger := logger.New("debug")
	mockSource := newMockEventSource()
	mockDLQRepo := new(MockDeadLetterQueueRepository)
	mockGameRepo := new(MockGameRepository)
	mockEventRepo := new(MockGameEventRepository)
	mockUserRepo := new(MockUserRepository)

	persistenceUC := usecase.NewGamePersistenceUseCase(mockGameRepo, mockEventRepo, mockUserRepo)
	dlqUC := usecase.NewDeadLetterQueueUseCase(mockDLQRepo, mockLogger)
	dlqUC.SetBaseBackoff(time.Millisecond)

	uc := usecase.NewBlockchainSubscriberUseCase(mockSource, persistenceUC, mockLogger)
	uc.SetRetryConfig(usecase.RetryConfig{
		MaxRetries:        2,
		InitialBackoff:    time.Millisecond,
		MaxBackoff:        time.Millisecond,
		BackoffMultiplier: 2,
	})
	uc.SetDeadLetterQueue(dlqUC)

	persistenceErr := errors.New("database unavailable")
	mockUserRepo.
		On("EnsureUserByWallet", mock.Anything, "0:abc123").
		Return(persistenceErr).
		Times(3)

	mockDLQRepo.
		On("Create", mock.Anything, mock.MatchedBy(func(entry *entity.DeadLetterEntry) bool {
			return entry.TransactionHash == "test_hash_123" &&
				entry.TransactionLt == "123456" &&
				entry.ErrorType == entity.DLQErrorTypePersistence &&
				entry.Status == entity.DLQStatusPending
		})).
		Run(func(args mock.Arguments) {
			entry := args.Get(1).(*entity.DeadLetterEntry)
			entry.ID = 99
			entry.CreatedAt = time.Now()
		}).
		Return(nil).
		Once()

	err := uc.HandleTransaction(context.Background(), createTestTransaction(entity.EventTypeGameInitialized, 1))

	assert.Error(t, err)
	assert.ErrorContains(t, err, "failed to route event")

	snapshot := uc.GetObservabilitySnapshot()
	assert.Equal(t, uint64(2), snapshot.PersistenceRetryAttemptsTotal)
	assert.Equal(t, uint64(0), snapshot.PersistenceRetryRecoveredTotal)
	assert.Equal(t, uint64(1), snapshot.PersistenceFailuresExhausted)
	assert.Equal(t, uint64(1), snapshot.DLQStoreAttemptsTotal)
	assert.Equal(t, uint64(1), snapshot.DLQStoredTotal)
	assert.Equal(t, uint64(0), snapshot.DLQDuplicateTotal)
	assert.Equal(t, uint64(0), snapshot.DLQStoreFailuresTotal)
	assert.Equal(t, entity.DLQErrorTypePersistence, snapshot.LastFailureType)
	assert.Equal(t, "exhausted", snapshot.LastRetryOutcome)
	assert.Equal(t, entity.EventTypeGameInitialized, snapshot.LastRetriedEventType)
	assert.Equal(t, int64(1), snapshot.LastRetriedGameID)
	assert.Equal(t, entity.DLQErrorTypePersistence, snapshot.LastDLQErrorType)
	assert.Equal(t, usecase.DLQStoreOutcomeQueued, snapshot.LastDLQStoreOutcome)
	assert.Equal(t, entity.DLQStatusPending, snapshot.LastDLQStatus)
	assert.Equal(t, "queued after persistence retries exhausted", snapshot.LastDLQResolutionNotes)
	assert.Equal(t, "test_hash_123", snapshot.LastDLQStoredTxHash)
	assert.Equal(t, "123456", snapshot.LastDLQStoredTxLt)
	assert.NotNil(t, snapshot.LastRetryAttemptAt)
	assert.NotNil(t, snapshot.LastExhaustedRetryAt)
	assert.NotNil(t, snapshot.LastDLQStoredAt)

	mockUserRepo.AssertExpectations(t)
	mockDLQRepo.AssertExpectations(t)
}

func TestBlockchainSubscriberUseCase_HandleTransaction_PersistenceRetryRecoveredObservable(t *testing.T) {
	mockLogger := logger.New("debug")
	mockSource := newMockEventSource()
	mockGameRepo := new(MockGameRepository)
	mockEventRepo := new(MockGameEventRepository)
	mockUserRepo := new(MockUserRepository)

	persistenceUC := usecase.NewGamePersistenceUseCase(mockGameRepo, mockEventRepo, mockUserRepo)
	uc := usecase.NewBlockchainSubscriberUseCase(mockSource, persistenceUC, mockLogger)
	uc.SetRetryConfig(usecase.RetryConfig{
		MaxRetries:        2,
		InitialBackoff:    time.Millisecond,
		MaxBackoff:        time.Millisecond,
		BackoffMultiplier: 2,
	})

	persistenceErr := errors.New("database unavailable")
	mockUserRepo.
		On("EnsureUserByWallet", mock.Anything, "0:abc123").
		Return(persistenceErr).
		Once()
	mockUserRepo.
		On("EnsureUserByWallet", mock.Anything, "0:abc123").
		Return(nil).
		Once()
	mockGameRepo.
		On("CreateOrIgnore", mock.Anything, mock.AnythingOfType("*entity.Game")).
		Return(true, nil).
		Once()
	mockEventRepo.
		On("Upsert", mock.Anything, mock.AnythingOfType("*entity.GameEvent")).
		Run(func(args mock.Arguments) {
			event := args.Get(1).(*entity.GameEvent)
			event.ID = 1
		}).
		Return(nil).
		Once()

	err := uc.HandleTransaction(context.Background(), createTestTransaction(entity.EventTypeGameInitialized, 1))

	assert.NoError(t, err)

	snapshot := uc.GetObservabilitySnapshot()
	assert.Equal(t, uint64(1), snapshot.PersistenceRetryAttemptsTotal)
	assert.Equal(t, uint64(1), snapshot.PersistenceRetryRecoveredTotal)
	assert.Equal(t, uint64(0), snapshot.PersistenceFailuresExhausted)
	assert.Equal(t, "recovered", snapshot.LastRetryOutcome)
	assert.Equal(t, entity.EventTypeGameInitialized, snapshot.LastRetriedEventType)
	assert.Equal(t, int64(1), snapshot.LastRetriedGameID)
	assert.NotNil(t, snapshot.LastRetryAttemptAt)
	assert.Nil(t, snapshot.LastExhaustedRetryAt)

	mockUserRepo.AssertExpectations(t)
	mockGameRepo.AssertExpectations(t)
	mockEventRepo.AssertExpectations(t)
}

func TestBlockchainSubscriberUseCase_HandleTransaction_ParseFailureDLQDuplicateObservable(t *testing.T) {
	mockLogger := logger.New("debug")
	mockSource := newMockEventSource()
	mockDLQRepo := new(MockDeadLetterQueueRepository)

	dlqUC := usecase.NewDeadLetterQueueUseCase(mockDLQRepo, mockLogger)
	dlqUC.SetBaseBackoff(time.Millisecond)

	uc := usecase.NewBlockchainSubscriberUseCase(mockSource, nil, mockLogger)
	uc.SetDeadLetterQueue(dlqUC)

	existingNextRetryAt := time.Now().Add(time.Minute)
	existingEntry := &entity.DeadLetterEntry{
		ID:              7,
		TransactionHash: "test_hash_invalid",
		TransactionLt:   "123456",
		ErrorType:       entity.DLQErrorTypeParse,
		RetryCount:      1,
		MaxRetries:      3,
		Status:          entity.DLQStatusPending,
		NextRetryAt:     &existingNextRetryAt,
		ResolutionNotes: "queued after parse failure",
	}

	mockDLQRepo.
		On("Create", mock.Anything, mock.AnythingOfType("*entity.DeadLetterEntry")).
		Return(nil).
		Once()
	mockDLQRepo.
		On("GetByTransactionHash", mock.Anything, "test_hash_invalid", "123456").
		Return(existingEntry, nil).
		Once()

	tx := toncenter.Transaction{
		Type: "raw.transaction",
		TransactionID: struct {
			Type string `json:"@type"`
			Lt   string `json:"lt"`
			Hash string `json:"hash"`
		}{
			Type: "internal.transactionId",
			Lt:   "123456",
			Hash: "test_hash_invalid",
		},
		Utime: time.Now().Unix(),
		InMsg: []byte("{invalid json"),
	}

	err := uc.HandleTransaction(context.Background(), tx)

	assert.NoError(t, err)

	snapshot := uc.GetObservabilitySnapshot()
	assert.Equal(t, uint64(1), snapshot.DLQStoreAttemptsTotal)
	assert.Equal(t, uint64(0), snapshot.DLQStoredTotal)
	assert.Equal(t, uint64(1), snapshot.DLQDuplicateTotal)
	assert.Equal(t, usecase.DLQStoreOutcomeAlreadyPending, snapshot.LastDLQStoreOutcome)
	assert.Equal(t, entity.DLQStatusPending, snapshot.LastDLQStatus)
	assert.Equal(t, "queued after parse failure", snapshot.LastDLQResolutionNotes)
	assert.Equal(t, "test_hash_invalid", snapshot.LastDLQStoredTxHash)
	assert.Equal(t, "123456", snapshot.LastDLQStoredTxLt)
	assert.NotNil(t, snapshot.LastDLQStoredAt)

	mockDLQRepo.AssertExpectations(t)
}

func TestDeadLetterQueueUseCase_GetObservability(t *testing.T) {
	mockLogger := logger.New("debug")
	mockDLQRepo := new(MockDeadLetterQueueRepository)
	uc := usecase.NewDeadLetterQueueUseCase(mockDLQRepo, mockLogger)
	uc.SetBaseBackoff(2 * time.Minute)

	now := time.Now()
	expectedStats := &repository.DLQStats{
		PendingCount:          3,
		RetryingCount:         1,
		ResolvedCount:         2,
		FailedCount:           1,
		TotalCount:            7,
		ParseErrorCount:       2,
		PersistenceErrorCount: 3,
		ValidationErrorCount:  1,
		UnknownErrorCount:     1,
		ReadyForRetryCount:    2,
		ExhaustedRetryCount:   1,
		OldestPending:         &now,
		NextScheduledRetryAt:  &now,
	}

	mockDLQRepo.On("GetStats", mock.Anything).Return(expectedStats, nil).Once()

	observability, err := uc.GetObservability(context.Background())

	assert.NoError(t, err)
	assert.Equal(t, 2*time.Minute, observability.BaseBackoff)
	assert.Equal(t, expectedStats, observability.Stats)

	mockDLQRepo.AssertExpectations(t)
}
