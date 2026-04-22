package entity

// RevealReservationStatus constants. Reveal-phase reservations intentionally
// reuse the same lifecycle values as lobby reservations; only the surrounding
// use-case semantics differ.
const (
	RevealReservationStatusActive   = ReservationStatusActive
	RevealReservationStatusReleased = ReservationStatusReleased
	RevealReservationStatusExpired  = ReservationStatusExpired
)

// RevealReservation is an advisory off-chain lock for the reveal/openBid phase.
//
// It intentionally aliases GameReservation because the persisted fields and
// lifecycle semantics are identical; only the domain meaning differs.
// Constitution II still applies: reveal reservations are advisory only, must
// be reconciled against on-chain events, and are safe to lose on restart.
type RevealReservation = GameReservation
