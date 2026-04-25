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

const (
	activityTestWallet      = "EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2"
	activityTestOtherWallet = "EQBvW8Z5huBkMJYdnfAEM5JqTNLuuU3FYxrVjxFBzXn3r95X"
)

//nolint:funlen // Queue classification coverage is clearer as one story-shaped test with focused subtests.
func TestGameActivityUseCase_GetQueue(t *testing.T) {
	t.Run("should keep owned waiting game out of joinable queue and include it in my active", func(t *testing.T) {
		mockRepo := new(MockGameRepository)
		ctx := context.Background()
		walletAddress := activityTestWallet
		otherWallet := activityTestOtherWallet
		now := time.Now().UTC()

		joinableGame := &entity.Game{
			GameID:           101,
			Status:           entity.GameStatusWaitingForOpponent,
			PlayerOneAddress: otherWallet,
			PlayerOneChoice:  entity.CoinSideHeads,
			BetAmount:        1000000000,
			CreatedAt:        now,
			InitTxHash:       "joinable-101",
		}
		ownedLobbyGame := &entity.Game{
			GameID:           102,
			Status:           entity.GameStatusWaitingForOpponent,
			PlayerOneAddress: walletAddress,
			PlayerOneChoice:  entity.CoinSideTails,
			BetAmount:        1200000000,
			CreatedAt:        now.Add(-time.Minute),
			InitTxHash:       "owned-102",
		}

		mockRepo.On("GetByStatus", ctx, entity.GameStatusWaitingForOpponent).
			Return([]*entity.Game{joinableGame, ownedLobbyGame}, nil).Once()
		mockRepo.On(
			"GetByPlayerAndStatuses",
			ctx,
			walletAddress,
			[]int{
				entity.GameStatusWaitingForOpponent,
				entity.GameStatusWaitingForOpenBids,
				entity.GameStatusEnded,
				entity.GameStatusPaid,
			},
		).Return([]*entity.Game{ownedLobbyGame}, nil).Once()

		uc := usecase.NewGameActivityUseCase(mockRepo, nil, nil, nil, usecase.GameActivityConfig{DefaultLimit: 20, MaxLimit: 100})

		queue, summary, err := uc.GetQueue(ctx, entity.ActivityQueueJoinable, walletAddress, 20, 0)

		require.NoError(t, err)
		require.NotNil(t, queue)
		require.NotNil(t, summary)
		require.Len(t, queue.Items, 1)
		assert.Equal(t, int64(101), queue.Items[0].Game.GameID)
		assert.Equal(t, entity.ActivityNextActionJoin, queue.Items[0].NextAction)
		assert.Equal(t, 1, summary.JoinableCount)
		assert.Equal(t, 1, summary.MyActiveCount)
		mockRepo.AssertExpectations(t)
	})

	t.Run("should classify hidden player choice into reveal required queue", func(t *testing.T) {
		mockRepo := new(MockGameRepository)
		ctx := context.Background()
		walletAddress := activityTestWallet
		otherWallet := activityTestOtherWallet
		now := time.Now().UTC()
		joinedAt := now.Add(-5 * time.Minute)
		playerTwoChoice := entity.CoinSideClosed

		revealGame := &entity.Game{
			GameID:           201,
			Status:           entity.GameStatusWaitingForOpenBids,
			PlayerOneAddress: walletAddress,
			PlayerTwoAddress: &otherWallet,
			PlayerOneChoice:  entity.CoinSideClosed,
			PlayerTwoChoice:  &playerTwoChoice,
			BetAmount:        1000000000,
			CreatedAt:        now.Add(-10 * time.Minute),
			JoinedAt:         &joinedAt,
			InitTxHash:       "reveal-201",
		}

		mockRepo.On("GetByStatus", ctx, entity.GameStatusWaitingForOpponent).
			Return([]*entity.Game{}, nil).Once()
		mockRepo.On(
			"GetByPlayerAndStatuses",
			ctx,
			walletAddress,
			[]int{
				entity.GameStatusWaitingForOpponent,
				entity.GameStatusWaitingForOpenBids,
				entity.GameStatusEnded,
				entity.GameStatusPaid,
			},
		).Return([]*entity.Game{revealGame}, nil).Once()

		uc := usecase.NewGameActivityUseCase(mockRepo, nil, nil, nil, usecase.GameActivityConfig{DefaultLimit: 20, MaxLimit: 100})

		queue, summary, err := uc.GetQueue(ctx, entity.ActivityQueueRevealRequired, walletAddress, 20, 0)

		require.NoError(t, err)
		require.NotNil(t, queue)
		require.Len(t, queue.Items, 1)
		assert.Equal(t, entity.ActivityQueueRevealRequired, queue.Items[0].QueueKey)
		assert.Equal(t, entity.ActivityNextActionReveal, queue.Items[0].NextAction)
		assert.True(t, queue.Items[0].RequiresAttention)
		assert.Equal(t, 1, summary.RevealRequiredCount)
		mockRepo.AssertExpectations(t)
	})

	t.Run("should separate ended games from paid history", func(t *testing.T) {
		mockRepo := new(MockGameRepository)
		ctx := context.Background()
		walletAddress := activityTestWallet
		otherWallet := activityTestOtherWallet
		now := time.Now().UTC()
		completedEnded := now.Add(-2 * time.Hour)
		completedPaid := now.Add(-time.Hour)
		playerTwoChoice := entity.CoinSideTails
		winnerAddress := walletAddress
		payoutAmount := int64(1900000000)

		endedGame := &entity.Game{
			GameID:           301,
			Status:           entity.GameStatusEnded,
			PlayerOneAddress: walletAddress,
			PlayerTwoAddress: &otherWallet,
			PlayerOneChoice:  entity.CoinSideHeads,
			PlayerTwoChoice:  &playerTwoChoice,
			BetAmount:        1000000000,
			CreatedAt:        now.Add(-3 * time.Hour),
			CompletedAt:      &completedEnded,
			InitTxHash:       "ended-301",
		}
		paidGame := &entity.Game{
			GameID:           302,
			Status:           entity.GameStatusPaid,
			PlayerOneAddress: walletAddress,
			PlayerTwoAddress: &otherWallet,
			PlayerOneChoice:  entity.CoinSideHeads,
			PlayerTwoChoice:  &playerTwoChoice,
			BetAmount:        1000000000,
			WinnerAddress:    &winnerAddress,
			PayoutAmount:     &payoutAmount,
			CreatedAt:        now.Add(-90 * time.Minute),
			CompletedAt:      &completedPaid,
			InitTxHash:       "paid-302",
		}

		mockRepo.On("GetByStatus", ctx, entity.GameStatusWaitingForOpponent).
			Return([]*entity.Game{}, nil).Once()
		mockRepo.On(
			"GetByPlayerAndStatuses",
			ctx,
			walletAddress,
			[]int{
				entity.GameStatusWaitingForOpponent,
				entity.GameStatusWaitingForOpenBids,
				entity.GameStatusEnded,
				entity.GameStatusPaid,
			},
		).Return([]*entity.Game{endedGame, paidGame}, nil).Once()

		uc := usecase.NewGameActivityUseCase(mockRepo, nil, nil, nil, usecase.GameActivityConfig{DefaultLimit: 20, MaxLimit: 100})

		queue, summary, err := uc.GetQueue(ctx, entity.ActivityQueueHistory, walletAddress, 20, 0)

		require.NoError(t, err)
		require.NotNil(t, queue)
		require.Len(t, queue.Items, 1)
		assert.Equal(t, int64(302), queue.Items[0].Game.GameID)
		assert.Equal(t, 1, summary.ExpiredAttentionCount)
		assert.Equal(t, 1, summary.HistoryCount)
		mockRepo.AssertExpectations(t)
	})

	t.Run("should reject invalid queue key", func(t *testing.T) {
		mockRepo := new(MockGameRepository)
		uc := usecase.NewGameActivityUseCase(mockRepo, nil, nil, nil, usecase.GameActivityConfig{DefaultLimit: 20, MaxLimit: 100})

		queue, summary, err := uc.GetQueue(context.Background(), entity.ActivityQueueKey("mystery"), "", 20, 0)

		assert.Error(t, err)
		assert.Nil(t, queue)
		assert.Nil(t, summary)
	})

	t.Run("should classify ended game with own expired claim as resumable attention", func(t *testing.T) {
		mockRepo := new(MockGameRepository)
		ctx := context.Background()
		walletAddress := activityTestWallet
		otherWallet := activityTestOtherWallet
		now := time.Now().UTC()
		completedAt := now.Add(-time.Minute)

		endedGame := &entity.Game{
			GameID:           401,
			Status:           entity.GameStatusEnded,
			PlayerOneAddress: walletAddress,
			PlayerTwoAddress: &otherWallet,
			PlayerOneChoice:  entity.CoinSideHeads,
			BetAmount:        1000000000,
			CreatedAt:        now.Add(-10 * time.Minute),
			CompletedAt:      &completedAt,
			InitTxHash:       "ended-401",
		}

		expiredClaimUC := usecase.NewExpiredClaimUseCase(mockRepo, nil, usecase.ExpiredClaimConfig{
			MaxPerWallet:           5,
			TimeoutSeconds:         120,
			CleanupIntervalSeconds: 5,
		})

		mockRepo.On("GetByID", ctx, int64(401)).Return(endedGame, nil).Twice()
		_, err := expiredClaimUC.Claim(ctx, 401, walletAddress)
		require.NoError(t, err)

		mockRepo.On("GetByStatus", ctx, entity.GameStatusWaitingForOpponent).
			Return([]*entity.Game{}, nil).Once()
		mockRepo.On(
			"GetByPlayerAndStatuses",
			ctx,
			walletAddress,
			[]int{
				entity.GameStatusWaitingForOpponent,
				entity.GameStatusWaitingForOpenBids,
				entity.GameStatusEnded,
				entity.GameStatusPaid,
			},
		).Return([]*entity.Game{endedGame}, nil).Once()

		uc := usecase.NewGameActivityUseCase(mockRepo, nil, nil, expiredClaimUC, usecase.GameActivityConfig{DefaultLimit: 20, MaxLimit: 100})

		queue, _, err := uc.GetQueue(ctx, entity.ActivityQueueExpiredAttention, walletAddress, 20, 0)

		require.NoError(t, err)
		require.Len(t, queue.Items, 1)
		assert.Equal(t, entity.ActivityNextActionResumeReview, queue.Items[0].NextAction)
		assert.True(t, queue.Items[0].RequiresAttention)
		require.Len(t, queue.Items[0].ActiveClaims, 1)
		assert.Equal(t, entity.ActionClaimTypeExpiredFollowUp, queue.Items[0].ActiveClaims[0].ClaimType)
		mockRepo.AssertExpectations(t)
	})

	t.Run("should classify ended game with another holder expired claim as busy", func(t *testing.T) {
		mockRepo := new(MockGameRepository)
		ctx := context.Background()
		walletAddress := activityTestWallet
		otherWallet := activityTestOtherWallet
		now := time.Now().UTC()
		completedAt := now.Add(-time.Minute)

		endedGame := &entity.Game{
			GameID:           402,
			Status:           entity.GameStatusEnded,
			PlayerOneAddress: walletAddress,
			PlayerTwoAddress: &otherWallet,
			PlayerOneChoice:  entity.CoinSideHeads,
			BetAmount:        1000000000,
			CreatedAt:        now.Add(-10 * time.Minute),
			CompletedAt:      &completedAt,
			InitTxHash:       "ended-402",
		}

		expiredClaimUC := usecase.NewExpiredClaimUseCase(mockRepo, nil, usecase.ExpiredClaimConfig{
			MaxPerWallet:           5,
			TimeoutSeconds:         120,
			CleanupIntervalSeconds: 5,
		})

		mockRepo.On("GetByID", ctx, int64(402)).Return(endedGame, nil).Twice()
		_, err := expiredClaimUC.Claim(ctx, 402, otherWallet)
		require.NoError(t, err)

		mockRepo.On("GetByStatus", ctx, entity.GameStatusWaitingForOpponent).
			Return([]*entity.Game{}, nil).Once()
		mockRepo.On(
			"GetByPlayerAndStatuses",
			ctx,
			walletAddress,
			[]int{
				entity.GameStatusWaitingForOpponent,
				entity.GameStatusWaitingForOpenBids,
				entity.GameStatusEnded,
				entity.GameStatusPaid,
			},
		).Return([]*entity.Game{endedGame}, nil).Once()

		uc := usecase.NewGameActivityUseCase(mockRepo, nil, nil, expiredClaimUC, usecase.GameActivityConfig{DefaultLimit: 20, MaxLimit: 100})

		queue, _, err := uc.GetQueue(ctx, entity.ActivityQueueExpiredAttention, walletAddress, 20, 0)

		require.NoError(t, err)
		require.Len(t, queue.Items, 1)
		assert.Equal(t, entity.ActivityNextActionWaitForReview, queue.Items[0].NextAction)
		assert.False(t, queue.Items[0].RequiresAttention)
		mockRepo.AssertExpectations(t)
	})
}

func TestGameActivityUseCase_Search(t *testing.T) {
	t.Run("should find matching activity by game id across visible queues", func(t *testing.T) {
		mockRepo := new(MockGameRepository)
		ctx := context.Background()
		walletAddress := activityTestWallet
		otherWallet := activityTestOtherWallet
		now := time.Now().UTC()
		completedAt := now.Add(-time.Minute)
		playerTwoChoice := entity.CoinSideTails
		winnerAddress := walletAddress
		payoutAmount := int64(1900000000)

		joinableGame := &entity.Game{
			GameID:           501,
			Status:           entity.GameStatusWaitingForOpponent,
			PlayerOneAddress: otherWallet,
			PlayerOneChoice:  entity.CoinSideHeads,
			BetAmount:        1000000000,
			CreatedAt:        now,
			InitTxHash:       "joinable-501",
		}
		paidGame := &entity.Game{
			GameID:           502,
			Status:           entity.GameStatusPaid,
			PlayerOneAddress: walletAddress,
			PlayerTwoAddress: &otherWallet,
			PlayerOneChoice:  entity.CoinSideHeads,
			PlayerTwoChoice:  &playerTwoChoice,
			WinnerAddress:    &winnerAddress,
			PayoutAmount:     &payoutAmount,
			BetAmount:        1000000000,
			CreatedAt:        now.Add(-10 * time.Minute),
			CompletedAt:      &completedAt,
			InitTxHash:       "paid-502",
		}

		mockRepo.On("GetByStatus", ctx, entity.GameStatusWaitingForOpponent).
			Return([]*entity.Game{joinableGame}, nil).Once()
		mockRepo.On(
			"GetByPlayerAndStatuses",
			ctx,
			walletAddress,
			[]int{
				entity.GameStatusWaitingForOpponent,
				entity.GameStatusWaitingForOpenBids,
				entity.GameStatusEnded,
				entity.GameStatusPaid,
			},
		).Return([]*entity.Game{paidGame}, nil).Once()

		uc := usecase.NewGameActivityUseCase(mockRepo, nil, nil, nil, usecase.GameActivityConfig{DefaultLimit: 20, MaxLimit: 100})

		items, summary, total, err := uc.Search(ctx, walletAddress, "502", "", 20, 0)

		require.NoError(t, err)
		require.NotNil(t, summary)
		require.Len(t, items, 1)
		assert.Equal(t, 1, total)
		assert.Equal(t, int64(502), items[0].Game.GameID)
		assert.Equal(t, entity.ActivityQueueHistory, items[0].QueueKey)
		assert.Equal(t, 1, summary.JoinableCount)
		assert.Equal(t, 1, summary.HistoryCount)
		mockRepo.AssertExpectations(t)
	})

	t.Run("should filter search results by queue scope and wallet variant", func(t *testing.T) {
		mockRepo := new(MockGameRepository)
		ctx := context.Background()
		walletAddress := activityTestWallet
		otherWallet := activityTestOtherWallet
		now := time.Now().UTC()
		playerTwoChoice := entity.CoinSideClosed

		revealGame := &entity.Game{
			GameID:           503,
			Status:           entity.GameStatusWaitingForOpenBids,
			PlayerOneAddress: walletAddress,
			PlayerTwoAddress: &otherWallet,
			PlayerOneChoice:  entity.CoinSideClosed,
			PlayerTwoChoice:  &playerTwoChoice,
			BetAmount:        1000000000,
			CreatedAt:        now,
			InitTxHash:       "reveal-503",
		}

		mockRepo.On("GetByStatus", ctx, entity.GameStatusWaitingForOpponent).
			Return([]*entity.Game{}, nil).Once()
		mockRepo.On(
			"GetByPlayerAndStatuses",
			ctx,
			walletAddress,
			[]int{
				entity.GameStatusWaitingForOpponent,
				entity.GameStatusWaitingForOpenBids,
				entity.GameStatusEnded,
				entity.GameStatusPaid,
			},
		).Return([]*entity.Game{revealGame}, nil).Once()

		uc := usecase.NewGameActivityUseCase(mockRepo, nil, nil, nil, usecase.GameActivityConfig{DefaultLimit: 20, MaxLimit: 100})

		items, _, total, err := uc.Search(ctx, walletAddress, otherWallet, entity.ActivityQueueRevealRequired, 20, 0)

		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, 1, total)
		assert.Equal(t, int64(503), items[0].Game.GameID)
		assert.Equal(t, entity.ActivityQueueRevealRequired, items[0].QueueKey)
		mockRepo.AssertExpectations(t)
	})

	t.Run("should return empty results for blank query without error", func(t *testing.T) {
		mockRepo := new(MockGameRepository)
		ctx := context.Background()

		uc := usecase.NewGameActivityUseCase(mockRepo, nil, nil, nil, usecase.GameActivityConfig{DefaultLimit: 20, MaxLimit: 100})

		items, _, total, err := uc.Search(ctx, activityTestWallet, "   ", "", 20, 0)

		require.NoError(t, err)
		assert.Empty(t, items)
		assert.Equal(t, 0, total)
	})
}
