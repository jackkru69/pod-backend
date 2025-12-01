package logger

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
)

// Logger wraps zerolog.Logger with convenience methods
type Logger struct {
	zerolog.Logger
}

// New creates a new structured logger with zerolog
func New(level string) *Logger {
	var l zerolog.Level
	switch level {
	case "debug":
		l = zerolog.DebugLevel
	case "info":
		l = zerolog.InfoLevel
	case "warn":
		l = zerolog.WarnLevel
	case "error":
		l = zerolog.ErrorLevel
	default:
		l = zerolog.InfoLevel
	}

	zerolog.TimeFieldFormat = time.RFC3339
	logger := zerolog.New(os.Stdout).
		Level(l).
		With().
		Timestamp().
		Caller().
		Logger()

	return &Logger{Logger: logger}
}

// NewWithWriter creates a new logger with custom writer (useful for testing)
func NewWithWriter(writer io.Writer, level string) *Logger {
	var l zerolog.Level
	switch level {
	case "debug":
		l = zerolog.DebugLevel
	case "info":
		l = zerolog.InfoLevel
	case "warn":
		l = zerolog.WarnLevel
	case "error":
		l = zerolog.ErrorLevel
	default:
		l = zerolog.InfoLevel
	}

	zerolog.TimeFieldFormat = time.RFC3339
	logger := zerolog.New(writer).
		Level(l).
		With().
		Timestamp().
		Caller().
		Logger()

	return &Logger{Logger: logger}
}

// WithRequestID adds request ID to logger context
func (l *Logger) WithRequestID(requestID string) *Logger {
	logger := l.Logger.With().Str("request_id", requestID).Logger()
	return &Logger{Logger: logger}
}

// WithGameID adds game ID to logger context
func (l *Logger) WithGameID(gameID int64) *Logger {
	logger := l.Logger.With().Int64("game_id", gameID).Logger()
	return &Logger{Logger: logger}
}

// WithWallet adds wallet address to logger context
func (l *Logger) WithWallet(wallet string) *Logger {
	logger := l.Logger.With().Str("wallet", wallet).Logger()
	return &Logger{Logger: logger}
}

// WithTxHash adds transaction hash to logger context
func (l *Logger) WithTxHash(txHash string) *Logger {
	logger := l.Logger.With().Str("tx_hash", txHash).Logger()
	return &Logger{Logger: logger}
}

// WithEvent adds event type to logger context
func (l *Logger) WithEvent(eventType string) *Logger {
	logger := l.Logger.With().Str("event_type", eventType).Logger()
	return &Logger{Logger: logger}
}
