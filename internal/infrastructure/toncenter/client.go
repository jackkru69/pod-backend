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
	V2BaseURL              string
	ContractAddress        string
	CircuitBreakerMaxFail  int
	CircuitBreakerTimeout  time.Duration
	HTTPTimeout            time.Duration
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

// Transaction represents a simplified TON blockchain transaction.
type Transaction struct {
	Hash        string          `json:"hash"`
	Lt          string          `json:"lt"`
	BlockNumber int64           `json:"block_number"`
	Timestamp   int64           `json:"utime"`
	Data        json.RawMessage `json:"data"` // Raw event data
}

// TransactionsResponse represents the API response for transaction queries.
type TransactionsResponse struct {
	OK          bool          `json:"ok"`
	Result      []Transaction `json:"result"`
	Error       string        `json:"error,omitempty"`
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

// doGetTransactions performs the actual HTTP request to TON Center API.
func (c *Client) doGetTransactions(ctx context.Context, fromBlock int64, limit int) ([]Transaction, error) {
	url := fmt.Sprintf("%s/api/v2/getTransactions?address=%s&limit=%d&from_block=%d",
		c.v2BaseURL,
		c.contractAddress,
		limit,
		fromBlock,
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

	var apiResp TransactionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if !apiResp.OK {
		return nil, fmt.Errorf("api error: %s", apiResp.Error)
	}

	return apiResp.Result, nil
}

// GetCircuitBreakerState returns the current state of the circuit breaker.
// States: StateClosed (normal), StateHalfOpen (testing), StateOpen (failing).
func (c *Client) GetCircuitBreakerState() gobreaker.State {
	return c.circuitBreaker.State()
}
