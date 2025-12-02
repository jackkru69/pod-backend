package entity

import (
	"errors"
	"time"
)

// Game status constants (MUST match smart contract)
const (
	GameStatusUninitialized      = 0 // Not yet created (should not exist in DB)
	GameStatusWaitingForOpponent = 1 // Player 1 created, waiting for Player 2
	GameStatusWaitingForOpenBids = 2 // Both players joined, waiting for reveals
	GameStatusEnded              = 3 // Game completed, winner determined (or draw)
	GameStatusPaid               = 4 // Payouts distributed
)

// Coin side constants (MUST match smart contract)
const (
	CoinSideUnknown = 0 // Choice not yet revealed (used during initialization)
	CoinSideClosed  = 1 // Unrevealed choice (encrypted)
	CoinSideHeads   = 2 // HEADS
	CoinSideTails   = 3 // TAILS
)

// Game represents a single coin flip gambling game instance.
// Tracks full game lifecycle from initialization through completion/cancellation.
type Game struct {
	GameID                int64      `json:"game_id"`
	Status                int        `json:"status"` // 0-4 (see constants above)
	PlayerOneAddress      string     `json:"player_one_address"`
	PlayerTwoAddress      *string    `json:"player_two_address,omitempty"`
	PlayerOneChoice       int        `json:"player_one_choice"` // 1=CLOSED, 2=HEADS, 3=TAILS
	PlayerTwoChoice       *int       `json:"player_two_choice,omitempty"`
	PlayerOneReferrer     *string    `json:"player_one_referrer,omitempty"`
	PlayerTwoReferrer     *string    `json:"player_two_referrer,omitempty"`
	BetAmount             int64      `json:"bet_amount"` // nanotons
	WinnerAddress         *string    `json:"winner_address,omitempty"`
	PayoutAmount          *int64     `json:"payout_amount,omitempty"`
	ServiceFeeNumerator   int64      `json:"service_fee_numerator"`
	ReferrerFeeNumerator  int64      `json:"referrer_fee_numerator"`
	WaitingTimeoutSeconds int64      `json:"waiting_timeout_seconds"`
	LowestBidAllowed      int64      `json:"lowest_bid_allowed"`
	HighestBidAllowed     int64      `json:"highest_bid_allowed"`
	FeeReceiverAddress    string     `json:"fee_receiver_address"`
	CreatedAt             time.Time  `json:"created_at"`
	JoinedAt              *time.Time `json:"joined_at,omitempty"`
	RevealedAt            *time.Time `json:"revealed_at,omitempty"`
	CompletedAt           *time.Time `json:"completed_at,omitempty"`
	InitTxHash            string     `json:"init_tx_hash"`
	JoinTxHash            *string    `json:"join_tx_hash,omitempty"`
	RevealTxHash          *string    `json:"reveal_tx_hash,omitempty"`
	CompleteTxHash        *string    `json:"complete_tx_hash,omitempty"`
}

// Validate validates the Game entity.
func (g *Game) Validate() error {
	if g.GameID <= 0 {
		return errors.New("game_id must be positive")
	}

	if g.Status < GameStatusUninitialized || g.Status > GameStatusPaid {
		return errors.New("status must be 0-4")
	}

	if !tonAddressRegex.MatchString(g.PlayerOneAddress) {
		return errors.New("player_one_address must be valid TON address")
	}

	if g.BetAmount <= 0 {
		return errors.New("bet_amount must be positive")
	}

	if g.PlayerOneChoice < CoinSideUnknown || g.PlayerOneChoice > CoinSideTails {
		return errors.New("player_one_choice must be 0-3")
	}

	// Status-dependent validation
	if g.Status >= GameStatusWaitingForOpenBids && g.PlayerTwoAddress == nil {
		return errors.New("player_two_address required for status >= 2")
	}

	if g.PlayerTwoAddress != nil && *g.PlayerTwoAddress == g.PlayerOneAddress {
		return errors.New("player_two_address must be different from player_one_address")
	}

	if g.InitTxHash == "" {
		return errors.New("init_tx_hash is required")
	}

	return nil
}

// CanTransitionTo checks if the game can transition to a new status.
// Status can only increase (no backwards transitions except 3->4 for PAID).
func (g *Game) CanTransitionTo(newStatus int) bool {
	if newStatus < g.Status {
		// Only allow 3 -> 4 transition (ENDED -> PAID)
		if g.Status == GameStatusEnded && newStatus == GameStatusPaid {
			return true
		}
		return false
	}
	return true
}
