package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"pod-backend/internal/entity"
	"pod-backend/internal/usecase"
)

func TestHandleGameFinished_ReleasesRevealReservation(t *testing.T) {
	ctx := context.Background()
	mockGameRepo := new(MockGameRepository)
	mockEventRepo := new(MockGameEventRepository)
	mockUserRepo := new(MockUserRepository)

	persistenceUC := usecase.NewGamePersistenceUseCase(mockGameRepo, mockEventRepo, mockUserRepo)
	revealUC := usecase.NewRevealReservationUseCase(mockGameRepo, nil, usecase.RevealReservationConfig{
		MaxPerWallet:           5,
		TimeoutSeconds:         90,
		CleanupIntervalSeconds: 5,
	})
	persistenceUC.SetRevealReservationUseCase(revealUC)

	winnerAddress := testWallet1
	loserAddress := testWallet2
	loserPtr := loserAddress
	existingGame := &entity.Game{
		GameID:           777,
		PlayerOneAddress: winnerAddress,
		PlayerTwoAddress: &loserPtr,
		BetAmount:        1000000000,
		Status:           entity.GameStatusWaitingForOpenBids,
	}

	// Reserve reveal before the terminal event.
	mockGameRepo.On("GetByID", ctx, int64(777)).Return(existingGame, nil).Times(2)
	_, err := revealUC.Reserve(ctx, 777, winnerAddress)
	require.NoError(t, err)

	event := &entity.GameEvent{
		EventType:       entity.EventTypeGameFinished,
		GameID:          777,
		TransactionHash: "tx_finish_777",
		BlockNumber:     1002,
		Timestamp:       time.Now(),
		EventData: map[string]interface{}{
			"game_id":        int64(777),
			"winner":         winnerAddress,
			"total_gainings": int64(1900000000),
		},
	}

	mockEventRepo.On("Upsert", mock.Anything, event).Run(func(args mock.Arguments) {
		e := args.Get(1).(*entity.GameEvent)
		e.ID = 1
	}).Return(nil)
	mockGameRepo.On("CompleteGame", mock.Anything, int64(777), winnerAddress, int64(1900000000), "tx_finish_777", event.Timestamp).Return(nil)
	mockUserRepo.On("IncrementGamesPlayed", mock.Anything, winnerAddress).Return(nil)
	mockUserRepo.On("IncrementWins", mock.Anything, winnerAddress).Return(nil)
	mockUserRepo.On("IncrementGamesPlayed", mock.Anything, loserAddress).Return(nil)
	mockUserRepo.On("IncrementLosses", mock.Anything, loserAddress).Return(nil)

	err = persistenceUC.HandleGameFinished(ctx, event)

	assert.NoError(t, err)
	reservation, getErr := revealUC.Get(ctx, 777)
	require.NoError(t, getErr)
	assert.Nil(t, reservation)
	mockGameRepo.AssertExpectations(t)
	mockEventRepo.AssertExpectations(t)
	mockUserRepo.AssertExpectations(t)
}
