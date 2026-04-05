package toncenter

import (
	"context"
	"math/big"
	"sort"
	"strconv"
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

	defaultLastProcessedLt = "0"
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
	ticker            *time.Ticker
	stopCh            chan struct{}
	backoffDuration   time.Duration   // Current exponential backoff duration (T103)
	consecutiveErrors int             // Count for exponential backoff
	isRunning         atomic.Bool     // Track if poller is running (T149)
	onLtUpdated       func(lt string) // Callback when last processed lt is updated
}

func normalizePollIntervals(minInterval, maxInterval time.Duration) (time.Duration, time.Duration) {
	if minInterval <= 0 {
		minInterval = MinPollInterval
	}
	if maxInterval <= 0 {
		maxInterval = MaxPollInterval
	}
	if maxInterval < minInterval {
		maxInterval = minInterval
	}

	return minInterval, maxInterval
}

// NewPoller creates a new adaptive blockchain poller.
// startBlock parameter is kept for API compatibility but ignored since TON uses logical time (lt).
func NewPoller(client *Client, handler EventHandler, logger logger.Interface, startBlock int64) *Poller {
	minInterval, maxInterval := normalizePollIntervals(MinPollInterval, MaxPollInterval)

	return &Poller{
		client:          client,
		handler:         handler,
		logger:          logger,
		currentInterval: maxInterval, // Start slow
		minInterval:     minInterval, // Use default constants
		maxInterval:     maxInterval,
		lastProcessedLt: defaultLastProcessedLt, // Start from beginning
		stopCh:          make(chan struct{}),
	}
}

// NewPollerWithIntervals creates a new poller with custom intervals.
func NewPollerWithIntervals(client *Client, handler EventHandler, logger logger.Interface, startBlock int64, minInterval, maxInterval time.Duration) *Poller {
	minInterval, maxInterval = normalizePollIntervals(minInterval, maxInterval)

	return &Poller{
		client:          client,
		handler:         handler,
		logger:          logger,
		currentInterval: maxInterval, // Start slow
		minInterval:     minInterval,
		maxInterval:     maxInterval,
		lastProcessedLt: defaultLastProcessedLt, // Start from beginning
		stopCh:          make(chan struct{}),
	}
}

func normalizeLt(lt string) string {
	if lt == "" {
		return defaultLastProcessedLt
	}

	return lt
}

func compareLtValues(lt1, lt2 string) int {
	n1 := new(big.Int)
	n2 := new(big.Int)

	n1.SetString(normalizeLt(lt1), 10)
	n2.SetString(normalizeLt(lt2), 10)

	return n1.Cmp(n2)
}

// SetOnLtUpdated sets a callback that is called when last_processed_lt is updated.
// Used to persist the state to database.
func (p *Poller) SetOnLtUpdated(callback func(lt string)) {
	p.onLtUpdated = callback
}

// Start begins the adaptive polling loop.
// Runs in a separate goroutine until Stop() is called.
func (p *Poller) Start(ctx context.Context) {
	p.minInterval, p.maxInterval = normalizePollIntervals(p.minInterval, p.maxInterval)
	if p.currentInterval <= 0 {
		p.currentInterval = p.maxInterval
	}

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
	return compareLtValues(lt1, lt2) > 0
}

func transactionDedupKey(tx Transaction) string {
	return normalizeLt(tx.Lt()) + "\x00" + tx.Hash()
}

func sortTransactionsByLt(txs []Transaction, ascending bool) {
	sort.Slice(txs, func(i, j int) bool {
		cmp := compareLtValues(txs[i].Lt(), txs[j].Lt())
		if cmp == 0 {
			return txs[i].Hash() < txs[j].Hash()
		}
		if ascending {
			return cmp < 0
		}
		return cmp > 0
	})
}

func normalizeTransactionsByLt(txs []Transaction, ascending bool) ([]Transaction, int) {
	if len(txs) == 0 {
		return nil, 0
	}

	uniqueTxs := make(map[string]Transaction, len(txs))
	duplicateCount := 0
	for _, tx := range txs {
		key := transactionDedupKey(tx)
		if _, exists := uniqueTxs[key]; exists {
			duplicateCount++
			continue
		}
		uniqueTxs[key] = tx
	}

	normalized := make([]Transaction, 0, len(uniqueTxs))
	for _, tx := range uniqueTxs {
		normalized = append(normalized, tx)
	}

	sortTransactionsByLt(normalized, ascending)

	return normalized, duplicateCount
}

// poll performs a single poll cycle.
func (p *Poller) poll(ctx context.Context) {
	p.logger.Debug("Polling from lt %s", p.lastProcessedLt)

	// Note: fromBlock parameter is not actually used by TON Center REST API
	txs, err := p.client.GetTransactions(ctx, PollBatchSize, nil, nil)
	if err != nil {
		p.logger.Error("Failed to fetch transactions", err)
		p.handleError() // Exponential backoff (T103)
		return
	}

	// Reset backoff on successful request
	p.consecutiveErrors = 0
	p.backoffDuration = BackoffInitial

	// Filter out already processed transactions (lt <= lastProcessedLt)
	// Use numeric comparison, not lexicographic (string) comparison
	var newTxs []Transaction
	for _, tx := range txs {
		if compareLt(tx.Lt(), p.lastProcessedLt) {
			newTxs = append(newTxs, tx)
		}
	}

	var duplicateCount int
	newTxs, duplicateCount = normalizeTransactionsByLt(newTxs, true)
	if duplicateCount > 0 {
		p.logger.Warn("Deduplicated %d HTTP polling transactions newer than checkpoint %s",
			duplicateCount, p.lastProcessedLt)
	}

	if len(newTxs) == 0 {
		p.logger.Debug("No new transactions (filtered %d already processed)", len(txs))
		p.adjustInterval(false) // Slow down when idle
		return
	}

	p.logger.Info("Found %d new transactions (filtered %d already processed)", len(newTxs), len(txs)-len(newTxs))

	// CRITICAL: Sort transactions by lt ASC (oldest first) to ensure correct event ordering.
	// TON API returns transactions newest-first, but we need to process GameInitialized
	// before GameStarted, GameFinished, etc. for the same game_id.
	sort.Slice(newTxs, func(i, j int) bool {
		n1 := new(big.Int)
		n2 := new(big.Int)
		n1.SetString(newTxs[i].Lt(), 10)
		n2.SetString(newTxs[j].Lt(), 10)
		return n1.Cmp(n2) < 0 // ASC order (oldest first)
	})

	// US-003: Check for transaction gaps and recursively backfill
	// If the oldest new transaction's previous lt is newer than our last processed lt,
	// it means we missed some intermediate transactions.
	if len(newTxs) > 0 && p.lastProcessedLt != "0" {
		oldestTx := newTxs[0]

		// Loop to fetch older pages until we bridge the gap to lastProcessedLt
		for oldestTx.PrevTransLt != "" && oldestTx.PrevTransLt != "0" && compareLt(oldestTx.PrevTransLt, p.lastProcessedLt) {
			p.logger.Info("Gap Detected: Backfilling from lt=%s hash=%s (target > %s)",
				oldestTx.PrevTransLt, oldestTx.PrevTransHash, p.lastProcessedLt)

			// Parse prev lt to uint64 for the client
			prevLt, err := strconv.ParseUint(oldestTx.PrevTransLt, 10, 64)
			if err != nil {
				p.logger.Error("Failed to parse prev lt %s: %v", oldestTx.PrevTransLt, err)
				break // Cannot continue backfilling
			}
			prevHash := oldestTx.PrevTransHash

			// Fetch previous batch using the oldest transaction's previous link
			backfilledTxs, err := p.client.GetTransactions(ctx, PollBatchSize, &prevLt, &prevHash)
			if err != nil {
				p.logger.Error("Failed to backfill transactions: %v", err)
				// If backfill fails, we stop processing to avoid skipping transactions.
				// We'll retry on the next poll cycle.
				p.handleError()
				return
			}

			if len(backfilledTxs) == 0 {
				p.logger.Warn("Backfill returned empty batch, stopping recursion")
				break
			}

			// Add valid transactions to our list
			// We only keep transactions > lastProcessedLt
			addedCount := 0
			for _, tx := range backfilledTxs {
				if compareLt(tx.Lt(), p.lastProcessedLt) {
					newTxs = append(newTxs, tx)
					addedCount++
				}
			}

			if addedCount == 0 {
				// All fetched transactions are <= lastProcessedLt, we are done
				break
			}

			var backfillDuplicateCount int
			newTxs, backfillDuplicateCount = normalizeTransactionsByLt(newTxs, true)
			if backfillDuplicateCount > 0 {
				p.logger.Warn("Deduplicated %d transactions while merging backfill batches",
					backfillDuplicateCount)
			}

			// Update oldestTx for next iteration
			oldestTx = newTxs[0]
		}
	}

	// Process transactions in chronological order (oldest first)
	// Track the last SUCCESSFULLY processed lt to ensure failed transactions are retried
	var lastSuccessfulLt string
	var hasFailure bool
	for _, tx := range newTxs {
		if err := p.handler.HandleTransaction(ctx, tx); err != nil {
			p.logger.Error("Failed to handle transaction hash=%s: %v", tx.Hash(), err)
			hasFailure = true
			// CRITICAL: Stop processing further transactions if one fails
			// This ensures we don't skip ahead and lose the failed transaction
			// The failed tx will be retried on next poll cycle
			break
		}

		// Update last SUCCESSFULLY processed logical time (lt)
		// Only update after successful handling to ensure failed txs are retried
		if compareLt(tx.Lt(), p.lastProcessedLt) {
			p.lastProcessedLt = tx.Lt()
			lastSuccessfulLt = tx.Lt()
		}
	}

	// Persist updated lt to database via callback (only if we made progress)
	if lastSuccessfulLt != "" && p.onLtUpdated != nil {
		p.onLtUpdated(lastSuccessfulLt)
	}

	// Slow down if we had failures (to avoid hammering on stuck transactions)
	if hasFailure {
		p.adjustInterval(false)
	} else {
		// Speed up when all transactions processed successfully
		p.adjustInterval(true)
	}
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
	if p.currentInterval != oldInterval && p.ticker != nil {
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
	p.lastProcessedLt = normalizeLt(lt)
	p.logger.Info("Set last processed lt to %s", p.lastProcessedLt)
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
