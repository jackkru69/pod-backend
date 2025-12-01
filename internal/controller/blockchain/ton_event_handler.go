package blockchain

import (
	"context"
	"time"

	"pod-backend/config"
	"pod-backend/internal/infrastructure/toncenter"
	"pod-backend/internal/usecase"
	"pod-backend/pkg/logger"
)

// TONEventHandler manages the blockchain event subscription lifecycle.
// Implements T094: Start blockchain subscription on service boot.
type TONEventHandler struct {
	subscriberUC *usecase.BlockchainSubscriberUseCase
	logger       logger.Interface
	ctx          context.Context
	cancel       context.CancelFunc
}

// NewTONEventHandler creates a new blockchain event handler.
// Initializes TON Center client, persistence use case, and blockchain subscriber.
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

	// Create TON Center client with circuit breaker
	tonClient := toncenter.NewClient(toncenter.ClientConfig{
		V2BaseURL:              cfg.GameBackend.TONCenterV2URL,
		ContractAddress:        cfg.GameBackend.TONGameContractAddr,
		CircuitBreakerMaxFail:  cfg.GameBackend.CircuitBreakerMaxFail,
		CircuitBreakerTimeout:  circuitBreakerTimeout,
		HTTPTimeout:            30 * time.Second, // Default HTTP timeout
	})

	// Create blockchain subscriber use case
	// Start from block 0 for now (TODO: persist last processed block in DB)
	subscriberUC := usecase.NewBlockchainSubscriberUseCase(
		tonClient,
		persistenceUC,
		logger,
		0, // startBlock
	)

	// Create context for subscription lifecycle
	ctx, cancel := context.WithCancel(context.Background())

	return &TONEventHandler{
		subscriberUC: subscriberUC,
		logger:       logger,
		ctx:          ctx,
		cancel:       cancel,
	}, nil
}

// Start begins the blockchain event subscription.
// Runs asynchronously in a separate goroutine (T094).
func (h *TONEventHandler) Start() error {
	h.logger.Info("Starting TON blockchain event subscription")

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

	// Stop poller
	h.subscriberUC.Stop()

	h.logger.Info("TON blockchain event subscription stopped successfully")
	return nil
}

// GetLastProcessedBlock returns the last successfully processed block number.
// Useful for health checks and monitoring.
func (h *TONEventHandler) GetLastProcessedBlock() int64 {
	return h.subscriberUC.GetLastProcessedBlock()
}
