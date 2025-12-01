package usecase

import (
	"context"
	"fmt"

	"pod-backend/internal/entity"
	"pod-backend/internal/repository"
)

// UserManagementUseCase handles user profile operations including Telegram-based
// user creation, profile retrieval, and referral statistics.
// Supports FR-003 (automatic profile creation), FR-013 (wallet association),
// FR-021 (referral stats exposure).
type UserManagementUseCase struct {
	userRepo repository.UserRepository
}

// NewUserManagementUseCase creates a new user management use case.
func NewUserManagementUseCase(userRepo repository.UserRepository) *UserManagementUseCase {
	return &UserManagementUseCase{
		userRepo: userRepo,
	}
}

// CreateOrUpdateUser creates a new user profile or updates an existing one.
// Used for automatic user profile creation from Telegram Mini App authentication (FR-003).
// Validates user entity before persistence.
func (uc *UserManagementUseCase) CreateOrUpdateUser(ctx context.Context, user *entity.User) error {
	// Validate user entity
	if err := user.Validate(); err != nil {
		return fmt.Errorf("user validation failed: %w", err)
	}

	// Delegate to repository
	if err := uc.userRepo.CreateOrUpdate(ctx, user); err != nil {
		return fmt.Errorf("failed to create or update user: %w", err)
	}

	return nil
}

// GetUserByWallet retrieves a user profile by TON wallet address.
// Supports FR-004 requirement to retrieve user game history and profile.
func (uc *UserManagementUseCase) GetUserByWallet(ctx context.Context, walletAddress string) (*entity.User, error) {
	if walletAddress == "" {
		return nil, fmt.Errorf("wallet address is required")
	}

	user, err := uc.userRepo.GetByWallet(ctx, walletAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to get user by wallet: %w", err)
	}

	return user, nil
}

// GetReferralStats retrieves aggregated referral statistics for a wallet address.
// Supports FR-021 requirement to expose referral metrics including total referrals,
// total referral earnings, and games referred.
func (uc *UserManagementUseCase) GetReferralStats(ctx context.Context, walletAddress string) (*entity.ReferralStats, error) {
	if walletAddress == "" {
		return nil, fmt.Errorf("wallet address is required")
	}

	stats, err := uc.userRepo.GetReferralStats(ctx, walletAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to get referral stats: %w", err)
	}

	return stats, nil
}
