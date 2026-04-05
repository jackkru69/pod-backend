package blockchain

import (
	"context"
	"strings"
	"time"

	"pod-backend/config"
	"pod-backend/internal/entity"
	"pod-backend/internal/infrastructure/metrics"
	"pod-backend/internal/infrastructure/toncenter"
	"pod-backend/internal/usecase"
	"pod-backend/pkg/logger"
)

// TONEventHandler manages the blockchain event subscription lifecycle.
// Implements T094: Start blockchain subscription on service boot.
// T154: Updated to use EventSourceFactory for WebSocket/HTTP flexibility.
type TONEventHandler struct {
	subscriberUC *usecase.BlockchainSubscriberUseCase
	factory      *toncenter.EventSourceFactory
	logger       logger.Interface
	ctx          context.Context
	cancel       context.CancelFunc
	onLtUpdated  func(lt string) // Callback to persist lt to database
	onCheckpoint func(SyncCheckpoint)
	onFallback   func(SyncCheckpoint)
}

// SyncCheckpoint is the resumable blockchain ingestion snapshot exposed by the handler.
type SyncCheckpoint struct {
	LastProcessedLt      string
	EventSourceType      string
	EventSourceConnected bool
}

// NewTONEventHandler creates a new blockchain event handler.
// Initializes EventSourceFactory, persistence use case, and blockchain subscriber.
// T154: Updated to use EventSourceFactory for WebSocket/HTTP event source selection.
func NewTONEventHandler(
	cfg *config.Config,
	persistenceUC *usecase.GamePersistenceUseCase,
	logger logger.Interface,
	checkpoint *entity.BlockchainSyncState,
) (*TONEventHandler, error) {
	// Parse timeout duration (default to 60s if parse fails)
	circuitBreakerTimeout, err := time.ParseDuration(cfg.GameBackend.CircuitBreakerTimeout)
	if err != nil {
		circuitBreakerTimeout = 60 * time.Second
		logger.Warn("Failed to parse circuit breaker timeout, using default 60s: %v", err)
	}

	// Parse WebSocket ping interval
	wsPingInterval, err := time.ParseDuration(cfg.GameBackend.WebSocketPingInterval)
	if err != nil {
		wsPingInterval = 30 * time.Second
		logger.Warn("Failed to parse WebSocket ping interval, using default 30s: %v", err)
	}

	// Parse polling intervals
	minPollInterval, err := time.ParseDuration(cfg.GameBackend.MinPollInterval)
	if err != nil {
		minPollInterval = 5 * time.Second
		logger.Warn("Failed to parse min poll interval, using default 5s: %v", err)
	}

	maxPollInterval, err := time.ParseDuration(cfg.GameBackend.MaxPollInterval)
	if err != nil {
		maxPollInterval = 30 * time.Second
		logger.Warn("Failed to parse max poll interval, using default 30s: %v", err)
	}

	handler := &TONEventHandler{
		logger: logger,
	}

	resolvedSourceType := resolveInitialEventSourceType(cfg, checkpoint)

	// Create EventSourceFactory with WebSocket/HTTP configuration (T154)
	factory := toncenter.NewEventSourceFactory(toncenter.FactoryConfig{
		// HTTP Polling Configuration
		V2BaseURL:             cfg.GameBackend.TONCenterV2URL,
		ContractAddress:       cfg.GameBackend.TONGameContractAddr,
		CircuitBreakerMaxFail: cfg.GameBackend.CircuitBreakerMaxFail,
		CircuitBreakerTimeout: circuitBreakerTimeout,
		HTTPTimeout:           30 * time.Second,
		MinPollInterval:       minPollInterval,
		MaxPollInterval:       maxPollInterval,

		// WebSocket Configuration (T154)
		V3WSURL:         cfg.GameBackend.TONCenterV3WSURL,
		EnableWebSocket: cfg.GameBackend.EnableWebSocket,
		EventSourceType: resolvedSourceType,
		MaxReconnect:    cfg.GameBackend.WebSocketReconnectMax,
		PingInterval:    wsPingInterval,

		// Fallback callback
		OnFallback: func() {
			logger.Warn("WebSocket event source failed, degraded to HTTP polling")
			handler.emitFallback()
		},
	}, logger)

	// Create a temporary handler to pass to factory
	// The actual handler will be set in the subscriber use case
	tempHandler := &tempEventHandler{}

	// Create event source based on configuration
	eventSource, err := factory.CreateEventSource(tempHandler)
	if err != nil {
		return nil, err
	}

	logger.Info("Event source created: type=%s, contract=%s",
		eventSource.GetSourceType(), cfg.GameBackend.TONGameContractAddr)

	// Create blockchain subscriber use case with EventSource (T152)
	subscriberUC := usecase.NewBlockchainSubscriberUseCase(
		eventSource,
		persistenceUC,
		logger,
	)

	// Create context for subscription lifecycle
	ctx, cancel := context.WithCancel(context.Background())

	handler.subscriberUC = subscriberUC
	handler.factory = factory
	handler.ctx = ctx
	handler.cancel = cancel

	if shouldResumeCheckpoint(cfg, checkpoint) {
		handler.SetLastProcessedLt(checkpoint.LastProcessedLt)
	}

	return handler, nil
}

// tempEventHandler is a placeholder that gets replaced by BlockchainSubscriberUseCase.
// This is needed because we need to create the event source before the use case.
type tempEventHandler struct{}

func (h *tempEventHandler) HandleTransaction(ctx context.Context, tx toncenter.Transaction) error {
	// This will never be called - the real handler is registered in NewBlockchainSubscriberUseCase
	return nil
}

// Start begins the blockchain event subscription.
// Runs asynchronously in a separate goroutine (T094).
func (h *TONEventHandler) Start() error {
	sourceType := h.factory.GetCurrentSourceType()
	lastLt := h.subscriberUC.GetLastProcessedLt()
	h.logger.Info("Starting TON blockchain event subscription via %s from lt=%s", sourceType, lastLt)

	// Start subscription in background
	go h.subscriberUC.Subscribe(h.ctx)

	h.logger.Info("TON blockchain event subscription started successfully")
	h.emitCheckpoint()
	return nil
}

// Stop gracefully stops the blockchain event subscription.
// Called during service shutdown.
func (h *TONEventHandler) Stop() error {
	h.logger.Info("Stopping TON blockchain event subscription")

	// Cancel subscription context
	h.cancel()

	// Stop event source
	h.subscriberUC.Stop()

	h.logger.Info("TON blockchain event subscription stopped successfully")
	h.emitCheckpoint()
	return nil
}

// GetLastProcessedLt returns the last successfully processed logical time (lt).
// Useful for health checks and monitoring.
// T154: Updated from block-based to lt-based for TON compatibility.
func (h *TONEventHandler) GetLastProcessedLt() string {
	return h.subscriberUC.GetLastProcessedLt()
}

// GetLastProcessedBlock returns the last successfully processed block number.
// Deprecated: Use GetLastProcessedLt() instead. TON uses logical time for ordering.
func (h *TONEventHandler) GetLastProcessedBlock() int64 {
	return h.subscriberUC.GetLastProcessedBlock()
}

// GetSourceType returns the type of event source being used ("websocket" or "http").
// T154: Added for monitoring and health checks.
func (h *TONEventHandler) GetSourceType() string {
	return h.factory.GetCurrentSourceType()
}

// IsConnected returns whether the event source is actively connected.
// T154: Added for health checks.
func (h *TONEventHandler) IsConnected() bool {
	return h.factory.IsConnected()
}

// SetMetrics sets the Prometheus metrics collector (T097).
// Delegates to the underlying BlockchainSubscriberUseCase.
func (h *TONEventHandler) SetMetrics(m *metrics.BlockchainMetrics) {
	h.subscriberUC.SetMetrics(m)
	h.logger.Info("Blockchain metrics enabled")
}

// SetLastProcessedLt sets the starting logical time for blockchain polling.
// Should be called before Start() to resume from database state.
func (h *TONEventHandler) SetLastProcessedLt(lt string) {
	if strings.TrimSpace(lt) == "" {
		h.logger.Warn("Empty last processed lt provided, defaulting to 0")
		lt = "0"
	}
	h.subscriberUC.SetLastProcessedLt(lt)
}

// SetOnLtUpdated sets a callback that is called when last_processed_lt is updated.
// Used to persist the state to database after processing transactions.
func (h *TONEventHandler) SetOnLtUpdated(callback func(lt string)) {
	h.onLtUpdated = callback
	h.refreshLtUpdateCallback()
}

// SetOnCheckpointUpdated sets a callback for persisted checkpoint updates.
func (h *TONEventHandler) SetOnCheckpointUpdated(callback func(SyncCheckpoint)) {
	h.onCheckpoint = callback
	h.refreshLtUpdateCallback()
}

// SetOnFallback sets a callback that runs after the event source falls back to HTTP polling.
func (h *TONEventHandler) SetOnFallback(callback func(SyncCheckpoint)) {
	h.onFallback = callback
}

// SetRetryConfig sets the retry configuration for blockchain event processing (Issue #9).
// Allows runtime configuration of retry parameters without code changes.
func (h *TONEventHandler) SetRetryConfig(cfg usecase.RetryConfig) {
	h.subscriberUC.SetRetryConfig(cfg)
	h.logger.Info("Retry config set: maxRetries=%d, initialBackoff=%v, maxBackoff=%v, multiplier=%.1f",
		cfg.MaxRetries, cfg.InitialBackoff, cfg.MaxBackoff, cfg.BackoffMultiplier)
}

// SetDeadLetterQueue sets the DLQ use case for failed transaction storage (Issue #6).
// Enables storing failed transactions for later retry and analysis.
func (h *TONEventHandler) SetDeadLetterQueue(dlq *usecase.DeadLetterQueueUseCase) {
	h.subscriberUC.SetDeadLetterQueue(dlq)
	h.logger.Info("Dead Letter Queue enabled for failed transaction storage")
}

// GetSyncCheckpoint returns the current resumable blockchain ingestion checkpoint.
func (h *TONEventHandler) GetSyncCheckpoint() SyncCheckpoint {
	checkpoint := SyncCheckpoint{
		LastProcessedLt: "0",
	}

	if h.subscriberUC != nil {
		checkpoint.LastProcessedLt = h.subscriberUC.GetLastProcessedLt()
	}

	if h.factory != nil {
		checkpoint.EventSourceType = h.factory.GetCurrentSourceType()
		checkpoint.EventSourceConnected = h.factory.IsConnected()
	}

	return checkpoint
}

func (h *TONEventHandler) refreshLtUpdateCallback() {
	if h.factory == nil {
		return
	}

	h.factory.SetOnLtUpdated(func(lt string) {
		if h.onLtUpdated != nil {
			h.onLtUpdated(lt)
		}
		h.emitCheckpoint()
	})
}

func (h *TONEventHandler) emitCheckpoint() {
	if h.onCheckpoint == nil {
		return
	}

	h.onCheckpoint(h.GetSyncCheckpoint())
}

func (h *TONEventHandler) emitFallback() {
	checkpoint := h.GetSyncCheckpoint()
	if h.onFallback != nil {
		h.onFallback(checkpoint)
	}
	if h.onCheckpoint != nil {
		h.onCheckpoint(checkpoint)
	}
}

func resolveInitialEventSourceType(cfg *config.Config, checkpoint *entity.BlockchainSyncState) string {
	sourceType := cfg.GameBackend.BlockchainEventSource
	if !cfg.GameBackend.ResumeEventSource || checkpoint == nil {
		return sourceType
	}

	switch checkpoint.EventSourceType {
	case toncenter.SourceTypeHTTP, toncenter.SourceTypeWebSocket:
		return checkpoint.EventSourceType
	default:
		return sourceType
	}
}

func shouldResumeCheckpoint(cfg *config.Config, checkpoint *entity.BlockchainSyncState) bool {
	return cfg.GameBackend.ResumeFromCheckpoint &&
		checkpoint != nil &&
		strings.TrimSpace(checkpoint.LastProcessedLt) != ""
}
