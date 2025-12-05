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

// Test constants
const (
	testWallet1 = "EQD4FPq-PRDieyQKkizFTRtSDyucUIqrj0v_zXJmqaDp6_0t"
	testWallet2 = "EQD4FPq-PRDieyQKkizFTRtSDyucUIqrj0v_zXJmqaDp6_0u"
	testWallet3 = "EQD4FPq-PRDieyQKkizFTRtSDyucUIqrj0v_zXJmqaDp6_0v"
)

// TestReserve_Success tests successful game reservation (T008)
func TestReserve_Success(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockRepo := new(MockGameRepository)
	broadcastUC := usecase.NewGameBroadcastUseCase()

	cfg := usecase.ReservationConfig{
		MaxPerWallet:           3,
		TimeoutSeconds:         60,
		CleanupIntervalSeconds: 5,
	}

	uc := usecase.NewReservationUseCase(mockRepo, broadcastUC, cfg)

	game := &entity.Game{
		GameID:           123,
		Status:           entity.GameStatusWaitingForOpponent,
		PlayerOneAddress: testWallet1,
		BetAmount:        1000000000, // 1 TON
	}

	mockRepo.On("GetByID", ctx, int64(123)).Return(game, nil)

	// Act
	reservation, err := uc.Reserve(ctx, 123, testWallet2)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, reservation)
	assert.Equal(t, int64(123), reservation.GameID)
	assert.Equal(t, testWallet2, reservation.WalletAddress)
	assert.Equal(t, entity.ReservationStatusActive, reservation.Status)
	assert.True(t, reservation.IsActive())
	assert.WithinDuration(t, time.Now().Add(60*time.Second), reservation.ExpiresAt, 2*time.Second)

	mockRepo.AssertExpectations(t)
}

// TestReserve_GameAlreadyReserved tests reservation when game is already reserved (T009)
func TestReserve_GameAlreadyReserved(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockRepo := new(MockGameRepository)
	broadcastUC := usecase.NewGameBroadcastUseCase()

	cfg := usecase.ReservationConfig{
		MaxPerWallet:           3,
		TimeoutSeconds:         60,
		CleanupIntervalSeconds: 5,
	}

	uc := usecase.NewReservationUseCase(mockRepo, broadcastUC, cfg)

	game := &entity.Game{
		GameID:           123,
		Status:           entity.GameStatusWaitingForOpponent,
		PlayerOneAddress: testWallet1,
		BetAmount:        1000000000,
	}

	mockRepo.On("GetByID", ctx, int64(123)).Return(game, nil)

	// First reservation succeeds
	_, err := uc.Reserve(ctx, 123, testWallet2)
	require.NoError(t, err)

	// Act - Second reservation should fail
	_, err = uc.Reserve(ctx, 123, testWallet3)

	// Assert
	assert.ErrorIs(t, err, entity.ErrGameAlreadyReserved)
}

// TestReserve_WalletAtLimit tests reservation when wallet has max reservations (T010)
func TestReserve_WalletAtLimit(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockRepo := new(MockGameRepository)
	broadcastUC := usecase.NewGameBroadcastUseCase()

	cfg := usecase.ReservationConfig{
		MaxPerWallet:           3,
		TimeoutSeconds:         60,
		CleanupIntervalSeconds: 5,
	}

	uc := usecase.NewReservationUseCase(mockRepo, broadcastUC, cfg)

	// Create 4 games (3 for limit + 1 extra)
	for i := int64(1); i <= 4; i++ {
		game := &entity.Game{
			GameID:           i,
			Status:           entity.GameStatusWaitingForOpponent,
			PlayerOneAddress: testWallet1,
			BetAmount:        1000000000,
		}
		mockRepo.On("GetByID", ctx, i).Return(game, nil)
	}

	// Reserve 3 games (at limit)
	for i := int64(1); i <= 3; i++ {
		_, err := uc.Reserve(ctx, i, testWallet2)
		require.NoError(t, err, "Reservation %d should succeed", i)
	}

	// Act - 4th reservation should fail
	_, err := uc.Reserve(ctx, 4, testWallet2)

	// Assert
	assert.ErrorIs(t, err, entity.ErrTooManyReservations)
}

// TestReserve_CannotReserveOwnGame tests reservation when wallet owns the game (T011)
func TestReserve_CannotReserveOwnGame(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockRepo := new(MockGameRepository)
	broadcastUC := usecase.NewGameBroadcastUseCase()

	cfg := usecase.ReservationConfig{
		MaxPerWallet:           3,
		TimeoutSeconds:         60,
		CleanupIntervalSeconds: 5,
	}

	uc := usecase.NewReservationUseCase(mockRepo, broadcastUC, cfg)

	game := &entity.Game{
		GameID:           123,
		Status:           entity.GameStatusWaitingForOpponent,
		PlayerOneAddress: testWallet1, // Owner
		BetAmount:        1000000000,
	}

	mockRepo.On("GetByID", ctx, int64(123)).Return(game, nil)

	// Act - Try to reserve own game
	_, err := uc.Reserve(ctx, 123, testWallet1)

	// Assert
	assert.ErrorIs(t, err, entity.ErrCannotReserveOwnGame)
}

// TestReserve_GameNotWaitingForOpponent tests reservation when game has wrong status (T012)
func TestReserve_GameNotWaitingForOpponent(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockRepo := new(MockGameRepository)
	broadcastUC := usecase.NewGameBroadcastUseCase()

	cfg := usecase.ReservationConfig{
		MaxPerWallet:           3,
		TimeoutSeconds:         60,
		CleanupIntervalSeconds: 5,
	}

	uc := usecase.NewReservationUseCase(mockRepo, broadcastUC, cfg)

	testCases := []struct {
		name   string
		status int
	}{
		{"Uninitialized", entity.GameStatusUninitialized},
		{"WaitingForOpenBids", entity.GameStatusWaitingForOpenBids},
		{"Ended", entity.GameStatusEnded},
		{"Paid", entity.GameStatusPaid},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			game := &entity.Game{
				GameID:           123,
				Status:           tc.status,
				PlayerOneAddress: testWallet1,
				BetAmount:        1000000000,
			}

			mockRepo.On("GetByID", ctx, int64(123)).Return(game, nil).Once()

			// Act
			_, err := uc.Reserve(ctx, 123, testWallet2)

			// Assert
			assert.ErrorIs(t, err, entity.ErrGameNotAvailable)
		})
	}
}

// TestReserve_GameNotFound tests reservation when game doesn't exist
func TestReserve_GameNotFound(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockRepo := new(MockGameRepository)
	broadcastUC := usecase.NewGameBroadcastUseCase()

	cfg := usecase.ReservationConfig{
		MaxPerWallet:           3,
		TimeoutSeconds:         60,
		CleanupIntervalSeconds: 5,
	}

	uc := usecase.NewReservationUseCase(mockRepo, broadcastUC, cfg)

	mockRepo.On("GetByID", ctx, int64(123)).Return(nil, nil)

	// Act
	_, err := uc.Reserve(ctx, 123, testWallet2)

	// Assert
	assert.ErrorIs(t, err, entity.ErrGameNotAvailable)
}

// TestGetReservation_Success tests getting an active reservation
func TestGetReservation_Success(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockRepo := new(MockGameRepository)
	broadcastUC := usecase.NewGameBroadcastUseCase()

	cfg := usecase.ReservationConfig{
		MaxPerWallet:           3,
		TimeoutSeconds:         60,
		CleanupIntervalSeconds: 5,
	}

	uc := usecase.NewReservationUseCase(mockRepo, broadcastUC, cfg)

	game := &entity.Game{
		GameID:           123,
		Status:           entity.GameStatusWaitingForOpponent,
		PlayerOneAddress: testWallet1,
		BetAmount:        1000000000,
	}

	mockRepo.On("GetByID", ctx, int64(123)).Return(game, nil)

	// Create reservation
	_, err := uc.Reserve(ctx, 123, testWallet2)
	require.NoError(t, err)

	// Act
	reservation, err := uc.GetReservation(ctx, 123)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, reservation)
	assert.Equal(t, int64(123), reservation.GameID)
	assert.Equal(t, testWallet2, reservation.WalletAddress)
}

// TestGetReservation_NotFound tests getting a non-existent reservation
func TestGetReservation_NotFound(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockRepo := new(MockGameRepository)
	broadcastUC := usecase.NewGameBroadcastUseCase()

	cfg := usecase.ReservationConfig{
		MaxPerWallet:           3,
		TimeoutSeconds:         60,
		CleanupIntervalSeconds: 5,
	}

	uc := usecase.NewReservationUseCase(mockRepo, broadcastUC, cfg)

	// Act
	reservation, err := uc.GetReservation(ctx, 123)

	// Assert
	require.NoError(t, err)
	assert.Nil(t, reservation)
}

// TestListByWallet_Success tests listing reservations for a wallet
func TestListByWallet_Success(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockRepo := new(MockGameRepository)
	broadcastUC := usecase.NewGameBroadcastUseCase()

	cfg := usecase.ReservationConfig{
		MaxPerWallet:           3,
		TimeoutSeconds:         60,
		CleanupIntervalSeconds: 5,
	}

	uc := usecase.NewReservationUseCase(mockRepo, broadcastUC, cfg)

	// Create 2 games
	for i := int64(1); i <= 2; i++ {
		game := &entity.Game{
			GameID:           i,
			Status:           entity.GameStatusWaitingForOpponent,
			PlayerOneAddress: testWallet1,
			BetAmount:        1000000000,
		}
		mockRepo.On("GetByID", ctx, i).Return(game, nil)
	}

	// Reserve 2 games
	_, err := uc.Reserve(ctx, 1, testWallet2)
	require.NoError(t, err)
	_, err = uc.Reserve(ctx, 2, testWallet2)
	require.NoError(t, err)

	// Act
	reservations, err := uc.ListByWallet(ctx, testWallet2)

	// Assert
	require.NoError(t, err)
	assert.Len(t, reservations, 2)
}

// TestListByWallet_Empty tests listing when wallet has no reservations
func TestListByWallet_Empty(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockRepo := new(MockGameRepository)
	broadcastUC := usecase.NewGameBroadcastUseCase()

	cfg := usecase.ReservationConfig{
		MaxPerWallet:           3,
		TimeoutSeconds:         60,
		CleanupIntervalSeconds: 5,
	}

	uc := usecase.NewReservationUseCase(mockRepo, broadcastUC, cfg)

	// Act
	reservations, err := uc.ListByWallet(ctx, testWallet2)

	// Assert
	require.NoError(t, err)
	assert.Empty(t, reservations)
}

// ==============================================================================
// User Story 2 Tests: Automatic Reservation Release
// ==============================================================================

// TestCancel_Success tests successful cancellation of own reservation (T026)
func TestCancel_Success(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockRepo := new(MockGameRepository)
	broadcastUC := usecase.NewGameBroadcastUseCase()

	cfg := usecase.ReservationConfig{
		MaxPerWallet:           3,
		TimeoutSeconds:         60,
		CleanupIntervalSeconds: 5,
	}

	uc := usecase.NewReservationUseCase(mockRepo, broadcastUC, cfg)

	game := &entity.Game{
		GameID:           123,
		Status:           entity.GameStatusWaitingForOpponent,
		PlayerOneAddress: testWallet1,
		BetAmount:        1000000000,
	}

	mockRepo.On("GetByID", ctx, int64(123)).Return(game, nil)

	// Create reservation first
	_, err := uc.Reserve(ctx, 123, testWallet2)
	require.NoError(t, err)

	// Verify reservation exists
	reservation, _ := uc.GetReservation(ctx, 123)
	require.NotNil(t, reservation)

	// Act - Cancel reservation
	err = uc.Cancel(ctx, 123, testWallet2)

	// Assert
	require.NoError(t, err)

	// Verify reservation no longer exists
	reservation, _ = uc.GetReservation(ctx, 123)
	assert.Nil(t, reservation)

	// Verify wallet reservations count decreased
	reservations, _ := uc.ListByWallet(ctx, testWallet2)
	assert.Empty(t, reservations)
}

// TestCancel_NotHolder tests cancellation by non-holder (T027)
func TestCancel_NotHolder(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockRepo := new(MockGameRepository)
	broadcastUC := usecase.NewGameBroadcastUseCase()

	cfg := usecase.ReservationConfig{
		MaxPerWallet:           3,
		TimeoutSeconds:         60,
		CleanupIntervalSeconds: 5,
	}

	uc := usecase.NewReservationUseCase(mockRepo, broadcastUC, cfg)

	game := &entity.Game{
		GameID:           123,
		Status:           entity.GameStatusWaitingForOpponent,
		PlayerOneAddress: testWallet1,
		BetAmount:        1000000000,
	}

	mockRepo.On("GetByID", ctx, int64(123)).Return(game, nil)

	// Create reservation by wallet2
	_, err := uc.Reserve(ctx, 123, testWallet2)
	require.NoError(t, err)

	// Act - Try to cancel by wallet3 (not the holder)
	err = uc.Cancel(ctx, 123, testWallet3)

	// Assert
	assert.ErrorIs(t, err, entity.ErrNotReservationHolder)

	// Verify reservation still exists
	reservation, _ := uc.GetReservation(ctx, 123)
	assert.NotNil(t, reservation)
}

// TestCancel_NotFound tests cancellation when no reservation exists
func TestCancel_NotFound(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockRepo := new(MockGameRepository)
	broadcastUC := usecase.NewGameBroadcastUseCase()

	cfg := usecase.ReservationConfig{
		MaxPerWallet:           3,
		TimeoutSeconds:         60,
		CleanupIntervalSeconds: 5,
	}

	uc := usecase.NewReservationUseCase(mockRepo, broadcastUC, cfg)

	// Act - Try to cancel non-existent reservation
	err := uc.Cancel(ctx, 123, testWallet2)

	// Assert
	assert.ErrorIs(t, err, entity.ErrReservationNotFound)
}

// TestCleanupExpired tests automatic cleanup of expired reservations (T028)
func TestCleanupExpired(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockRepo := new(MockGameRepository)
	broadcastUC := usecase.NewGameBroadcastUseCase()

	// Very short timeout for testing
	cfg := usecase.ReservationConfig{
		MaxPerWallet:           3,
		TimeoutSeconds:         1, // 1 second timeout
		CleanupIntervalSeconds: 1,
	}

	uc := usecase.NewReservationUseCase(mockRepo, broadcastUC, cfg)

	game := &entity.Game{
		GameID:           123,
		Status:           entity.GameStatusWaitingForOpponent,
		PlayerOneAddress: testWallet1,
		BetAmount:        1000000000,
	}

	mockRepo.On("GetByID", ctx, int64(123)).Return(game, nil)

	// Create reservation
	_, err := uc.Reserve(ctx, 123, testWallet2)
	require.NoError(t, err)

	// Verify reservation exists
	reservation, _ := uc.GetReservation(ctx, 123)
	require.NotNil(t, reservation)

	// Wait for expiration
	time.Sleep(1100 * time.Millisecond)

	// Act - Run cleanup
	uc.CleanupExpired(ctx)

	// Assert - Reservation should be gone
	reservation, _ = uc.GetReservation(ctx, 123)
	assert.Nil(t, reservation)

	// Verify wallet reservations also cleaned up
	reservations, _ := uc.ListByWallet(ctx, testWallet2)
	assert.Empty(t, reservations)
}

// TestReleaseOnJoin tests releasing reservation when game is joined (T029)
func TestReleaseOnJoin(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockRepo := new(MockGameRepository)
	broadcastUC := usecase.NewGameBroadcastUseCase()

	cfg := usecase.ReservationConfig{
		MaxPerWallet:           3,
		TimeoutSeconds:         60,
		CleanupIntervalSeconds: 5,
	}

	uc := usecase.NewReservationUseCase(mockRepo, broadcastUC, cfg)

	game := &entity.Game{
		GameID:           123,
		Status:           entity.GameStatusWaitingForOpponent,
		PlayerOneAddress: testWallet1,
		BetAmount:        1000000000,
	}

	mockRepo.On("GetByID", ctx, int64(123)).Return(game, nil)

	// Create reservation
	_, err := uc.Reserve(ctx, 123, testWallet2)
	require.NoError(t, err)

	// Verify reservation exists
	reservation, _ := uc.GetReservation(ctx, 123)
	require.NotNil(t, reservation)

	// Act - Release on join (simulating game_started event)
	uc.ReleaseOnJoin(ctx, 123)

	// Assert - Reservation should be released
	reservation, _ = uc.GetReservation(ctx, 123)
	assert.Nil(t, reservation)

	// Verify wallet reservations also released
	reservations, _ := uc.ListByWallet(ctx, testWallet2)
	assert.Empty(t, reservations)
}

// TestReleaseOnJoin_NoReservation tests ReleaseOnJoin when no reservation exists
func TestReleaseOnJoin_NoReservation(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockRepo := new(MockGameRepository)
	broadcastUC := usecase.NewGameBroadcastUseCase()

	cfg := usecase.ReservationConfig{
		MaxPerWallet:           3,
		TimeoutSeconds:         60,
		CleanupIntervalSeconds: 5,
	}

	uc := usecase.NewReservationUseCase(mockRepo, broadcastUC, cfg)

	// Act - Release on join when no reservation exists (should not panic)
	uc.ReleaseOnJoin(ctx, 123)

	// Assert - No error, no panic
}

// TestReserve_AfterCancel tests re-reserving after cancellation
func TestReserve_AfterCancel(t *testing.T) {
	// Arrange
	ctx := context.Background()
	mockRepo := new(MockGameRepository)
	broadcastUC := usecase.NewGameBroadcastUseCase()

	cfg := usecase.ReservationConfig{
		MaxPerWallet:           3,
		TimeoutSeconds:         60,
		CleanupIntervalSeconds: 5,
	}

	uc := usecase.NewReservationUseCase(mockRepo, broadcastUC, cfg)

	game := &entity.Game{
		GameID:           123,
		Status:           entity.GameStatusWaitingForOpponent,
		PlayerOneAddress: testWallet1,
		BetAmount:        1000000000,
	}

	mockRepo.On("GetByID", ctx, int64(123)).Return(game, nil)

	// Create reservation by wallet2
	_, err := uc.Reserve(ctx, 123, testWallet2)
	require.NoError(t, err)

	// Cancel reservation
	err = uc.Cancel(ctx, 123, testWallet2)
	require.NoError(t, err)

	// Act - wallet3 should now be able to reserve
	reservation, err := uc.Reserve(ctx, 123, testWallet3)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, reservation)
	assert.Equal(t, testWallet3, reservation.WalletAddress)
}
