package toncenter

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCompareLt tests the compareLt function for correct numeric comparison.
func TestCompareLt(t *testing.T) {
	tests := []struct {
		name     string
		lt1      string
		lt2      string
		expected bool // true if lt1 > lt2
	}{
		{
			name:     "simple greater",
			lt1:      "10",
			lt2:      "9",
			expected: true,
		},
		{
			name:     "simple less",
			lt1:      "9",
			lt2:      "10",
			expected: false,
		},
		{
			name:     "equal values",
			lt1:      "100",
			lt2:      "100",
			expected: false,
		},
		{
			name:     "large number comparison - bug case",
			lt1:      "9",
			lt2:      "10",
			expected: false, // String comparison would return true (bug)
		},
		{
			name:     "very large numbers",
			lt1:      "1000000000000000001",
			lt2:      "1000000000000000000",
			expected: true,
		},
		{
			name:     "TON realistic lt values",
			lt1:      "47234567890123456789",
			lt2:      "47234567890123456788",
			expected: true,
		},
		{
			name:     "zero comparison",
			lt1:      "1",
			lt2:      "0",
			expected: true,
		},
		{
			name:     "both zero",
			lt1:      "0",
			lt2:      "0",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compareLt(tt.lt1, tt.lt2)
			if result != tt.expected {
				t.Errorf("compareLt(%q, %q) = %v, want %v", tt.lt1, tt.lt2, result, tt.expected)
			}
		})
	}
}

// TestWebSocketSubscriberLtUpdate tests that HandleTransaction correctly updates lastProcessedLt.
func TestWebSocketSubscriberLtUpdate(t *testing.T) {
	sub := &WebSocketSubscriber{
		lastProcessedLt: defaultLastProcessedLt,
	}

	tests := []struct {
		name         string
		txLt         string
		expectedLt   string
		shouldUpdate bool
	}{
		{
			name:         "first transaction",
			txLt:         "100",
			expectedLt:   "100",
			shouldUpdate: true,
		},
		{
			name:         "newer transaction",
			txLt:         "200",
			expectedLt:   "200",
			shouldUpdate: true,
		},
		{
			name:         "older transaction - should not update",
			txLt:         "150",
			expectedLt:   "200", // Should stay at 200
			shouldUpdate: false,
		},
		{
			name:         "same lt - should not update",
			txLt:         "200",
			expectedLt:   "200",
			shouldUpdate: false,
		},
		{
			name:         "bug case: string 9 vs numeric 10",
			txLt:         "9",
			expectedLt:   "200", // Should NOT update because 9 < 200
			shouldUpdate: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock transaction
			tx := createMockTransaction(tt.txLt)

			// Call HandleTransaction
			_ = sub.HandleTransaction(context.Background(), tx)

			// Verify lastProcessedLt
			if sub.lastProcessedLt != tt.expectedLt {
				t.Errorf("lastProcessedLt = %q, want %q", sub.lastProcessedLt, tt.expectedLt)
			}
		})
	}
}

type failingEventHandler struct {
	err error
}

func (h *failingEventHandler) HandleTransaction(context.Context, Transaction) error {
	return h.err
}

type recordingEventHandler struct {
	handledLt []string
}

func (h *recordingEventHandler) HandleTransaction(_ context.Context, tx Transaction) error {
	h.handledLt = append(h.handledLt, tx.Lt())
	return nil
}

func TestWebSocketSubscriber_HandleTransaction_PersistsOnlySuccessfulProgress(t *testing.T) {
	t.Run("updates checkpoint and callback after successful handler", func(t *testing.T) {
		sub := &WebSocketSubscriber{
			lastProcessedLt: defaultLastProcessedLt,
			handler:         &MockEventHandler{},
		}

		var updatedLt string
		sub.SetOnLtUpdated(func(lt string) {
			updatedLt = lt
		})

		err := sub.HandleTransaction(context.Background(), createMockTransaction("321"))
		if err != nil {
			t.Fatalf("HandleTransaction() error = %v", err)
		}

		if sub.GetLastProcessedLt() != "321" {
			t.Fatalf("lastProcessedLt = %q, want %q", sub.GetLastProcessedLt(), "321")
		}
		if updatedLt != "321" {
			t.Fatalf("updatedLt = %q, want %q", updatedLt, "321")
		}
	})

	t.Run("does not advance checkpoint when handler fails", func(t *testing.T) {
		sub := &WebSocketSubscriber{
			lastProcessedLt: "500",
			handler:         &failingEventHandler{err: errors.New("boom")},
		}

		callbackCalled := false
		sub.SetOnLtUpdated(func(string) {
			callbackCalled = true
		})

		err := sub.HandleTransaction(context.Background(), createMockTransaction("600"))
		if err == nil {
			t.Fatal("expected handler error")
		}
		if sub.GetLastProcessedLt() != "500" {
			t.Fatalf("lastProcessedLt = %q, want %q", sub.GetLastProcessedLt(), "500")
		}
		if callbackCalled {
			t.Fatal("callback should not be called when handler fails")
		}
	})
}

func TestWebSocketSubscriber_HandleTransaction_SkipsDuplicateOrOlderTransactions(t *testing.T) {
	handler := &recordingEventHandler{}
	sub := &WebSocketSubscriber{
		lastProcessedLt:   "500",
		lastProcessedHash: "test_hash_500",
		handler:           handler,
		logger:            &MockLogger{},
	}

	var updatedLt []string
	sub.SetOnLtUpdated(func(lt string) {
		updatedLt = append(updatedLt, lt)
	})

	err := sub.HandleTransaction(context.Background(), createMockTransaction("499"))
	assert.NoError(t, err)

	err = sub.HandleTransaction(context.Background(), createMockTransaction("500"))
	assert.NoError(t, err)

	err = sub.HandleTransaction(context.Background(), createMockTransaction("501"))
	assert.NoError(t, err)

	assert.Equal(t, []string{"501"}, handler.handledLt)
	assert.Equal(t, []string{"501"}, updatedLt)
	assert.Equal(t, "501", sub.GetLastProcessedLt())
}

func TestWebSocketSubscriber_HandleTransaction_SkipsSameLtDifferentHash(t *testing.T) {
	handler := &recordingEventHandler{}
	sub := &WebSocketSubscriber{
		lastProcessedLt:   "500",
		lastProcessedHash: "hash_original",
		handler:           handler,
		logger:            &MockLogger{},
	}

	callbackCalled := false
	sub.SetOnLtUpdated(func(string) {
		callbackCalled = true
	})

	tx := createMockTransaction("500")
	tx.TransactionID.Hash = "hash_conflict"

	err := sub.HandleTransaction(context.Background(), tx)
	assert.NoError(t, err)
	assert.Empty(t, handler.handledLt)
	assert.False(t, callbackCalled)
	assert.Equal(t, "500", sub.GetLastProcessedLt())
}

// TestStringComparisonBug demonstrates why string comparison is wrong.
func TestStringComparisonBug(t *testing.T) {
	// This test demonstrates the bug that was fixed

	// String comparison (WRONG):
	// "9" > "10" is TRUE because '9' > '1' in ASCII
	stringResult := "9" > "10"
	if !stringResult {
		t.Error("Expected string comparison '9' > '10' to be true (demonstrating the bug)")
	}

	// Numeric comparison (CORRECT):
	// 9 > 10 is FALSE
	numericResult := compareLt("9", "10")
	if numericResult {
		t.Error("compareLt('9', '10') should return false")
	}
}

// createMockTransaction creates a mock Transaction for testing.
func createMockTransaction(lt string) Transaction {
	return Transaction{
		TransactionID: struct {
			Type string `json:"@type"`
			Lt   string `json:"lt"`
			Hash string `json:"hash"`
		}{
			Type: "internal.transactionId",
			Lt:   lt,
			Hash: "test_hash_" + lt,
		},
		Utime: 1234567890,
		InMsg: json.RawMessage(`{}`),
	}
}
