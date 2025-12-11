package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"pod-backend/internal/entity"
	"pod-backend/internal/repository"
	"pod-backend/pkg/postgres"
)

// Ensure DeadLetterQueueRepository implements repository.DeadLetterQueueRepository
var _ repository.DeadLetterQueueRepository = (*DeadLetterQueueRepository)(nil)

// DeadLetterQueueRepository implements repository.DeadLetterQueueRepository using PostgreSQL.
type DeadLetterQueueRepository struct {
	pg *postgres.Postgres
}

// NewDeadLetterQueueRepository creates a new PostgreSQL-backed DLQ repository.
func NewDeadLetterQueueRepository(pg *postgres.Postgres) *DeadLetterQueueRepository {
	return &DeadLetterQueueRepository{pg: pg}
}

// Create stores a new failed transaction in the DLQ.
func (r *DeadLetterQueueRepository) Create(ctx context.Context, entry *entity.DeadLetterEntry) error {
	if err := entry.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	sql, args, err := r.pg.Builder.
		Insert("dead_letter_queue").
		Columns(
			"transaction_hash",
			"transaction_lt",
			"raw_data",
			"error_message",
			"error_type",
			"retry_count",
			"max_retries",
			"status",
			"next_retry_at",
		).
		Values(
			entry.TransactionHash,
			entry.TransactionLt,
			entry.RawData,
			entry.ErrorMessage,
			entry.ErrorType,
			entry.RetryCount,
			entry.MaxRetries,
			entry.Status,
			entry.NextRetryAt,
		).
		Suffix("ON CONFLICT (transaction_hash, transaction_lt) WHERE status IN ('pending', 'retrying') DO NOTHING").
		Suffix("RETURNING id, created_at").
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	err = r.pg.Pool.QueryRow(ctx, sql, args...).Scan(&entry.ID, &entry.CreatedAt)
	if err != nil {
		// Check if it was a conflict (no rows returned)
		if err == pgx.ErrNoRows {
			return nil // Entry already exists, not an error
		}
		return fmt.Errorf("execute query: %w", err)
	}

	return nil
}

// GetByID retrieves a DLQ entry by ID.
func (r *DeadLetterQueueRepository) GetByID(ctx context.Context, id int64) (*entity.DeadLetterEntry, error) {
	sql, args, err := r.pg.Builder.
		Select(
			"id", "transaction_hash", "transaction_lt", "raw_data", "error_message",
			"error_type", "retry_count", "max_retries", "created_at", "last_retry_at",
			"next_retry_at", "resolved_at", "status", "resolution_notes",
		).
		From("dead_letter_queue").
		Where("id = ?", id).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	entry := &entity.DeadLetterEntry{}
	err = r.pg.Pool.QueryRow(ctx, sql, args...).Scan(
		&entry.ID,
		&entry.TransactionHash,
		&entry.TransactionLt,
		&entry.RawData,
		&entry.ErrorMessage,
		&entry.ErrorType,
		&entry.RetryCount,
		&entry.MaxRetries,
		&entry.CreatedAt,
		&entry.LastRetryAt,
		&entry.NextRetryAt,
		&entry.ResolvedAt,
		&entry.Status,
		&entry.ResolutionNotes,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("execute query: %w", err)
	}

	return entry, nil
}

// GetByTransactionHash retrieves a DLQ entry by transaction hash and lt.
func (r *DeadLetterQueueRepository) GetByTransactionHash(ctx context.Context, hash, lt string) (*entity.DeadLetterEntry, error) {
	sql, args, err := r.pg.Builder.
		Select(
			"id", "transaction_hash", "transaction_lt", "raw_data", "error_message",
			"error_type", "retry_count", "max_retries", "created_at", "last_retry_at",
			"next_retry_at", "resolved_at", "status", "resolution_notes",
		).
		From("dead_letter_queue").
		Where("transaction_hash = ? AND transaction_lt = ?", hash, lt).
		Where("status IN ('pending', 'retrying')").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	entry := &entity.DeadLetterEntry{}
	err = r.pg.Pool.QueryRow(ctx, sql, args...).Scan(
		&entry.ID,
		&entry.TransactionHash,
		&entry.TransactionLt,
		&entry.RawData,
		&entry.ErrorMessage,
		&entry.ErrorType,
		&entry.RetryCount,
		&entry.MaxRetries,
		&entry.CreatedAt,
		&entry.LastRetryAt,
		&entry.NextRetryAt,
		&entry.ResolvedAt,
		&entry.Status,
		&entry.ResolutionNotes,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("execute query: %w", err)
	}

	return entry, nil
}

// GetPendingForRetry retrieves entries ready for retry.
func (r *DeadLetterQueueRepository) GetPendingForRetry(ctx context.Context, limit int) ([]*entity.DeadLetterEntry, error) {
	sql, args, err := r.pg.Builder.
		Select(
			"id", "transaction_hash", "transaction_lt", "raw_data", "error_message",
			"error_type", "retry_count", "max_retries", "created_at", "last_retry_at",
			"next_retry_at", "resolved_at", "status", "resolution_notes",
		).
		From("dead_letter_queue").
		Where("status IN ('pending', 'retrying')").
		Where("(next_retry_at IS NULL OR next_retry_at <= NOW())").
		Where("retry_count < max_retries").
		OrderBy("created_at ASC").
		Limit(uint64(limit)).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	rows, err := r.pg.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("execute query: %w", err)
	}
	defer rows.Close()

	var entries []*entity.DeadLetterEntry
	for rows.Next() {
		entry := &entity.DeadLetterEntry{}
		err = rows.Scan(
			&entry.ID,
			&entry.TransactionHash,
			&entry.TransactionLt,
			&entry.RawData,
			&entry.ErrorMessage,
			&entry.ErrorType,
			&entry.RetryCount,
			&entry.MaxRetries,
			&entry.CreatedAt,
			&entry.LastRetryAt,
			&entry.NextRetryAt,
			&entry.ResolvedAt,
			&entry.Status,
			&entry.ResolutionNotes,
		)
		if err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// GetByStatus retrieves entries with a specific status.
func (r *DeadLetterQueueRepository) GetByStatus(ctx context.Context, status entity.DLQStatus, limit, offset int) ([]*entity.DeadLetterEntry, error) {
	sql, args, err := r.pg.Builder.
		Select(
			"id", "transaction_hash", "transaction_lt", "raw_data", "error_message",
			"error_type", "retry_count", "max_retries", "created_at", "last_retry_at",
			"next_retry_at", "resolved_at", "status", "resolution_notes",
		).
		From("dead_letter_queue").
		Where("status = ?", status).
		OrderBy("created_at DESC").
		Limit(uint64(limit)).
		Offset(uint64(offset)).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	rows, err := r.pg.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("execute query: %w", err)
	}
	defer rows.Close()

	var entries []*entity.DeadLetterEntry
	for rows.Next() {
		entry := &entity.DeadLetterEntry{}
		err = rows.Scan(
			&entry.ID,
			&entry.TransactionHash,
			&entry.TransactionLt,
			&entry.RawData,
			&entry.ErrorMessage,
			&entry.ErrorType,
			&entry.RetryCount,
			&entry.MaxRetries,
			&entry.CreatedAt,
			&entry.LastRetryAt,
			&entry.NextRetryAt,
			&entry.ResolvedAt,
			&entry.Status,
			&entry.ResolutionNotes,
		)
		if err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// Update updates an existing DLQ entry.
func (r *DeadLetterQueueRepository) Update(ctx context.Context, entry *entity.DeadLetterEntry) error {
	sql, args, err := r.pg.Builder.
		Update("dead_letter_queue").
		Set("error_message", entry.ErrorMessage).
		Set("error_type", entry.ErrorType).
		Set("retry_count", entry.RetryCount).
		Set("last_retry_at", entry.LastRetryAt).
		Set("next_retry_at", entry.NextRetryAt).
		Set("resolved_at", entry.ResolvedAt).
		Set("status", entry.Status).
		Set("resolution_notes", entry.ResolutionNotes).
		Where("id = ?", entry.ID).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	result, err := r.pg.Pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("execute query: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("entry not found: %d", entry.ID)
	}

	return nil
}

// UpdateStatus updates the status and related fields of a DLQ entry.
func (r *DeadLetterQueueRepository) UpdateStatus(ctx context.Context, id int64, status entity.DLQStatus, notes string) error {
	builder := r.pg.Builder.
		Update("dead_letter_queue").
		Set("status", status).
		Set("resolution_notes", notes).
		Where("id = ?", id)

	// Set resolved_at for terminal states
	if status == entity.DLQStatusResolved || status == entity.DLQStatusFailed {
		builder = builder.Set("resolved_at", time.Now())
	}

	sql, args, err := builder.ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	result, err := r.pg.Pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("execute query: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("entry not found: %d", id)
	}

	return nil
}

// IncrementRetryCount increments retry count and schedules next retry.
func (r *DeadLetterQueueRepository) IncrementRetryCount(ctx context.Context, id int64, nextRetryAt time.Time) error {
	// Use raw SQL for retry_count increment
	sql := `UPDATE dead_letter_queue SET retry_count = retry_count + 1, last_retry_at = $1, next_retry_at = $2, status = $3 WHERE id = $4`
	result, err := r.pg.Pool.Exec(ctx, sql, time.Now(), nextRetryAt, entity.DLQStatusRetrying, id)
	if err != nil {
		return fmt.Errorf("execute query: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("entry not found: %d", id)
	}

	return nil
}

// Delete removes a DLQ entry.
func (r *DeadLetterQueueRepository) Delete(ctx context.Context, id int64) error {
	sql, args, err := r.pg.Builder.
		Delete("dead_letter_queue").
		Where("id = ?", id).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	result, err := r.pg.Pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("execute query: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("entry not found: %d", id)
	}

	return nil
}

// CountByStatus returns the count of entries by status.
func (r *DeadLetterQueueRepository) CountByStatus(ctx context.Context, status entity.DLQStatus) (int64, error) {
	sql, args, err := r.pg.Builder.
		Select("COUNT(*)").
		From("dead_letter_queue").
		Where("status = ?", status).
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("build query: %w", err)
	}

	var count int64
	err = r.pg.Pool.QueryRow(ctx, sql, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("execute query: %w", err)
	}

	return count, nil
}

// GetStats returns DLQ statistics.
func (r *DeadLetterQueueRepository) GetStats(ctx context.Context) (*repository.DLQStats, error) {
	// Get counts by status in a single query
	sql := `
		SELECT 
			COALESCE(SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END), 0) as pending,
			COALESCE(SUM(CASE WHEN status = 'retrying' THEN 1 ELSE 0 END), 0) as retrying,
			COALESCE(SUM(CASE WHEN status = 'resolved' THEN 1 ELSE 0 END), 0) as resolved,
			COALESCE(SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END), 0) as failed,
			COUNT(*) as total,
			MIN(CASE WHEN status IN ('pending', 'retrying') THEN created_at END) as oldest_pending
		FROM dead_letter_queue
	`

	stats := &repository.DLQStats{}
	err := r.pg.Pool.QueryRow(ctx, sql).Scan(
		&stats.PendingCount,
		&stats.RetryingCount,
		&stats.ResolvedCount,
		&stats.FailedCount,
		&stats.TotalCount,
		&stats.OldestPending,
	)
	if err != nil {
		return nil, fmt.Errorf("execute query: %w", err)
	}

	return stats, nil
}
