package toncenter

import (
	"context"
	"time"

	"pod-backend/pkg/logger"
)

const (
	// Adaptive polling intervals (FR-007)
	MinPollInterval = 5 * time.Second  // When active
	MaxPollInterval = 30 * time.Second // When idle
	PollBatchSize   = 100              // Transactions per batch

	// Exponential backoff for TON Center reconnection (T103, FR-019)
	BackoffInitial = 1 * time.Second
	BackoffMax     = 16 * time.Second
)

// EventHandler processes blockchain transactions.
// Implementations should parse events and update database.
type EventHandler interface {
	HandleTransaction(ctx context.Context, tx Transaction) error
}

// Poller implements adaptive interval polling for blockchain events.
// Decreases interval when activity detected, increases when idle.
type Poller struct {
	client             *Client
	handler            EventHandler
	logger             logger.Interface
	currentInterval    time.Duration
	lastProcessedBlock int64
	ticker             *time.Ticker
	stopCh             chan struct{}
	backoffDuration    time.Duration // Current exponential backoff duration (T103)
	consecutiveErrors  int            // Count for exponential backoff
}

// NewPoller creates a new adaptive blockchain poller.
func NewPoller(client *Client, handler EventHandler, logger logger.Interface, startBlock int64) *Poller {
	return &Poller{
		client:             client,
		handler:            handler,
		logger:             logger,
		currentInterval:    MaxPollInterval, // Start slow
		lastProcessedBlock: startBlock,
		stopCh:             make(chan struct{}),
	}
}

// Start begins the adaptive polling loop.
// Runs in a separate goroutine until Stop() is called.
func (p *Poller) Start(ctx context.Context) {
	p.ticker = time.NewTicker(p.currentInterval)

	go func() {
		p.logger.Info("Starting blockchain poller from block %d", p.lastProcessedBlock)

		for {
			select {
			case <-ctx.Done():
				p.logger.Info("Poller context cancelled")
				return
			case <-p.stopCh:
				p.logger.Info("Poller stopped")
				return
			case <-p.ticker.C:
				p.poll(ctx)
			}
		}
	}()
}

// Stop gracefully stops the polling loop.
func (p *Poller) Stop() {
	if p.ticker != nil {
		p.ticker.Stop()
	}
	close(p.stopCh)
}

// poll performs a single poll cycle.
func (p *Poller) poll(ctx context.Context) {
	p.logger.Debug("Polling from block %d", p.lastProcessedBlock)

	txs, err := p.client.GetTransactions(ctx, p.lastProcessedBlock+1, PollBatchSize)
	if err != nil {
		p.logger.Error("Failed to fetch transactions", err)
		p.handleError() // Exponential backoff (T103)
		return
	}

	// Reset backoff on successful request
	p.consecutiveErrors = 0
	p.backoffDuration = BackoffInitial

	if len(txs) == 0 {
		p.logger.Debug("No new transactions")
		p.adjustInterval(false) // Slow down when idle
		return
	}

	p.logger.Info("Found %d new transactions", len(txs))

	// Process transactions
	for _, tx := range txs {
		if err := p.handler.HandleTransaction(ctx, tx); err != nil {
			p.logger.Error("Failed to handle transaction %s", err, tx.Hash)
			continue
		}

		// Update last processed block
		if tx.BlockNumber > p.lastProcessedBlock {
			p.lastProcessedBlock = tx.BlockNumber
		}
	}

	// Speed up when activity detected
	p.adjustInterval(true)
}

// handleError implements exponential backoff for TON Center API errors (T103, FR-019).
// Backoff sequence: 1s, 2s, 4s, 8s, 16s max as per research.md.
func (p *Poller) handleError() {
	p.consecutiveErrors++

	if p.backoffDuration == 0 {
		p.backoffDuration = BackoffInitial
	} else {
		// Double the backoff duration
		p.backoffDuration = p.backoffDuration * 2
		if p.backoffDuration > BackoffMax {
			p.backoffDuration = BackoffMax
		}
	}

	p.logger.Warn("TON Center API error (attempt %d), backing off for %v", p.consecutiveErrors, p.backoffDuration)

	// Wait for backoff duration
	time.Sleep(p.backoffDuration)

	// Also slow down polling interval
	p.adjustInterval(false)
}

// adjustInterval adjusts polling interval based on activity.
// Decreases when active, increases when idle (FR-007).
func (p *Poller) adjustInterval(hasActivity bool) {
	oldInterval := p.currentInterval

	if hasActivity {
		// Activity detected: decrease interval (poll faster)
		p.currentInterval = p.currentInterval * 2 / 3
		if p.currentInterval < MinPollInterval {
			p.currentInterval = MinPollInterval
		}
	} else {
		// No activity: increase interval (poll slower)
		p.currentInterval = p.currentInterval * 3 / 2
		if p.currentInterval > MaxPollInterval {
			p.currentInterval = MaxPollInterval
		}
	}

	// Update ticker if interval changed
	if p.currentInterval != oldInterval {
		p.ticker.Reset(p.currentInterval)
		p.logger.Debug("Adjusted poll interval: %v -> %v", oldInterval, p.currentInterval)
	}
}

// GetLastProcessedBlock returns the last successfully processed block number.
func (p *Poller) GetLastProcessedBlock() int64 {
	return p.lastProcessedBlock
}

// SetLastProcessedBlock updates the starting block for polling.
// Useful for resuming from database state.
func (p *Poller) SetLastProcessedBlock(block int64) {
	p.lastProcessedBlock = block
	p.logger.Info("Set last processed block to %d", block)
}
