package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

// Mock TON Center API server for testing (T115)

type MockTonCenter struct {
	mu                 sync.RWMutex
	transactions       []Transaction
	currentBlock       int64
	failureMode        bool
	circuitBreakerTest bool
}

type Transaction struct {
	Hash        string          `json:"hash"`
	Lt          string          `json:"lt"`
	BlockNumber int64           `json:"block"`
	Timestamp   int64           `json:"utime"`
	Data        json.RawMessage `json:"data"`
	PrevTransLt string          `json:"prev_trans_lt,omitempty"`
}

type GetTransactionsResponse struct {
	OK     bool          `json:"ok"`
	Result []Transaction `json:"result"`
}

type ErrorResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

func NewMockTonCenter() *MockTonCenter {
	return &MockTonCenter{
		transactions: make([]Transaction, 0),
		currentBlock: 1000,
	}
}

// Health check endpoint
func (m *MockTonCenter) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "healthy",
		"service": "mock-toncenter",
	})
}

// Get transactions endpoint (mock TON Center API v2)
func (m *MockTonCenter) handleGetTransactions(w http.ResponseWriter, r *http.Request) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Simulate circuit breaker test
	if m.circuitBreakerTest {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{
			OK:    false,
			Error: "simulated_failure",
		})
		return
	}

	// Simulate failure mode
	if m.failureMode {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(ErrorResponse{
			OK:    false,
			Error: "service_unavailable",
		})
		return
	}

	// Return mock transactions
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(GetTransactionsResponse{
		OK:     true,
		Result: m.transactions,
	})
}

// Add test transaction
func (m *MockTonCenter) handleAddTransaction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var tx Transaction
	if err := json.NewDecoder(r.Body).Decode(&tx); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{
			OK:    false,
			Error: err.Error(),
		})
		return
	}

	m.mu.Lock()
	m.transactions = append(m.transactions, tx)
	m.currentBlock++
	m.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":      true,
		"message": "transaction added",
		"hash":    tx.Hash,
	})
}

// Enable/disable failure mode
func (m *MockTonCenter) handleSetFailureMode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	m.mu.Lock()
	m.failureMode = req.Enabled
	m.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":           true,
		"failure_mode": req.Enabled,
	})
}

// Enable circuit breaker test mode
func (m *MockTonCenter) handleSetCircuitBreakerTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	m.mu.Lock()
	m.circuitBreakerTest = req.Enabled
	m.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":                   true,
		"circuit_breaker_test": req.Enabled,
	})
}

// Clear all transactions
func (m *MockTonCenter) handleClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	m.mu.Lock()
	m.transactions = make([]Transaction, 0)
	m.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":      true,
		"message": "transactions cleared",
	})
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}

	mock := NewMockTonCenter()

	// API routes
	http.HandleFunc("/health", mock.handleHealth)
	http.HandleFunc("/getTransactions", mock.handleGetTransactions)

	// Test control routes
	http.HandleFunc("/test/add-transaction", mock.handleAddTransaction)
	http.HandleFunc("/test/set-failure-mode", mock.handleSetFailureMode)
	http.HandleFunc("/test/set-circuit-breaker-test", mock.handleSetCircuitBreakerTest)
	http.HandleFunc("/test/clear", mock.handleClear)

	addr := fmt.Sprintf(":%s", port)
	log.Printf("Mock TON Center starting on %s", addr)

	server := &http.Server{
		Addr:         addr,
		Handler:      http.DefaultServeMux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
