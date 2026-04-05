package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"pod-backend/internal/entity"
	"pod-backend/internal/repository"
	"pod-backend/pkg/postgres"
)

// Ensure BlockchainSyncStateRepository implements repository.BlockchainSyncStateRepository interface
var _ repository.BlockchainSyncStateRepository = (*BlockchainSyncStateRepository)(nil)

// BlockchainSyncStateRepository implements repository.BlockchainSyncStateRepository using PostgreSQL.
// This is a singleton repository (only one row exists in the database with id=1).
type BlockchainSyncStateRepository struct {
	pg *postgres.Postgres
}

// NewBlockchainSyncStateRepository creates a new PostgreSQL-backed blockchain sync state repository.
func NewBlockchainSyncStateRepository(pg *postgres.Postgres) *BlockchainSyncStateRepository {
	return &BlockchainSyncStateRepository{pg: pg}
}

// Get retrieves the current blockchain sync state.
func (r *BlockchainSyncStateRepository) Get(ctx context.Context) (*entity.BlockchainSyncState, error) {
	sql, args, err := r.pg.Builder.
		Select(
			"id",
			"contract_address",
			"last_processed_block",
			"last_poll_timestamp",
			"updated_at",
			"COALESCE(event_source_type::text, 'http')",
			"COALESCE(last_processed_lt, '0')",
			"COALESCE(last_processed_hash, '')",
			"COALESCE(websocket_connected, false)",
			"COALESCE(fallback_count, 0)",
			"last_fallback_at",
		).
		From("blockchain_sync_state").
		Where("id = ?", 1).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	state := &entity.BlockchainSyncState{}
	err = r.pg.Pool.QueryRow(ctx, sql, args...).Scan(
		&state.ID,
		&state.ContractAddress,
		&state.LastProcessedBlock,
		&state.LastPollTimestamp,
		&state.UpdatedAt,
		&state.EventSourceType,
		&state.LastProcessedLt,
		&state.LastProcessedHash,
		&state.WebSocketConnected,
		&state.FallbackCount,
		&state.LastFallbackAt,
	)
	if err != nil {
		return nil, fmt.Errorf("execute query: %w", err)
	}

	return state, nil
}

// UpdateLastProcessedBlock atomically updates the last processed block number.
func (r *BlockchainSyncStateRepository) UpdateLastProcessedBlock(ctx context.Context, blockNumber int64) error {
	now := time.Now()
	sql, args, err := r.pg.Builder.
		Update("blockchain_sync_state").
		Set("last_processed_block", blockNumber).
		Set("last_poll_timestamp", now).
		Where("id = ?", 1).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = r.pg.Pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("execute query: %w", err)
	}

	return nil
}

// Initialize sets up the blockchain sync state for a contract address.
func (r *BlockchainSyncStateRepository) Initialize(ctx context.Context, contractAddress string, startingBlock int64) error {
	// Try to update first (idempotent operation)
	sql, args, err := r.pg.Builder.
		Update("blockchain_sync_state").
		Set("contract_address", contractAddress).
		Set("last_processed_block", startingBlock).
		Where("id = ?", 1).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	result, err := r.pg.Pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("execute query: %w", err)
	}

	// If no rows updated, the singleton row doesn't exist (shouldn't happen with migration)
	if result.RowsAffected() == 0 {
		return fmt.Errorf("blockchain_sync_state singleton row not found (id=1)")
	}

	return nil
}

// GetLastProcessedBlock returns just the last processed block number.
func (r *BlockchainSyncStateRepository) GetLastProcessedBlock(ctx context.Context) (int64, error) {
	sql, args, err := r.pg.Builder.
		Select("last_processed_block").
		From("blockchain_sync_state").
		Where("id = ?", 1).
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("build query: %w", err)
	}

	var blockNumber int64
	err = r.pg.Pool.QueryRow(ctx, sql, args...).Scan(&blockNumber)
	if err != nil {
		return 0, fmt.Errorf("execute query: %w", err)
	}

	return blockNumber, nil
}

// GetEventSourceType returns the current event source type (T147).
func (r *BlockchainSyncStateRepository) GetEventSourceType(ctx context.Context) (repository.EventSourceType, error) {
	sql, args, err := r.pg.Builder.
		Select("COALESCE(event_source_type::text, 'http')").
		From("blockchain_sync_state").
		Where("id = ?", 1).
		ToSql()
	if err != nil {
		return "", fmt.Errorf("build query: %w", err)
	}

	var sourceType string
	err = r.pg.Pool.QueryRow(ctx, sql, args...).Scan(&sourceType)
	if err != nil {
		return "", fmt.Errorf("execute query: %w", err)
	}

	return repository.EventSourceType(sourceType), nil
}

// SetEventSourceType updates the event source type and WebSocket connection status (T147).
func (r *BlockchainSyncStateRepository) SetEventSourceType(ctx context.Context, sourceType repository.EventSourceType, connected bool) error {
	sql, args, err := r.pg.Builder.
		Update("blockchain_sync_state").
		Set("event_source_type", string(sourceType)).
		Set("websocket_connected", connected).
		Where("id = ?", 1).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = r.pg.Pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("execute query: %w", err)
	}

	return nil
}

// UpdateLastProcessedLt atomically updates the last processed logical time (T147).
func (r *BlockchainSyncStateRepository) UpdateLastProcessedLt(ctx context.Context, lt string) error {
	if strings.TrimSpace(lt) == "" {
		lt = "0"
	}

	now := time.Now()
	const query = `
		UPDATE blockchain_sync_state
		SET
			last_processed_lt = CASE
				WHEN COALESCE(NULLIF(last_processed_lt, ''), '0')::numeric <= $1::numeric THEN $1::varchar
				ELSE COALESCE(NULLIF(last_processed_lt, ''), '0')
			END,
			last_poll_timestamp = CASE
				WHEN COALESCE(NULLIF(last_processed_lt, ''), '0')::numeric <= $1::numeric THEN $2
				ELSE last_poll_timestamp
			END
		WHERE id = 1
	`

	_, err := r.pg.Pool.Exec(ctx, query, lt, now)
	if err != nil {
		return fmt.Errorf("execute query: %w", err)
	}

	return nil
}

// PersistCheckpoint stores the resumable TON checkpoint together with source status.
func (r *BlockchainSyncStateRepository) PersistCheckpoint(
	ctx context.Context,
	lt string,
	sourceType repository.EventSourceType,
	connected bool,
) error {
	if strings.TrimSpace(lt) == "" {
		lt = "0"
	}

	if strings.TrimSpace(string(sourceType)) == "" {
		sourceType = repository.EventSourceHTTP
	}

	now := time.Now()
	const query = `
		UPDATE blockchain_sync_state
		SET
			last_processed_lt = CASE
				WHEN COALESCE(NULLIF(last_processed_lt, ''), '0')::numeric <= $1::numeric THEN $1::varchar
				ELSE COALESCE(NULLIF(last_processed_lt, ''), '0')
			END,
			last_poll_timestamp = CASE
				WHEN COALESCE(NULLIF(last_processed_lt, ''), '0')::numeric <= $1::numeric THEN $2
				ELSE last_poll_timestamp
			END,
			event_source_type = $3,
			websocket_connected = $4
		WHERE id = 1
	`

	_, err := r.pg.Pool.Exec(ctx, query, lt, now, string(sourceType), connected)
	if err != nil {
		return fmt.Errorf("execute query: %w", err)
	}

	return nil
}

// GetLastProcessedLt returns the last processed logical time (T147).
func (r *BlockchainSyncStateRepository) GetLastProcessedLt(ctx context.Context) (string, error) {
	sql, args, err := r.pg.Builder.
		Select("COALESCE(last_processed_lt, '0')").
		From("blockchain_sync_state").
		Where("id = ?", 1).
		ToSql()
	if err != nil {
		return "", fmt.Errorf("build query: %w", err)
	}

	var lt string
	err = r.pg.Pool.QueryRow(ctx, sql, args...).Scan(&lt)
	if err != nil {
		return "", fmt.Errorf("execute query: %w", err)
	}

	return lt, nil
}

// RecordFallback increments the fallback counter and sets the timestamp (T147).
func (r *BlockchainSyncStateRepository) RecordFallback(ctx context.Context) error {
	now := time.Now()
	const query = `
		UPDATE blockchain_sync_state
		SET
			fallback_count = COALESCE(fallback_count, 0) + 1,
			last_fallback_at = $1,
			event_source_type = $2,
			websocket_connected = false
		WHERE id = 1
	`

	_, err := r.pg.Pool.Exec(ctx, query, now, string(repository.EventSourceHTTP))
	if err != nil {
		return fmt.Errorf("execute query: %w", err)
	}

	return nil
}
