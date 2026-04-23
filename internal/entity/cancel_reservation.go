package entity

// CancelReservationStatus constants. Cancel coordination reuses the same
// lifecycle values as other advisory reservations; only the eligibility and
// release semantics differ.
const (
	CancelReservationStatusActive   = ReservationStatusActive
	CancelReservationStatusReleased = ReservationStatusReleased
	CancelReservationStatusExpired  = ReservationStatusExpired
)

// CancelReservation is an advisory off-chain lock for creator-driven game
// cancellation. It intentionally aliases GameReservation because the stored
// fields and lifecycle semantics are identical; only the domain meaning differs.
// Constitution II still applies: cancel reservations are advisory only, must be
// reconciled against on-chain events, and are safe to lose on restart.
type CancelReservation = GameReservation
