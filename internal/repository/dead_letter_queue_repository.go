package repository

import (
	"context"
	"time"

	"pod-backend/internal/entity"
)

// DeadLetterQueueRepository defines the interface for dead letter queue persistence.
// Stores failed blockchain transactions for retry and analysis (Issue #6).
type DeadLetterQueueRepository interface {
	// Create stores a new failed transaction in the DLQ.
	Create(ctx context.Context, entry *entity.DeadLetterEntry) error

	// GetByID retrieves a DLQ entry by ID.
	GetByID(ctx context.Context, id int64) (*entity.DeadLetterEntry, error)

	// GetByTransactionHash retrieves a DLQ entry by transaction hash and lt.
	// Returns nil if not found.
	GetByTransactionHash(ctx context.Context, hash, lt string) (*entity.DeadLetterEntry, error)

	// GetPendingForRetry retrieves entries ready for retry (next_retry_at <= now).
	GetPendingForRetry(ctx context.Context, limit int) ([]*entity.DeadLetterEntry, error)

	// GetByStatus retrieves entries with a specific status.
	GetByStatus(ctx context.Context, status entity.DLQStatus, limit, offset int) ([]*entity.DeadLetterEntry, error)

	// Update updates an existing DLQ entry.
	Update(ctx context.Context, entry *entity.DeadLetterEntry) error

	// UpdateStatus updates the status and related fields of a DLQ entry.
	UpdateStatus(ctx context.Context, id int64, status entity.DLQStatus, notes string) error

	// IncrementRetryCount increments retry count and schedules next retry.
	IncrementRetryCount(ctx context.Context, id int64, nextRetryAt time.Time) error

	// Delete removes a DLQ entry (for cleanup of resolved entries).
	Delete(ctx context.Context, id int64) error

	// CountByStatus returns the count of entries by status.
	CountByStatus(ctx context.Context, status entity.DLQStatus) (int64, error)

	// GetStats returns DLQ statistics.
	GetStats(ctx context.Context) (*DLQStats, error)
}

// DLQStats contains statistics about the dead letter queue.
type DLQStats struct {
	PendingCount  int64 `json:"pending_count"`
	RetryingCount int64 `json:"retrying_count"`
	ResolvedCount int64 `json:"resolved_count"`
	FailedCount   int64 `json:"failed_count"`
	TotalCount    int64 `json:"total_count"`
	OldestPending *time.Time `json:"oldest_pending,omitempty"`
}
