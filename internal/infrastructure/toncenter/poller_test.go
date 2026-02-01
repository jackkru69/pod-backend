package toncenter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"pod-backend/pkg/logger"

	"github.com/stretchr/testify/assert"
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
type MockEventHandler struct{}

func (m *MockEventHandler) HandleTransaction(ctx context.Context, tx Transaction) error {
	return nil
}

func TestPoller_Poll_GapDetection(t *testing.T) {
	// Setup mock server returning transactions with a gap
	// lastProcessedLt = 100
	// Txs: [ {lt: 300, prev_lt: 200}, {lt: 200, prev_lt: 150} ]
	// Sorted: 200 (prev 150), 300 (prev 200)
	// Gap: 150 > 100
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
						Lt:   "300",
						Hash: "hash300",
					},
					PrevTransLt: "200",
				},
				{
					TransactionID: struct {
						Type string `json:"@type"`
						Lt   string `json:"lt"`
						Hash string `json:"hash"`
					}{
						Lt:   "200",
						Hash: "hash200",
					},
					PrevTransLt: "150",
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
	poller.ticker = time.NewTicker(time.Hour) // Initialize ticker to avoid panic in adjustInterval

	// Run poll directly
	poller.poll(context.Background())

	// Check logs for warning
	foundGapWarning := false
	for _, log := range mockLogger.Logs {
		if contains(log, "Gap Detected") {
			foundGapWarning = true
			break
		}
	}
	assert.True(t, foundGapWarning, "Expected 'Gap Detected' warning in logs")
}

func TestPoller_Poll_NoGap(t *testing.T) {
	// Setup mock server returning transactions WITHOUT a gap
	// lastProcessedLt = 100
	// Txs: [ {lt: 200, prev_lt: 100} ]
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
	poller.ticker = time.NewTicker(time.Hour) // Initialize ticker to avoid panic in adjustInterval

	// Run poll directly
	poller.poll(context.Background())

	// Check logs for NO warning
	foundGapWarning := false
	for _, log := range mockLogger.Logs {
		if contains(log, "Gap Detected") {
			foundGapWarning = true
			break
		}
	}
	assert.False(t, foundGapWarning, "Did not expect 'Gap Detected' warning in logs")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[0:len(substr)] == substr || (len(s) > len(substr) && contains(s[1:], substr))
}
