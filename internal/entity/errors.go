package entity

import "errors"

// Reservation-related errors
var (
	// ErrGameAlreadyReserved is returned when attempting to reserve a game that is already reserved
	ErrGameAlreadyReserved = errors.New("game is already reserved by another player")

	// ErrTooManyReservations is returned when a wallet has reached the maximum reservation limit
	ErrTooManyReservations = errors.New("wallet has too many active reservations")

	// ErrCannotReserveOwnGame is returned when a player tries to reserve their own game
	ErrCannotReserveOwnGame = errors.New("cannot reserve your own game")

	// ErrNotReservationHolder is returned when a non-holder tries to cancel a reservation
	ErrNotReservationHolder = errors.New("only the reservation holder can perform this action")

	// ErrGameNotAvailable is returned when a game is not in waiting_for_opponent status
	ErrGameNotAvailable = errors.New("game is not available for reservation")

	// ErrReservationNotFound is returned when a reservation does not exist
	ErrReservationNotFound = errors.New("reservation not found")
)
