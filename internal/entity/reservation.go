package entity

import (
	"errors"
	"time"
)

// ReservationStatus constants
const (
	ReservationStatusActive   = "active"
	ReservationStatusReleased = "released"
	ReservationStatusExpired  = "expired"
)

// GameReservation represents a temporary lock on a game.
// Stored in-memory only - lost on server restart (acceptable per spec).
type GameReservation struct {
	GameID        int64     `json:"game_id"`
	WalletAddress string    `json:"wallet_address"`
	CreatedAt     time.Time `json:"created_at"`
	ExpiresAt     time.Time `json:"expires_at"`
	Status        string    `json:"status"` // "active", "released", "expired"
}

// Validate validates the GameReservation entity
func (r *GameReservation) Validate() error {
	if r.GameID <= 0 {
		return errors.New("game_id must be positive")
	}
	if r.WalletAddress == "" {
		return errors.New("wallet_address is required")
	}
	if r.Status != ReservationStatusActive &&
		r.Status != ReservationStatusReleased &&
		r.Status != ReservationStatusExpired {
		return errors.New("invalid status")
	}
	return nil
}

// IsExpired checks if the reservation has expired based on current time
func (r *GameReservation) IsExpired() bool {
	return time.Now().After(r.ExpiresAt)
}

// IsActive checks if the reservation is currently active (not expired and status is active)
func (r *GameReservation) IsActive() bool {
	return r.Status == ReservationStatusActive && !r.IsExpired()
}

// TimeRemaining returns the duration until the reservation expires
func (r *GameReservation) TimeRemaining() time.Duration {
	remaining := time.Until(r.ExpiresAt)
	if remaining < 0 {
		return 0
	}
	return remaining
}
