package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	gorillaws "github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	wscontroller "pod-backend/internal/controller/websocket"
	"pod-backend/internal/entity"
	postgresrepo "pod-backend/internal/repository/postgres"
	"pod-backend/internal/usecase"
)

func setupRecoveryWebSocketApp(t *testing.T) (*fiber.App, *usecase.GameBroadcastUseCase) {
	t.Helper()

	testDB = setupTestDatabase(t)

	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			var fiberErr *fiber.Error
			if errors.As(err, &fiberErr) {
				return c.Status(fiberErr.Code).JSON(fiber.Map{
					"error": fiberErr.Message,
				})
			}

			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": err.Error(),
			})
		},
	})

	gameRepo := postgresrepo.NewGameRepository(testDB.pg)
	broadcastUC := usecase.NewGameBroadcastUseCase()
	wsHandler := wscontroller.NewGameWebSocketHandler(gameRepo, broadcastUC)
	wsHandler.RegisterRoutes(app)

	return app, broadcastUC
}

func startFiberListener(t *testing.T, app *fiber.App) (string, func()) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	serveDone := make(chan error, 1)
	go func() {
		serveDone <- app.Listener(listener)
	}()

	shutdown := func() {
		_ = app.Shutdown()
		_ = listener.Close()
		select {
		case err := <-serveDone:
			if err != nil && !errors.Is(err, net.ErrClosed) {
				t.Logf("websocket test server stopped with error: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Log("websocket test server shutdown timed out")
		}
	}

	return listener.Addr().String(), shutdown
}

func dialWebSocketWithRetry(t *testing.T, url string) *gorillaws.Conn {
	t.Helper()

	var conn *gorillaws.Conn
	require.Eventually(t, func() bool {
		candidate, _, err := gorillaws.DefaultDialer.Dial(url, nil)
		if err != nil {
			return false
		}

		conn = candidate
		return true
	}, 3*time.Second, 50*time.Millisecond)

	require.NotNil(t, conn)
	return conn
}

func readWebSocketJSON(t *testing.T, conn *gorillaws.Conn, out interface{}) {
	t.Helper()

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, raw, err := conn.ReadMessage()
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, out))
}

func TestWebSocketRecoveryBroadcastsExpiredClaimLifecycle(t *testing.T) {
	app, broadcastUC := setupRecoveryWebSocketApp(t)
	defer cleanupTestDB(t)

	now := time.Now()
	otherWallet := "EQBvW8Z5huBkMJYdnfAEM5JqTNLuuU3FYxrVjxFBzXn3r95X"
	completedAt := now.Add(-2 * time.Minute)
	playerTwoChoice := entity.CoinSideTails

	for _, user := range []*entity.User{
		{
			TelegramUserID:   Int64Ptr(123456789),
			TelegramUsername: "player_one",
			WalletAddress:    "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2",
			CreatedAt:        now,
			UpdatedAt:        now,
		},
		{
			TelegramUserID:   Int64Ptr(987654321),
			TelegramUsername: "player_two",
			WalletAddress:    otherWallet,
			CreatedAt:        now,
			UpdatedAt:        now,
		},
	} {
		seedUser(t, user)
	}

	game := &entity.Game{
		GameID:                620,
		Status:                entity.GameStatusEnded,
		PlayerOneAddress:      "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2",
		PlayerTwoAddress:      &otherWallet,
		PlayerOneChoice:       entity.CoinSideHeads,
		PlayerTwoChoice:       &playerTwoChoice,
		BetAmount:             1000000000,
		ServiceFeeNumerator:   100,
		ReferrerFeeNumerator:  50,
		WaitingTimeoutSeconds: 3600,
		LowestBidAllowed:      100000000,
		HighestBidAllowed:     10000000000,
		FeeReceiverAddress:    otherWallet,
		InitTxHash:            "recovery-ws-620",
		CreatedAt:             now.Add(-20 * time.Minute),
		CompletedAt:           &completedAt,
	}
	seedGame(t, game)

	addr, shutdown := startFiberListener(t, app)
	defer shutdown()

	gameConn := dialWebSocketWithRetry(t, fmt.Sprintf("ws://%s/ws/games/%d", addr, game.GameID))
	defer gameConn.Close()

	globalConn := dialWebSocketWithRetry(t, fmt.Sprintf("ws://%s/ws/games", addr))
	defer globalConn.Close()

	require.Eventually(t, func() bool {
		return broadcastUC.GetGameSubscriberCount(game.GameID) == 1 &&
			broadcastUC.GetGameSubscriberCount(usecase.GlobalGameID) == 1
	}, 2*time.Second, 25*time.Millisecond)

	claim := &entity.ExpiredClaim{
		GameID:        game.GameID,
		WalletAddress: game.PlayerOneAddress,
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(2 * time.Minute),
		Status:        entity.ExpiredClaimStatusActive,
	}

	require.NoError(t, broadcastUC.BroadcastExpiredClaimCreated(context.Background(), claim))

	var gameCreated usecase.ExpiredClaimCreatedEvent
	readWebSocketJSON(t, gameConn, &gameCreated)
	assert.Equal(t, usecase.MessageTypeExpiredClaimCreated, gameCreated.Type)
	assert.Equal(t, game.GameID, gameCreated.GameID)
	assert.Equal(t, claim.WalletAddress, gameCreated.ReservedBy)
	assert.NotEmpty(t, gameCreated.ExpiresAt)
	_, err := time.Parse(time.RFC3339Nano, gameCreated.Timestamp)
	assert.NoError(t, err)

	var globalCreated usecase.ExpiredClaimCreatedEvent
	readWebSocketJSON(t, globalConn, &globalCreated)
	assert.Equal(t, usecase.MessageTypeExpiredClaimCreated, globalCreated.Type)
	assert.Equal(t, game.GameID, globalCreated.GameID)

	require.NoError(t, broadcastUC.BroadcastExpiredClaimReleased(context.Background(), game.GameID, "resolved"))

	var released usecase.ExpiredClaimReleasedEvent
	readWebSocketJSON(t, gameConn, &released)
	assert.Equal(t, usecase.MessageTypeExpiredClaimReleased, released.Type)
	assert.Equal(t, game.GameID, released.GameID)
	assert.Equal(t, "resolved", released.Reason)
	_, err = time.Parse(time.RFC3339Nano, released.Timestamp)
	assert.NoError(t, err)

	var globalReleased usecase.ExpiredClaimReleasedEvent
	readWebSocketJSON(t, globalConn, &globalReleased)
	assert.Equal(t, usecase.MessageTypeExpiredClaimReleased, globalReleased.Type)
	assert.Equal(t, game.GameID, globalReleased.GameID)
	assert.Equal(t, "resolved", globalReleased.Reason)
}
