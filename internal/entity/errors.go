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

// Reveal-phase reservation errors (spec 005-reveal-reservation).
var (
	// ErrRevealAlreadyReserved is returned when the reveal slot for a game is held by another wallet.
	ErrRevealAlreadyReserved = errors.New("reveal is already reserved by another player")

	// ErrRevealNotAvailable is returned when the game is not in waiting_for_open_bids status.
	ErrRevealNotAvailable = errors.New("game is not available for reveal reservation")

	// ErrNotAPlayer is returned when the caller is not a participant of the game.
	ErrNotAPlayer = errors.New("only participants of the game can reserve a reveal")

	// ErrTooManyRevealReservations is returned when a wallet has reached the reveal-reservation limit.
	ErrTooManyRevealReservations = errors.New("wallet has too many active reveal reservations")

	// ErrRevealReservationNotFound is returned when no reveal reservation exists for the game.
	ErrRevealReservationNotFound = errors.New("reveal reservation not found")

	// ErrNotRevealReservationHolder is returned when a non-holder tries to cancel a reveal reservation.
	ErrNotRevealReservationHolder = errors.New("only the reveal reservation holder can perform this action")
)
