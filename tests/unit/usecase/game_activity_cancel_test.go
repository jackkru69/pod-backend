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

func TestGameActivityUseCase_CancelClaims(t *testing.T) {
	t.Run("owner should see resume cancel in my active queue", func(t *testing.T) {
		mockRepo := new(MockGameRepository)
		ctx := context.Background()
		now := time.Now().UTC()

		game := &entity.Game{
			GameID:           801,
			Status:           entity.GameStatusWaitingForOpponent,
			PlayerOneAddress: activityTestWallet,
			PlayerOneChoice:  entity.CoinSideHeads,
			BetAmount:        1000000000,
			CreatedAt:        now,
			InitTxHash:       "cancel-owner-801",
		}

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

		mockRepo.On("GetByID", ctx, int64(801)).Return(game, nil).Once()
		_, err := cancelUC.Reserve(ctx, 801, activityTestWallet)
		require.NoError(t, err)

		mockRepo.On("GetByStatus", ctx, entity.GameStatusWaitingForOpponent).
			Return([]*entity.Game{game}, nil).Once()
		mockRepo.On(
			"GetByPlayerAndStatuses",
			ctx,
			activityTestWallet,
			[]int{
				entity.GameStatusWaitingForOpponent,
				entity.GameStatusWaitingForOpenBids,
				entity.GameStatusEnded,
				entity.GameStatusPaid,
			},
		).Return([]*entity.Game{game}, nil).Once()

		uc := usecase.NewGameActivityUseCase(mockRepo, reservationUC, nil, nil, usecase.GameActivityConfig{DefaultLimit: 20, MaxLimit: 100})
		uc.SetCancelReservationUseCase(cancelUC)

		queue, summary, err := uc.GetQueue(ctx, entity.ActivityQueueMyActive, activityTestWallet, 20, 0)

		require.NoError(t, err)
		require.NotNil(t, queue)
		require.NotNil(t, summary)
		require.Len(t, queue.Items, 1)
		assert.Equal(t, entity.ActivityNextActionResumeCancel, queue.Items[0].NextAction)
		assert.True(t, queue.Items[0].RequiresAttention)
		require.Len(t, queue.Items[0].ActiveClaims, 1)
		assert.Equal(t, entity.ActionClaimTypeCancel, queue.Items[0].ActiveClaims[0].ClaimType)
		mockRepo.AssertExpectations(t)
	})

	t.Run("joiners should see cancel flow as busy joinable slot", func(t *testing.T) {
		mockRepo := new(MockGameRepository)
		ctx := context.Background()
		now := time.Now().UTC()

		game := &entity.Game{
			GameID:           802,
			Status:           entity.GameStatusWaitingForOpponent,
			PlayerOneAddress: activityTestOtherWallet,
			PlayerOneChoice:  entity.CoinSideHeads,
			BetAmount:        1000000000,
			CreatedAt:        now,
			InitTxHash:       "cancel-joinable-802",
		}

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

		mockRepo.On("GetByID", ctx, int64(802)).Return(game, nil).Once()
		_, err := cancelUC.Reserve(ctx, 802, activityTestOtherWallet)
		require.NoError(t, err)

		mockRepo.On("GetByStatus", ctx, entity.GameStatusWaitingForOpponent).
			Return([]*entity.Game{game}, nil).Once()
		mockRepo.On(
			"GetByPlayerAndStatuses",
			ctx,
			activityTestWallet,
			[]int{
				entity.GameStatusWaitingForOpponent,
				entity.GameStatusWaitingForOpenBids,
				entity.GameStatusEnded,
				entity.GameStatusPaid,
			},
		).Return([]*entity.Game{}, nil).Once()

		uc := usecase.NewGameActivityUseCase(mockRepo, reservationUC, nil, nil, usecase.GameActivityConfig{DefaultLimit: 20, MaxLimit: 100})
		uc.SetCancelReservationUseCase(cancelUC)

		queue, _, err := uc.GetQueue(ctx, entity.ActivityQueueJoinable, activityTestWallet, 20, 0)

		require.NoError(t, err)
		require.NotNil(t, queue)
		require.Len(t, queue.Items, 1)
		assert.Equal(t, entity.ActivityNextActionWaitForCancel, queue.Items[0].NextAction)
		assert.False(t, queue.Items[0].RequiresAttention)
		require.Len(t, queue.Items[0].ActiveClaims, 1)
		assert.Equal(t, entity.ActionClaimTypeCancel, queue.Items[0].ActiveClaims[0].ClaimType)
		mockRepo.AssertExpectations(t)
	})
}
