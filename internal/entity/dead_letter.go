package entity

import (
	"fmt"
	"time"
)

// DLQErrorType represents the type of error that caused the DLQ entry.
type DLQErrorType string

const (
	DLQErrorTypeParse       DLQErrorType = "parse_error"
	DLQErrorTypePersistence DLQErrorType = "persistence_error"
	DLQErrorTypeValidation  DLQErrorType = "validation_error"
	DLQErrorTypeUnknown     DLQErrorType = "unknown"
)

// DLQStatus represents the status of a DLQ entry.
type DLQStatus string

const (
	DLQStatusPending  DLQStatus = "pending"
	DLQStatusRetrying DLQStatus = "retrying"
	DLQStatusResolved DLQStatus = "resolved"
	DLQStatusFailed   DLQStatus = "failed"
)

// DeadLetterEntry represents a failed blockchain transaction stored in the DLQ.
type DeadLetterEntry struct {
	ID              int64        `json:"id"`
	TransactionHash string       `json:"transaction_hash"`
	TransactionLt   string       `json:"transaction_lt"`
	RawData         string       `json:"raw_data"`
	ErrorMessage    string       `json:"error_message"`
	ErrorType       DLQErrorType `json:"error_type"`
	RetryCount      int          `json:"retry_count"`
	MaxRetries      int          `json:"max_retries"`
	CreatedAt       time.Time    `json:"created_at"`
	LastRetryAt     *time.Time   `json:"last_retry_at,omitempty"`
	NextRetryAt     *time.Time   `json:"next_retry_at,omitempty"`
	ResolvedAt      *time.Time   `json:"resolved_at,omitempty"`
	Status          DLQStatus    `json:"status"`
	ResolutionNotes string       `json:"resolution_notes,omitempty"`
}

// Validate validates the DeadLetterEntry entity.
func (e *DeadLetterEntry) Validate() error {
	if e.TransactionHash == "" {
		return fmt.Errorf("transaction_hash is required")
	}
	if e.TransactionLt == "" {
		return fmt.Errorf("transaction_lt is required")
	}
	if e.ErrorMessage == "" {
		return fmt.Errorf("error_message is required")
	}
	if e.ErrorType == "" {
		e.ErrorType = DLQErrorTypeUnknown
	}
	if e.Status == "" {
		e.Status = DLQStatusPending
	}
	if e.MaxRetries <= 0 {
		e.MaxRetries = 3
	}
	return nil
}

// CanRetry returns true if the entry can be retried.
func (e *DeadLetterEntry) CanRetry() bool {
	return e.Status != DLQStatusResolved &&
		e.Status != DLQStatusFailed &&
		e.RetryCount < e.MaxRetries
}

// MarkRetrying updates the entry for a retry attempt.
func (e *DeadLetterEntry) MarkRetrying() {
	e.Status = DLQStatusRetrying
	now := time.Now()
	e.LastRetryAt = &now
	e.RetryCount++
}

// MarkResolved marks the entry as successfully resolved.
func (e *DeadLetterEntry) MarkResolved(notes string) {
	e.Status = DLQStatusResolved
	now := time.Now()
	e.ResolvedAt = &now
	e.ResolutionNotes = notes
}

// MarkFailed marks the entry as permanently failed (max retries exceeded).
func (e *DeadLetterEntry) MarkFailed(notes string) {
	e.Status = DLQStatusFailed
	e.ResolutionNotes = notes
}

// ScheduleNextRetry calculates the next retry time with exponential backoff.
func (e *DeadLetterEntry) ScheduleNextRetry(baseBackoff time.Duration) {
	// Exponential backoff: baseBackoff * 2^retryCount
	multiplier := 1 << e.RetryCount // 2^retryCount
	backoff := baseBackoff * time.Duration(multiplier)
	
	// Cap at 1 hour
	maxBackoff := time.Hour
	if backoff > maxBackoff {
		backoff = maxBackoff
	}
	
	nextRetry := time.Now().Add(backoff)
	e.NextRetryAt = &nextRetry
}
