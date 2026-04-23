package usecase_test

import (
	"context"
	"testing"
	"time"

	"pod-backend/internal/entity"
	"pod-backend/internal/usecase"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCancelReservationReserve_Success(t *testing.T) {
	ctx := context.Background()
	mockRepo := new(MockGameRepository)
	broadcastUC := usecase.NewGameBroadcastUseCase()

	joinReservationUC := usecase.NewReservationUseCase(mockRepo, broadcastUC, usecase.ReservationConfig{
		MaxPerWallet:           3,
		TimeoutSeconds:         60,
		CleanupIntervalSeconds: 5,
	})
	cancelUC := usecase.NewCancelReservationUseCase(mockRepo, joinReservationUC, broadcastUC, usecase.CancelReservationConfig{
		MaxPerWallet:           3,
		TimeoutSeconds:         60,
		CleanupIntervalSeconds: 5,
	})

	game := &entity.Game{
		GameID:           700,
		Status:           entity.GameStatusWaitingForOpponent,
		PlayerOneAddress: testWallet1,
		BetAmount:        1000000000,
	}

	mockRepo.On("GetByID", ctx, int64(700)).Return(game, nil).Once()

	reservation, err := cancelUC.Reserve(ctx, 700, testWallet1)

	require.NoError(t, err)
	require.NotNil(t, reservation)
	assert.Equal(t, int64(700), reservation.GameID)
	assert.Equal(t, testWallet1, reservation.WalletAddress)
	assert.Equal(t, entity.CancelReservationStatusActive, reservation.Status)
	assert.True(t, reservation.IsActive())
	assert.WithinDuration(t, time.Now().Add(60*time.Second), reservation.ExpiresAt, 2*time.Second)

	stored, err := cancelUC.Get(ctx, 700)
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, testWallet1, stored.WalletAddress)

	mockRepo.AssertExpectations(t)
}

func TestCancelReservationReserve_BlocksWhenJoinReservationExists(t *testing.T) {
	ctx := context.Background()
	mockRepo := new(MockGameRepository)
	broadcastUC := usecase.NewGameBroadcastUseCase()

	joinReservationUC := usecase.NewReservationUseCase(mockRepo, broadcastUC, usecase.ReservationConfig{
		MaxPerWallet:           3,
		TimeoutSeconds:         60,
		CleanupIntervalSeconds: 5,
	})
	cancelUC := usecase.NewCancelReservationUseCase(mockRepo, joinReservationUC, broadcastUC, usecase.CancelReservationConfig{
		MaxPerWallet:           3,
		TimeoutSeconds:         60,
		CleanupIntervalSeconds: 5,
	})

	game := &entity.Game{
		GameID:           701,
		Status:           entity.GameStatusWaitingForOpponent,
		PlayerOneAddress: testWallet1,
		BetAmount:        1000000000,
	}

	mockRepo.On("GetByID", ctx, int64(701)).Return(game, nil).Twice()

	_, err := joinReservationUC.Reserve(ctx, 701, testWallet2)
	require.NoError(t, err)

	_, err = cancelUC.Reserve(ctx, 701, testWallet1)

	assert.ErrorIs(t, err, entity.ErrGameAlreadyReserved)
	mockRepo.AssertExpectations(t)
}

func TestCancelReservationReserve_RejectsNonCreator(t *testing.T) {
	ctx := context.Background()
	mockRepo := new(MockGameRepository)
	broadcastUC := usecase.NewGameBroadcastUseCase()

	joinReservationUC := usecase.NewReservationUseCase(mockRepo, broadcastUC, usecase.ReservationConfig{
		MaxPerWallet:           3,
		TimeoutSeconds:         60,
		CleanupIntervalSeconds: 5,
	})
	cancelUC := usecase.NewCancelReservationUseCase(mockRepo, joinReservationUC, broadcastUC, usecase.CancelReservationConfig{
		MaxPerWallet:           3,
		TimeoutSeconds:         60,
		CleanupIntervalSeconds: 5,
	})

	game := &entity.Game{
		GameID:           702,
		Status:           entity.GameStatusWaitingForOpponent,
		PlayerOneAddress: testWallet1,
		BetAmount:        1000000000,
	}

	mockRepo.On("GetByID", ctx, int64(702)).Return(game, nil).Once()

	_, err := cancelUC.Reserve(ctx, 702, testWallet2)

	assert.ErrorIs(t, err, entity.ErrNotGameCreator)
	mockRepo.AssertExpectations(t)
}

func TestReservationReserve_RejectsWhenCancelReservationActive(t *testing.T) {
	ctx := context.Background()
	mockRepo := new(MockGameRepository)
	broadcastUC := usecase.NewGameBroadcastUseCase()

	reservationUC := usecase.NewReservationUseCase(mockRepo, broadcastUC, usecase.ReservationConfig{
		MaxPerWallet:           3,
		TimeoutSeconds:         60,
		CleanupIntervalSeconds: 5,
	})
	cancelUC := usecase.NewCancelReservationUseCase(mockRepo, reservationUC, broadcastUC, usecase.CancelReservationConfig{
		MaxPerWallet:           3,
		TimeoutSeconds:         60,
		CleanupIntervalSeconds: 5,
	})
	reservationUC.SetCancelReservationUseCase(cancelUC)

	game := &entity.Game{
		GameID:           703,
		Status:           entity.GameStatusWaitingForOpponent,
		PlayerOneAddress: testWallet1,
		BetAmount:        1000000000,
	}

	mockRepo.On("GetByID", ctx, int64(703)).Return(game, nil).Twice()

	_, err := cancelUC.Reserve(ctx, 703, testWallet1)
	require.NoError(t, err)

	_, err = reservationUC.Reserve(ctx, 703, testWallet2)

	assert.ErrorIs(t, err, entity.ErrGameCancellationPending)
	mockRepo.AssertExpectations(t)
}
