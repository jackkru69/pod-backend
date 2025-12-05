package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pod-backend/internal/entity"
	"pod-backend/internal/usecase"
)

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
		{"Cancelled", 123, "cancelled"},
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
	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func(gameID int64) {
			reservation := &entity.GameReservation{
				GameID:        gameID,
				WalletAddress: "EQD4FPq-PRDieyQKkizFTRtSDyucUIqrj0v_zXJmqaDp6_0t",
				CreatedAt:     time.Now(),
				ExpiresAt:     time.Now().Add(60 * time.Second),
				Status:        entity.ReservationStatusActive,
			}
			broadcastUC.BroadcastReservationCreated(ctx, reservation)
			broadcastUC.BroadcastReservationReleased(ctx, gameID, "cancelled")
			done <- true
		}(int64(i))
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		select {
		case <-done:
		// OK
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
