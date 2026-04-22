package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"pod-backend/config"
	httpRouter "pod-backend/internal/controller/http"
	"pod-backend/internal/entity"
	postgresrepo "pod-backend/internal/repository/postgres"
	"pod-backend/internal/usecase"
	"pod-backend/pkg/logger"
	"pod-backend/pkg/postgres"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGETGames tests the GET /api/v1/games endpoint
func TestGETGames(t *testing.T) {
	t.Run("should return games list with default status filter", func(t *testing.T) {
		// Arrange
		app := setupTestApp(t)
		defer cleanupTestDB(t)

		// Seed user first (FK constraint)
		seedUser(t, &entity.User{
			TelegramUserID:   Int64Ptr(123456789),
			TelegramUsername: "player_one",
			WalletAddress:    "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2",
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		})

		// Seed test data
		seedGame(t, &entity.Game{
			GameID:                1,
			Status:                entity.GameStatusWaitingForOpponent,
			PlayerOneAddress:      "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2",
			PlayerOneChoice:       entity.CoinSideHeads,
			BetAmount:             1000000000,
			ServiceFeeNumerator:   100,
			ReferrerFeeNumerator:  50,
			WaitingTimeoutSeconds: 3600,
			LowestBidAllowed:      100000000,
			HighestBidAllowed:     10000000000,
			FeeReceiverAddress:    "EQBvW8Z5huBkMJYdnfAEM5JqTNLuuU3FYxrVjxFBzXn3r95X",
			InitTxHash:            "abc123def456",
			CreatedAt:             time.Now(),
		})

		// Act
		req := httptest.NewRequest("GET", "/api/v1/games?status=1", nil)
		resp, err := app.Test(req)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var result map[string]interface{}
		err = json.Unmarshal(body, &result)
		require.NoError(t, err)

		assert.Contains(t, result, "games")
		assert.Contains(t, result, "total")
		assert.Contains(t, result, "limit")
		assert.Contains(t, result, "offset")
	})

	t.Run("should return empty list when no games available", func(t *testing.T) {
		// Arrange
		app := setupTestApp(t)
		defer cleanupTestDB(t)

		// Act
		req := httptest.NewRequest("GET", "/api/v1/games?status=1", nil)
		resp, err := app.Test(req)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var result map[string]interface{}
		err = json.Unmarshal(body, &result)
		require.NoError(t, err)

		games := result["games"].([]interface{})
		assert.Empty(t, games)
		assert.Equal(t, float64(0), result["total"])
	})

	t.Run("should handle invalid status parameter", func(t *testing.T) {
		// Arrange
		app := setupTestApp(t)
		defer cleanupTestDB(t)

		// Act
		req := httptest.NewRequest("GET", "/api/v1/games?status=99", nil)
		resp, err := app.Test(req)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var result map[string]interface{}
		err = json.Unmarshal(body, &result)
		require.NoError(t, err)

		assert.Contains(t, result, "error")
	})

	t.Run("should support pagination with limit and offset", func(t *testing.T) {
		// Arrange
		app := setupTestApp(t)
		defer cleanupTestDB(t)

		// Seed user first (FK constraint)
		seedUser(t, &entity.User{
			TelegramUserID:   Int64Ptr(123456789),
			TelegramUsername: "player_one",
			WalletAddress:    "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2",
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		})

		// Seed multiple games
		for i := 1; i <= 5; i++ {
			seedGame(t, &entity.Game{
				GameID:                int64(i),
				Status:                entity.GameStatusWaitingForOpponent,
				PlayerOneAddress:      "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2",
				PlayerOneChoice:       entity.CoinSideHeads,
				BetAmount:             1000000000,
				ServiceFeeNumerator:   100,
				ReferrerFeeNumerator:  50,
				WaitingTimeoutSeconds: 3600,
				LowestBidAllowed:      100000000,
				HighestBidAllowed:     10000000000,
				FeeReceiverAddress:    "EQBvW8Z5huBkMJYdnfAEM5JqTNLuuU3FYxrVjxFBzXn3r95X",
				InitTxHash:            "abc" + string(rune(i)),
				CreatedAt:             time.Now(),
			})
		}

		// Act
		req := httptest.NewRequest("GET", "/api/v1/games?status=1&limit=2&offset=1", nil)
		resp, err := app.Test(req)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var result map[string]interface{}
		err = json.Unmarshal(body, &result)
		require.NoError(t, err)

		games := result["games"].([]interface{})
		assert.Len(t, games, 2) // Should return 2 games
		assert.Equal(t, float64(2), result["limit"])
		assert.Equal(t, float64(1), result["offset"])
	})
}

// TestGETGameByID tests the GET /api/v1/games/:id endpoint
func TestGETGameByID(t *testing.T) {
	t.Run("should return game details when found", func(t *testing.T) {
		// Arrange
		app := setupTestApp(t)
		defer cleanupTestDB(t)

		// Seed user first (FK constraint)
		seedUser(t, &entity.User{
			TelegramUserID:   Int64Ptr(123456789),
			TelegramUsername: "player_one",
			WalletAddress:    "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2",
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		})

		// Seed test data
		seedGame(t, &entity.Game{
			GameID:                123,
			Status:                entity.GameStatusWaitingForOpponent,
			PlayerOneAddress:      "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2",
			PlayerOneChoice:       entity.CoinSideHeads,
			BetAmount:             1000000000,
			ServiceFeeNumerator:   100,
			ReferrerFeeNumerator:  50,
			WaitingTimeoutSeconds: 3600,
			LowestBidAllowed:      100000000,
			HighestBidAllowed:     10000000000,
			FeeReceiverAddress:    "EQBvW8Z5huBkMJYdnfAEM5JqTNLuuU3FYxrVjxFBzXn3r95X",
			InitTxHash:            "abc123def456",
			CreatedAt:             time.Now(),
		})

		// Act
		req := httptest.NewRequest("GET", "/api/v1/games/123", nil)
		resp, err := app.Test(req)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var game entity.Game
		err = json.Unmarshal(body, &game)
		require.NoError(t, err)

		assert.Equal(t, int64(123), game.GameID)
		assert.Equal(t, entity.GameStatusWaitingForOpponent, game.Status)
		assert.Equal(t, "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2", game.PlayerOneAddress)
	})

	t.Run("should return 404 when game not found", func(t *testing.T) {
		// Arrange
		app := setupTestApp(t)
		defer cleanupTestDB(t)

		// Act
		req := httptest.NewRequest("GET", "/api/v1/games/999", nil)
		resp, err := app.Test(req)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var result map[string]interface{}
		err = json.Unmarshal(body, &result)
		require.NoError(t, err)

		assert.Contains(t, result, "error")
	})

	t.Run("should handle invalid game ID parameter", func(t *testing.T) {
		// Arrange
		app := setupTestApp(t)
		defer cleanupTestDB(t)

		// Act
		req := httptest.NewRequest("GET", "/api/v1/games/invalid", nil)
		resp, err := app.Test(req)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

// TestSwaggerEndpoint tests the /swagger endpoint
func TestSwaggerEndpoint(t *testing.T) {
	t.Run("should serve Swagger UI at /swagger endpoint", func(t *testing.T) {
		// Arrange
		app := setupTestApp(t)

		// Act
		req := httptest.NewRequest("GET", "/swagger/index.html", nil)
		resp, err := app.Test(req)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		// Verify it's HTML content
		bodyStr := string(body)
		assert.Contains(t, bodyStr, "swagger")
	})

	t.Run("should serve Swagger JSON spec at /swagger/doc.json", func(t *testing.T) {
		// Arrange
		app := setupTestApp(t)

		// Act
		req := httptest.NewRequest("GET", "/swagger/doc.json", nil)
		resp, err := app.Test(req)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var swaggerSpec map[string]interface{}
		err = json.Unmarshal(body, &swaggerSpec)
		require.NoError(t, err)

		// Verify Swagger 2.0 spec structure (not OpenAPI 3.0)
		assert.Contains(t, swaggerSpec, "swagger")
		assert.Contains(t, swaggerSpec, "info")
		assert.Contains(t, swaggerSpec, "paths")

		// Verify game endpoints are documented
		paths := swaggerSpec["paths"].(map[string]interface{})
		assert.Contains(t, paths, "/api/v1/games")
		assert.Contains(t, paths, "/api/v1/games/{gameId}")
		assert.Contains(t, paths, "/api/v1/health")
	})

	t.Run("should display all game endpoints in Swagger UI", func(t *testing.T) {
		// Arrange
		app := setupTestApp(t)

		// Act - Get Swagger JSON
		req := httptest.NewRequest("GET", "/swagger/doc.json", nil)
		resp, err := app.Test(req)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var swaggerSpec map[string]interface{}
		err = json.Unmarshal(body, &swaggerSpec)
		require.NoError(t, err)

		paths := swaggerSpec["paths"].(map[string]interface{})

		// Verify GET /api/v1/games endpoint
		gamesPath := paths["/api/v1/games"].(map[string]interface{})
		assert.Contains(t, gamesPath, "get")

		// Verify GET /api/v1/games/:id endpoint
		gameByIDPath := paths["/api/v1/games/{gameId}"].(map[string]interface{})
		assert.Contains(t, gameByIDPath, "get")
	})
}

// Helper functions

var testDB *testDatabase

type testDatabase struct {
	pg *postgres.Postgres
}

func setupTestApp(t *testing.T) *fiber.App {
	// Setup test database
	testDB = setupTestDatabase(t)

	// Create Fiber app
	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": err.Error(),
			})
		},
	})

	// Create minimal config
	cfg := &config.Config{
		GameBackend: config.GameBackend{
			TelegramBotToken:   "test_bot_token",
			CORSAllowedOrigins: "http://localhost:3000", // Use specific origin to avoid CORS AllowCredentials + wildcard error
		},
		Metrics: config.Metrics{
			Enabled: false,
		},
		Swagger: config.Swagger{
			Enabled: true,
		},
	}

	// Setup logger
	l := logger.New("info")

	// Create repositories
	gameRepo := postgresrepo.NewGameRepository(testDB.pg)
	userRepo := postgresrepo.NewUserRepository(testDB.pg)

	// Create use cases
	gameQueryUC := usecase.NewGameQueryUseCase(gameRepo)
	reservationUC := usecase.NewReservationUseCase(gameRepo, nil, usecase.ReservationConfig{
		MaxPerWallet:           3,
		TimeoutSeconds:         60,
		CleanupIntervalSeconds: 5,
	})
	userManagementUC := usecase.NewUserManagementUseCase(userRepo)

	// Setup routes with RouterDeps
	deps := httpRouter.RouterDeps{
		Logger:            l,
		GameQueryUC:       gameQueryUC,
		ReservationUC:     reservationUC,
		UserManagementUC:  userManagementUC,
		BroadcastUC:       nil, // Not needed for tests
		TONClient:         nil, // Not needed for tests
		BlockchainHandler: nil, // Not needed for tests
		PG:                testDB.pg,
		GameRepo:          gameRepo,
	}
	httpRouter.NewRouter(app, cfg, deps)

	return app
}

func cleanupTestDB(t *testing.T) {
	if testDB == nil {
		return
	}

	ctx := context.Background()

	// Truncate tables in correct order (respect foreign keys)
	tables := []string{"game_events", "games", "users"}
	for _, table := range tables {
		_, err := testDB.pg.Pool.Exec(ctx, fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table))
		if err != nil {
			t.Logf("Warning: failed to truncate table %s: %v", table, err)
		}
	}
}

func setupTestDatabase(t *testing.T) *testDatabase {
	// Prefer the isolated integration-test database when available.
	pgURL := os.Getenv("TEST_PG_URL")
	if pgURL == "" {
		pgURL = os.Getenv("PG_URL")
	}
	if pgURL == "" {
		pgURL = "postgresql://postgres:postgres@localhost:15433/pod_game_test?sslmode=disable"
	}

	pg, err := postgres.New(pgURL, postgres.MaxPoolSize(5))
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}

	return &testDatabase{pg: pg}
}

func seedGame(t *testing.T, game *entity.Game) {
	if testDB == nil {
		t.Fatal("Test database not initialized")
	}

	ctx := context.Background()
	gameRepo := postgresrepo.NewGameRepository(testDB.pg)

	err := gameRepo.Create(ctx, game)
	if err != nil {
		t.Fatalf("Failed to seed game: %v", err)
	}
}

func seedUser(t *testing.T, user *entity.User) {
	if testDB == nil {
		t.Fatal("Test database not initialized")
	}

	ctx := context.Background()
	userRepo := postgresrepo.NewUserRepository(testDB.pg)

	err := userRepo.CreateOrUpdate(ctx, user)
	if err != nil {
		t.Fatalf("Failed to seed user: %v", err)
	}
}

// seedGameWithUser creates a user for the game's player one address and then creates the game.
// This ensures FK constraint is satisfied.
func seedGameWithUser(t *testing.T, game *entity.Game) {
	// Create user for player one
	seedUser(t, &entity.User{
		TelegramUserID:   Int64Ptr(123456789),
		TelegramUsername: "player_one",
		WalletAddress:    game.PlayerOneAddress,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	})
	// Create the game
	seedGame(t, game)
}

// TestUserProfile tests user profile endpoints (T065-T068)
func TestUserProfile(t *testing.T) {
	t.Run("should return user profile by wallet address (T066)", func(t *testing.T) {
		// Arrange
		app := setupTestApp(t)
		defer cleanupTestDB(t)

		testUser := &entity.User{
			TelegramUserID:   Int64Ptr(123456789),
			TelegramUsername: "testuser",
			WalletAddress:    "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2",
			TotalGamesPlayed: 10,
			TotalWins:        6,
			TotalLosses:      4,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		}
		seedUser(t, testUser)

		// Act
		req := httptest.NewRequest("GET", "/api/v1/users/EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2", nil)
		resp, err := app.Test(req)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var user entity.User
		err = json.Unmarshal(body, &user)
		require.NoError(t, err)

		assert.Equal(t, testUser.WalletAddress, user.WalletAddress)
		assert.Equal(t, testUser.TelegramUsername, user.TelegramUsername)
		assert.Equal(t, 0, user.TotalGamesPlayed)
		assert.Equal(t, 0, user.TotalWins)
		assert.Equal(t, 0, user.TotalLosses)
	})

	t.Run("should derive profile counters from indexed games instead of stale user totals", func(t *testing.T) {
		app := setupTestApp(t)
		defer cleanupTestDB(t)

		walletAddress := "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2"
		playerTwoAddress := "EQBvW8Z5huBkMJYdnfAEM5JqTNLuuU3FYxrVjxFBzXn3r95X"
		playerThreeAddress := "EQBL8Ww6LLn6lYI6YJ6xntrENcGukdxbYlR5cYsEMFihZlyV"
		referrerAddress := "EQAFmK79N5h8nDgAbA1AZt0pOEtpJR-YWZLxE-nobVhtSGcM"
		now := time.Now()
		playerTwoChoice := entity.CoinSideTails
		winnerAddress := walletAddress
		otherWinnerAddress := playerThreeAddress
		drawPayout := int64(0)
		firstCompletedAt := now.Add(-3 * time.Hour)
		secondCompletedAt := now.Add(-90 * time.Minute)
		drawCompletedAt := now.Add(-60 * time.Minute)
		cancelledCompletedAt := now.Add(-45 * time.Minute)

		for _, user := range []*entity.User{
			{
				TelegramUserID:        Int64Ptr(123456789),
				TelegramUsername:      "testuser",
				WalletAddress:         walletAddress,
				TotalGamesPlayed:      99,
				TotalWins:             98,
				TotalLosses:           97,
				TotalReferrals:        96,
				TotalReferralEarnings: 500000000,
				CreatedAt:             now,
				UpdatedAt:             now,
			},
			{
				TelegramUserID:   Int64Ptr(987654321),
				TelegramUsername: "playertwo",
				WalletAddress:    playerTwoAddress,
				CreatedAt:        now,
				UpdatedAt:        now,
			},
			{
				TelegramUserID:   Int64Ptr(111222333),
				TelegramUsername: "playerthree",
				WalletAddress:    playerThreeAddress,
				CreatedAt:        now,
				UpdatedAt:        now,
			},
			{
				TelegramUserID:   Int64Ptr(444555666),
				TelegramUsername: "referrer",
				WalletAddress:    referrerAddress,
				CreatedAt:        now,
				UpdatedAt:        now,
			},
		} {
			seedUser(t, user)
		}

		for _, game := range []*entity.Game{
			{
				GameID:                1,
				Status:                entity.GameStatusPaid,
				PlayerOneAddress:      walletAddress,
				PlayerTwoAddress:      &playerTwoAddress,
				PlayerOneChoice:       entity.CoinSideHeads,
				PlayerTwoChoice:       &playerTwoChoice,
				PlayerOneReferrer:     &referrerAddress,
				BetAmount:             1000000000,
				WinnerAddress:         &winnerAddress,
				PayoutAmount:          Int64Ptr(1900000000),
				ServiceFeeNumerator:   100,
				ReferrerFeeNumerator:  50,
				WaitingTimeoutSeconds: 3600,
				LowestBidAllowed:      100000000,
				HighestBidAllowed:     10000000000,
				FeeReceiverAddress:    playerThreeAddress,
				CreatedAt:             now.Add(-4 * time.Hour),
				CompletedAt:           &firstCompletedAt,
				InitTxHash:            "profile-derived-1",
			},
			{
				GameID:                2,
				Status:                entity.GameStatusPaid,
				PlayerOneAddress:      playerThreeAddress,
				PlayerTwoAddress:      &walletAddress,
				PlayerOneChoice:       entity.CoinSideHeads,
				PlayerTwoChoice:       &playerTwoChoice,
				BetAmount:             1000000000,
				WinnerAddress:         &otherWinnerAddress,
				PayoutAmount:          Int64Ptr(1900000000),
				ServiceFeeNumerator:   100,
				ReferrerFeeNumerator:  50,
				WaitingTimeoutSeconds: 3600,
				LowestBidAllowed:      100000000,
				HighestBidAllowed:     10000000000,
				FeeReceiverAddress:    playerTwoAddress,
				CreatedAt:             now.Add(-2 * time.Hour),
				CompletedAt:           &secondCompletedAt,
				InitTxHash:            "profile-derived-2",
			},
			{
				GameID:                3,
				Status:                entity.GameStatusPaid,
				PlayerOneAddress:      walletAddress,
				PlayerTwoAddress:      &playerThreeAddress,
				PlayerOneChoice:       entity.CoinSideHeads,
				PlayerTwoChoice:       &playerTwoChoice,
				BetAmount:             1000000000,
				WinnerAddress:         nil,
				PayoutAmount:          &drawPayout,
				ServiceFeeNumerator:   100,
				ReferrerFeeNumerator:  50,
				WaitingTimeoutSeconds: 3600,
				LowestBidAllowed:      100000000,
				HighestBidAllowed:     10000000000,
				FeeReceiverAddress:    playerTwoAddress,
				CreatedAt:             now.Add(-70 * time.Minute),
				CompletedAt:           &drawCompletedAt,
				InitTxHash:            "profile-derived-3",
			},
			{
				GameID:                4,
				Status:                entity.GameStatusPaid,
				PlayerOneAddress:      walletAddress,
				PlayerTwoAddress:      &playerTwoAddress,
				PlayerOneChoice:       entity.CoinSideHeads,
				PlayerTwoChoice:       &playerTwoChoice,
				BetAmount:             1000000000,
				WinnerAddress:         nil,
				PayoutAmount:          nil,
				ServiceFeeNumerator:   100,
				ReferrerFeeNumerator:  50,
				WaitingTimeoutSeconds: 3600,
				LowestBidAllowed:      100000000,
				HighestBidAllowed:     10000000000,
				FeeReceiverAddress:    playerThreeAddress,
				CreatedAt:             now.Add(-50 * time.Minute),
				CompletedAt:           &cancelledCompletedAt,
				InitTxHash:            "profile-derived-4",
			},
		} {
			seedGame(t, game)
		}

		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/users/%s", walletAddress), nil)
		resp, err := app.Test(req)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var user entity.User
		err = json.Unmarshal(body, &user)
		require.NoError(t, err)

		assert.Equal(t, 3, user.TotalGamesPlayed)
		assert.Equal(t, 1, user.TotalWins)
		assert.Equal(t, 1, user.TotalLosses)
		assert.Equal(t, 0, user.TotalReferrals)
		assert.Equal(t, int64(500000000), user.TotalReferralEarnings)
	})

	t.Run("should return 404 when user not found", func(t *testing.T) {
		// Arrange
		app := setupTestApp(t)
		defer cleanupTestDB(t)

		// Act
		req := httptest.NewRequest("GET", "/api/v1/users/EQNonExistentWalletAddress123456789", nil)
		resp, err := app.Test(req)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

func TestUserGameHistory(t *testing.T) {
	t.Run("should return paginated game history for user (T067)", func(t *testing.T) {
		// Arrange
		app := setupTestApp(t)
		defer cleanupTestDB(t)

		walletAddress := "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2"
		playerTwoAddress := "EQBvW8Z5huBkMJYdnfAEM5JqTNLuuU3FYxrVjxFBzXn3r95X"

		// Seed user
		testUser := &entity.User{
			TelegramUserID:   Int64Ptr(123456789),
			TelegramUsername: "testuser",
			WalletAddress:    walletAddress,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		}
		seedUser(t, testUser)

		// Seed player two user for FK constraint
		playerTwoUser := &entity.User{
			TelegramUserID:   Int64Ptr(987654321),
			TelegramUsername: "playertwo",
			WalletAddress:    playerTwoAddress,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		}
		seedUser(t, playerTwoUser)

		// Seed multiple games
		for i := 1; i <= 5; i++ {
			game := &entity.Game{
				GameID:                int64(i),
				Status:                entity.GameStatusEnded,
				PlayerOneAddress:      walletAddress,
				PlayerTwoAddress:      &playerTwoAddress,
				PlayerOneChoice:       entity.CoinSideHeads,
				BetAmount:             1000000000,
				ServiceFeeNumerator:   100,
				ReferrerFeeNumerator:  50,
				WaitingTimeoutSeconds: 3600,
				LowestBidAllowed:      100000000,
				HighestBidAllowed:     10000000000,
				FeeReceiverAddress:    "EQBvW8Z5huBkMJYdnfAEM5JqTNLuuU3FYxrVjxFBzXn3r95X",
				InitTxHash:            fmt.Sprintf("tx%d", i),
				CreatedAt:             time.Now(),
			}
			seedGame(t, game)
		}

		// Act
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/users/%s/history?limit=2&offset=1", walletAddress), nil)
		resp, err := app.Test(req)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var result map[string]interface{}
		err = json.Unmarshal(body, &result)
		require.NoError(t, err)

		assert.Contains(t, result, "games")
		assert.Contains(t, result, "wallet_address")
		assert.Contains(t, result, "limit")
		assert.Contains(t, result, "offset")
		assert.Contains(t, result, "total")
		assert.Equal(t, walletAddress, result["wallet_address"])
		assert.Equal(t, float64(2), result["limit"])
		assert.Equal(t, float64(1), result["offset"])
		assert.Equal(t, float64(5), result["total"])

		games := result["games"].([]interface{})
		assert.LessOrEqual(t, len(games), 2) // Should respect limit
	})

	t.Run("should handle invalid pagination parameters", func(t *testing.T) {
		// Arrange
		app := setupTestApp(t)
		defer cleanupTestDB(t)

		walletAddress := "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2"

		// Act
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/users/%s/history?limit=invalid", walletAddress), nil)
		resp, err := app.Test(req)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

func TestUserReferralStats(t *testing.T) {
	t.Run("should return referral statistics (T068, FR-021)", func(t *testing.T) {
		// Arrange
		app := setupTestApp(t)
		defer cleanupTestDB(t)

		walletAddress := "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2"

		// Seed user
		testUser := &entity.User{
			TelegramUserID:        Int64Ptr(123456789),
			TelegramUsername:      "testuser",
			WalletAddress:         walletAddress,
			TotalReferrals:        5,
			TotalReferralEarnings: 500000000, // 0.5 TON in nanotons
			CreatedAt:             time.Now(),
			UpdatedAt:             time.Now(),
		}
		seedUser(t, testUser)

		referredWallet := "EQBvW8Z5huBkMJYdnfAEM5JqTNLuuU3FYxrVjxFBzXn3r95X"
		opponentWallet := "EQBL8Ww6LLn6lYI6YJ6xntrENcGukdxbYlR5cYsEMFihZlyV"
		now := time.Now()
		playerTwoChoice := entity.CoinSideTails
		payout := int64(1900000000)
		completedAt := now.Add(-30 * time.Minute)

		seedUser(t, &entity.User{
			TelegramUserID:   Int64Ptr(987654321),
			TelegramUsername: "referred",
			WalletAddress:    referredWallet,
			CreatedAt:        now,
			UpdatedAt:        now,
		})
		seedUser(t, &entity.User{
			TelegramUserID:   Int64Ptr(111222333),
			TelegramUsername: "opponent",
			WalletAddress:    opponentWallet,
			CreatedAt:        now,
			UpdatedAt:        now,
		})
		seedGame(t, &entity.Game{
			GameID:                1,
			Status:                entity.GameStatusPaid,
			PlayerOneAddress:      referredWallet,
			PlayerTwoAddress:      &opponentWallet,
			PlayerOneChoice:       entity.CoinSideHeads,
			PlayerTwoChoice:       &playerTwoChoice,
			PlayerOneReferrer:     &walletAddress,
			BetAmount:             1000000000,
			PayoutAmount:          &payout,
			ServiceFeeNumerator:   100,
			ReferrerFeeNumerator:  50,
			WaitingTimeoutSeconds: 3600,
			LowestBidAllowed:      100000000,
			HighestBidAllowed:     10000000000,
			FeeReceiverAddress:    opponentWallet,
			CreatedAt:             now.Add(-1 * time.Hour),
			CompletedAt:           &completedAt,
			InitTxHash:            "referral-stats-1",
		})

		// Act
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/users/%s/referrals", walletAddress), nil)
		resp, err := app.Test(req)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var stats entity.ReferralStats
		err = json.Unmarshal(body, &stats)
		require.NoError(t, err)

		assert.Equal(t, int64(1), stats.TotalReferrals)
		assert.Equal(t, int64(1), stats.GamesReferred)
		assert.Equal(t, int64(500000000), stats.TotalReferralEarnings)
	})

	t.Run("should report unique referred wallets separately from referred games", func(t *testing.T) {
		app := setupTestApp(t)
		defer cleanupTestDB(t)

		referrerAddress := "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2"
		referredOne := "EQBvW8Z5huBkMJYdnfAEM5JqTNLuuU3FYxrVjxFBzXn3r95X"
		referredTwo := "EQBL8Ww6LLn6lYI6YJ6xntrENcGukdxbYlR5cYsEMFihZlyV"
		opponentAddress := "EQAFmK79N5h8nDgAbA1AZt0pOEtpJR-YWZLxE-nobVhtSGcM"
		now := time.Now()
		playerTwoChoice := entity.CoinSideTails
		payout := int64(1900000000)
		firstCompletedAt := now.Add(-3 * time.Hour)
		secondCompletedAt := now.Add(-90 * time.Minute)

		for _, user := range []*entity.User{
			{
				TelegramUserID:        Int64Ptr(123456789),
				TelegramUsername:      "referrer",
				WalletAddress:         referrerAddress,
				TotalReferrals:        999,
				TotalReferralEarnings: 500000000,
				CreatedAt:             now,
				UpdatedAt:             now,
			},
			{
				TelegramUserID:   Int64Ptr(987654321),
				TelegramUsername: "referredone",
				WalletAddress:    referredOne,
				CreatedAt:        now,
				UpdatedAt:        now,
			},
			{
				TelegramUserID:   Int64Ptr(111222333),
				TelegramUsername: "referredtwo",
				WalletAddress:    referredTwo,
				CreatedAt:        now,
				UpdatedAt:        now,
			},
			{
				TelegramUserID:   Int64Ptr(444555666),
				TelegramUsername: "opponent",
				WalletAddress:    opponentAddress,
				CreatedAt:        now,
				UpdatedAt:        now,
			},
		} {
			seedUser(t, user)
		}

		for _, game := range []*entity.Game{
			{
				GameID:                1,
				Status:                entity.GameStatusPaid,
				PlayerOneAddress:      referredOne,
				PlayerTwoAddress:      &opponentAddress,
				PlayerOneChoice:       entity.CoinSideHeads,
				PlayerTwoChoice:       &playerTwoChoice,
				PlayerOneReferrer:     &referrerAddress,
				BetAmount:             1000000000,
				PayoutAmount:          &payout,
				ServiceFeeNumerator:   100,
				ReferrerFeeNumerator:  50,
				WaitingTimeoutSeconds: 3600,
				LowestBidAllowed:      100000000,
				HighestBidAllowed:     10000000000,
				FeeReceiverAddress:    opponentAddress,
				CreatedAt:             now.Add(-4 * time.Hour),
				CompletedAt:           &firstCompletedAt,
				InitTxHash:            "referral-derived-1",
			},
			{
				GameID:                2,
				Status:                entity.GameStatusPaid,
				PlayerOneAddress:      referredOne,
				PlayerTwoAddress:      &referredTwo,
				PlayerOneChoice:       entity.CoinSideHeads,
				PlayerTwoChoice:       &playerTwoChoice,
				PlayerOneReferrer:     &referrerAddress,
				PlayerTwoReferrer:     &referrerAddress,
				BetAmount:             1000000000,
				PayoutAmount:          &payout,
				ServiceFeeNumerator:   100,
				ReferrerFeeNumerator:  50,
				WaitingTimeoutSeconds: 3600,
				LowestBidAllowed:      100000000,
				HighestBidAllowed:     10000000000,
				FeeReceiverAddress:    opponentAddress,
				CreatedAt:             now.Add(-2 * time.Hour),
				CompletedAt:           &secondCompletedAt,
				InitTxHash:            "referral-derived-2",
			},
		} {
			seedGame(t, game)
		}

		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/users/%s/referrals", referrerAddress), nil)
		resp, err := app.Test(req)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var stats entity.ReferralStats
		err = json.Unmarshal(body, &stats)
		require.NoError(t, err)

		assert.Equal(t, int64(2), stats.TotalReferrals)
		assert.Equal(t, int64(2), stats.GamesReferred)
		assert.Equal(t, int64(500000000), stats.TotalReferralEarnings)
	})

	t.Run("should return zero stats for user with no referrals", func(t *testing.T) {
		// Arrange
		app := setupTestApp(t)
		defer cleanupTestDB(t)

		walletAddress := "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2"

		// Seed user with no referrals
		testUser := &entity.User{
			TelegramUserID:        Int64Ptr(987654321),
			TelegramUsername:      "newuser",
			WalletAddress:         walletAddress,
			TotalReferrals:        0,
			TotalReferralEarnings: 0,
			CreatedAt:             time.Now(),
			UpdatedAt:             time.Now(),
		}
		seedUser(t, testUser)

		// Act
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/users/%s/referrals", walletAddress), nil)
		resp, err := app.Test(req)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var stats entity.ReferralStats
		err = json.Unmarshal(body, &stats)
		require.NoError(t, err)

		assert.Equal(t, int64(0), stats.TotalReferrals)
		assert.Equal(t, int64(0), stats.TotalReferralEarnings)
	})
}
