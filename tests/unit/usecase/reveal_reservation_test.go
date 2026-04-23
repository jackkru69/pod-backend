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

func TestRevealReserve_Success(t *testing.T) {
	ctx := context.Background()
	mockRepo := new(MockGameRepository)
	broadcastUC := usecase.NewGameBroadcastUseCase()

	uc := usecase.NewRevealReservationUseCase(mockRepo, broadcastUC, usecase.RevealReservationConfig{
		MaxPerWallet:           5,
		TimeoutSeconds:         90,
		CleanupIntervalSeconds: 5,
	})

	playerTwo := testWallet2
	game := &entity.Game{
		GameID:           321,
		Status:           entity.GameStatusWaitingForOpenBids,
		PlayerOneAddress: testWallet1,
		PlayerTwoAddress: &playerTwo,
		BetAmount:        1000000000,
	}

	mockRepo.On("GetByID", ctx, int64(321)).Return(game, nil)

	reservation, err := uc.Reserve(ctx, 321, testWallet2)

	require.NoError(t, err)
	require.NotNil(t, reservation)
	assert.Equal(t, int64(321), reservation.GameID)
	assert.Equal(t, testWallet2, reservation.WalletAddress)
	assert.Equal(t, entity.RevealReservationStatusActive, reservation.Status)
	assert.True(t, reservation.IsActive())
	assert.WithinDuration(t, time.Now().Add(90*time.Second), reservation.ExpiresAt, 2*time.Second)
	mockRepo.AssertExpectations(t)
}

func TestRevealReserve_NotParticipant(t *testing.T) {
	ctx := context.Background()
	mockRepo := new(MockGameRepository)
	broadcastUC := usecase.NewGameBroadcastUseCase()

	uc := usecase.NewRevealReservationUseCase(mockRepo, broadcastUC, usecase.RevealReservationConfig{
		MaxPerWallet:           5,
		TimeoutSeconds:         90,
		CleanupIntervalSeconds: 5,
	})

	playerTwo := testWallet2
	game := &entity.Game{
		GameID:           322,
		Status:           entity.GameStatusWaitingForOpenBids,
		PlayerOneAddress: testWallet1,
		PlayerTwoAddress: &playerTwo,
		BetAmount:        1000000000,
	}

	mockRepo.On("GetByID", ctx, int64(322)).Return(game, nil)

	_, err := uc.Reserve(ctx, 322, testWallet3)

	assert.ErrorIs(t, err, entity.ErrNotAPlayer)
	mockRepo.AssertExpectations(t)
}

func TestRevealReservation_ReleaseOnTerminal(t *testing.T) {
	ctx := context.Background()
	mockRepo := new(MockGameRepository)
	broadcastUC := usecase.NewGameBroadcastUseCase()

	uc := usecase.NewRevealReservationUseCase(mockRepo, broadcastUC, usecase.RevealReservationConfig{
		MaxPerWallet:           5,
		TimeoutSeconds:         90,
		CleanupIntervalSeconds: 5,
	})

	playerTwo := testWallet2
	game := &entity.Game{
		GameID:           555,
		Status:           entity.GameStatusWaitingForOpenBids,
		PlayerOneAddress: testWallet1,
		PlayerTwoAddress: &playerTwo,
		BetAmount:        1000000000,
	}

	mockRepo.On("GetByID", ctx, int64(555)).Return(game, nil)

	_, err := uc.Reserve(ctx, 555, testWallet1)
	require.NoError(t, err)

	uc.ReleaseOnTerminal(ctx, 555)

	reservation, getErr := uc.Get(ctx, 555)
	require.NoError(t, getErr)
	assert.Nil(t, reservation)
	mockRepo.AssertExpectations(t)
}

func TestRevealReservation_GetAndListByWallet_EnableResume(t *testing.T) {
	ctx := context.Background()
	mockRepo := new(MockGameRepository)
	broadcastUC := usecase.NewGameBroadcastUseCase()

	uc := usecase.NewRevealReservationUseCase(mockRepo, broadcastUC, usecase.RevealReservationConfig{
		MaxPerWallet:           5,
		TimeoutSeconds:         90,
		CleanupIntervalSeconds: 5,
	})

	playerTwo := testWallet2
	game := &entity.Game{
		GameID:           556,
		Status:           entity.GameStatusWaitingForOpenBids,
		PlayerOneAddress: testWallet1,
		PlayerTwoAddress: &playerTwo,
		BetAmount:        1000000000,
	}

	mockRepo.On("GetByID", ctx, int64(556)).Return(game, nil)

	created, err := uc.Reserve(ctx, 556, testWallet2)
	require.NoError(t, err)
	require.NotNil(t, created)

	restored, err := uc.Get(ctx, 556)
	require.NoError(t, err)
	require.NotNil(t, restored)
	assert.Equal(t, testWallet2, restored.WalletAddress)

	reservations, err := uc.ListByWallet(ctx, testWallet2)
	require.NoError(t, err)
	require.Len(t, reservations, 1)
	assert.Equal(t, int64(556), reservations[0].GameID)
	assert.Equal(t, testWallet2, reservations[0].WalletAddress)

	mockRepo.AssertExpectations(t)
}

func TestRevealReservation_CancelHolderOnly(t *testing.T) {
	ctx := context.Background()
	mockRepo := new(MockGameRepository)
	broadcastUC := usecase.NewGameBroadcastUseCase()

	uc := usecase.NewRevealReservationUseCase(mockRepo, broadcastUC, usecase.RevealReservationConfig{
		MaxPerWallet:           5,
		TimeoutSeconds:         90,
		CleanupIntervalSeconds: 5,
	})

	playerTwo := testWallet2
	game := &entity.Game{
		GameID:           557,
		Status:           entity.GameStatusWaitingForOpenBids,
		PlayerOneAddress: testWallet1,
		PlayerTwoAddress: &playerTwo,
		BetAmount:        1000000000,
	}

	mockRepo.On("GetByID", ctx, int64(557)).Return(game, nil)

	_, err := uc.Reserve(ctx, 557, testWallet2)
	require.NoError(t, err)

	err = uc.Cancel(ctx, 557, testWallet3)
	assert.ErrorIs(t, err, entity.ErrNotRevealReservationHolder)

	reservation, getErr := uc.Get(ctx, 557)
	require.NoError(t, getErr)
	require.NotNil(t, reservation)

	err = uc.Cancel(ctx, 557, testWallet2)
	require.NoError(t, err)

	reservation, getErr = uc.Get(ctx, 557)
	require.NoError(t, getErr)
	assert.Nil(t, reservation)

	mockRepo.AssertExpectations(t)
}

func TestRevealReservation_ExpiresFromRecoverySurface(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockRepo := new(MockGameRepository)
	broadcastUC := usecase.NewGameBroadcastUseCase()

	uc := usecase.NewRevealReservationUseCase(mockRepo, broadcastUC, usecase.RevealReservationConfig{
		MaxPerWallet:           5,
		TimeoutSeconds:         1,
		CleanupIntervalSeconds: 1,
	})
	defer uc.StopCleanupLoop()

	playerTwo := testWallet2
	game := &entity.Game{
		GameID:           558,
		Status:           entity.GameStatusWaitingForOpenBids,
		PlayerOneAddress: testWallet1,
		PlayerTwoAddress: &playerTwo,
		BetAmount:        1000000000,
	}

	mockRepo.On("GetByID", ctx, int64(558)).Return(game, nil)

	_, err := uc.Reserve(ctx, 558, testWallet2)
	require.NoError(t, err)

	uc.StartCleanupLoop(ctx)
	time.Sleep(2200 * time.Millisecond)

	reservation, getErr := uc.Get(ctx, 558)
	require.NoError(t, getErr)
	assert.Nil(t, reservation)

	reservations, listErr := uc.ListByWallet(ctx, testWallet2)
	require.NoError(t, listErr)
	assert.Empty(t, reservations)

	mockRepo.AssertExpectations(t)
}
