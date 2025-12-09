package toncenter

import (
	"context"
	"math/big"
	"sync/atomic"
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

// Ensure Poller implements EventSource interface (T149)
var _ EventSource = (*Poller)(nil)

// Poller implements adaptive interval polling for blockchain events.
// Decreases interval when activity detected, increases when idle.
// Implements EventSource interface for abstraction (T149).
type Poller struct {
	client            *Client
	handler           EventHandler
	logger            logger.Interface
	currentInterval   time.Duration
	minInterval       time.Duration // Configurable minimum interval
	maxInterval       time.Duration // Configurable maximum interval
	lastProcessedLt   string        // Last processed logical time (lt)
	lastProcessedHash string        // Last processed transaction hash (required with lt)
	ticker            *time.Ticker
	stopCh            chan struct{}
	backoffDuration   time.Duration                 // Current exponential backoff duration (T103)
	consecutiveErrors int                           // Count for exponential backoff
	isRunning         atomic.Bool                   // Track if poller is running (T149)
	onLtUpdated       func(lt string, hash string) // Callback when last processed lt and hash are updated
}

// NewPoller creates a new adaptive blockchain poller.
// startBlock parameter is kept for API compatibility but ignored since TON uses logical time (lt).
func NewPoller(client *Client, handler EventHandler, logger logger.Interface, startBlock int64) *Poller {
	return &Poller{
		client:          client,
		handler:         handler,
		logger:          logger,
		currentInterval: MaxPollInterval, // Start slow
		minInterval:     MinPollInterval, // Use default constants
		maxInterval:     MaxPollInterval,
		lastProcessedLt: "0", // Start from beginning
		stopCh:          make(chan struct{}),
	}
}

// NewPollerWithIntervals creates a new poller with custom intervals.
func NewPollerWithIntervals(client *Client, handler EventHandler, logger logger.Interface, startBlock int64, minInterval, maxInterval time.Duration) *Poller {
	return &Poller{
		client:          client,
		handler:         handler,
		logger:          logger,
		currentInterval: maxInterval, // Start slow
		minInterval:     minInterval,
		maxInterval:     maxInterval,
		lastProcessedLt: "0", // Start from beginning
		stopCh:          make(chan struct{}),
	}
}

// SetOnLtUpdated sets a callback that is called when last_processed_lt and hash are updated.
// Used to persist the state to database.
func (p *Poller) SetOnLtUpdated(callback func(lt string, hash string)) {
	p.onLtUpdated = callback
}

// Start begins the adaptive polling loop.
// Runs in a separate goroutine until Stop() is called.
func (p *Poller) Start(ctx context.Context) {
	p.ticker = time.NewTicker(p.currentInterval)
	p.isRunning.Store(true)

	go func() {
		p.logger.Info("Starting blockchain poller from lt %s", p.lastProcessedLt)
		defer p.isRunning.Store(false)

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
	p.isRunning.Store(false)
	close(p.stopCh)
}

// compareLt compares two logical time strings as big integers.
// Returns true if lt1 > lt2 (lt1 is newer than lt2).
func compareLt(lt1, lt2 string) bool {
	// Parse as big integers to handle large lt values
	n1 := new(big.Int)
	n2 := new(big.Int)

	n1.SetString(lt1, 10)
	n2.SetString(lt2, 10)

	return n1.Cmp(n2) > 0
}

// poll performs a single poll cycle.
func (p *Poller) poll(ctx context.Context) {
	p.logger.Debug("Polling from lt %s hash %s", p.lastProcessedLt, p.lastProcessedHash)

	// Use GetTransactionsFromLt with lt and hash for proper pagination
	// According to TON Center API, lt and hash must be sent together
	txs, err := p.client.GetTransactionsFromLt(ctx, p.lastProcessedLt, p.lastProcessedHash, PollBatchSize)
	if err != nil {
		p.logger.Error("Failed to fetch transactions", err)
		p.handleError() // Exponential backoff (T103)
		return
	}

	// Reset backoff on successful request
	p.consecutiveErrors = 0
	p.backoffDuration = BackoffInitial

	// Special case: if starting from lt=0, take the first (latest) transaction as starting point
	if p.lastProcessedLt == "0" && len(txs) > 0 {
		// Take the most recent transaction as our starting point
		latestTx := txs[0]
		p.lastProcessedLt = latestTx.Lt()
		p.lastProcessedHash = latestTx.Hash()
		p.logger.Info("Starting from latest transaction lt=%s hash=%s", p.lastProcessedLt, p.lastProcessedHash)

		// Persist initial position to database
		if p.onLtUpdated != nil {
			p.onLtUpdated(p.lastProcessedLt, p.lastProcessedHash)
		}

		p.adjustInterval(false) // No new transactions to process yet
		return
	}

	// Filter out already processed transactions (lt <= lastProcessedLt)
	// Server-side pagination gives us transactions starting from our position
	// but we still need to skip the starting transaction itself
	var newTxs []Transaction
	for _, tx := range txs {
		// Skip if this is the same transaction we started from
		if tx.Lt() == p.lastProcessedLt && tx.Hash() == p.lastProcessedHash {
			continue
		}
		// Skip if already processed (shouldn't happen with correct API usage)
		if compareLt(tx.Lt(), p.lastProcessedLt) || tx.Lt() == p.lastProcessedLt {
			continue
		}
		newTxs = append(newTxs, tx)
	}

	if len(newTxs) == 0 {
		p.logger.Debug("No new transactions (fetched %d, filtered %d)", len(txs), len(txs)-len(newTxs))
		p.adjustInterval(false) // Slow down when idle
		return
	}

	p.logger.Info("Found %d new transactions (fetched %d total)", len(newTxs), len(txs))

	// Process transactions
	var maxLt string
	var maxHash string
	for _, tx := range newTxs {
		if err := p.handler.HandleTransaction(ctx, tx); err != nil {
			p.logger.Error("Failed to handle transaction hash=%s: %v", tx.Hash(), err)
			continue
		}

		// Update last processed logical time (lt) and hash
		// TON uses lt (logical time) for transaction ordering, higher lt means newer
		// We need both lt and hash for proper API pagination
		if compareLt(tx.Lt(), p.lastProcessedLt) {
			p.lastProcessedLt = tx.Lt()
			p.lastProcessedHash = tx.Hash()
			maxLt = tx.Lt()
			maxHash = tx.Hash()
		}
	}

	// Persist updated lt and hash to database via callback
	if maxLt != "" && p.onLtUpdated != nil {
		p.onLtUpdated(maxLt, maxHash)
		p.logger.Debug("Updated last processed position to lt=%s hash=%s", maxLt, maxHash)
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
		if p.currentInterval < p.minInterval {
			p.currentInterval = p.minInterval
		}
	} else {
		// No activity: increase interval (poll slower)
		p.currentInterval = p.currentInterval * 3 / 2
		if p.currentInterval > p.maxInterval {
			p.currentInterval = p.maxInterval
		}
	}

	// Update ticker if interval changed
	if p.currentInterval != oldInterval {
		p.ticker.Reset(p.currentInterval)
		p.logger.Debug("Adjusted poll interval: %v -> %v", oldInterval, p.currentInterval)
	}
}

// GetLastProcessedBlock returns the last successfully processed block number.
// Note: For TON blockchain, we track logical time (lt) instead of block numbers.
// This method is kept for API compatibility.
func (p *Poller) GetLastProcessedBlock() int64 {
	// Return 0 as we don't track block numbers anymore
	return 0
}

// SetLastProcessedBlock updates the starting block for polling.
// Useful for resuming from database state.
// Note: For TON blockchain, this sets the logical time (lt) as a string.
func (p *Poller) SetLastProcessedBlock(block int64) {
	// For compatibility, we ignore the block number
	p.logger.Info("SetLastProcessedBlock called with %d (ignored for TON)", block)
}

// GetLastProcessedLt returns the last successfully processed logical time.
func (p *Poller) GetLastProcessedLt() string {
	return p.lastProcessedLt
}

// SetLastProcessedLt updates the starting logical time for polling.
// Useful for resuming from database state.
func (p *Poller) SetLastProcessedLt(lt string) {
	p.lastProcessedLt = lt
	p.logger.Info("Set last processed lt to %s", lt)
}

// SetLastProcessedHash updates the starting transaction hash for polling.
// Must be used together with SetLastProcessedLt for proper TON API pagination.
// Useful for resuming from database state.
func (p *Poller) SetLastProcessedHash(hash string) {
	p.lastProcessedHash = hash
	p.logger.Info("Set last processed hash to %s", hash)
}

// Subscribe registers an event handler (T149).
// For Poller, this replaces the existing handler.
func (p *Poller) Subscribe(handler EventHandler) {
	p.handler = handler
}

// IsConnected returns whether the poller is actively running (T149).
func (p *Poller) IsConnected() bool {
	return p.isRunning.Load()
}

// GetSourceType returns "http" for the HTTP polling source (T149).
func (p *Poller) GetSourceType() string {
	return SourceTypeHTTP
}
