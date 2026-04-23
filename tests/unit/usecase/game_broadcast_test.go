package usecase_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pod-backend/internal/entity"
	"pod-backend/internal/usecase"
)

type mockBroadcastConn struct {
	messages      [][]byte
	writeDeadline time.Time
}

func (m *mockBroadcastConn) WriteMessage(_ int, data []byte) error {
	msgCopy := make([]byte, len(data))
	copy(msgCopy, data)
	m.messages = append(m.messages, msgCopy)
	return nil
}

func (m *mockBroadcastConn) Close() error {
	return nil
}

func (m *mockBroadcastConn) SetWriteDeadline(t time.Time) error {
	m.writeDeadline = t
	return nil
}

// TestBroadcastReservationCreated tests broadcasting reservation created events (T040)
func TestBroadcastReservationCreated(t *testing.T) {
	ctx := context.Background()
	broadcastUC := usecase.NewGameBroadcastUseCase()

	// Create a test reservation
	reservation := &entity.GameReservation{
		GameID:        123,
		WalletAddress: "EQD4FPq-PRDieyQKkizFTRtSDyucUIqrj0v_zXJmqaDp6_0t",
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(60 * time.Second),
		Status:        entity.ReservationStatusActive,
	}

	// Call BroadcastReservationCreated (should not panic even without subscribers)
	err := broadcastUC.BroadcastReservationCreated(ctx, reservation)

	// Should not return error (logs debug message about no subscribers)
	require.NoError(t, err)
}

// TestBroadcastReservationReleased tests broadcasting reservation released events (T041)
func TestBroadcastReservationReleased(t *testing.T) {
	ctx := context.Background()
	broadcastUC := usecase.NewGameBroadcastUseCase()

	testCases := []struct {
		name   string
		gameID int64
		reason string
	}{
		{"Canceled", 123, "cancelled"},
		{"Expired", 456, "expired"},
		{"Joined", 789, "joined"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Call BroadcastReservationReleased (should not panic even without subscribers)
			err := broadcastUC.BroadcastReservationReleased(ctx, tc.gameID, tc.reason)

			// Should not return error
			require.NoError(t, err)
		})
	}
}

// TestBroadcastReservationReleased_EmptyReason tests broadcasting with empty reason
func TestBroadcastReservationReleased_EmptyReason(t *testing.T) {
	ctx := context.Background()
	broadcastUC := usecase.NewGameBroadcastUseCase()

	// Call with empty reason
	err := broadcastUC.BroadcastReservationReleased(ctx, 123, "")

	// Should handle gracefully
	require.NoError(t, err)
}

// TestBroadcastUseCase_ConcurrentAccess tests thread safety of broadcast operations
func TestBroadcastUseCase_ConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	broadcastUC := usecase.NewGameBroadcastUseCase()

	// Run multiple broadcasts concurrently
	done := make(chan error)

	for i := 0; i < 10; i++ {
		go func(gameID int64) {
			reservation := &entity.GameReservation{
				GameID:        gameID,
				WalletAddress: "EQD4FPq-PRDieyQKkizFTRtSDyucUIqrj0v_zXJmqaDp6_0t",
				CreatedAt:     time.Now(),
				ExpiresAt:     time.Now().Add(60 * time.Second),
				Status:        entity.ReservationStatusActive,
			}
			if err := broadcastUC.BroadcastReservationCreated(ctx, reservation); err != nil {
				done <- err
				return
			}
			if err := broadcastUC.BroadcastReservationReleased(ctx, gameID, "cancelled"); err != nil {
				done <- err
				return
			}
			done <- nil
		}(int64(i))
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for concurrent broadcasts")
		}
	}
}

// TestReservationCreatedEvent_Format tests the event struct format
func TestReservationCreatedEvent_Format(t *testing.T) {
	reservation := &entity.GameReservation{
		GameID:        123,
		WalletAddress: "test_wallet",
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(60 * time.Second),
		Status:        entity.ReservationStatusActive,
	}

	assert.Equal(t, int64(123), reservation.GameID)
	assert.Equal(t, "test_wallet", reservation.WalletAddress)
	assert.Equal(t, entity.ReservationStatusActive, reservation.Status)
	assert.True(t, reservation.IsActive())
}

func TestBroadcastGameUpdateWithEvent_IncludesTimestamp(t *testing.T) {
	ctx := context.Background()
	broadcastUC := usecase.NewGameBroadcastUseCase()
	conn := &mockBroadcastConn{}

	broadcastUC.Subscribe(ctx, 123, "game-client", conn)

	game := &entity.Game{
		GameID:           123,
		Status:           entity.GameStatusPaid,
		PlayerOneAddress: "EQD4FPq-PRDieyQKkizFTRtSDyucUIqrj0v_zXJmqaDp6_0t",
		PlayerOneChoice:  entity.CoinSideHeads,
		BetAmount:        1000000000,
		InitTxHash:       "init-hash",
		CreatedAt:        time.Now(),
	}

	err := broadcastUC.BroadcastGameUpdateWithEvent(ctx, game, usecase.GameEventTypeFinished)
	require.NoError(t, err)
	require.Len(t, conn.messages, 1)

	var payload struct {
		Type      string      `json:"type"`
		Timestamp string      `json:"timestamp"`
		EventType string      `json:"event_type"`
		Data      entity.Game `json:"data"`
	}
	require.NoError(t, json.Unmarshal(conn.messages[0], &payload))

	assert.Equal(t, usecase.MessageTypeGameStateUpdate, payload.Type)
	assert.Equal(t, string(usecase.GameEventTypeFinished), payload.EventType)
	assert.Equal(t, game.GameID, payload.Data.GameID)
	_, err = time.Parse(time.RFC3339Nano, payload.Timestamp)
	assert.NoError(t, err)
}

func TestBroadcastReservationEvents_IncludeTimestamp(t *testing.T) {
	ctx := context.Background()
	broadcastUC := usecase.NewGameBroadcastUseCase()
	conn := &mockBroadcastConn{}

	broadcastUC.Subscribe(ctx, 123, "game-client", conn)

	reservation := &entity.GameReservation{
		GameID:        123,
		WalletAddress: "EQD4FPq-PRDieyQKkizFTRtSDyucUIqrj0v_zXJmqaDp6_0t",
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(60 * time.Second),
		Status:        entity.ReservationStatusActive,
	}

	require.NoError(t, broadcastUC.BroadcastReservationCreated(ctx, reservation))
	require.NoError(t, broadcastUC.BroadcastReservationReleased(ctx, reservation.GameID, "joined"))
	require.Len(t, conn.messages, 2)

	for _, rawMessage := range conn.messages {
		var payload struct {
			Type      string `json:"type"`
			Timestamp string `json:"timestamp"`
		}
		require.NoError(t, json.Unmarshal(rawMessage, &payload))
		assert.NotEmpty(t, payload.Type)
		_, err := time.Parse(time.RFC3339Nano, payload.Timestamp)
		assert.NoError(t, err)
	}
}

func TestBroadcastExpiredClaimEvents_IncludeTimestamp(t *testing.T) {
	ctx := context.Background()
	broadcastUC := usecase.NewGameBroadcastUseCase()
	conn := &mockBroadcastConn{}

	broadcastUC.Subscribe(ctx, 321, "expired-client", conn)

	claim := &entity.ExpiredClaim{
		GameID:        321,
		WalletAddress: "EQD4FPq-PRDieyQKkizFTRtSDyucUIqrj0v_zXJmqaDp6_0t",
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(2 * time.Minute),
		Status:        entity.ExpiredClaimStatusActive,
	}

	require.NoError(t, broadcastUC.BroadcastExpiredClaimCreated(ctx, claim))
	require.NoError(t, broadcastUC.BroadcastExpiredClaimReleased(ctx, claim.GameID, "resolved"))
	require.Len(t, conn.messages, 2)

	for _, rawMessage := range conn.messages {
		var payload struct {
			Type      string `json:"type"`
			Timestamp string `json:"timestamp"`
		}
		require.NoError(t, json.Unmarshal(rawMessage, &payload))
		assert.NotEmpty(t, payload.Type)
		_, err := time.Parse(time.RFC3339Nano, payload.Timestamp)
		assert.NoError(t, err)
	}
}
