package postgres

import (
	"context"
	"fmt"

	"pod-backend/internal/entity"
	"pod-backend/internal/repository"
	"pod-backend/pkg/postgres"
)

// Ensure UserRepository implements repository.UserRepository interface
var _ repository.UserRepository = (*UserRepository)(nil)

// UserRepository implements repository.UserRepository using PostgreSQL.
type UserRepository struct {
	pg *postgres.Postgres
}

// NewUserRepository creates a new PostgreSQL-backed user repository.
func NewUserRepository(pg *postgres.Postgres) *UserRepository {
	return &UserRepository{pg: pg}
}

// Create creates a new user profile.
func (r *UserRepository) Create(ctx context.Context, user *entity.User) error {
	if err := user.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	sql, args, err := r.pg.Builder.
		Insert("users").
		Columns(
			"telegram_user_id",
			"telegram_username",
			"wallet_address",
			"total_games_played",
			"total_wins",
			"total_losses",
			"total_referrals",
			"total_referral_earnings",
		).
		Values(
			user.TelegramUserID,
			user.TelegramUsername,
			user.WalletAddress,
			user.TotalGamesPlayed,
			user.TotalWins,
			user.TotalLosses,
			user.TotalReferrals,
			user.TotalReferralEarnings,
		).
		Suffix("RETURNING id, created_at, updated_at").
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	err = r.pg.Pool.QueryRow(ctx, sql, args...).Scan(
		&user.ID,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("execute query: %w", err)
	}

	return nil
}

// CreateOrUpdate creates a new user or updates existing one (upsert operation).
// Used for automatic user profile creation from Telegram auth (FR-003).
func (r *UserRepository) CreateOrUpdate(ctx context.Context, user *entity.User) error {
	if err := user.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	sql, args, err := r.pg.Builder.
		Insert("users").
		Columns(
			"telegram_user_id",
			"telegram_username",
			"wallet_address",
			"total_games_played",
			"total_wins",
			"total_losses",
			"total_referrals",
			"total_referral_earnings",
		).
		Values(
			user.TelegramUserID,
			user.TelegramUsername,
			user.WalletAddress,
			user.TotalGamesPlayed,
			user.TotalWins,
			user.TotalLosses,
			user.TotalReferrals,
			user.TotalReferralEarnings,
		).
		Suffix(`ON CONFLICT (wallet_address) DO UPDATE SET
			telegram_user_id = EXCLUDED.telegram_user_id,
			telegram_username = EXCLUDED.telegram_username,
			updated_at = NOW()
			RETURNING id, created_at, updated_at`).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	err = r.pg.Pool.QueryRow(ctx, sql, args...).Scan(
		&user.ID,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("execute query: %w", err)
	}

	return nil
}

// GetByWalletAddress retrieves a user by their wallet address.
func (r *UserRepository) GetByWalletAddress(ctx context.Context, walletAddress string) (*entity.User, error) {
	sql, args, err := r.pg.Builder.
		Select(
			"id",
			"telegram_user_id",
			"telegram_username",
			"wallet_address",
			"total_games_played",
			"total_wins",
			"total_losses",
			"total_referrals",
			"total_referral_earnings",
			"created_at",
			"updated_at",
		).
		From("users").
		Where("wallet_address = ?", walletAddress).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	user := &entity.User{}
	err = r.pg.Pool.QueryRow(ctx, sql, args...).Scan(
		&user.ID,
		&user.TelegramUserID,
		&user.TelegramUsername,
		&user.WalletAddress,
		&user.TotalGamesPlayed,
		&user.TotalWins,
		&user.TotalLosses,
		&user.TotalReferrals,
		&user.TotalReferralEarnings,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("execute query: %w", err)
	}

	return user, nil
}

// GetByWallet is an alias for GetByWalletAddress for cleaner use case code.
func (r *UserRepository) GetByWallet(ctx context.Context, walletAddress string) (*entity.User, error) {
	return r.GetByWalletAddress(ctx, walletAddress)
}

// GetByTelegramUserID retrieves all users associated with a Telegram user ID.
func (r *UserRepository) GetByTelegramUserID(ctx context.Context, telegramUserID int64) ([]*entity.User, error) {
	sql, args, err := r.pg.Builder.
		Select(
			"id",
			"telegram_user_id",
			"telegram_username",
			"wallet_address",
			"total_games_played",
			"total_wins",
			"total_losses",
			"total_referrals",
			"total_referral_earnings",
			"created_at",
			"updated_at",
		).
		From("users").
		Where("telegram_user_id = ?", telegramUserID).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	rows, err := r.pg.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("execute query: %w", err)
	}
	defer rows.Close()

	var users []*entity.User
	for rows.Next() {
		user := &entity.User{}
		err = rows.Scan(
			&user.ID,
			&user.TelegramUserID,
			&user.TelegramUsername,
			&user.WalletAddress,
			&user.TotalGamesPlayed,
			&user.TotalWins,
			&user.TotalLosses,
			&user.TotalReferrals,
			&user.TotalReferralEarnings,
			&user.CreatedAt,
			&user.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		users = append(users, user)
	}

	return users, nil
}

// Update updates an existing user profile.
func (r *UserRepository) Update(ctx context.Context, user *entity.User) error {
	if err := user.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	sql, args, err := r.pg.Builder.
		Update("users").
		Set("telegram_user_id", user.TelegramUserID).
		Set("telegram_username", user.TelegramUsername).
		Set("total_games_played", user.TotalGamesPlayed).
		Set("total_wins", user.TotalWins).
		Set("total_losses", user.TotalLosses).
		Set("total_referrals", user.TotalReferrals).
		Set("total_referral_earnings", user.TotalReferralEarnings).
		Where("wallet_address = ?", user.WalletAddress).
		Suffix("RETURNING updated_at").
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	err = r.pg.Pool.QueryRow(ctx, sql, args...).Scan(&user.UpdatedAt)
	if err != nil {
		return fmt.Errorf("execute query: %w", err)
	}

	return nil
}

// IncrementGamesPlayed atomically increments total_games_played counter.
func (r *UserRepository) IncrementGamesPlayed(ctx context.Context, walletAddress string) error {
	sql, args, err := r.pg.Builder.
		Update("users").
		Set("total_games_played", "total_games_played + 1").
		Where("wallet_address = ?", walletAddress).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = r.pg.Pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("execute query: %w", err)
	}

	return nil
}

// IncrementWins atomically increments total_wins counter.
func (r *UserRepository) IncrementWins(ctx context.Context, walletAddress string) error {
	sql, args, err := r.pg.Builder.
		Update("users").
		Set("total_wins", "total_wins + 1").
		Where("wallet_address = ?", walletAddress).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = r.pg.Pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("execute query: %w", err)
	}

	return nil
}

// IncrementLosses atomically increments total_losses counter.
func (r *UserRepository) IncrementLosses(ctx context.Context, walletAddress string) error {
	sql, args, err := r.pg.Builder.
		Update("users").
		Set("total_losses", "total_losses + 1").
		Where("wallet_address = ?", walletAddress).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = r.pg.Pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("execute query: %w", err)
	}

	return nil
}

// IncrementReferrals atomically increments total_referrals and adds to total_referral_earnings.
func (r *UserRepository) IncrementReferrals(ctx context.Context, walletAddress string, earningsNanotons int64) error {
	sql, args, err := r.pg.Builder.
		Update("users").
		Set("total_referrals", "total_referrals + 1").
		Set("total_referral_earnings", fmt.Sprintf("total_referral_earnings + %d", earningsNanotons)).
		Where("wallet_address = ?", walletAddress).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = r.pg.Pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("execute query: %w", err)
	}

	return nil
}

// GetReferralStats retrieves referral statistics for a user (FR-021).
// Returns aggregated referral metrics including total referrals, earnings, and games referred.
func (r *UserRepository) GetReferralStats(ctx context.Context, walletAddress string) (*entity.ReferralStats, error) {
	// Query user stats from users table
	userSQL, userArgs, err := r.pg.Builder.
		Select("total_referrals", "total_referral_earnings").
		From("users").
		Where("wallet_address = ?", walletAddress).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build user query: %w", err)
	}

	stats := &entity.ReferralStats{
		WalletAddress: walletAddress,
	}

	var totalReferrals int
	var totalEarnings int64
	err = r.pg.Pool.QueryRow(ctx, userSQL, userArgs...).Scan(&totalReferrals, &totalEarnings)
	if err != nil {
		// If user doesn't exist, return zero stats
		return stats, nil
	}

	stats.TotalReferrals = int64(totalReferrals)
	stats.TotalReferralEarnings = totalEarnings

	// Count games where this wallet was a referrer
	gamesSQL, gamesArgs, err := r.pg.Builder.
		Select("COUNT(*)").
		From("games").
		Where("player_one_referrer = ? OR player_two_referrer = ?", walletAddress, walletAddress).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build games query: %w", err)
	}

	var gamesReferred int64
	err = r.pg.Pool.QueryRow(ctx, gamesSQL, gamesArgs...).Scan(&gamesReferred)
	if err != nil {
		// If query fails, set to 0
		gamesReferred = 0
	}

	stats.GamesReferred = gamesReferred

	return stats, nil
}

// DeleteOlderThan deletes users whose created_at is older than the specified date.
func (r *UserRepository) DeleteOlderThan(ctx context.Context, olderThanDate string) (int64, error) {
	sql, args, err := r.pg.Builder.
		Delete("users").
		Where("created_at < ?", olderThanDate).
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("build query: %w", err)
	}

	result, err := r.pg.Pool.Exec(ctx, sql, args...)
	if err != nil {
		return 0, fmt.Errorf("execute query: %w", err)
	}

	return result.RowsAffected(), nil
}
