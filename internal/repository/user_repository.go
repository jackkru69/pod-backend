package repository

import (
	"context"

	"pod-backend/internal/entity"
)

// UserRepository defines the interface for user data persistence.
// Implementations must handle database operations for user profiles.
type UserRepository interface {
	// Create creates a new user profile.
	// Returns error if wallet_address already exists.
	Create(ctx context.Context, user *entity.User) error

	// CreateOrUpdate creates a new user or updates existing one (upsert operation).
	// Used for automatic user profile creation from Telegram auth (FR-003).
	CreateOrUpdate(ctx context.Context, user *entity.User) error

	// EnsureUserByWallet ensures a user exists for the given wallet address.
	// If user doesn't exist, creates a minimal "blockchain-only" user with just the wallet.
	// If user exists, does nothing. Used for FK constraint satisfaction in game creation.
	EnsureUserByWallet(ctx context.Context, walletAddress string) error

	// GetByWalletAddress retrieves a user by their wallet address.
	// Returns nil if not found.
	GetByWalletAddress(ctx context.Context, walletAddress string) (*entity.User, error)

	// GetByWallet is an alias for GetByWalletAddress for cleaner use case code.
	GetByWallet(ctx context.Context, walletAddress string) (*entity.User, error)

	// GetByTelegramUserID retrieves all users associated with a Telegram user ID.
	// A single Telegram user can have multiple wallet addresses.
	// Returns empty slice if not found.
	GetByTelegramUserID(ctx context.Context, telegramUserID int64) ([]*entity.User, error)

	// Update updates an existing user profile.
	// Returns error if user doesn't exist.
	Update(ctx context.Context, user *entity.User) error

	// IncrementGamesPlayed atomically increments total_games_played counter.
	IncrementGamesPlayed(ctx context.Context, walletAddress string) error

	// IncrementGamesPlayedWithQuerier performs IncrementGamesPlayed using provided Querier (for transaction support).
	IncrementGamesPlayedWithQuerier(ctx context.Context, q Querier, walletAddress string) error

	// IncrementWins atomically increments total_wins counter.
	IncrementWins(ctx context.Context, walletAddress string) error

	// IncrementWinsWithQuerier performs IncrementWins using provided Querier (for transaction support).
	IncrementWinsWithQuerier(ctx context.Context, q Querier, walletAddress string) error

	// IncrementLosses atomically increments total_losses counter.
	IncrementLosses(ctx context.Context, walletAddress string) error

	// IncrementLossesWithQuerier performs IncrementLosses using provided Querier (for transaction support).
	IncrementLossesWithQuerier(ctx context.Context, q Querier, walletAddress string) error

	// IncrementReferrals atomically increments total_referrals and adds to total_referral_earnings.
	// Used when a referred player completes a game.
	IncrementReferrals(ctx context.Context, walletAddress string, earningsNanotons int64) error

	// IncrementReferralsWithQuerier performs IncrementReferrals using provided Querier (for transaction support).
	IncrementReferralsWithQuerier(ctx context.Context, q Querier, walletAddress string, earningsNanotons int64) error

	// GetReferralStats retrieves referral statistics for a user (FR-021).
	// Returns aggregated referral metrics including total referrals, earnings, and games referred.
	// Returns zero values if user doesn't exist.
	GetReferralStats(ctx context.Context, walletAddress string) (*entity.ReferralStats, error)

	// DeleteOlderThan deletes users whose created_at is older than the specified duration.
	// Used for data retention compliance (FR-017: 1 year retention).
	// Returns number of users deleted.
	DeleteOlderThan(ctx context.Context, olderThanDate string) (int64, error)
}
