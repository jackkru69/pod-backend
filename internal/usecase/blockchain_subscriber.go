package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"pod-backend/internal/entity"
	"pod-backend/internal/infrastructure/metrics"
	"pod-backend/internal/infrastructure/toncenter"
	"pod-backend/pkg/logger"
)

// BlockchainSubscriberUseCase subscribes to TON Center blockchain events and routes them
// to GamePersistenceUseCase for processing.
// Implements FR-001 (subscribe to blockchain), FR-008 (monitor game state changes),
// FR-019 (resilient polling).
// T097: Integrated with Prometheus metrics for monitoring.
type BlockchainSubscriberUseCase struct {
	poller             *toncenter.Poller
	persistenceUseCase *GamePersistenceUseCase
	logger             logger.Interface
	metrics            *metrics.BlockchainMetrics // Optional metrics (T097)
}

// NewBlockchainSubscriberUseCase creates a new blockchain subscriber use case.
func NewBlockchainSubscriberUseCase(
	client *toncenter.Client,
	persistenceUseCase *GamePersistenceUseCase,
	logger logger.Interface,
	startBlock int64,
) *BlockchainSubscriberUseCase {
	uc := &BlockchainSubscriberUseCase{
		persistenceUseCase: persistenceUseCase,
		logger:             logger,
		metrics:            nil, // Set via SetMetrics
	}

	// Create poller with this use case as the event handler
	uc.poller = toncenter.NewPoller(client, uc, logger, startBlock)

	return uc
}

// SetMetrics sets the Prometheus metrics collector (T097).
// This is optional - if not set, metrics collection is disabled.
func (uc *BlockchainSubscriberUseCase) SetMetrics(m *metrics.BlockchainMetrics) {
	uc.metrics = m
}

// HandleTransaction implements toncenter.EventHandler interface.
// Parses blockchain transaction into GameEvent and routes to appropriate handler.
// T096: Comprehensive logging (INFO for events, ERROR for persistence, WARN for validation)
// T097: Prometheus metrics for monitoring
func (uc *BlockchainSubscriberUseCase) HandleTransaction(ctx context.Context, tx toncenter.Transaction) error {
	startTime := time.Now()

	uc.logger.Info("Received blockchain transaction hash=%s block=%d lt=%s",
		tx.Hash, tx.BlockNumber, tx.Lt)

	// Parse transaction data into GameEvent
	event, err := uc.parseTransaction(tx)
	if err != nil {
		uc.logger.Warn("Failed to parse transaction hash=%s block=%d: %v",
			tx.Hash, tx.BlockNumber, err)
		if uc.metrics != nil {
			uc.metrics.RecordEventFailed("unknown", "parse_error")
		}
		return fmt.Errorf("failed to parse transaction: %w", err)
	}

	// Record event received (T097)
	if uc.metrics != nil {
		uc.metrics.RecordEventReceived(event.EventType)
	}

	uc.logger.Info("Parsed %s event for game_id=%d from transaction hash=%s",
		event.EventType, event.GameID, event.TransactionHash)

	// Route event to appropriate handler based on event type
	if err := uc.routeEvent(ctx, event); err != nil {
		uc.logger.Error("Failed to persist %s event for game_id=%d: %v",
			event.EventType, event.GameID, err)
		if uc.metrics != nil {
			uc.metrics.RecordEventFailed(event.EventType, "persistence_error")
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
// Extracts event type, game ID, and event-specific data from transaction payload.
// T096: Logs WARN for validation failures.
func (uc *BlockchainSubscriberUseCase) parseTransaction(tx toncenter.Transaction) (*entity.GameEvent, error) {
	// Parse transaction data as JSON
	var txData map[string]interface{}
	if err := json.Unmarshal(tx.Data, &txData); err != nil {
		uc.logger.Warn("Failed to unmarshal transaction data hash=%s: %v", tx.Hash, err)
		return nil, fmt.Errorf("failed to unmarshal transaction data: %w", err)
	}

	// Extract event type
	eventType, ok := txData["event_type"].(string)
	if !ok {
		uc.logger.Warn("Missing or invalid event_type in transaction hash=%s", tx.Hash)
		return nil, fmt.Errorf("missing or invalid event_type in transaction data")
	}

	// Extract game ID
	gameIDFloat, ok := txData["game_id"].(float64)
	if !ok {
		uc.logger.Warn("Missing or invalid game_id in transaction hash=%s event_type=%s",
			tx.Hash, eventType)
		return nil, fmt.Errorf("missing or invalid game_id in transaction data")
	}
	gameID := int64(gameIDFloat)

	// Create GameEvent entity
	event := &entity.GameEvent{
		EventType:       eventType,
		GameID:          gameID,
		TransactionHash: tx.Hash,
		BlockNumber:     tx.BlockNumber,
		Timestamp:       time.Unix(tx.Timestamp, 0),
		EventData:       txData,
	}

	// Validate event entity (FR-011, T096)
	if err := event.Validate(); err != nil {
		uc.logger.Warn("Event validation failed for game_id=%d event_type=%s tx=%s: %v",
			gameID, eventType, tx.Hash, err)
		return nil, fmt.Errorf("event validation failed: %w", err)
	}

	return event, nil
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

// Subscribe starts the blockchain polling loop.
// Runs asynchronously until context is cancelled or Stop is called.
// T096: Logs INFO for lifecycle events.
func (uc *BlockchainSubscriberUseCase) Subscribe(ctx context.Context) {
	lastBlock := uc.poller.GetLastProcessedBlock()
	uc.logger.Info("Starting blockchain subscription from block %d", lastBlock)
	uc.poller.Start(ctx)
}

// Stop gracefully stops the blockchain polling loop.
// T096: Logs INFO for lifecycle events.
func (uc *BlockchainSubscriberUseCase) Stop() {
	lastBlock := uc.poller.GetLastProcessedBlock()
	uc.logger.Info("Stopping blockchain subscription at block %d", lastBlock)
	uc.poller.Stop()
}

// GetLastProcessedBlock returns the last successfully processed block number.
// Useful for tracking progress and resuming after restart.
func (uc *BlockchainSubscriberUseCase) GetLastProcessedBlock() int64 {
	return uc.poller.GetLastProcessedBlock()
}

// SetLastProcessedBlock updates the starting block for polling.
// Useful for resuming from database state after restart.
// T096: Logs INFO when resuming from saved state.
func (uc *BlockchainSubscriberUseCase) SetLastProcessedBlock(block int64) {
	uc.logger.Info("Resuming blockchain subscription from block %d", block)
	uc.poller.SetLastProcessedBlock(block)
}
