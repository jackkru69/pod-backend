package testdata

import (
	"time"

	"pod-backend/internal/entity"
)

// Test fixtures for entities (T110, T111, T112)

// User Fixtures (T110)

// ValidUser returns a valid user for testing
func ValidUser() *entity.User {
	return &entity.User{
		ID:                    1,
		WalletAddress:         "EQabc123def456789012345678901234567890123456789",
		TelegramUserID:        12345678,
		TelegramUsername:      "testuser",
		TotalGamesPlayed:      10,
		TotalWins:             5,
		TotalLosses:           3,
		TotalReferrals:        3,
		TotalReferralEarnings: 150000000,
		CreatedAt:             time.Now().Add(-30 * 24 * time.Hour),
		UpdatedAt:             time.Now(),
	}
}

// InvalidWalletUser returns a user with invalid wallet format
func InvalidWalletUser() *entity.User {
	u := ValidUser()
	u.WalletAddress = "invalid-wallet"
	return u
}

// EmptyWalletUser returns a user with empty wallet
func EmptyWalletUser() *entity.User {
	u := ValidUser()
	u.WalletAddress = ""
	return u
}

// NewUser returns a user with no games played
func NewUser() *entity.User {
	return &entity.User{
		ID:                    2,
		WalletAddress:         "EQdef456abc789012345678901234567890123456789012",
		TelegramUserID:        87654321,
		TelegramUsername:      "newuser",
		TotalGamesPlayed:      0,
		TotalWins:             0,
		TotalLosses:           0,
		TotalReferrals:        0,
		TotalReferralEarnings: 0,
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}
}

// Game Fixtures (T111)

// WaitingGame returns a game waiting for opponent
func WaitingGame() *entity.Game {
	return &entity.Game{
		GameID:                1,
		Status:                entity.GameStatusWaitingForOpponent,
		PlayerOneAddress:      "0:abc123def456789012345678901234567890123456789012345678901234567890",
		PlayerOneChoice:       1,
		BetAmount:             1000000000, // 1 TON
		ServiceFeeNumerator:   500,        // 5%
		ReferrerFeeNumerator:  100,        // 1%
		WaitingTimeoutSeconds: 300,
		LowestBidAllowed:      100000000,   // 0.1 TON
		HighestBidAllowed:     10000000000, // 10 TON
		FeeReceiverAddress:    "0:fee_receiver",
		InitTxHash:            "init_tx_hash_123",
		CreatedAt:             time.Now().Add(-5 * time.Minute),
	}
}

// ActiveGame returns an active game with both players
func ActiveGame() *entity.Game {
	return &entity.Game{
		GameID:                2,
		Status:                entity.GameStatusActive,
		PlayerOneAddress:      "0:abc123def456789012345678901234567890123456789012345678901234567890",
		PlayerTwoAddress:      strPtr("0:def456abc789012345678901234567890123456789012345678901234567890123"),
		PlayerOneChoice:       1,
		PlayerTwoChoice:       intPtr(2),
		BetAmount:             2000000000, // 2 TON
		ServiceFeeNumerator:   500,
		ReferrerFeeNumerator:  100,
		WaitingTimeoutSeconds: 300,
		LowestBidAllowed:      100000000,
		HighestBidAllowed:     10000000000,
		FeeReceiverAddress:    "0:fee_receiver",
		InitTxHash:            "init_tx_hash_456",
		JoinTxHash:            strPtr("join_tx_hash_456"),
		CreatedAt:             time.Now().Add(-10 * time.Minute),
		JoinedAt:              timePtr(time.Now().Add(-8 * time.Minute)),
	}
}

// FinishedGame returns a completed game
func FinishedGame() *entity.Game {
	return &entity.Game{
		GameID:                3,
		Status:                entity.GameStatusFinished,
		PlayerOneAddress:      "0:abc123def456789012345678901234567890123456789012345678901234567890",
		PlayerTwoAddress:      strPtr("0:def456abc789012345678901234567890123456789012345678901234567890123"),
		PlayerOneChoice:       1,
		PlayerTwoChoice:       intPtr(2),
		WinnerAddress:         strPtr("0:abc123def456789012345678901234567890123456789012345678901234567890"),
		BetAmount:             3000000000,              // 3 TON
		PayoutAmount:          intPtr(int(5700000000)), // 5.7 TON (after fees)
		ServiceFeeNumerator:   500,
		ReferrerFeeNumerator:  100,
		WaitingTimeoutSeconds: 300,
		LowestBidAllowed:      100000000,
		HighestBidAllowed:     10000000000,
		FeeReceiverAddress:    "0:fee_receiver",
		InitTxHash:            "init_tx_hash_789",
		JoinTxHash:            strPtr("join_tx_hash_789"),
		RevealTxHash:          strPtr("reveal_tx_hash_789"),
		CompleteTxHash:        strPtr("complete_tx_hash_789"),
		CreatedAt:             time.Now().Add(-20 * time.Minute),
		JoinedAt:              timePtr(time.Now().Add(-18 * time.Minute)),
		RevealedAt:            timePtr(time.Now().Add(-15 * time.Minute)),
		CompletedAt:           timePtr(time.Now().Add(-12 * time.Minute)),
	}
}

// CancelledGame returns a cancelled game
func CancelledGame() *entity.Game {
	return &entity.Game{
		GameID:                4,
		Status:                entity.GameStatusCancelled,
		PlayerOneAddress:      "0:abc123def456789012345678901234567890123456789012345678901234567890",
		PlayerOneChoice:       1,
		BetAmount:             1000000000,
		ServiceFeeNumerator:   500,
		ReferrerFeeNumerator:  100,
		WaitingTimeoutSeconds: 300,
		LowestBidAllowed:      100000000,
		HighestBidAllowed:     10000000000,
		FeeReceiverAddress:    "0:fee_receiver",
		InitTxHash:            "init_tx_hash_cancel",
		CompleteTxHash:        strPtr("cancel_tx_hash_123"),
		CreatedAt:             time.Now().Add(-30 * time.Minute),
		CompletedAt:           timePtr(time.Now().Add(-25 * time.Minute)),
	}
}

// GameEvent Fixtures (T112)

// GameInitializedEvent returns a game initialization event
func GameInitializedEvent() *entity.GameEvent {
	return &entity.GameEvent{
		ID:              1,
		GameID:          1,
		EventType:       entity.EventTypeGameInitialized,
		TransactionHash: "init_tx_hash_123",
		BlockNumber:     1000,
		Payload: map[string]interface{}{
			"player_one":        "0:abc123def456789012345678901234567890123456789012345678901234567890",
			"bet_amount":        "1000000000",
			"player_one_choice": 1,
			"secret_hash":       "secret_hash_123",
		},
		CreatedAt: time.Now(),
	}
}

// GameStartedEvent returns a game started event
func GameStartedEvent() *entity.GameEvent {
	return &entity.GameEvent{
		ID:              2,
		GameID:          1,
		EventType:       entity.EventTypeGameStarted,
		TransactionHash: "join_tx_hash_456",
		BlockNumber:     1001,
		Payload: map[string]interface{}{
			"player_two":        "0:def456abc789012345678901234567890123456789012345678901234567890123",
			"player_two_choice": 2,
		},
		CreatedAt: time.Now(),
	}
}

// GameFinishedEvent returns a game finished event
func GameFinishedEvent() *entity.GameEvent {
	return &entity.GameEvent{
		ID:              3,
		GameID:          1,
		EventType:       entity.EventTypeGameFinished,
		TransactionHash: "complete_tx_hash_789",
		BlockNumber:     1002,
		Payload: map[string]interface{}{
			"winner":          "0:abc123def456789012345678901234567890123456789012345678901234567890",
			"revealed_choice": 1,
			"secret":          "secret_123",
		},
		CreatedAt: time.Now(),
	}
}

// DrawEvent returns a draw event
func DrawEvent() *entity.GameEvent {
	return &entity.GameEvent{
		ID:              4,
		GameID:          2,
		EventType:       entity.EventTypeDraw,
		TransactionHash: "draw_tx_hash_123",
		BlockNumber:     1003,
		Payload: map[string]interface{}{
			"revealed_choice": 1,
			"secret":          "secret_456",
		},
		CreatedAt: time.Now(),
	}
}

// GameCancelledEvent returns a cancelled event
func GameCancelledEvent() *entity.GameEvent {
	return &entity.GameEvent{
		ID:              5,
		GameID:          4,
		EventType:       entity.EventTypeGameCancelled,
		TransactionHash: "cancel_tx_hash_123",
		BlockNumber:     1004,
		Payload:         map[string]interface{}{},
		CreatedAt:       time.Now(),
	}
}

// SecretOpenedEvent returns a secret opened event
func SecretOpenedEvent() *entity.GameEvent {
	return &entity.GameEvent{
		ID:              6,
		GameID:          1,
		EventType:       entity.EventTypeSecretOpened,
		TransactionHash: "reveal_tx_hash_789",
		BlockNumber:     1005,
		Payload: map[string]interface{}{
			"secret":          "secret_123",
			"revealed_choice": 1,
		},
		CreatedAt: time.Now(),
	}
}

// InsufficientBalanceEvent returns an insufficient balance event
func InsufficientBalanceEvent() *entity.GameEvent {
	return &entity.GameEvent{
		ID:              7,
		GameID:          5,
		EventType:       entity.EventTypeInsufficientBalance,
		TransactionHash: "insufficient_balance_tx_123",
		BlockNumber:     1006,
		Payload: map[string]interface{}{
			"player": "0:poor_player",
		},
		CreatedAt: time.Now(),
	}
}

// Helper functions

func strPtr(s string) *string {
	return &s
}

func intPtr(i int) *int {
	return &i
}

func timePtr(t time.Time) *time.Time {
	return &t
}
