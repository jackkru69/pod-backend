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

func TestExpiredClaim_SuccessAndResume(t *testing.T) {
	ctx := context.Background()
	mockRepo := new(MockGameRepository)
	broadcastUC := usecase.NewGameBroadcastUseCase()
	otherWallet := testWallet2
	game := &entity.Game{
		GameID:           901,
		Status:           entity.GameStatusEnded,
		PlayerOneAddress: testWallet1,
		PlayerTwoAddress: &otherWallet,
		BetAmount:        1000000000,
	}

	uc := usecase.NewExpiredClaimUseCase(mockRepo, broadcastUC, usecase.ExpiredClaimConfig{
		MaxPerWallet:           5,
		TimeoutSeconds:         120,
		CleanupIntervalSeconds: 5,
	})

	mockRepo.On("GetByID", ctx, int64(901)).Return(game, nil).Twice()

	claim, err := uc.Claim(ctx, 901, testWallet1)
	require.NoError(t, err)
	require.NotNil(t, claim)
	assert.Equal(t, int64(901), claim.GameID)
	assert.Equal(t, testWallet1, claim.WalletAddress)
	assert.Equal(t, entity.ExpiredClaimStatusActive, claim.Status)
	assert.WithinDuration(t, time.Now().Add(120*time.Second), claim.ExpiresAt, 2*time.Second)

	resumedClaim, err := uc.Claim(ctx, 901, testWallet1)
	require.NoError(t, err)
	assert.Same(t, claim, resumedClaim)
	assert.Equal(t, 1, uc.GetActiveCount())
	mockRepo.AssertExpectations(t)
}

func TestExpiredClaim_NotParticipant(t *testing.T) {
	ctx := context.Background()
	mockRepo := new(MockGameRepository)
	broadcastUC := usecase.NewGameBroadcastUseCase()
	otherWallet := testWallet2
	game := &entity.Game{
		GameID:           902,
		Status:           entity.GameStatusEnded,
		PlayerOneAddress: testWallet1,
		PlayerTwoAddress: &otherWallet,
		BetAmount:        1000000000,
	}

	uc := usecase.NewExpiredClaimUseCase(mockRepo, broadcastUC, usecase.ExpiredClaimConfig{
		MaxPerWallet:           5,
		TimeoutSeconds:         120,
		CleanupIntervalSeconds: 5,
	})

	mockRepo.On("GetByID", ctx, int64(902)).Return(game, nil).Once()

	_, err := uc.Claim(ctx, 902, testWallet3)

	assert.ErrorIs(t, err, entity.ErrNotExpiredClaimParticipant)
	mockRepo.AssertExpectations(t)
}

func TestExpiredClaim_GetReleasesWhenGameResolved(t *testing.T) {
	ctx := context.Background()
	mockRepo := new(MockGameRepository)
	broadcastUC := usecase.NewGameBroadcastUseCase()
	otherWallet := testWallet2
	endedGame := &entity.Game{
		GameID:           903,
		Status:           entity.GameStatusEnded,
		PlayerOneAddress: testWallet1,
		PlayerTwoAddress: &otherWallet,
		BetAmount:        1000000000,
	}
	paidGame := &entity.Game{
		GameID:           903,
		Status:           entity.GameStatusPaid,
		PlayerOneAddress: testWallet1,
		PlayerTwoAddress: &otherWallet,
		BetAmount:        1000000000,
	}

	uc := usecase.NewExpiredClaimUseCase(mockRepo, broadcastUC, usecase.ExpiredClaimConfig{
		MaxPerWallet:           5,
		TimeoutSeconds:         120,
		CleanupIntervalSeconds: 5,
	})

	mockRepo.On("GetByID", ctx, int64(903)).Return(endedGame, nil).Once()
	_, err := uc.Claim(ctx, 903, testWallet1)
	require.NoError(t, err)

	mockRepo.On("GetByID", ctx, int64(903)).Return(paidGame, nil).Once()
	claim, err := uc.Get(ctx, 903)

	require.NoError(t, err)
	assert.Nil(t, claim)
	assert.Equal(t, 0, uc.GetActiveCount())
	mockRepo.AssertExpectations(t)
}

func TestExpiredClaim_ReleaseHolderOnly(t *testing.T) {
	ctx := context.Background()
	mockRepo := new(MockGameRepository)
	broadcastUC := usecase.NewGameBroadcastUseCase()
	otherWallet := testWallet2
	game := &entity.Game{
		GameID:           904,
		Status:           entity.GameStatusEnded,
		PlayerOneAddress: testWallet1,
		PlayerTwoAddress: &otherWallet,
		BetAmount:        1000000000,
	}

	uc := usecase.NewExpiredClaimUseCase(mockRepo, broadcastUC, usecase.ExpiredClaimConfig{
		MaxPerWallet:           5,
		TimeoutSeconds:         120,
		CleanupIntervalSeconds: 5,
	})

	mockRepo.On("GetByID", ctx, int64(904)).Return(game, nil).Once()
	_, err := uc.Claim(ctx, 904, testWallet1)
	require.NoError(t, err)

	err = uc.Release(ctx, 904, testWallet2)
	assert.ErrorIs(t, err, entity.ErrNotExpiredClaimHolder)

	err = uc.Release(ctx, 904, testWallet1)
	require.NoError(t, err)
	assert.Equal(t, 0, uc.GetActiveCount())
	mockRepo.AssertExpectations(t)
}
