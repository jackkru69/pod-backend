package usecase

import (
	"context"
	"fmt"
	"time"

	"pod-backend/internal/entity"
	"pod-backend/internal/infrastructure/metrics"
	"pod-backend/internal/infrastructure/toncenter"
	"pod-backend/pkg/logger"
)

// Default retry configuration constants (used when not configured via config)
const (
	defaultMaxRetries        = 3
	defaultInitialBackoff    = 100 * time.Millisecond
	defaultMaxBackoff        = 2 * time.Second
	defaultBackoffMultiplier = 2.0
)

// RetryConfig holds retry/backoff configuration parameters.
// Can be set via SetRetryConfig or uses defaults.
type RetryConfig struct {
	MaxRetries        int
	InitialBackoff    time.Duration
	MaxBackoff        time.Duration
	BackoffMultiplier float64
}

// DefaultRetryConfig returns the default retry configuration.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:        defaultMaxRetries,
		InitialBackoff:    defaultInitialBackoff,
		MaxBackoff:        defaultMaxBackoff,
		BackoffMultiplier: defaultBackoffMultiplier,
	}
}

// BlockchainSubscriberUseCase subscribes to TON Center blockchain events and routes them
// to GamePersistenceUseCase for processing.
// Implements FR-001 (subscribe to blockchain), FR-008 (monitor game state changes),
// FR-019 (resilient polling).
// T097: Integrated with Prometheus metrics for monitoring.
// T152: Updated to use EventSource abstraction for WebSocket/HTTP flexibility.
// Issue #6: Integrated with DeadLetterQueueUseCase for failed transaction storage.
type BlockchainSubscriberUseCase struct {
	eventSource        toncenter.EventSource
	persistenceUseCase *GamePersistenceUseCase
	dlqUseCase         *DeadLetterQueueUseCase // Optional DLQ for failed transactions (Issue #6)
	logger             logger.Interface
	metrics            *metrics.BlockchainMetrics // Optional metrics (T097)
	retryConfig        RetryConfig                // Retry configuration (Issue #9)
}

// NewBlockchainSubscriberUseCase creates a new blockchain subscriber use case.
// Uses EventSource abstraction to support both WebSocket and HTTP polling (T152).
func NewBlockchainSubscriberUseCase(
	eventSource toncenter.EventSource,
	persistenceUseCase *GamePersistenceUseCase,
	logger logger.Interface,
) *BlockchainSubscriberUseCase {
	uc := &BlockchainSubscriberUseCase{
		eventSource:        eventSource,
		persistenceUseCase: persistenceUseCase,
		logger:             logger,
		metrics:            nil,                  // Set via SetMetrics
		retryConfig:        DefaultRetryConfig(), // Use defaults, can override via SetRetryConfig
	}

	// Register this use case as the event handler
	eventSource.Subscribe(uc)

	return uc
}

// NewBlockchainSubscriberUseCaseWithPoller creates a new blockchain subscriber use case
// with direct Poller (backward compatibility for existing code).
// Deprecated: Use NewBlockchainSubscriberUseCase with EventSourceFactory instead.
func NewBlockchainSubscriberUseCaseWithPoller(
	client *toncenter.Client,
	persistenceUseCase *GamePersistenceUseCase,
	logger logger.Interface,
	startBlock int64,
) *BlockchainSubscriberUseCase {
	uc := &BlockchainSubscriberUseCase{
		persistenceUseCase: persistenceUseCase,
		logger:             logger,
		metrics:            nil,                  // Set via SetMetrics
		retryConfig:        DefaultRetryConfig(), // Use defaults
	}

	// Create poller with this use case as the event handler
	poller := toncenter.NewPoller(client, uc, logger, startBlock)
	uc.eventSource = poller

	return uc
}

// SetMetrics sets the Prometheus metrics collector (T097).
// This is optional - if not set, metrics collection is disabled.
func (uc *BlockchainSubscriberUseCase) SetMetrics(m *metrics.BlockchainMetrics) {
	uc.metrics = m
}

// SetRetryConfig sets custom retry configuration (Issue #9).
// This allows runtime configuration of retry parameters without code changes.
func (uc *BlockchainSubscriberUseCase) SetRetryConfig(cfg RetryConfig) {
	// Validate and apply sensible defaults for invalid values
	if cfg.MaxRetries < 0 {
		cfg.MaxRetries = defaultMaxRetries
	}
	if cfg.InitialBackoff <= 0 {
		cfg.InitialBackoff = defaultInitialBackoff
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = defaultMaxBackoff
	}
	if cfg.BackoffMultiplier <= 1.0 {
		cfg.BackoffMultiplier = defaultBackoffMultiplier
	}
	uc.retryConfig = cfg
}

// SetDeadLetterQueue sets the DLQ use case for failed transaction storage (Issue #6).
// This is optional - if not set, failed transactions are only logged.
func (uc *BlockchainSubscriberUseCase) SetDeadLetterQueue(dlq *DeadLetterQueueUseCase) {
	uc.dlqUseCase = dlq
}

// HandleTransaction implements toncenter.EventHandler interface.
// Parses blockchain transaction into GameEvent and routes to appropriate handler.
// Includes retry logic with exponential backoff for transient failures.
// T096: Comprehensive logging (INFO for events, ERROR for persistence, WARN for validation)
// T097: Prometheus metrics for monitoring
func (uc *BlockchainSubscriberUseCase) HandleTransaction(ctx context.Context, tx toncenter.Transaction) error {
	startTime := time.Now()

	// Use Debug level for all transactions, Info only for game events
	uc.logger.Debug("Received blockchain transaction hash=%s lt=%s utime=%d",
		tx.Hash(), tx.Lt(), tx.Utime)

	// Parse transaction data into GameEvent
	event, err := uc.parseTransaction(tx)
	if err != nil {
		// Most transactions won't be game events, so this is normal - use Debug level
		uc.logger.Debug("Skipping non-game transaction hash=%s lt=%s: %v",
			tx.Hash(), tx.Lt(), err)
		// Don't record metrics for non-game transactions
		return nil // Return nil instead of error to continue processing
	}

	// Record event received (T097)
	if uc.metrics != nil {
		uc.metrics.RecordEventReceived(event.EventType)
	}

	uc.logger.Info("Parsed %s event for game_id=%d from transaction hash=%s",
		event.EventType, event.GameID, event.TransactionHash)

	// Route event to appropriate handler with retry logic
	if err := uc.routeEventWithRetry(ctx, event); err != nil {
		uc.logger.Error("Failed to persist %s event for game_id=%d after %d retries: %v",
			event.EventType, event.GameID, uc.retryConfig.MaxRetries, err)
		if uc.metrics != nil {
			uc.metrics.RecordEventFailed(event.EventType, "persistence_error")
		}

		// Store in DLQ for later retry (Issue #6)
		if uc.dlqUseCase != nil {
			if dlqErr := uc.dlqUseCase.StoreFailedTransaction(ctx, tx, err, entity.DLQErrorTypePersistence); dlqErr != nil {
				uc.logger.Error("Failed to store transaction in DLQ: tx=%s, error=%v", tx.Hash(), dlqErr)
			}
		}

		return fmt.Errorf("failed to route event: %w", err)
	}

	// Record successful processing (T097)
	duration := time.Since(startTime)
	if uc.metrics != nil {
		uc.metrics.RecordEventProcessed(event.EventType, duration)
		uc.metrics.UpdateLastProcessedBlock(event.BlockNumber)
	}

	uc.logger.Info("Successfully processed %s event for game_id=%d (tx=%s, block=%d, duration=%v)",
		event.EventType, event.GameID, event.TransactionHash, event.BlockNumber, duration)

	return nil
}

// parseTransaction converts a blockchain transaction into a GameEvent entity.
// Uses MessageParserV2 to decode BOC-encoded TON messages using tonutils-go Cell parser.
// T096: Logs WARN for validation failures.
func (uc *BlockchainSubscriberUseCase) parseTransaction(tx toncenter.Transaction) (*entity.GameEvent, error) {
	// Check if in_msg exists and is not null
	if len(tx.InMsg) == 0 || string(tx.InMsg) == "null" {
		// This is a normal blockchain transaction without game event data
		return nil, fmt.Errorf("transaction has no in_msg data (not a game event)")
	}

	// Use MessageParserV2 to decode the TON message using Cell parser
	parser := toncenter.NewMessageParserV2()
	parsedMsg, err := parser.ParseInMsg(tx.InMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to parse TON message: %w", err)
	}

	// Build event data map from parsed message for persistence
	eventData := map[string]interface{}{
		"event_type": parsedMsg.EventType,
		"game_id":    parsedMsg.GameID,
		"opcode":     parsedMsg.Opcode,
	}

	// Add event-specific fields
	if parsedMsg.PlayerOne != "" {
		eventData["player_one"] = parsedMsg.PlayerOne
	}
	if parsedMsg.PlayerTwo != "" {
		eventData["player_two"] = parsedMsg.PlayerTwo
	}
	if parsedMsg.BidValue != nil {
		// Use bet_amount key for compatibility with game_persistence.go
		eventData["bet_amount"] = parsedMsg.BidValue.String()
	}
	if parsedMsg.TotalGainings != nil {
		eventData["total_gainings"] = parsedMsg.TotalGainings.String()
	}
	if parsedMsg.Winner != "" {
		eventData["winner"] = parsedMsg.Winner
	}
	if parsedMsg.Looser != "" {
		eventData["looser"] = parsedMsg.Looser
	}
	if parsedMsg.Player != "" {
		eventData["player"] = parsedMsg.Player
	}
	if parsedMsg.CoinSide != 0 {
		eventData["coin_side"] = parsedMsg.CoinSide
	}
	if parsedMsg.Required != nil {
		eventData["required"] = parsedMsg.Required.String()
	}
	if parsedMsg.Actual != nil {
		eventData["actual"] = parsedMsg.Actual.String()
	}

	// Create GameEvent entity
	// Note: BlockNumber is set to 0 as TON uses logical time (lt) for ordering
	event := &entity.GameEvent{
		EventType:       parsedMsg.EventType,
		GameID:          parsedMsg.GameID,
		TransactionHash: tx.Hash(),
		BlockNumber:     0, // TON doesn't use block numbers in the same way
		Timestamp:       time.Unix(tx.Utime, 0),
		EventData:       eventData,
	}

	// Validate event entity (FR-011, T096)
	if err := event.Validate(); err != nil {
		uc.logger.Warn("Event validation failed for game_id=%d event_type=%s tx=%s: %v",
			parsedMsg.GameID, parsedMsg.EventType, tx.Hash(), err)
		return nil, fmt.Errorf("event validation failed: %w", err)
	}

	return event, nil
}

// routeEventWithRetry routes a parsed GameEvent with retry logic and exponential backoff.
// Retries transient failures (database connectivity, etc.) up to retryConfig.MaxRetries times.
// Uses configurable backoff parameters from retryConfig (Issue #9).
func (uc *BlockchainSubscriberUseCase) routeEventWithRetry(ctx context.Context, event *entity.GameEvent) error {
	var lastErr error
	backoff := uc.retryConfig.InitialBackoff

	for attempt := 0; attempt <= uc.retryConfig.MaxRetries; attempt++ {
		if attempt > 0 {
			uc.logger.Warn("Retrying %s event for game_id=%d (attempt %d/%d, backoff=%v)",
				event.EventType, event.GameID, attempt, uc.retryConfig.MaxRetries, backoff)

			// Wait with backoff before retry
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}

			// Increase backoff for next retry (exponential with cap)
			backoff = time.Duration(float64(backoff) * uc.retryConfig.BackoffMultiplier)
			if backoff > uc.retryConfig.MaxBackoff {
				backoff = uc.retryConfig.MaxBackoff
			}
		}

		err := uc.routeEvent(ctx, event)
		if err == nil {
			if attempt > 0 {
				uc.logger.Info("Successfully processed %s event for game_id=%d after %d retries",
					event.EventType, event.GameID, attempt)
			}
			return nil
		}
		lastErr = err

		// Check if context is cancelled
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Log retry attempt
		if attempt < uc.retryConfig.MaxRetries {
			uc.logger.Warn("Attempt %d failed for %s event game_id=%d: %v",
				attempt+1, event.EventType, event.GameID, err)
		}
	}

	return fmt.Errorf("all %d retry attempts failed: %w", uc.retryConfig.MaxRetries, lastErr)
}

// routeEvent routes a parsed GameEvent to the appropriate handler in GamePersistenceUseCase.
// Supports all game event types defined in entity package.
// T096: Logs INFO for routing, ERROR for unknown event types.
func (uc *BlockchainSubscriberUseCase) routeEvent(ctx context.Context, event *entity.GameEvent) error {
	uc.logger.Debug("Routing %s event for game_id=%d to persistence handler",
		event.EventType, event.GameID)

	var err error
	switch event.EventType {
	case entity.EventTypeGameInitialized:
		err = uc.persistenceUseCase.HandleGameInitialized(ctx, event)

	case entity.EventTypeGameStarted:
		err = uc.persistenceUseCase.HandleGameStarted(ctx, event)

	case entity.EventTypeGameFinished:
		err = uc.persistenceUseCase.HandleGameFinished(ctx, event)

	case entity.EventTypeGameCancelled:
		err = uc.persistenceUseCase.HandleGameCancelled(ctx, event)

	case entity.EventTypeDraw:
		err = uc.persistenceUseCase.HandleDraw(ctx, event)

	case entity.EventTypeSecretOpened:
		err = uc.persistenceUseCase.HandleSecretOpened(ctx, event)

	case entity.EventTypeInsufficientBalance:
		err = uc.persistenceUseCase.HandleInsufficientBalance(ctx, event)

	default:
		uc.logger.Error("Unknown event type: %s for game_id=%d tx=%s",
			event.EventType, event.GameID, event.TransactionHash)
		return fmt.Errorf("unknown event type: %s", event.EventType)
	}

	return err
}

// Subscribe starts the blockchain event subscription.
// Runs asynchronously until context is cancelled or Stop is called.
// T096: Logs INFO for lifecycle events.
// T152: Updated to use EventSource abstraction (supports WebSocket and HTTP).
func (uc *BlockchainSubscriberUseCase) Subscribe(ctx context.Context) {
	lastLt := uc.eventSource.GetLastProcessedLt()
	sourceType := uc.eventSource.GetSourceType()
	uc.logger.Info("Starting blockchain subscription via %s from lt=%s", sourceType, lastLt)
	uc.eventSource.Start(ctx)
}

// Stop gracefully stops the blockchain event subscription.
// T096: Logs INFO for lifecycle events.
// T152: Updated to use EventSource abstraction.
func (uc *BlockchainSubscriberUseCase) Stop() {
	lastLt := uc.eventSource.GetLastProcessedLt()
	sourceType := uc.eventSource.GetSourceType()
	uc.logger.Info("Stopping blockchain subscription via %s at lt=%s", sourceType, lastLt)
	uc.eventSource.Stop()
}

// GetLastProcessedLt returns the last successfully processed logical time (lt).
// Useful for tracking progress and resuming after restart.
// T152: Updated from block-based to lt-based tracking for TON compatibility.
func (uc *BlockchainSubscriberUseCase) GetLastProcessedLt() string {
	return uc.eventSource.GetLastProcessedLt()
}

// SetLastProcessedLt updates the starting logical time for event processing.
// Useful for resuming from database state after restart.
// T096: Logs INFO when resuming from saved state.
// T152: Updated from block-based to lt-based tracking for TON compatibility.
func (uc *BlockchainSubscriberUseCase) SetLastProcessedLt(lt string) {
	uc.logger.Info("Resuming blockchain subscription from lt=%s", lt)
	uc.eventSource.SetLastProcessedLt(lt)
}

// GetSourceType returns the type of event source being used ("websocket" or "http").
// T152: Added for runtime monitoring and logging.
func (uc *BlockchainSubscriberUseCase) GetSourceType() string {
	return uc.eventSource.GetSourceType()
}

// IsConnected returns whether the event source is actively receiving events.
// T152: Added for health check and monitoring.
func (uc *BlockchainSubscriberUseCase) IsConnected() bool {
	return uc.eventSource.IsConnected()
}

// GetLastProcessedBlock returns the last successfully processed block number.
// Deprecated: Use GetLastProcessedLt() instead. TON uses logical time (lt) for ordering.
// This method is kept for backward compatibility.
func (uc *BlockchainSubscriberUseCase) GetLastProcessedBlock() int64 {
	// For backward compatibility, return 0 as TON doesn't use block numbers
	return 0
}

// SetLastProcessedBlock updates the starting block for polling.
// Deprecated: Use SetLastProcessedLt() instead. TON uses logical time (lt) for ordering.
// This method is kept for backward compatibility.
func (uc *BlockchainSubscriberUseCase) SetLastProcessedBlock(block int64) {
	uc.logger.Warn("SetLastProcessedBlock is deprecated, use SetLastProcessedLt instead")
	// No-op for backward compatibility
}
