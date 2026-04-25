package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
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
	parserMetricsLabel       = "parser"
	dlqMetricsLabel          = "dlq"
)

var (
	errTransactionHasNoInMsg      = errors.New("transaction has no in_msg data")
	errTransactionHasNoGameEvents = errors.New("transaction has no parsable game event messages")
	errUnknownEventType           = errors.New("unknown event type")
)

type transactionMessageCandidate struct {
	label   string
	payload json.RawMessage
}

// RetryConfig holds retry/backoff configuration parameters.
// Can be set via SetRetryConfig or uses defaults.
type RetryConfig struct {
	MaxRetries        int
	InitialBackoff    time.Duration
	MaxBackoff        time.Duration
	BackoffMultiplier float64
}

// BlockchainSubscriberObservability captures parser failures, retry posture, and DLQ outcomes.
type BlockchainSubscriberObservability struct {
	ParseFailuresTotal             uint64              `json:"parse_failures_total"`
	ValidationFailuresTotal        uint64              `json:"validation_failures_total"`
	PersistenceRetryAttemptsTotal  uint64              `json:"persistence_retry_attempts_total"`
	PersistenceRetryRecoveredTotal uint64              `json:"persistence_retry_recovered_total"`
	PersistenceFailuresExhausted   uint64              `json:"persistence_failures_exhausted_total"`
	DLQStoreAttemptsTotal          uint64              `json:"dlq_store_attempts_total"`
	DLQStoredTotal                 uint64              `json:"dlq_stored_total"`
	DLQDuplicateTotal              uint64              `json:"dlq_duplicate_total"`
	DLQStoreFailuresTotal          uint64              `json:"dlq_store_failures_total"`
	LastFailureType                entity.DLQErrorType `json:"last_failure_type,omitempty"`
	LastFailureError               string              `json:"last_failure_error,omitempty"`
	LastFailureTxHash              string              `json:"last_failure_tx_hash,omitempty"`
	LastFailureTxLt                string              `json:"last_failure_tx_lt,omitempty"`
	LastFailureAt                  *time.Time          `json:"last_failure_at,omitempty"`
	LastRetryOutcome               string              `json:"last_retry_outcome,omitempty"`
	LastRetriedEventType           string              `json:"last_retried_event_type,omitempty"`
	LastRetriedGameID              int64               `json:"last_retried_game_id,omitempty"`
	LastRetryAttemptAt             *time.Time          `json:"last_retry_attempt_at,omitempty"`
	LastExhaustedRetryAt           *time.Time          `json:"last_exhausted_retry_at,omitempty"`
	LastDLQErrorType               entity.DLQErrorType `json:"last_dlq_error_type,omitempty"`
	LastDLQStoreOutcome            DLQStoreOutcome     `json:"last_dlq_store_outcome,omitempty"`
	LastDLQStatus                  entity.DLQStatus    `json:"last_dlq_status,omitempty"`
	LastDLQResolutionNotes         string              `json:"last_dlq_resolution_notes,omitempty"`
	LastDLQStoredTxHash            string              `json:"last_dlq_stored_tx_hash,omitempty"`
	LastDLQStoredTxLt              string              `json:"last_dlq_stored_tx_lt,omitempty"`
	LastDLQStoredAt                *time.Time          `json:"last_dlq_stored_at,omitempty"`
	LastDLQStoreFailure            string              `json:"last_dlq_store_failure,omitempty"`
	LastDLQStoreFailureTxHash      string              `json:"last_dlq_store_failure_tx_hash,omitempty"`
	LastDLQStoreFailureTxLt        string              `json:"last_dlq_store_failure_tx_lt,omitempty"`
	LastDLQStoreFailureAt          *time.Time          `json:"last_dlq_store_failure_at,omitempty"`
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
	parser             toncenter.InMsgParser
	persistenceUseCase *GamePersistenceUseCase
	dlqUseCase         *DeadLetterQueueUseCase // Optional DLQ for failed transactions (Issue #6)
	logger             logger.Interface
	metrics            *metrics.BlockchainMetrics // Optional metrics (T097)
	retryConfig        RetryConfig                // Retry configuration (Issue #9)
	observabilityMu    sync.RWMutex
	observability      BlockchainSubscriberObservability
}

// NewBlockchainSubscriberUseCase creates a new blockchain subscriber use case.
// Uses EventSource abstraction to support both WebSocket and HTTP polling (T152).
func NewBlockchainSubscriberUseCase(
	eventSource toncenter.EventSource,
	persistenceUseCase *GamePersistenceUseCase,
	log logger.Interface,
) *BlockchainSubscriberUseCase {
	uc := &BlockchainSubscriberUseCase{
		eventSource:        eventSource,
		parser:             toncenter.NewRuntimeMessageParser(),
		persistenceUseCase: persistenceUseCase,
		logger:             log,
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
	log logger.Interface,
	startBlock int64,
) *BlockchainSubscriberUseCase {
	uc := &BlockchainSubscriberUseCase{
		parser:             toncenter.NewRuntimeMessageParser(),
		persistenceUseCase: persistenceUseCase,
		logger:             log,
		metrics:            nil,                  // Set via SetMetrics
		retryConfig:        DefaultRetryConfig(), // Use defaults
	}

	// Create poller with this use case as the event handler
	poller := toncenter.NewPoller(client, uc, log, startBlock)
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
		if errorType, observable := classifyParseError(err); observable {
			uc.recordParseFailure(&tx, err, errorType)
			if uc.metrics != nil {
				uc.metrics.RecordEventFailed(parserMetricsLabel, string(errorType))
			}
			uc.logger.Warn("Failed to parse blockchain transaction hash=%s lt=%s: %v",
				tx.Hash(), tx.Lt(), err)
			uc.storeFailedTransaction(ctx, &tx, err, errorType)
			return nil
		}

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

		uc.storeFailedTransaction(ctx, &tx, err, entity.DLQErrorTypePersistence)

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
// Uses the runtime TON message parser so all production decoding goes through a
// single authoritative parser path.
// T096: Logs WARN for validation failures.
func (uc *BlockchainSubscriberUseCase) parseTransaction(tx toncenter.Transaction) (*entity.GameEvent, error) {
	parser := uc.parser
	if parser == nil {
		parser = toncenter.NewRuntimeMessageParser()
	}

	parsedMsg, err := parseTransactionMessage(parser, tx)
	if err != nil {
		return nil, err
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
	if parsedMsg.ServiceFeeNumerator != 0 {
		eventData["service_fee_numerator"] = parsedMsg.ServiceFeeNumerator
	}
	if parsedMsg.ReferrerFeeNumerator != 0 {
		eventData["referrer_fee_numerator"] = parsedMsg.ReferrerFeeNumerator
	}
	if parsedMsg.WaitingForOpenBidSeconds != 0 {
		eventData["waiting_for_open_bid_seconds"] = parsedMsg.WaitingForOpenBidSeconds
	}
	if parsedMsg.LowestBid != nil {
		eventData["lowest_bid"] = parsedMsg.LowestBid.String()
	}
	if parsedMsg.HighestBid != nil {
		eventData["highest_bid"] = parsedMsg.HighestBid.String()
	}
	if parsedMsg.MinReferrerPayoutValue != nil {
		eventData["min_referrer_payout_value"] = parsedMsg.MinReferrerPayoutValue.String()
	}
	if parsedMsg.FeeReceiver != "" {
		eventData["fee_receiver"] = parsedMsg.FeeReceiver
	}
	if parsedMsg.ProtocolVersion != 0 {
		eventData["protocol_version"] = parsedMsg.ProtocolVersion
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

func parseTransactionMessage(parser toncenter.InMsgParser, tx toncenter.Transaction) (*toncenter.ParsedMessage, error) {
	candidates, err := collectTransactionMessages(tx)
	if err != nil {
		return nil, fmt.Errorf("failed to parse TON message envelope: %w", err)
	}
	if len(candidates) == 0 {
		return nil, errTransactionHasNoGameEvents
	}

	var firstParseErr error
	for _, candidate := range candidates {
		parsedMsg, err := parser.ParseInMsg(candidate.payload)
		if err == nil {
			return parsedMsg, nil
		}

		if isIgnorableMessageParseError(err) {
			continue
		}

		if firstParseErr == nil {
			firstParseErr = fmt.Errorf("%s: %w", candidate.label, err)
		}
	}

	if firstParseErr != nil {
		return nil, fmt.Errorf("failed to parse TON message: %w", firstParseErr)
	}

	return nil, errTransactionHasNoGameEvents
}

func collectTransactionMessages(tx toncenter.Transaction) ([]transactionMessageCandidate, error) {
	var candidates []transactionMessageCandidate

	if hasTransactionMessage(tx.InMsg) {
		candidates = append(candidates, transactionMessageCandidate{
			label:   "in_msg",
			payload: tx.InMsg,
		})
	}

	if hasTransactionMessage(tx.OutMsgs) {
		var outMsgs []json.RawMessage
		if err := json.Unmarshal(tx.OutMsgs, &outMsgs); err != nil {
			return nil, fmt.Errorf("decode out_msgs: %w", err)
		}

		for index, outMsg := range outMsgs {
			if !hasTransactionMessage(outMsg) {
				continue
			}

			candidates = append(candidates, transactionMessageCandidate{
				label:   fmt.Sprintf("out_msgs[%d]", index),
				payload: outMsg,
			})
		}
	}

	return candidates, nil
}

func hasTransactionMessage(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && trimmed != "null"
}

func isIgnorableMessageParseError(err error) bool {
	if err == nil {
		return false
	}

	message := err.Error()
	return strings.Contains(message, "unknown opcode") ||
		strings.Contains(message, "no message body found in in_msg") ||
		strings.Contains(message, "empty in_msg") ||
		strings.Contains(message, "message too short")
}

// routeEventWithRetry routes a parsed GameEvent with retry logic and exponential backoff.
// Retries transient failures (database connectivity, etc.) up to retryConfig.MaxRetries times.
// Uses configurable backoff parameters from retryConfig (Issue #9).
func (uc *BlockchainSubscriberUseCase) routeEventWithRetry(ctx context.Context, event *entity.GameEvent) error {
	var lastErr error
	backoff := uc.retryConfig.InitialBackoff

	for attempt := 0; attempt <= uc.retryConfig.MaxRetries; attempt++ {
		if attempt > 0 {
			nextBackoff, err := uc.waitForRetry(ctx, event, lastErr, attempt, backoff)
			if err != nil {
				return err
			}

			backoff = nextBackoff
		}

		err := uc.routeEvent(ctx, event)
		if err == nil {
			if attempt > 0 {
				uc.logger.Info("Successfully processed %s event for game_id=%d after %d retries",
					event.EventType, event.GameID, attempt)
				uc.recordRetryRecovered(event)
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

	uc.recordRetryExhausted(event, lastErr)
	if uc.metrics != nil {
		uc.metrics.RecordEventFailed(event.EventType, "retry_exhausted")
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
		return fmt.Errorf("%w: %s", errUnknownEventType, event.EventType)
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
func (uc *BlockchainSubscriberUseCase) SetLastProcessedBlock(_ int64) {
	uc.logger.Warn("SetLastProcessedBlock is deprecated, use SetLastProcessedLt instead")
	// No-op for backward compatibility
}

// GetObservabilitySnapshot returns parser, retry, and DLQ handling state for monitoring.
func (uc *BlockchainSubscriberUseCase) GetObservabilitySnapshot() BlockchainSubscriberObservability {
	uc.observabilityMu.RLock()
	defer uc.observabilityMu.RUnlock()

	snapshot := uc.observability
	if snapshot.LastFailureAt != nil {
		lastFailureAt := *snapshot.LastFailureAt
		snapshot.LastFailureAt = &lastFailureAt
	}
	if snapshot.LastRetryAttemptAt != nil {
		lastRetryAttemptAt := *snapshot.LastRetryAttemptAt
		snapshot.LastRetryAttemptAt = &lastRetryAttemptAt
	}
	if snapshot.LastExhaustedRetryAt != nil {
		lastExhaustedRetryAt := *snapshot.LastExhaustedRetryAt
		snapshot.LastExhaustedRetryAt = &lastExhaustedRetryAt
	}
	if snapshot.LastDLQStoredAt != nil {
		lastDLQStoredAt := *snapshot.LastDLQStoredAt
		snapshot.LastDLQStoredAt = &lastDLQStoredAt
	}
	if snapshot.LastDLQStoreFailureAt != nil {
		lastDLQStoreFailureAt := *snapshot.LastDLQStoreFailureAt
		snapshot.LastDLQStoreFailureAt = &lastDLQStoreFailureAt
	}

	return snapshot
}

func classifyParseError(err error) (entity.DLQErrorType, bool) {
	if err == nil {
		return "", false
	}

	message := err.Error()
	switch {
	case errors.Is(err, errTransactionHasNoInMsg), strings.Contains(message, errTransactionHasNoInMsg.Error()):
		return "", false
	case errors.Is(err, errTransactionHasNoGameEvents), strings.Contains(message, errTransactionHasNoGameEvents.Error()):
		return "", false
	case strings.Contains(message, "unknown opcode"):
		return "", false
	case strings.Contains(message, "event validation failed"):
		return entity.DLQErrorTypeValidation, true
	default:
		return entity.DLQErrorTypeParse, true
	}
}

func (uc *BlockchainSubscriberUseCase) storeFailedTransaction(
	ctx context.Context,
	tx *toncenter.Transaction,
	err error,
	errorType entity.DLQErrorType,
) {
	if uc.dlqUseCase == nil {
		return
	}

	uc.recordDLQStoreAttempt(errorType)

	result, dlqErr := uc.dlqUseCase.StoreFailedTransaction(ctx, *tx, err, errorType)
	if dlqErr != nil {
		if uc.metrics != nil {
			uc.metrics.RecordEventFailed(dlqMetricsLabel, "store_error")
		}
		uc.recordDLQStoreFailure(tx, dlqErr)
		uc.logger.Error("Failed to store transaction in DLQ: tx=%s, error=%v", tx.Hash(), dlqErr)
		return
	}

	switch {
	case result == nil:
		uc.recordDLQStored(tx, errorType, nil)
	case result.Outcome == DLQStoreOutcomeAlreadyPending:
		if uc.metrics != nil {
			uc.metrics.RecordEventFailed(dlqMetricsLabel, "duplicate")
		}
		uc.recordDLQDuplicate(tx, errorType, result)
	default:
		uc.recordDLQStored(tx, errorType, result)
	}
}

func (uc *BlockchainSubscriberUseCase) recordParseFailure(
	tx *toncenter.Transaction,
	err error,
	errorType entity.DLQErrorType,
) {
	now := time.Now()

	uc.observabilityMu.Lock()
	defer uc.observabilityMu.Unlock()

	switch errorType {
	case entity.DLQErrorTypeParse, entity.DLQErrorTypePersistence, entity.DLQErrorTypeUnknown:
		uc.observability.ParseFailuresTotal++
	case entity.DLQErrorTypeValidation:
		uc.observability.ValidationFailuresTotal++
	}

	uc.observability.LastFailureType = errorType
	uc.observability.LastFailureError = err.Error()
	uc.observability.LastFailureTxHash = tx.Hash()
	uc.observability.LastFailureTxLt = tx.Lt()
	uc.observability.LastFailureAt = &now
}

func (uc *BlockchainSubscriberUseCase) recordRetryAttempt(event *entity.GameEvent, err error) {
	now := time.Now()

	uc.observabilityMu.Lock()
	defer uc.observabilityMu.Unlock()

	uc.observability.PersistenceRetryAttemptsTotal++
	uc.observability.LastFailureType = entity.DLQErrorTypePersistence
	uc.observability.LastFailureError = err.Error()
	uc.observability.LastRetryOutcome = "retrying"
	uc.observability.LastRetriedEventType = event.EventType
	uc.observability.LastRetriedGameID = event.GameID
	uc.observability.LastRetryAttemptAt = &now
}

func (uc *BlockchainSubscriberUseCase) recordRetryRecovered(event *entity.GameEvent) {
	uc.observabilityMu.Lock()
	defer uc.observabilityMu.Unlock()

	uc.observability.PersistenceRetryRecoveredTotal++
	uc.observability.LastRetryOutcome = "recovered"
	uc.observability.LastRetriedEventType = event.EventType
	uc.observability.LastRetriedGameID = event.GameID
}

func (uc *BlockchainSubscriberUseCase) recordRetryExhausted(event *entity.GameEvent, err error) {
	now := time.Now()

	uc.observabilityMu.Lock()
	defer uc.observabilityMu.Unlock()

	uc.observability.PersistenceFailuresExhausted++
	uc.observability.LastFailureType = entity.DLQErrorTypePersistence
	uc.observability.LastFailureError = err.Error()
	uc.observability.LastRetryOutcome = "exhausted"
	uc.observability.LastRetriedEventType = event.EventType
	uc.observability.LastRetriedGameID = event.GameID
	uc.observability.LastExhaustedRetryAt = &now
}

func (uc *BlockchainSubscriberUseCase) recordDLQStoreAttempt(errorType entity.DLQErrorType) {
	uc.observabilityMu.Lock()
	defer uc.observabilityMu.Unlock()

	uc.observability.DLQStoreAttemptsTotal++
	uc.observability.LastDLQErrorType = errorType
	uc.observability.LastDLQStoreOutcome = ""
}

func (uc *BlockchainSubscriberUseCase) recordDLQStored(
	tx *toncenter.Transaction,
	errorType entity.DLQErrorType,
	result *StoreFailedTransactionResult,
) {
	uc.observabilityMu.Lock()
	defer uc.observabilityMu.Unlock()

	uc.observability.DLQStoredTotal++
	uc.applyDLQStoreResult(tx, errorType, DLQStoreOutcomeQueued, result)
}

func (uc *BlockchainSubscriberUseCase) recordDLQDuplicate(
	tx *toncenter.Transaction,
	errorType entity.DLQErrorType,
	result *StoreFailedTransactionResult,
) {
	uc.observabilityMu.Lock()
	defer uc.observabilityMu.Unlock()

	uc.observability.DLQDuplicateTotal++
	uc.applyDLQStoreResult(tx, errorType, DLQStoreOutcomeAlreadyPending, result)
}

func (uc *BlockchainSubscriberUseCase) recordDLQStoreFailure(tx *toncenter.Transaction, err error) {
	now := time.Now()

	uc.observabilityMu.Lock()
	defer uc.observabilityMu.Unlock()

	uc.observability.DLQStoreFailuresTotal++
	uc.observability.LastDLQStoreFailure = err.Error()
	uc.observability.LastDLQStoreFailureTxHash = tx.Hash()
	uc.observability.LastDLQStoreFailureTxLt = tx.Lt()
	uc.observability.LastDLQStoreFailureAt = &now
}

func (uc *BlockchainSubscriberUseCase) applyDLQStoreResult(
	tx *toncenter.Transaction,
	errorType entity.DLQErrorType,
	outcome DLQStoreOutcome,
	result *StoreFailedTransactionResult,
) {
	now := time.Now()

	uc.observability.LastDLQErrorType = errorType
	uc.observability.LastDLQStoreOutcome = outcome
	uc.observability.LastDLQStatus = entity.DLQStatusPending
	uc.observability.LastDLQResolutionNotes = ""
	uc.observability.LastDLQStoredTxHash = tx.Hash()
	uc.observability.LastDLQStoredTxLt = tx.Lt()
	uc.observability.LastDLQStoredAt = &now
	if result != nil {
		uc.observability.LastDLQStatus = result.Status
		uc.observability.LastDLQResolutionNotes = result.ResolutionNotes
	}
}

func (uc *BlockchainSubscriberUseCase) waitForRetry(
	ctx context.Context,
	event *entity.GameEvent,
	lastErr error,
	attempt int,
	backoff time.Duration,
) (time.Duration, error) {
	uc.recordRetryAttempt(event, lastErr)

	if uc.metrics != nil {
		uc.metrics.RecordEventFailed(event.EventType, "retry")
	}

	uc.logger.Warn("Retrying %s event for game_id=%d (attempt %d/%d, backoff=%v)",
		event.EventType, event.GameID, attempt, uc.retryConfig.MaxRetries, backoff)

	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-time.After(backoff):
	}

	nextBackoff := time.Duration(float64(backoff) * uc.retryConfig.BackoffMultiplier)
	if nextBackoff > uc.retryConfig.MaxBackoff {
		nextBackoff = uc.retryConfig.MaxBackoff
	}

	return nextBackoff, nil
}
