package toncenter

import (
	"context"
	"encoding/json"
	"testing"
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
		lastProcessedLt: "0",
	}

	tests := []struct {
		name        string
		txLt        string
		expectedLt  string
		shouldUpdate bool
	}{
		{
			name:        "first transaction",
			txLt:        "100",
			expectedLt:  "100",
			shouldUpdate: true,
		},
		{
			name:        "newer transaction",
			txLt:        "200",
			expectedLt:  "200",
			shouldUpdate: true,
		},
		{
			name:        "older transaction - should not update",
			txLt:        "150",
			expectedLt:  "200", // Should stay at 200
			shouldUpdate: false,
		},
		{
			name:        "same lt - should not update",
			txLt:        "200",
			expectedLt:  "200",
			shouldUpdate: false,
		},
		{
			name:        "bug case: string 9 vs numeric 10",
			txLt:        "9",
			expectedLt:  "200", // Should NOT update because 9 < 200
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
