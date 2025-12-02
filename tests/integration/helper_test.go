package integration_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"pod-backend/internal/entity"
	"pod-backend/pkg/postgres"
	"pod-backend/tests/testdata"
)

// Integration test helper (T114)
// Provides setup/teardown and utilities for integration tests

// TestHelper provides utilities for integration tests
type TestHelper struct {
	DB        *pgxpool.Pool
	pg        *postgres.Postgres // Wrapped postgres instance for repositories
	Logger    *zerolog.Logger
	CleanupFn func()
}

// Postgres returns a *postgres.Postgres wrapper for use with repositories.
// This wrapper provides the Builder field needed by repository constructors.
func (h *TestHelper) Postgres() *postgres.Postgres {
	return h.pg
}

// NewTestHelper creates a new test helper with database connection
func NewTestHelper(t *testing.T) *TestHelper {
	t.Helper()

	// Setup logger
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	logger := log.Logger

	// Get test database URL from environment (check both TEST_PG_URL and PG_URL)
	dbURL := os.Getenv("TEST_PG_URL")
	if dbURL == "" {
		dbURL = os.Getenv("PG_URL")
	}
	if dbURL == "" {
		dbURL = "postgresql://user:myAwEsOm3pa55%40w0rd@localhost:5433/db?sslmode=disable"
	}

	// Connect to database
	poolConfig, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		t.Fatalf("Failed to parse database URL: %v", err)
	}

	poolConfig.MaxConns = 5

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("Failed to ping test database: %v", err)
	}

	logger.Info().Msg("Connected to test database")

	// Create postgres.Postgres wrapper for repositories
	pg := &postgres.Postgres{
		Pool:    pool,
		Builder: squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar),
	}

	helper := &TestHelper{
		DB:     pool,
		pg:     pg,
		Logger: &logger,
		CleanupFn: func() {
			pool.Close()
		},
	}

	// Clean database before tests
	helper.CleanDatabase(t)

	return helper
}

// Cleanup closes connections and cleans up resources
func (h *TestHelper) Cleanup() {
	if h.CleanupFn != nil {
		h.CleanupFn()
	}
}

// CleanDatabase removes all data from test database
func (h *TestHelper) CleanDatabase(t *testing.T) {
	t.Helper()

	ctx := context.Background()

	// Delete in correct order (respecting foreign keys)
	tables := []string{
		"game_events",
		"games",
		"users",
		"blockchain_sync_state",
	}

	for _, table := range tables {
		_, err := h.DB.Exec(ctx, fmt.Sprintf("DELETE FROM %s", table))
		if err != nil {
			t.Logf("Warning: Failed to clean table %s: %v", table, err)
		}
	}

	h.Logger.Debug().Msg("Test database cleaned")
}

// SeedUser inserts a test user into database
func (h *TestHelper) SeedUser(t *testing.T, user *entity.User) {
	t.Helper()

	ctx := context.Background()

	query := `
		INSERT INTO users (
			wallet_address, telegram_user_id, telegram_username,
			total_games_played, total_wins, total_losses,
			total_referrals, total_referral_earnings,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (wallet_address) DO NOTHING
	`

	_, err := h.DB.Exec(ctx, query,
		user.WalletAddress,
		user.TelegramUserID,
		user.TelegramUsername,
		user.TotalGamesPlayed,
		user.TotalWins,
		user.TotalLosses,
		user.TotalReferrals,
		user.TotalReferralEarnings,
		user.CreatedAt,
		user.UpdatedAt,
	)

	if err != nil {
		t.Fatalf("Failed to seed user: %v", err)
	}
}

// SeedGame inserts a test game into database
func (h *TestHelper) SeedGame(t *testing.T, game *entity.Game) {
	t.Helper()

	ctx := context.Background()

	query := `
		INSERT INTO games (
			game_id, status, player_one_address, player_two_address,
			player_one_choice, player_two_choice,
			bet_amount, winner_address, payout_amount,
			service_fee_numerator, referrer_fee_numerator,
			waiting_timeout_seconds, lowest_bid_allowed, highest_bid_allowed,
			fee_receiver_address,
			created_at, joined_at, revealed_at, completed_at,
			init_tx_hash, join_tx_hash, reveal_tx_hash, complete_tx_hash
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23)
		ON CONFLICT (game_id) DO NOTHING
	`

	_, err := h.DB.Exec(ctx, query,
		game.GameID,
		game.Status,
		game.PlayerOneAddress,
		game.PlayerTwoAddress,
		game.PlayerOneChoice,
		game.PlayerTwoChoice,
		game.BetAmount,
		game.WinnerAddress,
		game.PayoutAmount,
		game.ServiceFeeNumerator,
		game.ReferrerFeeNumerator,
		game.WaitingTimeoutSeconds,
		game.LowestBidAllowed,
		game.HighestBidAllowed,
		game.FeeReceiverAddress,
		game.CreatedAt,
		game.JoinedAt,
		game.RevealedAt,
		game.CompletedAt,
		game.InitTxHash,
		game.JoinTxHash,
		game.RevealTxHash,
		game.CompleteTxHash,
	)

	if err != nil {
		t.Fatalf("Failed to seed game: %v", err)
	}
}

// SeedGameEvent inserts a test game event into database
func (h *TestHelper) SeedGameEvent(t *testing.T, event *entity.GameEvent) {
	t.Helper()

	ctx := context.Background()

	query := `
		INSERT INTO game_events (
			game_id, event_type, transaction_hash, block_number,
			payload, created_at
		) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (transaction_hash) DO NOTHING
	`

	_, err := h.DB.Exec(ctx, query,
		event.GameID,
		event.EventType,
		event.TransactionHash,
		event.BlockNumber,
		event.Payload,
		event.CreatedAt,
	)

	if err != nil {
		t.Fatalf("Failed to seed game event: %v", err)
	}
}

// SeedDefaultData seeds database with default test data
func (h *TestHelper) SeedDefaultData(t *testing.T) {
	t.Helper()

	// Seed users
	h.SeedUser(t, testdata.ValidUser())
	h.SeedUser(t, testdata.NewUser())

	// Seed games
	h.SeedGame(t, testdata.WaitingGame())
	h.SeedGame(t, testdata.ActiveGame())
	h.SeedGame(t, testdata.FinishedGame())

	// Seed events
	h.SeedGameEvent(t, testdata.GameInitializedEvent())
	h.SeedGameEvent(t, testdata.GameStartedEvent())
	h.SeedGameEvent(t, testdata.GameFinishedEvent())

	h.Logger.Info().Msg("Default test data seeded")
}

// AssertRowCount checks if table has expected number of rows
func (h *TestHelper) AssertRowCount(t *testing.T, table string, expected int64) {
	t.Helper()

	ctx := context.Background()

	var count int64
	err := h.DB.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count rows in %s: %v", table, err)
	}

	if count != expected {
		t.Errorf("Expected %d rows in %s, got %d", expected, table, count)
	}
}

// GetGameByID retrieves a game from database by ID
func (h *TestHelper) GetGameByID(t *testing.T, gameID int64) *entity.Game {
	t.Helper()

	ctx := context.Background()

	var game entity.Game
	query := `
		SELECT game_id, status, player_one_address, player_two_address,
			   player_one_choice, player_two_choice,
			   bet_amount, winner_address, payout_amount,
			   service_fee_numerator, referrer_fee_numerator,
			   waiting_timeout_seconds, lowest_bid_allowed, highest_bid_allowed,
			   fee_receiver_address,
			   created_at, joined_at, revealed_at, completed_at,
			   init_tx_hash, join_tx_hash, reveal_tx_hash, complete_tx_hash
		FROM games WHERE game_id = $1
	`

	err := h.DB.QueryRow(ctx, query, gameID).Scan(
		&game.GameID,
		&game.Status,
		&game.PlayerOneAddress,
		&game.PlayerTwoAddress,
		&game.PlayerOneChoice,
		&game.PlayerTwoChoice,
		&game.BetAmount,
		&game.WinnerAddress,
		&game.PayoutAmount,
		&game.ServiceFeeNumerator,
		&game.ReferrerFeeNumerator,
		&game.WaitingTimeoutSeconds,
		&game.LowestBidAllowed,
		&game.HighestBidAllowed,
		&game.FeeReceiverAddress,
		&game.CreatedAt,
		&game.JoinedAt,
		&game.RevealedAt,
		&game.CompletedAt,
		&game.InitTxHash,
		&game.JoinTxHash,
		&game.RevealTxHash,
		&game.CompleteTxHash,
	)

	if err != nil {
		t.Fatalf("Failed to get game %d: %v", gameID, err)
	}

	return &game
}
