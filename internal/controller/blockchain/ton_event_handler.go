package blockchain

import (
	"context"
	"time"

	"pod-backend/config"
	"pod-backend/internal/infrastructure/metrics"
	"pod-backend/internal/infrastructure/toncenter"
	"pod-backend/internal/usecase"
	"pod-backend/pkg/logger"
)

// TONEventHandler manages the blockchain event subscription lifecycle.
// Implements T094: Start blockchain subscription on service boot.
// T154: Updated to use EventSourceFactory for WebSocket/HTTP flexibility.
type TONEventHandler struct {
	subscriberUC  *usecase.BlockchainSubscriberUseCase
	factory       *toncenter.EventSourceFactory
	logger        logger.Interface
	ctx           context.Context
	cancel        context.CancelFunc
	onLtUpdated   func(lt string) // Callback to persist lt to database
}

// NewTONEventHandler creates a new blockchain event handler.
// Initializes EventSourceFactory, persistence use case, and blockchain subscriber.
// T154: Updated to use EventSourceFactory for WebSocket/HTTP event source selection.
func NewTONEventHandler(
	cfg *config.Config,
	persistenceUC *usecase.GamePersistenceUseCase,
	logger logger.Interface,
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

	// Create EventSourceFactory with WebSocket/HTTP configuration (T154)
	factory := toncenter.NewEventSourceFactory(toncenter.FactoryConfig{
		// HTTP Polling Configuration
		V2BaseURL:             cfg.GameBackend.TONCenterV2URL,
		ContractAddress:       cfg.GameBackend.TONGameContractAddr,
		CircuitBreakerMaxFail: cfg.GameBackend.CircuitBreakerMaxFail,
		CircuitBreakerTimeout: circuitBreakerTimeout,
		HTTPTimeout:           30 * time.Second,

		// WebSocket Configuration (T154)
		V3WSURL:         cfg.GameBackend.TONCenterV3WSURL,
		EnableWebSocket: cfg.GameBackend.EnableWebSocket,
		EventSourceType: cfg.GameBackend.BlockchainEventSource,
		MaxReconnect:    cfg.GameBackend.WebSocketReconnectMax,
		PingInterval:    wsPingInterval,

		// Fallback callback
		OnFallback: func() {
			logger.Warn("WebSocket event source failed, degraded to HTTP polling")
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

	return &TONEventHandler{
		subscriberUC: subscriberUC,
		factory:      factory,
		logger:       logger,
		ctx:          ctx,
		cancel:       cancel,
	}, nil
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
	h.logger.Info("Starting TON blockchain event subscription via %s", sourceType)

	// Start subscription in background
	go h.subscriberUC.Subscribe(h.ctx)

	h.logger.Info("TON blockchain event subscription started successfully")
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
	h.subscriberUC.SetLastProcessedLt(lt)
}

// SetOnLtUpdated sets a callback that is called when last_processed_lt is updated.
// Used to persist the state to database after processing transactions.
func (h *TONEventHandler) SetOnLtUpdated(callback func(lt string)) {
	h.onLtUpdated = callback
	// Also set callback on the factory/event source
	h.factory.SetOnLtUpdated(callback)
}
