package integration_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pod-backend/internal/entity"
)

// TestGETGames tests the GET /api/v1/games endpoint
func TestGETGames(t *testing.T) {
	t.Run("should return games list with default status filter", func(t *testing.T) {
		// Arrange
		app := setupTestApp(t)
		defer cleanupTestDB(t)

		// Seed test data
		seedGame(t, &entity.Game{
			GameID:           1,
			Status:           entity.GameStatusWaitingForOpponent,
			PlayerOneAddress: "EQAbcdefghijklmnopqrstuvwxyz0123456789ABCDEFGH",
			PlayerOneChoice:  entity.CoinSideHeads,
			BetAmount:        1000000000,
			ServiceFeeNumerator: 100,
			ReferrerFeeNumerator: 50,
			WaitingTimeoutSeconds: 3600,
			LowestBidAllowed: 100000000,
			HighestBidAllowed: 10000000000,
			FeeReceiverAddress: "EQXyzabcdefghijklmnopqrstuvwxyz0123456789ABCDE",
			InitTxHash:       "abc123def456",
			CreatedAt:        time.Now(),
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

		// Seed multiple games
		for i := 1; i <= 5; i++ {
			seedGame(t, &entity.Game{
				GameID:           int64(i),
				Status:           entity.GameStatusWaitingForOpponent,
				PlayerOneAddress: "EQAbcdefghijklmnopqrstuvwxyz0123456789ABCDEFGH",
				PlayerOneChoice:  entity.CoinSideHeads,
				BetAmount:        1000000000,
				ServiceFeeNumerator: 100,
				ReferrerFeeNumerator: 50,
				WaitingTimeoutSeconds: 3600,
				LowestBidAllowed: 100000000,
				HighestBidAllowed: 10000000000,
				FeeReceiverAddress: "EQXyzabcdefghijklmnopqrstuvwxyz0123456789ABCDE",
				InitTxHash:       "abc" + string(rune(i)),
				CreatedAt:        time.Now(),
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

		// Seed test data
		seedGame(t, &entity.Game{
			GameID:           123,
			Status:           entity.GameStatusWaitingForOpponent,
			PlayerOneAddress: "EQAbcdefghijklmnopqrstuvwxyz0123456789ABCDEFGH",
			PlayerOneChoice:  entity.CoinSideHeads,
			BetAmount:        1000000000,
			ServiceFeeNumerator: 100,
			ReferrerFeeNumerator: 50,
			WaitingTimeoutSeconds: 3600,
			LowestBidAllowed: 100000000,
			HighestBidAllowed: 10000000000,
			FeeReceiverAddress: "EQXyzabcdefghijklmnopqrstuvwxyz0123456789ABCDE",
			InitTxHash:       "abc123def456",
			CreatedAt:        time.Now(),
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
		assert.Equal(t, "EQAbcdefghijklmnopqrstuvwxyz0123456789ABCDEFGH", game.PlayerOneAddress)
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

		// Verify OpenAPI spec structure
		assert.Contains(t, swaggerSpec, "openapi")
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

// Helper functions (to be implemented)

func setupTestApp(t *testing.T) *fiber.App {
	// TODO: Implement once handlers are created
	// This will:
	// 1. Create a Fiber app
	// 2. Setup test database connection
	// 3. Wire up all routes
	// 4. Return configured app
	app := fiber.New()
	return app
}

func cleanupTestDB(t *testing.T) {
	// TODO: Implement once database is setup
	// This will:
	// 1. Truncate all test tables
	// 2. Close database connections
}

func seedGame(t *testing.T, game *entity.Game) {
	// TODO: Implement once repository is created
	// This will:
	// 1. Insert game into test database
	// 2. Handle any related entities (users, etc.)
	ctx := context.Background()
	_ = ctx
	_ = game
}
