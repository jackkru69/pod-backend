package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"pod-backend/internal/entity"
	"pod-backend/internal/usecase"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// TestCreateOrUpdateUser_Success tests successful user creation
func TestCreateOrUpdateUser_Success(t *testing.T) {
	mockRepo := new(MockUserRepository)
	uc := usecase.NewUserManagementUseCase(mockRepo)

	telegramID := int64(123456789)
	user := &entity.User{
		TelegramUserID:   &telegramID,
		TelegramUsername: "testuser",
		WalletAddress:    activityTestWallet,
	}

	mockRepo.On("CreateOrUpdate", mock.Anything, user).Return(nil)

	err := uc.CreateOrUpdateUser(context.Background(), user)

	assert.NoError(t, err)
	mockRepo.AssertExpectations(t)
}

// TestCreateOrUpdateUser_RepositoryError tests repository error handling
func TestCreateOrUpdateUser_RepositoryError(t *testing.T) {
	mockRepo := new(MockUserRepository)
	uc := usecase.NewUserManagementUseCase(mockRepo)

	telegramID := int64(123456789)
	user := &entity.User{
		TelegramUserID:   &telegramID,
		TelegramUsername: "testuser",
		WalletAddress:    activityTestWallet,
	}

	expectedError := errors.New("database connection failed")
	mockRepo.On("CreateOrUpdate", mock.Anything, user).Return(expectedError)

	err := uc.CreateOrUpdateUser(context.Background(), user)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database connection failed")
	mockRepo.AssertExpectations(t)
}

// TestGetUserByWallet_Success tests successful user retrieval
func TestGetUserByWallet_Success(t *testing.T) {
	mockRepo := new(MockUserRepository)
	uc := usecase.NewUserManagementUseCase(mockRepo)

	walletAddress := activityTestWallet
	telegramID := int64(123456789)
	expectedUser := &entity.User{
		ID:               1,
		TelegramUserID:   &telegramID,
		TelegramUsername: "testuser",
		WalletAddress:    walletAddress,
		TotalGamesPlayed: 10,
		TotalWins:        6,
		TotalLosses:      4,
		CreatedAt:        time.Now(),
	}

	mockRepo.On("GetByWallet", mock.Anything, walletAddress).Return(expectedUser, nil)

	user, err := uc.GetUserByWallet(context.Background(), walletAddress)

	assert.NoError(t, err)
	assert.Equal(t, expectedUser, user)
	mockRepo.AssertExpectations(t)
}

// TestGetUserByWallet_NotFound tests user not found scenario
func TestGetUserByWallet_NotFound(t *testing.T) {
	mockRepo := new(MockUserRepository)
	uc := usecase.NewUserManagementUseCase(mockRepo)

	walletAddress := activityTestWallet
	mockRepo.On("GetByWallet", mock.Anything, walletAddress).Return(nil, errors.New("user not found"))

	user, err := uc.GetUserByWallet(context.Background(), walletAddress)

	assert.Error(t, err)
	assert.Nil(t, user)
	mockRepo.AssertExpectations(t)
}

// TestGetUserByWallet_EmptyWallet tests empty wallet validation
func TestGetUserByWallet_EmptyWallet(t *testing.T) {
	mockRepo := new(MockUserRepository)
	uc := usecase.NewUserManagementUseCase(mockRepo)

	user, err := uc.GetUserByWallet(context.Background(), "")

	assert.Error(t, err)
	assert.Nil(t, user)
	assert.Contains(t, err.Error(), "wallet address is required")
}

// TestGetReferralStats_Success tests successful referral stats retrieval
func TestGetReferralStats_Success(t *testing.T) {
	mockRepo := new(MockUserRepository)
	uc := usecase.NewUserManagementUseCase(mockRepo)

	walletAddress := activityTestWallet
	expectedStats := &entity.ReferralStats{
		WalletAddress:         walletAddress,
		TotalReferrals:        15,
		TotalReferralEarnings: 1500000000, // 1.5 TON in nanotons
		GamesReferred:         25,
	}

	mockRepo.On("GetReferralStats", mock.Anything, walletAddress).Return(expectedStats, nil)

	stats, err := uc.GetReferralStats(context.Background(), walletAddress)

	assert.NoError(t, err)
	assert.Equal(t, expectedStats, stats)
	assert.Equal(t, int64(15), stats.TotalReferrals)
	assert.Equal(t, int64(1500000000), stats.TotalReferralEarnings)
	mockRepo.AssertExpectations(t)
}

// TestGetReferralStats_NoReferrals tests zero referrals scenario
func TestGetReferralStats_NoReferrals(t *testing.T) {
	mockRepo := new(MockUserRepository)
	uc := usecase.NewUserManagementUseCase(mockRepo)

	walletAddress := activityTestWallet
	expectedStats := &entity.ReferralStats{
		WalletAddress:         walletAddress,
		TotalReferrals:        0,
		TotalReferralEarnings: 0,
		GamesReferred:         0,
	}

	mockRepo.On("GetReferralStats", mock.Anything, walletAddress).Return(expectedStats, nil)

	stats, err := uc.GetReferralStats(context.Background(), walletAddress)

	assert.NoError(t, err)
	assert.Equal(t, int64(0), stats.TotalReferrals)
	assert.Equal(t, int64(0), stats.TotalReferralEarnings)
	mockRepo.AssertExpectations(t)
}

// TestGetReferralStats_RepositoryError tests repository error handling
func TestGetReferralStats_RepositoryError(t *testing.T) {
	mockRepo := new(MockUserRepository)
	uc := usecase.NewUserManagementUseCase(mockRepo)

	walletAddress := activityTestWallet
	expectedError := errors.New("database query failed")
	mockRepo.On("GetReferralStats", mock.Anything, walletAddress).Return(nil, expectedError)

	stats, err := uc.GetReferralStats(context.Background(), walletAddress)

	assert.Error(t, err)
	assert.Nil(t, stats)
	assert.Contains(t, err.Error(), "database query failed")
	mockRepo.AssertExpectations(t)
}

// TestGetReferralStats_EmptyWallet tests empty wallet validation
func TestGetReferralStats_EmptyWallet(t *testing.T) {
	mockRepo := new(MockUserRepository)
	uc := usecase.NewUserManagementUseCase(mockRepo)

	stats, err := uc.GetReferralStats(context.Background(), "")

	assert.Error(t, err)
	assert.Nil(t, stats)
	assert.Contains(t, err.Error(), "wallet address is required")
}
