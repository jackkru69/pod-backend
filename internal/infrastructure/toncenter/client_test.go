package toncenter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestClient_GetTransactions_URLConstruction(t *testing.T) {
	// Setup mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify query parameters
		query := r.URL.Query()
		assert.Equal(t, "10", query.Get("limit"))
		assert.Equal(t, "EQD...", query.Get("address"))
		assert.Equal(t, "true", query.Get("archival"))

		// Check for optional parameters if provided
		if query.Get("lt") != "" {
			assert.Equal(t, "12345", query.Get("lt"))
		}
		if query.Get("hash") != "" {
			assert.Equal(t, "abc", query.Get("hash"))
		}

		// Return success response
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok": true, "result": []}`))
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

	// Test case 1: Basic call (existing behavior)
	// GetTransactions(ctx, limit, lt, hash)
	_, err := client.GetTransactions(context.Background(), 10, nil, nil)
	assert.NoError(t, err)

	// Test case 2: With lt and hash
	lt := uint64(12345)
	hash := "abc"
	_, err = client.GetTransactions(context.Background(), 10, &lt, &hash)
	assert.NoError(t, err)
}

func TestClient_GetTransactions_NormalizesOrderAndDuplicates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"ok": true,
			"result": [
				{"transaction_id":{"lt":"100","hash":"hash_100"}},
				{"transaction_id":{"lt":"300","hash":"hash_300"}},
				{"transaction_id":{"lt":"200","hash":"hash_200"}},
				{"transaction_id":{"lt":"300","hash":"hash_300"}}
			]
		}`))
	}))
	defer server.Close()

	cfg := ClientConfig{
		V2BaseURL:             server.URL,
		ContractAddress:       "EQD...",
		CircuitBreakerMaxFail: 5,
		CircuitBreakerTimeout: time.Second,
		HTTPTimeout:           time.Second,
	}
	client := NewClient(cfg)

	txs, err := client.GetTransactions(context.Background(), 10, nil, nil)
	assert.NoError(t, err)
	assert.Len(t, txs, 3)
	assert.Equal(t, []string{"300", "200", "100"}, []string{txs[0].Lt(), txs[1].Lt(), txs[2].Lt()})
}
