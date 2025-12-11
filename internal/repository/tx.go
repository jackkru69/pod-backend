// Package repository defines data persistence interfaces.
package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Querier is an interface that both pgxpool.Pool and pgx.Tx implement.
// This allows repository methods to work with both regular connections and transactions.
type Querier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// TxManager defines the interface for transaction management.
// Implementations provide transactional execution of multiple operations.
type TxManager interface {
	// WithTx executes the given function within a database transaction.
	// If the function returns nil, the transaction is committed.
	// If the function returns an error, the transaction is rolled back.
	// The Querier passed to fn can be used for database operations within the transaction.
	WithTx(ctx context.Context, fn func(tx Querier) error) error
}
