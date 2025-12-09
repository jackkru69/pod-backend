package toncenter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sony/gobreaker"
)

// Client wraps TON Center API communication with circuit breaker pattern.
// Implements resilient HTTP polling for blockchain data (FR-019).
type Client struct {
	v2BaseURL       string
	httpClient      *http.Client
	circuitBreaker  *gobreaker.CircuitBreaker
	contractAddress string
}

// ClientConfig holds configuration for TON Center client.
type ClientConfig struct {
	V2BaseURL             string
	ContractAddress       string
	CircuitBreakerMaxFail int
	CircuitBreakerTimeout time.Duration
	HTTPTimeout           time.Duration
}

// NewClient creates a new TON Center API client with circuit breaker.
func NewClient(cfg ClientConfig) *Client {
	cbSettings := gobreaker.Settings{
		Name:        "TONCenterAPI",
		MaxRequests: uint32(cfg.CircuitBreakerMaxFail),
		Timeout:     cfg.CircuitBreakerTimeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= uint32(cfg.CircuitBreakerMaxFail)
		},
	}

	return &Client{
		v2BaseURL:       cfg.V2BaseURL,
		contractAddress: cfg.ContractAddress,
		circuitBreaker:  gobreaker.NewCircuitBreaker(cbSettings),
		httpClient: &http.Client{
			Timeout: cfg.HTTPTimeout,
		},
	}
}

// Transaction represents a TON blockchain transaction from TON Center API.
// The API returns raw transaction data from the blockchain.
// The Data field contains base64-encoded BOC (Bag of Cells) format that needs parsing.
type Transaction struct {
	Type          string `json:"@type"` // Transaction type (e.g., "raw.transaction")
	TransactionID struct {
		Type string `json:"@type"` // internal.transactionId
		Lt   string `json:"lt"`    // Logical time
		Hash string `json:"hash"`  // Transaction hash (base64)
	} `json:"transaction_id"`
	Utime      int64           `json:"utime"`       // Unix timestamp
	Data       string          `json:"data"`        // Base64-encoded BOC transaction data
	InMsg      json.RawMessage `json:"in_msg"`      // Incoming message data
	OutMsgs    json.RawMessage `json:"out_msgs"`    // Outgoing messages data
	Fee        string          `json:"fee"`         // Transaction fee in nanotons
	StorageFee string          `json:"storage_fee"` // Storage fee in nanotons
	OtherFee   string          `json:"other_fee"`   // Other fees in nanotons
	Address    json.RawMessage `json:"address"`     // Account address info
}

// Hash returns the transaction hash for convenience
func (t *Transaction) Hash() string {
	return t.TransactionID.Hash
}

// Lt returns the logical time for convenience
func (t *Transaction) Lt() string {
	return t.TransactionID.Lt
}

// GetTransactions retrieves transactions for the contract starting from a specific block.
// Uses circuit breaker to prevent cascading failures.
func (c *Client) GetTransactions(ctx context.Context, fromBlock int64, limit int) ([]Transaction, error) {
	result, err := c.circuitBreaker.Execute(func() (interface{}, error) {
		return c.doGetTransactions(ctx, fromBlock, limit)
	})

	if err != nil {
		return nil, fmt.Errorf("circuit breaker: %w", err)
	}

	return result.([]Transaction), nil
}

// doGetTransactions performs the actual HTTP request to TON Center API v2 (REST).
func (c *Client) doGetTransactions(ctx context.Context, fromBlock int64, limit int) ([]Transaction, error) {
	// TON Center API v2 uses REST format with /getTransactions endpoint
	// The base URL should not include /api/v2/ as it's added here
	// Note: We omit to_lt to fetch the latest transactions (TON API returns newest first)
	// Filtering by lastProcessedLt is done in poller.go after fetching
	url := fmt.Sprintf("%s/getTransactions?address=%s&limit=%d&archival=true",
		c.v2BaseURL,
		c.contractAddress,
		limit,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	// TON Center returns wrapped response with ok and result fields
	var response struct {
		OK     bool          `json:"ok"`
		Result []Transaction `json:"result"`
		Error  string        `json:"error,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("decode transactions: %w", err)
	}

	if !response.OK {
		return nil, fmt.Errorf("API error: %s", response.Error)
	}

	return response.Result, nil
}

// GetCircuitBreakerState returns the current state of the circuit breaker.
// States: StateClosed (normal), StateHalfOpen (testing), StateOpen (failing).
func (c *Client) GetCircuitBreakerState() gobreaker.State {
	return c.circuitBreaker.State()
}
