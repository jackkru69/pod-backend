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
