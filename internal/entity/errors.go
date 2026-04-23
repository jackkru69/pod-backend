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

	// ErrExpiredClaimWalletRequired is returned when an expired-follow-up claim request omits the holder wallet.
	ErrExpiredClaimWalletRequired = errors.New("wallet address is required")
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

// Expired-follow-up claim errors.
var (
	// ErrExpiredClaimAlreadyClaimed is returned when another wallet already holds the expired-follow-up claim.
	ErrExpiredClaimAlreadyClaimed = errors.New("expired follow-up is already claimed by another player")

	// ErrExpiredClaimNotAvailable is returned when the game is not currently in the expired-attention state.
	ErrExpiredClaimNotAvailable = errors.New("game is not available for expired follow-up")

	// ErrNotExpiredClaimParticipant is returned when the caller is not a participant of the game.
	ErrNotExpiredClaimParticipant = errors.New("only participants of the game can claim expired follow-up")

	// ErrTooManyExpiredClaims is returned when a wallet exceeds the expired-follow-up claim limit.
	ErrTooManyExpiredClaims = errors.New("wallet has too many active expired follow-up claims")

	// ErrExpiredClaimNotFound is returned when no expired-follow-up claim exists for the game.
	ErrExpiredClaimNotFound = errors.New("expired follow-up claim not found")

	// ErrNotExpiredClaimHolder is returned when a non-holder tries to release a claim.
	ErrNotExpiredClaimHolder = errors.New("only the expired follow-up claim holder can perform this action")
)

// Activity-surface errors.
var (
	// ErrInvalidActivityQueueKey is returned when a queue key is unknown.
	ErrInvalidActivityQueueKey = errors.New("invalid queue key")

	// ErrInvalidActivityOffset is returned when the requested queue offset is negative.
	ErrInvalidActivityOffset = errors.New("offset cannot be negative")

	// ErrActivityWalletRequired is returned when a wallet-scoped activity summary is requested without a wallet.
	ErrActivityWalletRequired = errors.New("wallet address is required")
)
