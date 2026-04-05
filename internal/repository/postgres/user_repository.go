package postgres

import (
	"context"
	"fmt"

	sq "github.com/Masterminds/squirrel"
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

// EnsureUserByWallet ensures a user exists for the given wallet address.
// If user doesn't exist, creates a minimal "blockchain-only" user with just the wallet.
// If user exists, does nothing. Used for FK constraint satisfaction in game creation.
func (r *UserRepository) EnsureUserByWallet(ctx context.Context, walletAddress string) error {
	sql, args, err := r.pg.Builder.
		Insert("users").
		Columns(
			"wallet_address",
			"total_games_played",
			"total_wins",
			"total_losses",
			"total_referrals",
			"total_referral_earnings",
		).
		Values(
			walletAddress,
			0,
			0,
			0,
			0,
			0,
		).
		Suffix("ON CONFLICT (wallet_address) DO NOTHING").
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

// GetByWalletAddress retrieves a user by their wallet address.
func (r *UserRepository) GetByWalletAddress(ctx context.Context, walletAddress string) (*entity.User, error) {
	user := &entity.User{}
	err := r.pg.Pool.QueryRow(ctx, `
		SELECT
			u.id,
			u.telegram_user_id,
			u.telegram_username,
			u.wallet_address,
			COALESCE(game_stats.total_games_played, 0) AS total_games_played,
			COALESCE(game_stats.total_wins, 0) AS total_wins,
			COALESCE(game_stats.total_losses, 0) AS total_losses,
			COALESCE(referral_stats.total_referrals, 0) AS total_referrals,
			u.total_referral_earnings,
			u.created_at,
			u.updated_at
		FROM users u
		LEFT JOIN LATERAL (
			SELECT
				COUNT(*) FILTER (
					WHERE g.status IN ($2, $3)
						AND NOT (g.winner_address IS NULL AND g.payout_amount IS NULL)
				) AS total_games_played,
				COUNT(*) FILTER (
					WHERE g.status IN ($2, $3)
						AND g.winner_address = $1
				) AS total_wins,
				COUNT(*) FILTER (
					WHERE g.status IN ($2, $3)
						AND g.winner_address IS NOT NULL
						AND g.winner_address <> $1
				) AS total_losses
			FROM games g
			WHERE g.player_one_address = $1 OR g.player_two_address = $1
		) AS game_stats ON TRUE
		LEFT JOIN LATERAL (
			SELECT COUNT(DISTINCT referred_wallet) AS total_referrals
			FROM (
				SELECT g.player_one_address AS referred_wallet
				FROM games g
				WHERE g.player_one_referrer = $1
				UNION ALL
				SELECT g.player_two_address AS referred_wallet
				FROM games g
				WHERE g.player_two_referrer = $1 AND g.player_two_address IS NOT NULL
			) referred
		) AS referral_stats ON TRUE
		WHERE u.wallet_address = $1
	`, walletAddress, entity.GameStatusEnded, entity.GameStatusPaid).Scan(
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
	return r.IncrementGamesPlayedWithQuerier(ctx, r.pg.Pool, walletAddress)
}

// IncrementGamesPlayedWithQuerier performs IncrementGamesPlayed using provided Querier (for transaction support).
func (r *UserRepository) IncrementGamesPlayedWithQuerier(ctx context.Context, q repository.Querier, walletAddress string) error {
	sql, args, err := r.pg.Builder.
		Update("users").
		Set("total_games_played", sq.Expr("total_games_played + 1")).
		Where("wallet_address = ?", walletAddress).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = q.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("execute query: %w", err)
	}

	return nil
}

// IncrementWins atomically increments total_wins counter.
func (r *UserRepository) IncrementWins(ctx context.Context, walletAddress string) error {
	return r.IncrementWinsWithQuerier(ctx, r.pg.Pool, walletAddress)
}

// IncrementWinsWithQuerier performs IncrementWins using provided Querier (for transaction support).
func (r *UserRepository) IncrementWinsWithQuerier(ctx context.Context, q repository.Querier, walletAddress string) error {
	sql, args, err := r.pg.Builder.
		Update("users").
		Set("total_wins", sq.Expr("total_wins + 1")).
		Where("wallet_address = ?", walletAddress).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = q.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("execute query: %w", err)
	}

	return nil
}

// IncrementLosses atomically increments total_losses counter.
func (r *UserRepository) IncrementLosses(ctx context.Context, walletAddress string) error {
	return r.IncrementLossesWithQuerier(ctx, r.pg.Pool, walletAddress)
}

// IncrementLossesWithQuerier performs IncrementLosses using provided Querier (for transaction support).
func (r *UserRepository) IncrementLossesWithQuerier(ctx context.Context, q repository.Querier, walletAddress string) error {
	sql, args, err := r.pg.Builder.
		Update("users").
		Set("total_losses", sq.Expr("total_losses + 1")).
		Where("wallet_address = ?", walletAddress).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = q.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("execute query: %w", err)
	}

	return nil
}

// IncrementReferrals atomically increments total_referrals and adds to total_referral_earnings.
func (r *UserRepository) IncrementReferrals(ctx context.Context, walletAddress string, earningsNanotons int64) error {
	return r.IncrementReferralsWithQuerier(ctx, r.pg.Pool, walletAddress, earningsNanotons)
}

// IncrementReferralsWithQuerier performs IncrementReferrals using provided Querier (for transaction support).
func (r *UserRepository) IncrementReferralsWithQuerier(ctx context.Context, q repository.Querier, walletAddress string, earningsNanotons int64) error {
	sql, args, err := r.pg.Builder.
		Update("users").
		Set("total_referrals", sq.Expr("total_referrals + 1")).
		Set("total_referral_earnings", sq.Expr(fmt.Sprintf("total_referral_earnings + %d", earningsNanotons))).
		Where("wallet_address = ?", walletAddress).
		ToSql()
	if err != nil {
		return fmt.Errorf("build query: %w", err)
	}

	_, err = q.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("execute query: %w", err)
	}

	return nil
}

// GetReferralStats retrieves referral statistics for a user (FR-021).
// Returns aggregated referral metrics including total referrals, earnings, and games referred.
func (r *UserRepository) GetReferralStats(ctx context.Context, walletAddress string) (*entity.ReferralStats, error) {
	stats := &entity.ReferralStats{
		WalletAddress: walletAddress,
	}

	err := r.pg.Pool.QueryRow(ctx, `
		SELECT
			COALESCE((
				SELECT COUNT(DISTINCT referred_wallet)
				FROM (
					SELECT g.player_one_address AS referred_wallet
					FROM games g
					WHERE g.player_one_referrer = $1
					UNION ALL
					SELECT g.player_two_address AS referred_wallet
					FROM games g
					WHERE g.player_two_referrer = $1 AND g.player_two_address IS NOT NULL
				) referred
			), 0) AS total_referrals,
			COALESCE((
				SELECT total_referral_earnings
				FROM users
				WHERE wallet_address = $1
			), 0) AS total_referral_earnings,
			COALESCE((
				SELECT COUNT(*)
				FROM games g
				WHERE g.player_one_referrer = $1 OR g.player_two_referrer = $1
			), 0) AS games_referred
	`, walletAddress).Scan(&stats.TotalReferrals, &stats.TotalReferralEarnings, &stats.GamesReferred)
	if err != nil {
		return nil, fmt.Errorf("execute query: %w", err)
	}

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
