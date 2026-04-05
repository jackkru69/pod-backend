package toncenter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPoller_DeepBackfilling simulates a scenario where the poller is far behind
// (more than multiple pages of transactions) and must backfill recursively
// to bridge the gap before processing.
//
// Scenario:
// - Last processed LT: 50
// - Current Blockchain State: 300 transactions (LT 300 down to 1)
// - Page Size: 100
// - Expected Behavior:
//  1. Initial Poll gets LT 300..201. Oldest is 201. Prev is 200. Gap > 50.
//  2. Backfill 1 requests LT 200. Gets 200..101. Oldest is 101. Prev is 100. Gap > 50.
//  3. Backfill 2 requests LT 100. Gets 100..1.
//  4. Poller filters out <= 50. Keeps 100..51.
//  5. Final list is 300..51.
//  6. Handler processes 51..300 in order.
func TestPoller_DeepBackfilling(t *testing.T) {
	// 1. Generate chain of transactions
	totalTxs := 300
	chain := make([]Transaction, totalTxs)
	for i := 0; i < totalTxs; i++ {
		// LT goes from 1 to 300
		lt := i + 1
		sLt := strconv.Itoa(lt)
		prevLt := strconv.Itoa(lt - 1)
		if lt == 1 {
			prevLt = "0"
		}

		chain[i] = Transaction{
			TransactionID: struct {
				Type string `json:"@type"`
				Lt   string `json:"lt"`
				Hash string `json:"hash"`
			}{
				Type: "internal.transactionId",
				Lt:   sLt,
				Hash: fmt.Sprintf("hash_%d", lt),
			},
			PrevTransLt:   prevLt,
			PrevTransHash: fmt.Sprintf("hash_%d", lt-1),
		}
	}

	// Helper to get tx by LT
	getTxByLt := func(ltStr string) (Transaction, int) {
		lt, _ := strconv.Atoi(ltStr)
		if lt < 1 || lt > totalTxs {
			return Transaction{}, -1
		}
		return chain[lt-1], lt - 1
	}

	// 2. Setup Mock Server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		ltStr := query.Get("lt")
		limitStr := query.Get("limit")
		limit := 100
		if limitStr != "" {
			l, _ := strconv.Atoi(limitStr)
			if l > 0 {
				limit = l
			}
		}

		var result []Transaction

		if ltStr == "" {
			// No LT provided, return latest (highest LTs)
			// Sort chain descending (Newest first)
			// For simulation, we construct the slice from end of chain
			startIdx := len(chain) - 1
			endIdx := startIdx - limit + 1
			if endIdx < 0 {
				endIdx = 0
			}

			// Collect in reverse order (Newest First) as per TON API
			for i := startIdx; i >= endIdx; i-- {
				result = append(result, chain[i])
			}
		} else {
			// LT provided. Return transactions starting from this LT (inclusive) backwards
			// Find index of requested LT
			_, idx := getTxByLt(ltStr)

			if idx != -1 {
				// Start from idx and go backwards
				count := 0
				for i := idx; i >= 0 && count < limit; i-- {
					result = append(result, chain[i])
					count++
				}
			}
		}

		resp := struct {
			OK     bool          `json:"ok"`
			Result []Transaction `json:"result"`
		}{
			OK:     true,
			Result: result,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// 3. Setup Poller
	cfg := ClientConfig{
		V2BaseURL:             server.URL,
		ContractAddress:       "addr",
		CircuitBreakerMaxFail: 5,
		CircuitBreakerTimeout: 1 * time.Second,
		HTTPTimeout:           1 * time.Second,
	}
	client := NewClient(cfg)
	mockLogger := &MockLogger{}
	handler := &MockEventHandler{}

	poller := NewPoller(client, handler, mockLogger, 0)

	// Start from LT 50
	poller.lastProcessedLt = "50"
	// Initialize ticker since we are calling poll() directly without Start()
	poller.ticker = time.NewTicker(time.Hour)

	// Force batch size to match our test expectation if needed,
	// but PollBatchSize const is 100 which matches our logic.

	// 4. Run Poll
	poller.poll(context.Background())

	// 5. Assertions

	// Check that we processed all transactions from 51 to 300 (250 txs)
	expectedCount := 250
	assert.Equal(t, expectedCount, len(handler.HandledTxs), "Should have processed 250 transactions")

	if len(handler.HandledTxs) > 0 {
		// Check order: Should be Oldest First (51, 52, ... 300)
		first := handler.HandledTxs[0]
		last := handler.HandledTxs[len(handler.HandledTxs)-1]

		assert.Equal(t, "51", first.Lt(), "First processed should be LT 51")
		assert.Equal(t, "300", last.Lt(), "Last processed should be LT 300")

		// Verify continuity
		for i, tx := range handler.HandledTxs {
			expectedLt := 51 + i
			assert.Equal(t, strconv.Itoa(expectedLt), tx.Lt(), "Transaction order mismatch at index %d", i)
		}
	}

	// Verify logs for backfilling
	backfillCount := 0
	for _, log := range mockLogger.Logs {
		if strings.Contains(log, "Backfilling") {
			backfillCount++
		}
	}
	// We expect backfills.
	// Initial fetch: 300..201. Gap (200 > 50).
	// Backfill 1: 200..101. Gap (100 > 50).
	// Backfill 2: 100..1. Gap (0 <= 50) -> stop.
	// So we expect at least 1 backfill log (depends on how many times it logs inside loop).
	// The code logs "Gap Detected..." at start of loop.
	// Loop 1: oldest=201, prev=200. Log.
	// Loop 2: oldest=101, prev=100. Log.
	// Loop 3: oldest=51, prev=50. Condition fails.
	// So 2 logs.
	assert.GreaterOrEqual(t, backfillCount, 2, "Should have logged backfilling at least twice")
}

type pollerFailOnLtOnceHandler struct {
	failLt    string
	failed    bool
	handledLt []string
}

func (h *pollerFailOnLtOnceHandler) HandleTransaction(_ context.Context, tx Transaction) error {
	h.handledLt = append(h.handledLt, tx.Lt())
	if tx.Lt() == h.failLt && !h.failed {
		h.failed = true
		return errors.New("boom")
	}

	return nil
}

func TestPoller_RetriesFailedTransactionFromLastSuccessfulCheckpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := struct {
			OK     bool          `json:"ok"`
			Result []Transaction `json:"result"`
		}{
			OK: true,
			Result: []Transaction{
				createTransactionWithPrev("120", "110"),
				createTransactionWithPrev("110", "105"),
				createTransactionWithPrev("105", "100"),
			},
		}
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	cfg := ClientConfig{
		V2BaseURL:             server.URL,
		ContractAddress:       "addr",
		CircuitBreakerMaxFail: 5,
		CircuitBreakerTimeout: time.Second,
		HTTPTimeout:           time.Second,
	}
	client := NewClient(cfg)
	logger := &MockLogger{}
	handler := &pollerFailOnLtOnceHandler{failLt: "110"}

	poller := NewPoller(client, handler, logger, 0)
	poller.lastProcessedLt = "100"
	poller.ticker = time.NewTicker(time.Hour)
	defer poller.ticker.Stop()

	var persistedLt []string
	poller.SetOnLtUpdated(func(lt string) {
		persistedLt = append(persistedLt, lt)
	})

	poller.poll(context.Background())

	assert.Equal(t, []string{"105", "110"}, handler.handledLt)
	assert.Equal(t, "105", poller.GetLastProcessedLt())
	assert.Equal(t, []string{"105"}, persistedLt)

	poller.poll(context.Background())

	assert.Equal(t, []string{"105", "110", "110", "120"}, handler.handledLt)
	assert.Equal(t, "120", poller.GetLastProcessedLt())
	assert.Equal(t, []string{"105", "120"}, persistedLt)
}

func TestPoller_DeduplicatesInitialAndBackfilledTransactions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lt := r.URL.Query().Get("lt")

		resp := struct {
			OK     bool          `json:"ok"`
			Result []Transaction `json:"result"`
		}{
			OK: true,
		}

		switch lt {
		case "":
			resp.Result = []Transaction{
				createTransactionWithPrev("120", "110"),
				createTransactionWithPrev("120", "110"),
				createTransactionWithPrev("110", "105"),
			}
		case "105":
			resp.Result = []Transaction{
				createTransactionWithPrev("110", "105"),
				createTransactionWithPrev("105", "100"),
			}
		}

		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	cfg := ClientConfig{
		V2BaseURL:             server.URL,
		ContractAddress:       "addr",
		CircuitBreakerMaxFail: 5,
		CircuitBreakerTimeout: time.Second,
		HTTPTimeout:           time.Second,
	}
	client := NewClient(cfg)
	logger := &MockLogger{}
	handler := &MockEventHandler{}

	poller := NewPoller(client, handler, logger, 0)
	poller.lastProcessedLt = "100"
	poller.ticker = time.NewTicker(time.Hour)
	defer poller.ticker.Stop()

	poller.poll(context.Background())

	assert.Equal(t, []string{"105", "110", "120"}, []string{
		handler.HandledTxs[0].Lt(),
		handler.HandledTxs[1].Lt(),
		handler.HandledTxs[2].Lt(),
	})
	assert.Equal(t, "120", poller.GetLastProcessedLt())
}

func TestPoller_ProcessesOutOfOrderTransactionsChronologically(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := struct {
			OK     bool          `json:"ok"`
			Result []Transaction `json:"result"`
		}{
			OK: true,
			Result: []Transaction{
				createTransactionWithPrev("120", "110"),
				createTransactionWithPrev("105", "100"),
				createTransactionWithPrev("110", "105"),
			},
		}

		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	cfg := ClientConfig{
		V2BaseURL:             server.URL,
		ContractAddress:       "addr",
		CircuitBreakerMaxFail: 5,
		CircuitBreakerTimeout: time.Second,
		HTTPTimeout:           time.Second,
	}
	client := NewClient(cfg)
	logger := &MockLogger{}
	handler := &MockEventHandler{}

	poller := NewPoller(client, handler, logger, 0)
	poller.lastProcessedLt = "100"
	poller.ticker = time.NewTicker(time.Hour)
	defer poller.ticker.Stop()

	poller.poll(context.Background())

	assert.Equal(t, []string{"105", "110", "120"}, []string{
		handler.HandledTxs[0].Lt(),
		handler.HandledTxs[1].Lt(),
		handler.HandledTxs[2].Lt(),
	})
	assert.Equal(t, "120", poller.GetLastProcessedLt())
}

func createTransactionWithPrev(lt string, prevLt string) Transaction {
	return Transaction{
		TransactionID: struct {
			Type string `json:"@type"`
			Lt   string `json:"lt"`
			Hash string `json:"hash"`
		}{
			Type: "internal.transactionId",
			Lt:   lt,
			Hash: "hash_" + lt,
		},
		PrevTransLt:   prevLt,
		PrevTransHash: "hash_" + prevLt,
	}
}
