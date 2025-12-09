package entity

import (
	"errors"
	"time"
)

// BlockchainSyncState tracks polling progress to prevent event reprocessing.
// This is a singleton entity (only one row exists in the database).
// Updated atomically after each successful poll batch.
type BlockchainSyncState struct {
	ID                 int       `json:"id"`                   // Always 1 (singleton)
	ContractAddress    string    `json:"contract_address"`     // TON contract being monitored
	LastProcessedBlock int64     `json:"last_processed_block"` // Last block number successfully processed
	LastPollTimestamp  time.Time `json:"last_poll_timestamp"`  // When the last poll occurred
	UpdatedAt          time.Time `json:"updated_at"`           // Auto-updated on changes
	// WebSocket event streaming fields (Phase 10)
	EventSourceType    string     `json:"event_source_type"`   // "websocket" or "http"
	LastProcessedLt    string     `json:"last_processed_lt"`   // Last processed logical time (lt) for TON
	LastProcessedHash  string     `json:"last_processed_hash"` // Last processed transaction hash (base64) - required with lt for TON API pagination
	WebSocketConnected bool       `json:"websocket_connected"` // Whether WebSocket is currently connected
	FallbackCount      int        `json:"fallback_count"`      // Number of fallback events
	LastFallbackAt     *time.Time `json:"last_fallback_at"`    // When last fallback occurred
}

// Validate validates the BlockchainSyncState entity.
func (s *BlockchainSyncState) Validate() error {
	if s.ID != 1 {
		return errors.New("id must be 1 (singleton)")
	}

	if s.ContractAddress == "" {
		return errors.New("contract_address is required")
	}

	if !tonAddressRegex.MatchString(s.ContractAddress) {
		return errors.New("contract_address must be valid TON address format (EQ...)")
	}

	if s.LastProcessedBlock < 0 {
		return errors.New("last_processed_block cannot be negative")
	}

	// Validate event source type
	if s.EventSourceType != "" && s.EventSourceType != "websocket" && s.EventSourceType != "http" {
		return errors.New("event_source_type must be 'websocket' or 'http'")
	}

	return nil
}
