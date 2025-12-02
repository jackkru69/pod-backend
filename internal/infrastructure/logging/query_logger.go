package logging

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

// QueryTracer implements pgx.QueryTracer to log slow queries (T119)
type QueryTracer struct {
	slowQueryThreshold time.Duration
}

// NewQueryTracer creates a new query tracer that logs queries slower than threshold
func NewQueryTracer(threshold time.Duration) *QueryTracer {
	return &QueryTracer{
		slowQueryThreshold: threshold,
	}
}

// TraceQueryStart is called at the beginning of Query, QueryRow, and Exec calls
func (qt *QueryTracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	ctx = context.WithValue(ctx, "query_start_time", time.Now())
	ctx = context.WithValue(ctx, "query_sql", data.SQL)
	return ctx
}

// TraceQueryEnd is called at the end of Query, QueryRow, and Exec calls
func (qt *QueryTracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	startTime, ok := ctx.Value("query_start_time").(time.Time)
	if !ok {
		return
	}

	sql, _ := ctx.Value("query_sql").(string)
	duration := time.Since(startTime)

	// Log slow queries at WARN level (T119)
	if duration > qt.slowQueryThreshold {
		log.Warn().
			Dur("duration", duration).
			Str("sql", sql).
			Int("rows_affected", int(data.CommandTag.RowsAffected())).
			Msg("Slow query detected")
	} else {
		// Log normal queries at DEBUG level
		log.Debug().
			Dur("duration", duration).
			Str("sql", sql).
			Int("rows_affected", int(data.CommandTag.RowsAffected())).
			Msg("Query executed")
	}
}
