package postgres

import (
	"context"
	"fmt"

	"pod-backend/internal/entity"
	"pod-backend/internal/repository"
	"pod-backend/pkg/postgres"
)

// Ensure GameEventRepository implements repository.GameEventRepository interface
var _ repository.GameEventRepository = (*GameEventRepository)(nil)

// GameEventRepository implements repository.GameEventRepository using PostgreSQL.
type GameEventRepository struct {
	pg *postgres.Postgres
}

// NewGameEventRepository creates a new PostgreSQL-backed game event repository.
func NewGameEventRepository(pg *postgres.Postgres) *GameEventRepository {
	return &GameEventRepository{pg: pg}
}

// Create creates a new game event record.
func (r *GameEventRepository) Create(ctx context.Context, event *entity.GameEvent) error {
	if err := event.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	sql, args, err := r.pg.Builder.
		Insert("game_events").
		Columns(
			"game_id",
			"event_type",
			"transaction_hash",
			"block_number",
			"timestamp",
			"payload",
		).
		Values(
			event.GameID,
			event.EventType,
			event.TransactionHash,
			event.BlockNumber,
			event.Timestamp,
			event.Payload,
		).
		Suffix("RETURNING id, created_at").
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	err = r.pg.Pool.QueryRow(ctx, sql, args...).Scan(&event.ID, &event.CreatedAt)
	if err != nil {
		return fmt.Errorf("execute query: %w", err)
	}

	return nil
}

// Upsert creates a new game event or ignores if already exists (idempotent operation).
func (r *GameEventRepository) Upsert(ctx context.Context, event *entity.GameEvent) error {
	return r.UpsertWithQuerier(ctx, r.pg.Pool, event)
}

// UpsertWithQuerier performs Upsert using the provided Querier (for transaction support).
func (r *GameEventRepository) UpsertWithQuerier(ctx context.Context, q repository.Querier, event *entity.GameEvent) error {
	if err := event.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	sql, args, err := r.pg.Builder.
		Insert("game_events").
		Columns(
			"game_id",
			"event_type",
			"transaction_hash",
			"block_number",
			"timestamp",
			"payload",
		).
		Values(
			event.GameID,
			event.EventType,
			event.TransactionHash,
			event.BlockNumber,
			event.Timestamp,
			event.Payload,
		).
		Suffix("ON CONFLICT ON CONSTRAINT unique_event_per_tx DO NOTHING RETURNING id, created_at").
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	// QueryRow will return no rows if ON CONFLICT DO NOTHING triggered (duplicate)
	// This is expected and not an error
	err = q.QueryRow(ctx, sql, args...).Scan(&event.ID, &event.CreatedAt)
	if err != nil && err.Error() != "no rows in result set" {
		return fmt.Errorf("execute query: %w", err)
	}

	return nil
}

// GetByGameID retrieves all events for a specific game.
func (r *GameEventRepository) GetByGameID(ctx context.Context, gameID int64) ([]*entity.GameEvent, error) {
	sql, args, err := r.pg.Builder.
		Select(
			"id",
			"game_id",
			"event_type",
			"transaction_hash",
			"block_number",
			"timestamp",
			"payload",
			"created_at",
		).
		From("game_events").
		Where("game_id = ?", gameID).
		OrderBy("timestamp ASC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	return r.queryEvents(ctx, sql, args...)
}

// GetByTransactionHash retrieves the event by transaction hash.
func (r *GameEventRepository) GetByTransactionHash(ctx context.Context, txHash string) (*entity.GameEvent, error) {
	sql, args, err := r.pg.Builder.
		Select(
			"id",
			"game_id",
			"event_type",
			"transaction_hash",
			"block_number",
			"timestamp",
			"payload",
			"created_at",
		).
		From("game_events").
		Where("transaction_hash = ?", txHash).
		Limit(1).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	event := &entity.GameEvent{}
	err = r.pg.Pool.QueryRow(ctx, sql, args...).Scan(
		&event.ID,
		&event.GameID,
		&event.EventType,
		&event.TransactionHash,
		&event.BlockNumber,
		&event.Timestamp,
		&event.Payload,
		&event.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("execute query: %w", err)
	}

	return event, nil
}

// GetByEventType retrieves all events of a specific type.
func (r *GameEventRepository) GetByEventType(ctx context.Context, eventType string) ([]*entity.GameEvent, error) {
	sql, args, err := r.pg.Builder.
		Select(
			"id",
			"game_id",
			"event_type",
			"transaction_hash",
			"block_number",
			"timestamp",
			"payload",
			"created_at",
		).
		From("game_events").
		Where("event_type = ?", eventType).
		OrderBy("timestamp ASC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	return r.queryEvents(ctx, sql, args...)
}

// GetByBlockRange retrieves all events within a block number range.
func (r *GameEventRepository) GetByBlockRange(ctx context.Context, startBlock, endBlock int64) ([]*entity.GameEvent, error) {
	sql, args, err := r.pg.Builder.
		Select(
			"id",
			"game_id",
			"event_type",
			"transaction_hash",
			"block_number",
			"timestamp",
			"payload",
			"created_at",
		).
		From("game_events").
		Where("block_number >= ? AND block_number <= ?", startBlock, endBlock).
		OrderBy("block_number ASC, timestamp ASC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	return r.queryEvents(ctx, sql, args...)
}

// queryEvents is a helper to execute queries returning multiple events.
func (r *GameEventRepository) queryEvents(ctx context.Context, sql string, args ...interface{}) ([]*entity.GameEvent, error) {
	rows, err := r.pg.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("execute query: %w", err)
	}
	defer rows.Close()

	var events []*entity.GameEvent
	for rows.Next() {
		event := &entity.GameEvent{}
		err = rows.Scan(
			&event.ID,
			&event.GameID,
			&event.EventType,
			&event.TransactionHash,
			&event.BlockNumber,
			&event.Timestamp,
			&event.Payload,
			&event.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		events = append(events, event)
	}

	return events, nil
}

// Exists checks if an event already exists.
func (r *GameEventRepository) Exists(ctx context.Context, gameID int64, txHash string, eventType string) (bool, error) {
	sql, args, err := r.pg.Builder.
		Select("COUNT(*)").
		From("game_events").
		Where("game_id = ? AND transaction_hash = ? AND event_type = ?", gameID, txHash, eventType).
		ToSql()
	if err != nil {
		return false, fmt.Errorf("build query: %w", err)
	}

	var count int
	err = r.pg.Pool.QueryRow(ctx, sql, args...).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("execute query: %w", err)
	}

	return count > 0, nil
}

// ExistsByTxHash checks if an event with the given transaction hash already exists.
func (r *GameEventRepository) ExistsByTxHash(ctx context.Context, txHash string) (bool, error) {
	sql, args, err := r.pg.Builder.
		Select("COUNT(*)").
		From("game_events").
		Where("transaction_hash = ?", txHash).
		ToSql()
	if err != nil {
		return false, fmt.Errorf("build query: %w", err)
	}

	var count int
	err = r.pg.Pool.QueryRow(ctx, sql, args...).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("execute query: %w", err)
	}

	return count > 0, nil
}

// GetLatestByGameID retrieves the most recent event for a specific game.
func (r *GameEventRepository) GetLatestByGameID(ctx context.Context, gameID int64) (*entity.GameEvent, error) {
	sql, args, err := r.pg.Builder.
		Select(
			"id",
			"game_id",
			"event_type",
			"transaction_hash",
			"block_number",
			"timestamp",
			"payload",
			"created_at",
		).
		From("game_events").
		Where("game_id = ?", gameID).
		OrderBy("timestamp DESC").
		Limit(1).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	event := &entity.GameEvent{}
	err = r.pg.Pool.QueryRow(ctx, sql, args...).Scan(
		&event.ID,
		&event.GameID,
		&event.EventType,
		&event.TransactionHash,
		&event.BlockNumber,
		&event.Timestamp,
		&event.Payload,
		&event.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("execute query: %w", err)
	}

	return event, nil
}
