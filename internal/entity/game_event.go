package entity

import (
	"errors"
	"time"
)

// Event type constants (matching smart contract event names)
const (
	EventGameInitialized     = "GameInitializedNotify"
	EventGameStarted         = "GameStartedNotify"
	EventGameFinished        = "GameFinishedNotify"
	EventGameCancelled       = "GameCancelledNotify"
	EventDraw                = "DrawNotify"
	EventSecretOpened        = "SecretOpenedNotify"
	EventInsufficientBalance = "InsufficientBalanceNotify"

	// Aliases with EventType prefix for consistency
	EventTypeGameInitialized     = EventGameInitialized
	EventTypeGameStarted         = EventGameStarted
	EventTypeGameFinished        = EventGameFinished
	EventTypeGameCancelled       = EventGameCancelled
	EventTypeDraw                = EventDraw
	EventTypeSecretOpened        = EventSecretOpened
	EventTypeInsufficientBalance = EventInsufficientBalance
)

// GameEvent represents a blockchain event related to a game.
// Enables re-sync from blockchain and duplicate detection.
type GameEvent struct {
	ID              int64                  `json:"id"`
	GameID          int64                  `json:"game_id"`
	EventType       string                 `json:"event_type"`
	TransactionHash string                 `json:"transaction_hash"`
	BlockNumber     int64                  `json:"block_number"`
	Timestamp       time.Time              `json:"timestamp"`
	Payload         string                 `json:"payload"`    // JSON-encoded event data (for database storage)
	EventData       map[string]interface{} `json:"event_data"` // Parsed event data (for use cases)
	CreatedAt       time.Time              `json:"created_at"`
}

var validEventTypes = []string{
	EventGameInitialized,
	EventGameStarted,
	EventGameFinished,
	EventGameCancelled,
	EventDraw,
	EventSecretOpened,
	EventInsufficientBalance,
}

// Validate validates the GameEvent entity.
func (e *GameEvent) Validate() error {
	if !contains(validEventTypes, e.EventType) {
		return errors.New("invalid event_type")
	}

	if e.TransactionHash == "" {
		return errors.New("transaction_hash is required")
	}

	// Payload is optional (can be empty for EventData-only events in tests/use cases)
	// When persisting to DB, Payload should be set from EventData

	if e.GameID <= 0 {
		return errors.New("game_id must be positive")
	}

	if e.BlockNumber < 0 {
		return errors.New("block_number cannot be negative")
	}

	return nil
}

// contains checks if a string slice contains a specific string.
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
