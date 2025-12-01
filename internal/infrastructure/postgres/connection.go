package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	_defaultMaxPoolSize  = 10
	_defaultConnAttempts = 3
	_defaultConnTimeout  = time.Second * 5
)

// Connection represents a PostgreSQL connection pool
type Connection struct {
	Pool *pgxpool.Pool
}

// Config holds PostgreSQL connection configuration
type Config struct {
	URL         string
	MaxPoolSize int
}

// New creates a new PostgreSQL connection pool with retries
func New(cfg Config) (*Connection, error) {
	if cfg.MaxPoolSize == 0 {
		cfg.MaxPoolSize = _defaultMaxPoolSize
	}

	poolConfig, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}

	poolConfig.MaxConns = int32(cfg.MaxPoolSize)
	poolConfig.MinConns = 2
	poolConfig.MaxConnLifetime = time.Hour
	poolConfig.MaxConnIdleTime = time.Minute * 30
	poolConfig.HealthCheckPeriod = time.Minute

	var pool *pgxpool.Pool
	for attempt := 1; attempt <= _defaultConnAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), _defaultConnTimeout)
		pool, err = pgxpool.NewWithConfig(ctx, poolConfig)
		cancel()

		if err == nil {
			// Verify connection
			ctx, cancel := context.WithTimeout(context.Background(), _defaultConnTimeout)
			err = pool.Ping(ctx)
			cancel()

			if err == nil {
				break
			}
		}

		if attempt < _defaultConnAttempts {
			time.Sleep(time.Second * time.Duration(attempt))
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres after %d attempts: %w", _defaultConnAttempts, err)
	}

	return &Connection{Pool: pool}, nil
}

// Close closes the connection pool
func (c *Connection) Close() {
	if c.Pool != nil {
		c.Pool.Close()
	}
}

// Ping checks if the database is alive
func (c *Connection) Ping(ctx context.Context) error {
	return c.Pool.Ping(ctx)
}

// Stats returns connection pool statistics
func (c *Connection) Stats() *pgxpool.Stat {
	return c.Pool.Stat()
}
