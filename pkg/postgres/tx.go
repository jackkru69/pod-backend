// Package postgres provides PostgreSQL transaction management utilities.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"pod-backend/internal/repository"
)

// Querier is an interface that both pgxpool.Pool and pgx.Tx implement.
// This allows repository methods to work with both regular connections and transactions.
type Querier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Ensure pgxpool.Pool implements Querier
var _ Querier = (*pgxpool.Pool)(nil)

// Ensure pgx.Tx implements Querier
var _ Querier = (pgx.Tx)(nil)

// TxManager provides transaction management functionality.
// It wraps operations in transactions with automatic commit/rollback.
type TxManager struct {
	pool *pgxpool.Pool
}

// Ensure TxManager implements repository.TxManager
var _ repository.TxManager = (*TxManager)(nil)

// NewTxManager creates a new transaction manager.
func NewTxManager(pool *pgxpool.Pool) *TxManager {
	return &TxManager{pool: pool}
}

// WithTx executes the given function within a transaction.
// If the function returns an error, the transaction is rolled back.
// Otherwise, the transaction is committed.
// The Querier passed to fn can be used for all database operations.
func (m *TxManager) WithTx(ctx context.Context, fn func(tx repository.Querier) error) error {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	// Ensure rollback on panic or error
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p) // re-throw after rollback
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("rollback failed: %v (original error: %w)", rbErr, err)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// WithTxOptions executes the given function within a transaction with custom options.
func (m *TxManager) WithTxOptions(ctx context.Context, opts pgx.TxOptions, fn func(tx repository.Querier) error) error {
	tx, err := m.pool.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p)
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("rollback failed: %v (original error: %w)", rbErr, err)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// Pool returns the underlying pool for non-transactional operations.
func (m *TxManager) Pool() *pgxpool.Pool {
	return m.pool
}
