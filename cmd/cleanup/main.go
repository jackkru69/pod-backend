package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"pod-backend/config"
	"pod-backend/pkg/logger"
	"pod-backend/pkg/postgres"
)

// Cleanup job for data retention (T123)
// Removes old records according to retention policy
func main() {
	// Parse command line flags
	dryRun := flag.Bool("dry-run", false, "Run in dry-run mode (no actual deletion)")
	retentionDays := flag.Int("retention-days", 365, "Number of days to retain data (default: 365)")
	flag.Parse()

	// Initialize logger
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	log.Info().Msg("Starting data retention cleanup job")

	// Load configuration
	cfg, err := config.NewConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	// Set log level
	_ = logger.New(cfg.Log.Level) // Initialize logger level

	// Connect to database
	pg, err := postgres.New(cfg.PG.URL, postgres.MaxPoolSize(cfg.PG.PoolMax))
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database")
	}
	defer pg.Close()

	log.Info().Msg("Connected to database")

	// Calculate cutoff date
	cutoffDate := time.Now().AddDate(0, 0, -*retentionDays).Format("2006-01-02")
	log.Info().
		Str("cutoff_date", cutoffDate).
		Int("retention_days", *retentionDays).
		Bool("dry_run", *dryRun).
		Msg("Cleanup parameters")

	ctx := context.Background()

	if *dryRun {
		log.Info().Msg("DRY RUN MODE - No data will be deleted")
		err = dryRunCleanup(ctx, pg, cutoffDate)
	} else {
		log.Warn().Msg("LIVE MODE - Data will be permanently deleted")
		err = performCleanup(ctx, pg, cutoffDate)
	}

	if err != nil {
		log.Fatal().Err(err).Msg("Cleanup failed")
	}

	log.Info().Msg("Cleanup job completed successfully")
}

// dryRunCleanup counts records that would be deleted
func dryRunCleanup(ctx context.Context, pg *postgres.Postgres, cutoffDate string) error {
	// Count old game_events
	var eventsCount int64
	err := pg.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM game_events WHERE created_at < $1",
		cutoffDate,
	).Scan(&eventsCount)
	if err != nil {
		return fmt.Errorf("count game_events: %w", err)
	}

	// Count old games
	var gamesCount int64
	err = pg.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM games WHERE created_at < $1",
		cutoffDate,
	).Scan(&gamesCount)
	if err != nil {
		return fmt.Errorf("count games: %w", err)
	}

	// Count inactive users (no games in retention period)
	var usersCount int64
	err = pg.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM users
		 WHERE wallet_address NOT IN (
			SELECT DISTINCT player_one_address FROM games WHERE created_at >= $1
			UNION
			SELECT DISTINCT player_two_address FROM games WHERE created_at >= $1 AND player_two_address IS NOT NULL
		 )`,
		cutoffDate,
	).Scan(&usersCount)
	if err != nil {
		return fmt.Errorf("count users: %w", err)
	}

	log.Info().
		Int64("game_events", eventsCount).
		Int64("games", gamesCount).
		Int64("inactive_users", usersCount).
		Msg("Records that would be deleted (dry run)")

	return nil
}

// performCleanup deletes old records
func performCleanup(ctx context.Context, pg *postgres.Postgres, cutoffDate string) error {
	// Start transaction
	tx, err := pg.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Delete old game_events (cascade from games)
	result, err := tx.Exec(ctx,
		"DELETE FROM game_events WHERE created_at < $1",
		cutoffDate,
	)
	if err != nil {
		return fmt.Errorf("delete game_events: %w", err)
	}
	eventsDeleted := result.RowsAffected()
	log.Info().Int64("deleted", eventsDeleted).Msg("Deleted old game events")

	// Delete old games
	result, err = tx.Exec(ctx,
		"DELETE FROM games WHERE created_at < $1",
		cutoffDate,
	)
	if err != nil {
		return fmt.Errorf("delete games: %w", err)
	}
	gamesDeleted := result.RowsAffected()
	log.Info().Int64("deleted", gamesDeleted).Msg("Deleted old games")

	// Delete inactive users (optional - commented out by default)
	// Uncomment if you want to remove users without recent activity
	/*
		result, err = tx.Exec(ctx,
			`DELETE FROM users
			 WHERE wallet_address NOT IN (
				SELECT DISTINCT player_one_address FROM games WHERE created_at >= $1
				UNION
				SELECT DISTINCT player_two_address FROM games WHERE created_at >= $1 AND player_two_address IS NOT NULL
			 )`,
			cutoffDate,
		)
		if err != nil {
			return fmt.Errorf("delete users: %w", err)
		}
		usersDeleted := result.RowsAffected()
		log.Info().Int64("deleted", usersDeleted).Msg("Deleted inactive users")
	*/

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	log.Info().
		Int64("game_events_deleted", eventsDeleted).
		Int64("games_deleted", gamesDeleted).
		Msg("Cleanup completed")

	return nil
}
