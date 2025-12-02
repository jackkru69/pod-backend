package toncenter

import (
	"context"
)

// EventSource defines the interface for blockchain event sources (T148).
// Implementations can use either WebSocket streaming or HTTP polling.
// This abstraction enables runtime switching between event sources.
type EventSource interface {
	// Start begins receiving blockchain events.
	// Runs asynchronously until Stop() is called or context is cancelled.
	Start(ctx context.Context)

	// Stop gracefully stops the event source.
	Stop()

	// Subscribe registers an event handler to receive transaction events.
	// The handler is called for each new transaction from the contract.
	Subscribe(handler EventHandler)

	// GetLastProcessedLt returns the last processed logical time (lt).
	// Used for resuming from saved state and tracking progress.
	GetLastProcessedLt() string

	// SetLastProcessedLt sets the starting logical time for event processing.
	// Used when resuming from database state.
	SetLastProcessedLt(lt string)

	// IsConnected returns whether the event source is actively receiving events.
	IsConnected() bool

	// GetSourceType returns the type of event source ("websocket" or "http").
	GetSourceType() string
}

// EventSourceType constants for identifying event source type.
const (
	SourceTypeWebSocket = "websocket"
	SourceTypeHTTP      = "http"
)
