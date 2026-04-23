package entity

// ExpiredClaimStatus constants intentionally reuse the advisory reservation
// lifecycle values because expired follow-up claims are the same in-memory
// coordination primitive with different gameplay semantics.
const (
	ExpiredClaimStatusActive   = ReservationStatusActive
	ExpiredClaimStatusReleased = ReservationStatusReleased
	ExpiredClaimStatusExpired  = ReservationStatusExpired
)

// ExpiredClaim is an advisory off-chain claim that coordinates follow-up work
// for games that remain in the expired-attention queue.
//
// It aliases GameReservation because the stored fields and lifecycle semantics
// are identical; only the queue-specific meaning differs.
type ExpiredClaim = GameReservation
