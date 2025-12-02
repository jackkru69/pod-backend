package entity

import (
	"errors"
	"regexp"
	"time"
)

// User represents a player in the gambling platform.
// Each TON wallet address is tracked independently, allowing one Telegram user
// to participate with multiple wallets.
type User struct {
	ID                    int64     `json:"id"`
	TelegramUserID        int64     `json:"telegram_user_id"`
	TelegramUsername      string    `json:"telegram_username"`
	WalletAddress         string    `json:"wallet_address"`
	TotalGamesPlayed      int       `json:"total_games_played"`
	TotalWins             int       `json:"total_wins"`
	TotalLosses           int       `json:"total_losses"`
	TotalReferrals        int       `json:"total_referrals"`
	TotalReferralEarnings int64     `json:"total_referral_earnings"` // nanotons
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

var tonAddressRegex = regexp.MustCompile(`^EQ[A-Za-z0-9_-]{46}$`)

// Validate validates the User entity.
func (u *User) Validate() error {
	if u.WalletAddress == "" {
		return errors.New("wallet_address is required")
	}

	if !tonAddressRegex.MatchString(u.WalletAddress) {
		return errors.New("wallet_address must be valid TON address format (EQ...)")
	}

	if u.TelegramUserID <= 0 {
		return errors.New("telegram_user_id must be positive")
	}

	if u.TotalGamesPlayed < 0 {
		return errors.New("total_games_played cannot be negative")
	}

	if u.TotalWins < 0 {
		return errors.New("total_wins cannot be negative")
	}

	if u.TotalLosses < 0 {
		return errors.New("total_losses cannot be negative")
	}

	if u.TotalReferrals < 0 {
		return errors.New("total_referrals cannot be negative")
	}

	if u.TotalReferralEarnings < 0 {
		return errors.New("total_referral_earnings cannot be negative")
	}

	return nil
}

// ReferralStats represents aggregated referral statistics for a user's wallet address.
// Supports FR-021 requirement to expose referral metrics.
type ReferralStats struct {
	WalletAddress         string `json:"wallet_address"`
	TotalReferrals        int64  `json:"total_referrals"`         // Total unique referrals made by this wallet
	TotalReferralEarnings int64  `json:"total_referral_earnings"` // Total earnings in nanotons
	GamesReferred         int64  `json:"games_referred"`          // Total games where this wallet was referrer
}
