package toncenter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"pod-backend/pkg/logger"
)

// MockLogger implements logger.Interface and captures logs
type MockLogger struct {
	Logs []string
}

// Ensure MockLogger implements logger.Interface
var _ logger.Interface = (*MockLogger)(nil)

func (m *MockLogger) Debug(message interface{}, args ...interface{}) {
	m.log("DEBUG", message, args...)
}

func (m *MockLogger) Info(message string, args ...interface{}) {
	m.log("INFO", message, args...)
}

func (m *MockLogger) Warn(message string, args ...interface{}) {
	m.log("WARN", message, args...)
}

func (m *MockLogger) Error(message interface{}, args ...interface{}) {
	m.log("ERROR", message, args...)
}

func (m *MockLogger) Fatal(message interface{}, args ...interface{}) {
	m.log("FATAL", message, args...)
}

func (m *MockLogger) log(level string, message interface{}, args ...interface{}) {
	msg := fmt.Sprintf("%v", message)
	if len(args) > 0 {
		msg = fmt.Sprintf(msg, args...)
	}
	m.Logs = append(m.Logs, fmt.Sprintf("[%s] %s", level, msg))
}

// MockEventHandler implements EventHandler
type MockEventHandler struct {
	HandledTxs []Transaction
}

func (m *MockEventHandler) HandleTransaction(ctx context.Context, tx Transaction) error {
	m.HandledTxs = append(m.HandledTxs, tx)
	return nil
}

func TestPoller_Backfilling(t *testing.T) {
	// Setup Mock Server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lt := r.URL.Query().Get("lt")

		var txs []Transaction

		// Scenario: Last processed 50.
		// Latest on chain: 200 -> 100 -> 75 -> 50.

		if lt == "" {
			// Initial fetch (latest)
			txs = []Transaction{
				{
					TransactionID: struct {
						Type string `json:"@type"`
						Lt   string `json:"lt"`
						Hash string `json:"hash"`
					}{Type: "internal.transactionId", Lt: "200", Hash: "hash_200"},
					PrevTransLt:   "100",
					PrevTransHash: "hash_100",
				},
			}
		} else if lt == "100" {
			// Backfill 1
			txs = []Transaction{
				{
					TransactionID: struct {
						Type string `json:"@type"`
						Lt   string `json:"lt"`
						Hash string `json:"hash"`
					}{Type: "internal.transactionId", Lt: "100", Hash: "hash_100"},
					PrevTransLt:   "75",
					PrevTransHash: "hash_75",
				},
			}
		} else if lt == "75" {
			// Backfill 2
			txs = []Transaction{
				{
					TransactionID: struct {
						Type string `json:"@type"`
						Lt   string `json:"lt"`
						Hash string `json:"hash"`
					}{Type: "internal.transactionId", Lt: "75", Hash: "hash_75"},
					PrevTransLt:   "50",
					PrevTransHash: "hash_50",
				},
			}
		} else {
			// Older or other requests
			txs = []Transaction{}
		}

		resp := struct {
			OK     bool          `json:"ok"`
			Result []Transaction `json:"result"`
		}{
			OK:     true,
			Result: txs,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

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
	poller.lastProcessedLt = "50"
	poller.ticker = time.NewTicker(time.Hour)

	poller.poll(context.Background())

	// Verify HandledTxs
	// Order should be 75, 100, 200 (Oldest First)
	assert.Equal(t, 3, len(handler.HandledTxs))
	if len(handler.HandledTxs) == 3 {
		assert.Equal(t, "75", handler.HandledTxs[0].Lt())
		assert.Equal(t, "100", handler.HandledTxs[1].Lt())
		assert.Equal(t, "200", handler.HandledTxs[2].Lt())
	}

	// Verify logs contain "Backfilling"
	foundBackfill := false
	for _, log := range mockLogger.Logs {
		if strings.Contains(log, "Backfilling") {
			foundBackfill = true
			break
		}
	}
	assert.True(t, foundBackfill, "Expected backfilling log")
}

func TestPoller_Poll_NoGap(t *testing.T) {
	// Setup mock server returning transactions WITHOUT a gap
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		resp := struct {
			OK     bool          `json:"ok"`
			Result []Transaction `json:"result"`
		}{
			OK: true,
			Result: []Transaction{
				{
					TransactionID: struct {
						Type string `json:"@type"`
						Lt   string `json:"lt"`
						Hash string `json:"hash"`
					}{
						Lt:   "200",
						Hash: "hash200",
					},
					PrevTransLt: "100",
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := ClientConfig{
		V2BaseURL:             server.URL,
		ContractAddress:       "EQD...",
		CircuitBreakerMaxFail: 5,
		CircuitBreakerTimeout: 1 * time.Second,
		HTTPTimeout:           1 * time.Second,
	}
	client := NewClient(cfg)
	mockLogger := &MockLogger{}
	handler := &MockEventHandler{}

	poller := NewPoller(client, handler, mockLogger, 0)
	poller.lastProcessedLt = "100"
	poller.ticker = time.NewTicker(time.Hour)

	poller.poll(context.Background())

	// Check logs for NO backfilling warning
	foundBackfill := false
	for _, log := range mockLogger.Logs {
		if strings.Contains(log, "Backfilling") {
			foundBackfill = true
			break
		}
	}
	assert.False(t, foundBackfill, "Did not expect backfilling log")
}

func TestNewPollerWithIntervals_DefaultsNonPositiveIntervals(t *testing.T) {
	poller := NewPollerWithIntervals(nil, &MockEventHandler{}, &MockLogger{}, 0, 0, 0)

	assert.Equal(t, MinPollInterval, poller.minInterval)
	assert.Equal(t, MaxPollInterval, poller.maxInterval)
	assert.Equal(t, MaxPollInterval, poller.currentInterval)
}
