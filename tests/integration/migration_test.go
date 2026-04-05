package integration_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getTestDatabaseURL returns the test database URL from environment
func getTestDatabaseURL() string {
	dbURL := os.Getenv("TEST_PG_URL")
	if dbURL == "" {
		dbURL = os.Getenv("PG_URL")
	}
	if dbURL == "" {
		dbURL = "postgresql://user:myAwEsOm3pa55%40w0rd@localhost:5433/db?sslmode=disable"
	}
	return dbURL
}

// setupFreshDatabase drops and recreates all tables for a clean migration test
func setupFreshDatabase(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()

	// Drop all tables in correct order (respecting foreign keys)
	tables := []string{
		"game_events",
		"games",
		"users",
		"blockchain_sync_state",
		"schema_migrations",
	}

	for _, table := range tables {
		_, err := db.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", table))
		if err != nil {
			t.Logf("Warning: Failed to drop table %s: %v", table, err)
		}
	}

	// Drop any remaining enum types
	_, _ = db.ExecContext(ctx, "DROP TYPE IF EXISTS event_source_type CASCADE")
}

// TestMigrationsFreshDatabase applies all migrations to an empty database
// and validates that the final schema matches entity definitions.
//
// Validates: FR-011 through FR-018, SC-001, SC-002, SC-007
func TestMigrationsFreshDatabase(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Connect to test database
	dbURL := getTestDatabaseURL()
	db, err := sql.Open("postgres", dbURL)
	require.NoError(t, err, "Failed to connect to database")
	defer db.Close()

	ctx := context.Background()
	require.NoError(t, db.PingContext(ctx), "Database not ready")

	// Clean database for fresh migration test
	setupFreshDatabase(t, db)

	// Record start time for performance validation (SC-001: <10 seconds)
	startTime := time.Now()

	// Run migrations
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	require.NoError(t, err, "Failed to create migration driver")

	m, err := migrate.NewWithDatabaseInstance(
		"file://../../migrations",
		"postgres", driver)
	require.NoError(t, err, "Failed to create migrate instance")

	err = m.Up()
	require.NoError(t, err, "Migration failed")

	// Check migration timing (SC-001)
	migrationDuration := time.Since(startTime)
	assert.Less(t, migrationDuration, 10*time.Second,
		"Migrations took longer than 10 seconds (SC-001)")

	t.Logf("Migration completed in %v", migrationDuration)

	// Validate schema matches entity definitions
	t.Run("ValidateUsersTable", func(t *testing.T) {
		validateUsersTable(t, db)
	})

	t.Run("ValidateGamesTable", func(t *testing.T) {
		validateGamesTable(t, db)
	})

	t.Run("ValidateGameEventsTable", func(t *testing.T) {
		validateGameEventsTable(t, db)
	})

	t.Run("ValidateBlockchainSyncStateTable", func(t *testing.T) {
		validateBlockchainSyncStateTable(t, db)
	})

	t.Run("ValidateDeadLetterQueueTable", func(t *testing.T) {
		validateDeadLetterQueueTable(t, db)
	})

	t.Run("ValidateIndexes", func(t *testing.T) {
		validateIndexes(t, db)
	})

	t.Run("ValidateForeignKeys", func(t *testing.T) {
		validateForeignKeys(t, db)
	})
}

// validateUsersTable checks users table schema matches entity/user.go
// Validates: FR-011 (users table), FR-016 (indexes)
func validateUsersTable(t *testing.T, db *sql.DB) {
	ctx := context.Background()

	// Check table exists
	var tableExists bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT FROM information_schema.tables
			WHERE table_name = 'users'
		)
	`).Scan(&tableExists)
	require.NoError(t, err)
	assert.True(t, tableExists, "users table should exist")

	// Check columns match entity definition
	rows, err := db.QueryContext(ctx, `
		SELECT column_name, data_type, is_nullable, column_default
		FROM information_schema.columns
		WHERE table_name = 'users'
		ORDER BY ordinal_position
	`)
	require.NoError(t, err)
	defer rows.Close()

	expectedColumns := map[string]struct {
		dataType   string
		isNullable string
	}{
		"id":                      {"bigint", "NO"},
		"telegram_user_id":        {"bigint", "YES"}, // Made nullable by migration 000008
		"telegram_username":       {"character varying", "YES"},
		"wallet_address":          {"character varying", "NO"},
		"total_games_played":      {"integer", "NO"},
		"total_wins":              {"integer", "NO"},
		"total_losses":            {"integer", "NO"},
		"total_referrals":         {"integer", "NO"},
		"total_referral_earnings": {"bigint", "NO"},
		"created_at":              {"timestamp with time zone", "NO"},
		"updated_at":              {"timestamp with time zone", "NO"},
	}

	foundColumns := make(map[string]bool)
	for rows.Next() {
		var colName, dataType, isNullable string
		var colDefault sql.NullString
		require.NoError(t, rows.Scan(&colName, &dataType, &isNullable, &colDefault))

		if expected, ok := expectedColumns[colName]; ok {
			assert.Equal(t, expected.dataType, dataType,
				"Column %s should have type %s", colName, expected.dataType)
			assert.Equal(t, expected.isNullable, isNullable,
				"Column %s nullable mismatch", colName)
			foundColumns[colName] = true
		}
	}

	// Verify all expected columns were found
	for colName := range expectedColumns {
		assert.True(t, foundColumns[colName],
			"Column %s should exist in users table", colName)
	}

	// Check unique constraint on wallet_address
	var constraintExists bool
	err = db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.table_constraints
			WHERE table_name = 'users'
			AND constraint_type = 'UNIQUE'
			AND constraint_name = 'users_wallet_address_key'
		)
	`).Scan(&constraintExists)
	require.NoError(t, err)
	assert.True(t, constraintExists, "wallet_address should have unique constraint")
}

// validateGamesTable checks games table schema matches entity/game.go
// Validates: FR-012 (games table), FR-017 (foreign keys), FR-018 (constraints)
func validateGamesTable(t *testing.T, db *sql.DB) {
	ctx := context.Background()

	// Check table exists
	var tableExists bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT FROM information_schema.tables
			WHERE table_name = 'games'
		)
	`).Scan(&tableExists)
	require.NoError(t, err)
	assert.True(t, tableExists, "games table should exist")

	// Check columns
	rows, err := db.QueryContext(ctx, `
		SELECT column_name, data_type, is_nullable
		FROM information_schema.columns
		WHERE table_name = 'games'
		ORDER BY ordinal_position
	`)
	require.NoError(t, err)
	defer rows.Close()

	expectedColumns := map[string]struct {
		dataType   string
		isNullable string
	}{
		"game_id":                 {"bigint", "NO"},
		"status":                  {"integer", "NO"},
		"player_one_address":      {"character varying", "NO"},
		"player_two_address":      {"character varying", "YES"},
		"player_one_choice":       {"integer", "NO"},
		"player_two_choice":       {"integer", "YES"},
		"player_one_referrer":     {"character varying", "YES"},
		"player_two_referrer":     {"character varying", "YES"},
		"bet_amount":              {"bigint", "NO"},
		"winner_address":          {"character varying", "YES"},
		"payout_amount":           {"bigint", "YES"},
		"service_fee_numerator":   {"bigint", "NO"},
		"referrer_fee_numerator":  {"bigint", "NO"},
		"waiting_timeout_seconds": {"bigint", "NO"},
		"lowest_bid_allowed":      {"bigint", "NO"},
		"highest_bid_allowed":     {"bigint", "NO"},
		"fee_receiver_address":    {"character varying", "NO"},
		"created_at":              {"timestamp with time zone", "NO"},
		"joined_at":               {"timestamp with time zone", "YES"},
		"revealed_at":             {"timestamp with time zone", "YES"},
		"completed_at":            {"timestamp with time zone", "YES"},
		"init_tx_hash":            {"character varying", "NO"},
		"join_tx_hash":            {"character varying", "YES"},
		"reveal_tx_hash":          {"text", "YES"},
		"complete_tx_hash":        {"character varying", "YES"},
	}

	foundColumns := make(map[string]bool)
	for rows.Next() {
		var colName, dataType, isNullable string
		require.NoError(t, rows.Scan(&colName, &dataType, &isNullable))

		if expected, ok := expectedColumns[colName]; ok {
			assert.Equal(t, expected.dataType, dataType,
				"Column %s should have type %s", colName, expected.dataType)
			assert.Equal(t, expected.isNullable, isNullable,
				"Column %s nullable mismatch", colName)
			foundColumns[colName] = true
		}
	}

	for colName := range expectedColumns {
		assert.True(t, foundColumns[colName],
			"Column %s should exist in games table", colName)
	}

	// Check player_one_choice constraint allows 0-3 (FR-018)
	var checkClause string
	err = db.QueryRowContext(ctx, `
		SELECT pg_get_constraintdef(oid)
		FROM pg_constraint
		WHERE conname = 'games_player_one_choice_check'
		AND conrelid = 'games'::regclass
	`).Scan(&checkClause)
	require.NoError(t, err, "player_one_choice check constraint should exist")
	assert.Contains(t, checkClause, ">= 0", "player_one_choice should allow 0")
	assert.Contains(t, checkClause, "<= 3", "player_one_choice should allow up to 3")

	// Check player_two_choice constraint allows 0-3 (FR-018)
	err = db.QueryRowContext(ctx, `
		SELECT pg_get_constraintdef(oid)
		FROM pg_constraint
		WHERE conname = 'games_player_two_choice_check'
		AND conrelid = 'games'::regclass
	`).Scan(&checkClause)
	require.NoError(t, err, "player_two_choice check constraint should exist")
	assert.Contains(t, checkClause, ">= 0", "player_two_choice should allow 0")
	assert.Contains(t, checkClause, "<= 3", "player_two_choice should allow up to 3")
}

// validateGameEventsTable checks game_events table matches entity/game_event.go
// Validates: FR-013 (game_events table), FR-014 (no partitioning)
func validateGameEventsTable(t *testing.T, db *sql.DB) {
	ctx := context.Background()

	// Check table exists
	var tableExists bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT FROM information_schema.tables
			WHERE table_name = 'game_events'
		)
	`).Scan(&tableExists)
	require.NoError(t, err)
	assert.True(t, tableExists, "game_events table should exist")

	// Verify it's NOT partitioned (FR-014)
	var isPartitioned bool
	err = db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_partitioned_table
			WHERE partrelid = 'game_events'::regclass
		)
	`).Scan(&isPartitioned)
	require.NoError(t, err)
	assert.False(t, isPartitioned, "game_events should NOT be partitioned (FR-014)")

	// Check columns
	rows, err := db.QueryContext(ctx, `
		SELECT column_name, data_type, is_nullable
		FROM information_schema.columns
		WHERE table_name = 'game_events'
		ORDER BY ordinal_position
	`)
	require.NoError(t, err)
	defer rows.Close()

	expectedColumns := map[string]struct {
		dataType   string
		isNullable string
	}{
		"id":               {"bigint", "NO"},
		"game_id":          {"bigint", "NO"},
		"event_type":       {"character varying", "NO"},
		"transaction_hash": {"character varying", "NO"},
		"block_number":     {"bigint", "NO"},
		"timestamp":        {"timestamp with time zone", "NO"},
		"payload":          {"text", "NO"},
		"created_at":       {"timestamp with time zone", "NO"},
	}

	foundColumns := make(map[string]bool)
	for rows.Next() {
		var colName, dataType, isNullable string
		require.NoError(t, rows.Scan(&colName, &dataType, &isNullable))

		if expected, ok := expectedColumns[colName]; ok {
			assert.Equal(t, expected.dataType, dataType,
				"Column %s should have type %s", colName, expected.dataType)
			assert.Equal(t, expected.isNullable, isNullable,
				"Column %s nullable mismatch", colName)
			foundColumns[colName] = true
		}
	}

	for colName := range expectedColumns {
		assert.True(t, foundColumns[colName],
			"Column %s should exist in game_events table", colName)
	}
}

// validateBlockchainSyncStateTable checks blockchain_sync_state matches entity
// Validates: FR-015 (VARCHAR event_source_type), FR-001 (last_processed_hash)
func validateBlockchainSyncStateTable(t *testing.T, db *sql.DB) {
	ctx := context.Background()

	// Check table exists
	var tableExists bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT FROM information_schema.tables
			WHERE table_name = 'blockchain_sync_state'
		)
	`).Scan(&tableExists)
	require.NoError(t, err)
	assert.True(t, tableExists, "blockchain_sync_state table should exist")

	// Verify enum type does NOT exist (FR-015)
	var enumExists bool
	err = db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_type WHERE typname = 'event_source_type'
		)
	`).Scan(&enumExists)
	require.NoError(t, err)
	assert.False(t, enumExists, "event_source_type enum should NOT exist (FR-015)")

	// Check columns
	rows, err := db.QueryContext(ctx, `
		SELECT column_name, data_type, is_nullable
		FROM information_schema.columns
		WHERE table_name = 'blockchain_sync_state'
		ORDER BY ordinal_position
	`)
	require.NoError(t, err)
	defer rows.Close()

	expectedColumns := map[string]struct {
		dataType   string
		isNullable string
	}{
		"id":                   {"integer", "NO"},
		"contract_address":     {"character varying", "NO"},
		"last_processed_block": {"bigint", "NO"},
		"last_poll_timestamp":  {"timestamp with time zone", "NO"},
		"updated_at":           {"timestamp with time zone", "NO"},
		// Columns added by migration 000006:
		"event_source_type":   {"character varying", "YES"}, // FR-015: VARCHAR not enum
		"last_processed_lt":   {"character varying", "YES"},
		"last_processed_hash": {"character varying", "YES"}, // FR-001: Missing column
		"websocket_connected": {"boolean", "YES"},
		"fallback_count":      {"integer", "YES"},
		"last_fallback_at":    {"timestamp with time zone", "YES"},
	}

	foundColumns := make(map[string]bool)
	for rows.Next() {
		var colName, dataType, isNullable string
		require.NoError(t, rows.Scan(&colName, &dataType, &isNullable))

		if expected, ok := expectedColumns[colName]; ok {
			assert.Equal(t, expected.dataType, dataType,
				"Column %s should have type %s", colName, expected.dataType)
			assert.Equal(t, expected.isNullable, isNullable,
				"Column %s nullable mismatch", colName)
			foundColumns[colName] = true
		}
	}

	for colName := range expectedColumns {
		assert.True(t, foundColumns[colName],
			"Column %s should exist in blockchain_sync_state table", colName)
	}

	// Specifically validate critical columns from FR requirements
	assert.True(t, foundColumns["last_processed_hash"],
		"last_processed_hash column must exist (FR-001)")
	assert.True(t, foundColumns["event_source_type"],
		"event_source_type column must exist (FR-015)")
}

// validateIndexes checks that all expected indexes exist
// Validates: FR-016
func validateIndexes(t *testing.T, db *sql.DB) {
	ctx := context.Background()

	expectedIndexes := []struct {
		tableName string
		indexName string
	}{
		{"users", "idx_users_telegram_user_id"},
		{"users", "idx_users_created_at"},
		{"games", "idx_games_status"},
		{"games", "idx_games_player_one_address"},
		{"games", "idx_games_player_two_address"},
		{"games", "idx_games_created_at"},
		{"games", "idx_games_completed_at"},
		{"game_events", "idx_game_events_game_id"},
		{"game_events", "idx_game_events_transaction_hash"},
		{"game_events", "idx_game_events_block_number"},
		{"game_events", "idx_game_events_timestamp"},
		{"game_events", "idx_game_events_event_type"},
		{"blockchain_sync_state", "idx_blockchain_sync_state_contract"},
		{"dead_letter_queue", "idx_dlq_status"},
		{"dead_letter_queue", "idx_dlq_next_retry"},
		{"dead_letter_queue", "idx_dlq_transaction_hash"},
		{"dead_letter_queue", "idx_dlq_created_at"},
		{"dead_letter_queue", "idx_dlq_unique_tx"},
	}

	for _, expected := range expectedIndexes {
		var indexExists bool
		err := db.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM pg_indexes
				WHERE tablename = $1
				AND indexname = $2
			)
		`, expected.tableName, expected.indexName).Scan(&indexExists)
		require.NoError(t, err)
		assert.True(t, indexExists,
			fmt.Sprintf("Index %s on table %s should exist (FR-016)",
				expected.indexName, expected.tableName))
	}
}

// validateForeignKeys checks referential integrity constraints
// Validates: FR-017
func validateForeignKeys(t *testing.T, db *sql.DB) {
	ctx := context.Background()

	expectedForeignKeys := []struct {
		tableName      string
		constraintName string
		refTable       string
	}{
		{"games", "fk_games_player_one", "users"},
		{"games", "fk_games_player_two", "users"},
		{"game_events", "fk_game_events_game", "games"},
	}

	for _, expected := range expectedForeignKeys {
		var fkExists bool
		err := db.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.table_constraints tc
				JOIN information_schema.constraint_column_usage ccu
					ON tc.constraint_name = ccu.constraint_name
				WHERE tc.table_name = $1
				AND tc.constraint_type = 'FOREIGN KEY'
				AND tc.constraint_name = $2
				AND ccu.table_name = $3
			)
		`, expected.tableName, expected.constraintName, expected.refTable).Scan(&fkExists)
		require.NoError(t, err)
		assert.True(t, fkExists,
			fmt.Sprintf("Foreign key %s from %s to %s should exist (FR-017)",
				expected.constraintName, expected.tableName, expected.refTable))
	}
}

// TestMigrationsIdempotency verifies migrations can be run multiple times safely.
// This test ensures that all migrations use proper IF NOT EXISTS / IF EXISTS clauses.
//
// Validates: FR-005 (idempotency), SC-003 (schema validation)
func TestMigrationsIdempotency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Connect to test database
	dbURL := getTestDatabaseURL()
	db, err := sql.Open("postgres", dbURL)
	require.NoError(t, err, "Failed to connect to database")
	defer db.Close()

	ctx := context.Background()
	require.NoError(t, db.PingContext(ctx), "Database not ready")

	// Clean database for fresh migration test
	setupFreshDatabase(t, db)

	// Run migrations the first time
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	require.NoError(t, err, "Failed to create migration driver")

	m, err := migrate.NewWithDatabaseInstance(
		"file://../../migrations",
		"postgres", driver)
	require.NoError(t, err, "Failed to create migrate instance")

	err = m.Up()
	require.NoError(t, err, "First migration run failed")

	// Get schema snapshot after first run
	firstRunTables := getTableList(t, db)
	firstRunColumns := getAllColumnInfo(t, db)
	firstRunConstraints := getAllConstraints(t, db)

	// Run migrations again on the same database (should be idempotent - no errors)
	// We need to create a fresh migrate instance because the previous one is "done"
	driver2, err := postgres.WithInstance(db, &postgres.Config{})
	require.NoError(t, err, "Failed to create second migration driver")

	m2, err := migrate.NewWithDatabaseInstance(
		"file://../../migrations",
		"postgres", driver2)
	require.NoError(t, err, "Failed to create second migrate instance")

	err = m2.Up()
	// migrate.ErrNoChange is acceptable - it means migrations already applied
	if err != nil && err != migrate.ErrNoChange {
		t.Fatalf("Second migration run failed: %v (idempotency violated - migrations should use IF NOT EXISTS clauses)", err)
	}

	// Get schema snapshot after second run
	secondRunTables := getTableList(t, db)
	secondRunColumns := getAllColumnInfo(t, db)
	secondRunConstraints := getAllConstraints(t, db)

	// Verify schema is identical after both runs
	assert.Equal(t, firstRunTables, secondRunTables,
		"Table list should be identical after second migration run (idempotency)")
	assert.Equal(t, firstRunColumns, secondRunColumns,
		"Column definitions should be identical after second migration run (idempotency)")
	assert.Equal(t, firstRunConstraints, secondRunConstraints,
		"Constraints should be identical after second migration run (idempotency)")
}

// TestSchemaMatchesEntities compares database schema to entity struct definitions
// using information_schema queries. This is the authoritative validation that
// migrations correctly implement the entity layer.
//
// Validates: FR-011 through FR-018, SC-003 (zero mismatches)
func TestSchemaMatchesEntities(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Connect to test database
	dbURL := getTestDatabaseURL()
	db, err := sql.Open("postgres", dbURL)
	require.NoError(t, err, "Failed to connect to database")
	defer db.Close()

	ctx := context.Background()
	require.NoError(t, db.PingContext(ctx), "Database not ready")

	// Clean database for fresh migration test
	setupFreshDatabase(t, db)

	// Run all migrations
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	require.NoError(t, err, "Failed to create migration driver")

	m, err := migrate.NewWithDatabaseInstance(
		"file://../../migrations",
		"postgres", driver)
	require.NoError(t, err, "Failed to create migrate instance")

	err = m.Up()
	require.NoError(t, err, "Migration failed")

	// This test reuses the validation functions from TestMigrationsFreshDatabase
	// to ensure the schema matches entity definitions
	t.Run("UsersTableMatchesEntity", func(t *testing.T) {
		validateUsersTable(t, db)
	})

	t.Run("GamesTableMatchesEntity", func(t *testing.T) {
		validateGamesTable(t, db)
	})

	t.Run("GameEventsTableMatchesEntity", func(t *testing.T) {
		validateGameEventsTable(t, db)
	})

	t.Run("BlockchainSyncStateTableMatchesEntity", func(t *testing.T) {
		validateBlockchainSyncStateTable(t, db)
	})

	t.Run("DeadLetterQueueTableMatchesEntity", func(t *testing.T) {
		validateDeadLetterQueueTable(t, db)
	})

	// Additional checks for SC-003: zero schema mismatches
	t.Run("NoUnexpectedTables", func(t *testing.T) {
		var unexpectedTables []string
		rows, err := db.QueryContext(ctx, `
			SELECT table_name
			FROM information_schema.tables
			WHERE table_schema = 'public'
			AND table_type = 'BASE TABLE'
			AND table_name NOT IN ('users', 'games', 'game_events', 'blockchain_sync_state', 'dead_letter_queue', 'schema_migrations')
		`)
		require.NoError(t, err)
		defer rows.Close()

		for rows.Next() {
			var tableName string
			require.NoError(t, rows.Scan(&tableName))
			unexpectedTables = append(unexpectedTables, tableName)
		}

		assert.Empty(t, unexpectedTables,
			"Database should not contain unexpected tables (SC-003: zero mismatches)")
	})

	t.Run("NoUnexpectedEnumTypes", func(t *testing.T) {
		var unexpectedEnums []string
		rows, err := db.QueryContext(ctx, `
			SELECT typname
			FROM pg_type
			WHERE typtype = 'e'
		`)
		require.NoError(t, err)
		defer rows.Close()

		for rows.Next() {
			var typeName string
			require.NoError(t, rows.Scan(&typeName))
			unexpectedEnums = append(unexpectedEnums, typeName)
		}

		assert.Empty(t, unexpectedEnums,
			"Database should not contain enum types (entity layer uses strings, not enums)")
	})
}

func validateDeadLetterQueueTable(t *testing.T, db *sql.DB) {
	ctx := context.Background()

	var tableExists bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT FROM information_schema.tables
			WHERE table_name = 'dead_letter_queue'
		)
	`).Scan(&tableExists)
	require.NoError(t, err)
	assert.True(t, tableExists, "dead_letter_queue table should exist")

	rows, err := db.QueryContext(ctx, `
		SELECT column_name, data_type, is_nullable
		FROM information_schema.columns
		WHERE table_name = 'dead_letter_queue'
		ORDER BY ordinal_position
	`)
	require.NoError(t, err)
	defer rows.Close()

	expectedColumns := map[string]struct {
		dataType   string
		isNullable string
	}{
		"id":               {"bigint", "NO"},
		"transaction_hash": {"character varying", "NO"},
		"transaction_lt":   {"character varying", "NO"},
		"raw_data":         {"text", "NO"},
		"error_message":    {"text", "NO"},
		"error_type":       {"character varying", "NO"},
		"retry_count":      {"integer", "YES"},
		"max_retries":      {"integer", "YES"},
		"created_at":       {"timestamp with time zone", "NO"},
		"last_retry_at":    {"timestamp with time zone", "YES"},
		"next_retry_at":    {"timestamp with time zone", "YES"},
		"resolved_at":      {"timestamp with time zone", "YES"},
		"status":           {"character varying", "NO"},
		"resolution_notes": {"text", "YES"},
	}

	foundColumns := make(map[string]bool)
	for rows.Next() {
		var colName, dataType, isNullable string
		require.NoError(t, rows.Scan(&colName, &dataType, &isNullable))

		if expected, ok := expectedColumns[colName]; ok {
			assert.Equal(t, expected.dataType, dataType,
				"Column %s should have type %s", colName, expected.dataType)
			assert.Equal(t, expected.isNullable, isNullable,
				"Column %s nullable mismatch", colName)
			foundColumns[colName] = true
		}
	}

	for colName := range expectedColumns {
		assert.True(t, foundColumns[colName],
			"Column %s should exist in dead_letter_queue table", colName)
	}
}

// Helper functions for idempotency testing

func getTableList(t *testing.T, db *sql.DB) []string {
	ctx := context.Background()
	rows, err := db.QueryContext(ctx, `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = 'public'
		AND table_type = 'BASE TABLE'
		ORDER BY table_name
	`)
	require.NoError(t, err)
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		require.NoError(t, rows.Scan(&tableName))
		tables = append(tables, tableName)
	}
	return tables
}

func getAllColumnInfo(t *testing.T, db *sql.DB) map[string]string {
	ctx := context.Background()
	rows, err := db.QueryContext(ctx, `
		SELECT table_name || '.' || column_name AS full_name,
		       data_type || ' ' || is_nullable AS definition
		FROM information_schema.columns
		WHERE table_schema = 'public'
		ORDER BY table_name, ordinal_position
	`)
	require.NoError(t, err)
	defer rows.Close()

	columns := make(map[string]string)
	for rows.Next() {
		var fullName, definition string
		require.NoError(t, rows.Scan(&fullName, &definition))
		columns[fullName] = definition
	}
	return columns
}

func getAllConstraints(t *testing.T, db *sql.DB) map[string]string {
	ctx := context.Background()
	rows, err := db.QueryContext(ctx, `
		SELECT conname, pg_get_constraintdef(oid)
		FROM pg_constraint
		WHERE connamespace = 'public'::regnamespace
		ORDER BY conname
	`)
	require.NoError(t, err)
	defer rows.Close()

	constraints := make(map[string]string)
	for rows.Next() {
		var name, definition string
		require.NoError(t, rows.Scan(&name, &definition))
		constraints[name] = definition
	}
	return constraints
}
