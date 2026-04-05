package rest

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
	"pod-backend/internal/entity"
	"pod-backend/internal/usecase"
)

// UserHandler handles HTTP requests for user profile operations.
// Supports FR-003 (automatic profile creation), FR-006 (game history),
// FR-021 (referral statistics).
type UserHandler struct {
	userUseCase *usecase.UserManagementUseCase
	gameUseCase *usecase.GameQueryUseCase
	logger      zerolog.Logger
}

// NewUserHandler creates a new user handler.
func NewUserHandler(
	userUseCase *usecase.UserManagementUseCase,
	gameUseCase *usecase.GameQueryUseCase,
	logger zerolog.Logger,
) *UserHandler {
	return &UserHandler{
		userUseCase: userUseCase,
		gameUseCase: gameUseCase,
		logger:      logger.With().Str("component", "user_handler").Logger(),
	}
}

// GetUserProfile retrieves a user profile by wallet address.
//
// @Summary Get user profile
// @Description Retrieve user profile information including statistics (total games, wins, losses)
// @Tags users
// @Accept json
// @Produce json
// @Param address path string true "TON wallet address (EQ...)" example("EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2")
// @Success 200 {object} entity.User "User profile with statistics"
// @Failure 400 {object} ErrorResponse "Invalid wallet address format"
// @Failure 404 {object} ErrorResponse "User not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /api/v1/users/{address} [get]
func (h *UserHandler) GetUserProfile(c *fiber.Ctx) error {
	walletAddress := c.Params("address")

	h.logger.Debug().Str("wallet_address", walletAddress).Msg("Get user profile request")

	// Retrieve user profile
	user, err := h.userUseCase.GetUserByWallet(c.Context(), walletAddress)
	if err != nil {
		h.logger.Error().Err(err).Str("wallet_address", walletAddress).Msg("Failed to get user profile")
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
			Error: "User not found",
		})
	}

	h.logger.Info().Str("wallet_address", walletAddress).Int64("user_id", user.ID).Msg("User profile retrieved")

	return c.JSON(user)
}

// GetUserGameHistory retrieves paginated game history for a user.
//
// @Summary Get user game history
// @Description Retrieve paginated list of games where the user participated (as player 1 or player 2)
// @Tags users
// @Accept json
// @Produce json
// @Param address path string true "TON wallet address (EQ...)" example("EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2")
// @Param limit query int false "Number of games to return (default: 20, max: 100)" default(20)
// @Param offset query int false "Offset for pagination (default: 0)" default(0)
// @Success 200 {object} GameHistoryResponse "Paginated game history"
// @Failure 400 {object} ErrorResponse "Invalid query parameters"
// @Failure 404 {object} ErrorResponse "User not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /api/v1/users/{address}/history [get]
func (h *UserHandler) GetUserGameHistory(c *fiber.Ctx) error {
	walletAddress := c.Params("address")

	// Parse pagination parameters
	limit := 20
	if limitParam := c.Query("limit"); limitParam != "" {
		parsedLimit, err := strconv.Atoi(limitParam)
		if err != nil || parsedLimit <= 0 || parsedLimit > 100 {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error: "Invalid limit parameter (must be between 1 and 100)",
			})
		}
		limit = parsedLimit
	}

	offset := 0
	if offsetParam := c.Query("offset"); offsetParam != "" {
		parsedOffset, err := strconv.Atoi(offsetParam)
		if err != nil || parsedOffset < 0 {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error: "Invalid offset parameter (must be non-negative)",
			})
		}
		offset = parsedOffset
	}

	h.logger.Debug().
		Str("wallet_address", walletAddress).
		Int("limit", limit).
		Int("offset", offset).
		Msg("Get user game history request")

	// Verify user exists
	_, err := h.userUseCase.GetUserByWallet(c.Context(), walletAddress)
	if err != nil {
		h.logger.Error().Err(err).Str("wallet_address", walletAddress).Msg("User not found for history query")
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
			Error: "User not found",
		})
	}

	// Retrieve game history (FR-006)
	games, total, err := h.gameUseCase.GetGamesByPlayerPage(c.Context(), walletAddress, limit, offset)
	if err != nil {
		h.logger.Error().Err(err).Str("wallet_address", walletAddress).Msg("Failed to retrieve game history")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error: "Failed to retrieve game history",
		})
	}

	h.logger.Info().
		Str("wallet_address", walletAddress).
		Int("count", len(games)).
		Msg("Game history retrieved")

	return c.JSON(GameHistoryResponse{
		WalletAddress: walletAddress,
		Games:         games,
		Limit:         limit,
		Offset:        offset,
		Total:         total,
	})
}

// GetReferralStats retrieves referral statistics for a user.
//
// @Summary Get referral statistics
// @Description Retrieve aggregated referral statistics including total referrals, total earnings, and games referred
// @Tags users
// @Accept json
// @Produce json
// @Param address path string true "TON wallet address (EQ...)" example("EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2")
// @Success 200 {object} entity.ReferralStats "Referral statistics"
// @Failure 400 {object} ErrorResponse "Invalid wallet address format"
// @Failure 404 {object} ErrorResponse "User not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /api/v1/users/{address}/referrals [get]
func (h *UserHandler) GetReferralStats(c *fiber.Ctx) error {
	walletAddress := c.Params("address")

	h.logger.Debug().Str("wallet_address", walletAddress).Msg("Get referral stats request")

	// Retrieve referral statistics (FR-021)
	stats, err := h.userUseCase.GetReferralStats(c.Context(), walletAddress)
	if err != nil {
		h.logger.Error().Err(err).Str("wallet_address", walletAddress).Msg("Failed to get referral stats")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error: "Failed to retrieve referral statistics",
		})
	}

	h.logger.Info().
		Str("wallet_address", walletAddress).
		Int64("total_referrals", stats.TotalReferrals).
		Int64("total_earnings", stats.TotalReferralEarnings).
		Msg("Referral stats retrieved")

	return c.JSON(stats)
}

// GameHistoryResponse represents the response for user game history.
type GameHistoryResponse struct {
	WalletAddress string         `json:"wallet_address"`
	Games         []*entity.Game `json:"games"`
	Limit         int            `json:"limit"`
	Offset        int            `json:"offset"`
	Total         int            `json:"total"`
}
