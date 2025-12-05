package rest

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"

	"pod-backend/internal/entity"
	"pod-backend/internal/usecase"
)

// ReservationHandler handles HTTP requests for game reservation endpoints
type ReservationHandler struct {
	reservationUC *usecase.ReservationUseCase
	logger        *zerolog.Logger
}

// NewReservationHandler creates a new ReservationHandler
func NewReservationHandler(reservationUC *usecase.ReservationUseCase, logger *zerolog.Logger) *ReservationHandler {
	return &ReservationHandler{
		reservationUC: reservationUC,
		logger:        logger,
	}
}

// ReserveRequest represents the request body for reserving a game
type ReserveRequest struct {
	WalletAddress string `json:"wallet_address" validate:"required"`
}

// ReservationResponse represents a single reservation in the response
type ReservationResponse struct {
	GameID        int64  `json:"game_id"`
	WalletAddress string `json:"wallet_address"`
	CreatedAt     string `json:"created_at"`
	ExpiresAt     string `json:"expires_at"`
	Status        string `json:"status"`
}

// ReserveGameResponse represents the response for reserve game endpoint
type ReserveGameResponse struct {
	Reservation ReservationResponse `json:"reservation"`
}

// ListReservationsResponse represents the response for list reservations endpoint
type ListReservationsResponse struct {
	Reservations []ReservationResponse `json:"reservations"`
}

// toReservationResponse converts entity to response
func toReservationResponse(r *entity.GameReservation) ReservationResponse {
	return ReservationResponse{
		GameID:        r.GameID,
		WalletAddress: r.WalletAddress,
		CreatedAt:     r.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		ExpiresAt:     r.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
		Status:        r.Status,
	}
}

// ReserveGame godoc
// @Summary Reserve a game
// @Description Reserve a game for a specific wallet address to prevent other players from joining
// @Tags reservations
// @Accept json
// @Produce json
// @Param gameId path int true "Game ID" minimum(1)
// @Param request body ReserveRequest true "Reservation request"
// @Success 201 {object} ReserveGameResponse "Reservation created"
// @Failure 400 {object} ErrorResponse "Invalid request"
// @Failure 404 {object} ErrorResponse "Game not found"
// @Failure 409 {object} ErrorResponse "Game already reserved or conflict"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /api/v1/games/{gameId}/reserve [post]
func (h *ReservationHandler) ReserveGame(c *fiber.Ctx) error {
	// Parse game ID from path
	gameIDStr := c.Params("gameId")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		h.logger.Warn().
			Str("gameId", gameIDStr).
			Msg("Invalid game ID format")
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid game ID format",
		})
	}

	// Parse request body
	var req ReserveRequest
	if err := c.BodyParser(&req); err != nil {
		h.logger.Warn().
			Err(err).
			Msg("Failed to parse request body")
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid request body",
		})
	}

	// Validate wallet address
	if req.WalletAddress == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "bad_request",
			Message: "wallet_address is required",
		})
	}

	// Log request
	h.logger.Info().
		Int64("gameId", gameID).
		Str("wallet", req.WalletAddress).
		Msg("ReserveGame request")

	// Call use case
	reservation, err := h.reservationUC.Reserve(c.Context(), gameID, req.WalletAddress)
	if err != nil {
		h.logger.Warn().
			Err(err).
			Int64("gameId", gameID).
			Str("wallet", req.WalletAddress).
			Msg("Failed to reserve game")

		switch err {
		case entity.ErrGameAlreadyReserved:
			return c.Status(fiber.StatusConflict).JSON(ErrorResponse{
				Error:   "conflict",
				Message: err.Error(),
			})
		case entity.ErrTooManyReservations:
			return c.Status(fiber.StatusConflict).JSON(ErrorResponse{
				Error:   "too_many_reservations",
				Message: err.Error(),
			})
		case entity.ErrCannotReserveOwnGame:
			return c.Status(fiber.StatusForbidden).JSON(ErrorResponse{
				Error:   "forbidden",
				Message: err.Error(),
			})
		case entity.ErrGameNotAvailable:
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error:   "not_found",
				Message: err.Error(),
			})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
				Error:   "internal_error",
				Message: "Failed to reserve game",
			})
		}
	}

	// Log success
	h.logger.Info().
		Int64("gameId", gameID).
		Str("wallet", req.WalletAddress).
		Time("expiresAt", reservation.ExpiresAt).
		Msg("Game reserved successfully")

	return c.Status(fiber.StatusCreated).JSON(ReserveGameResponse{
		Reservation: toReservationResponse(reservation),
	})
}

// GetReservation godoc
// @Summary Get game reservation
// @Description Get the current reservation status for a specific game
// @Tags reservations
// @Accept json
// @Produce json
// @Param gameId path int true "Game ID" minimum(1)
// @Success 200 {object} ReserveGameResponse "Reservation details"
// @Success 204 "No reservation exists for this game"
// @Failure 400 {object} ErrorResponse "Invalid game ID"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /api/v1/games/{gameId}/reservation [get]
func (h *ReservationHandler) GetReservation(c *fiber.Ctx) error {
	// Parse game ID from path
	gameIDStr := c.Params("gameId")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		h.logger.Warn().
			Str("gameId", gameIDStr).
			Msg("Invalid game ID format")
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid game ID format",
		})
	}

	// Log request
	h.logger.Debug().
		Int64("gameId", gameID).
		Msg("GetReservation request")

	// Call use case
	reservation, err := h.reservationUC.GetReservation(c.Context(), gameID)
	if err != nil {
		h.logger.Error().
			Err(err).
			Int64("gameId", gameID).
			Msg("Failed to get reservation")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to get reservation",
		})
	}

	// No reservation exists
	if reservation == nil {
		return c.SendStatus(fiber.StatusNoContent)
	}

	return c.JSON(ReserveGameResponse{
		Reservation: toReservationResponse(reservation),
	})
}

// ListReservationsByWallet godoc
// @Summary List reservations by wallet
// @Description Get all active reservations for a specific wallet address
// @Tags reservations
// @Accept json
// @Produce json
// @Param wallet query string true "Wallet address"
// @Success 200 {object} ListReservationsResponse "List of reservations"
// @Failure 400 {object} ErrorResponse "Invalid wallet address"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /api/v1/reservations [get]
func (h *ReservationHandler) ListReservationsByWallet(c *fiber.Ctx) error {
	// Get wallet from query parameter
	wallet := c.Query("wallet")
	if wallet == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "bad_request",
			Message: "wallet query parameter is required",
		})
	}

	// Log request
	h.logger.Debug().
		Str("wallet", wallet).
		Msg("ListReservationsByWallet request")

	// Call use case
	reservations, err := h.reservationUC.ListByWallet(c.Context(), wallet)
	if err != nil {
		h.logger.Error().
			Err(err).
			Str("wallet", wallet).
			Msg("Failed to list reservations")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to list reservations",
		})
	}

	// Convert to response
	result := make([]ReservationResponse, len(reservations))
	for i, r := range reservations {
		result[i] = toReservationResponse(r)
	}

	return c.JSON(ListReservationsResponse{
		Reservations: result,
	})
}

// CancelReservation godoc
// @Summary Cancel a game reservation
// @Description Cancel an existing reservation for a game. Only the reservation holder can cancel.
// @Tags reservations
// @Accept json
// @Produce json
// @Param gameId path int true "Game ID" minimum(1)
// @Param request body ReserveRequest true "Cancellation request with wallet address"
// @Success 204 "Reservation cancelled"
// @Failure 400 {object} ErrorResponse "Invalid request"
// @Failure 403 {object} ErrorResponse "Not the reservation holder"
// @Failure 404 {object} ErrorResponse "Reservation not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /api/v1/games/{gameId}/reserve [delete]
func (h *ReservationHandler) CancelReservation(c *fiber.Ctx) error {
	// Parse game ID from path
	gameIDStr := c.Params("gameId")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		h.logger.Warn().
			Str("gameId", gameIDStr).
			Msg("Invalid game ID format")
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid game ID format",
		})
	}

	// Parse request body
	var req ReserveRequest
	if err := c.BodyParser(&req); err != nil {
		h.logger.Warn().
			Err(err).
			Msg("Failed to parse request body")
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid request body",
		})
	}

	// Validate wallet address
	if req.WalletAddress == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "bad_request",
			Message: "wallet_address is required",
		})
	}

	// Log request
	h.logger.Info().
		Int64("gameId", gameID).
		Str("wallet", req.WalletAddress).
		Msg("CancelReservation request")

	// Call use case
	err = h.reservationUC.Cancel(c.Context(), gameID, req.WalletAddress)
	if err != nil {
		h.logger.Warn().
			Err(err).
			Int64("gameId", gameID).
			Str("wallet", req.WalletAddress).
			Msg("Failed to cancel reservation")

		switch err {
		case entity.ErrReservationNotFound:
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error:   "not_found",
				Message: err.Error(),
			})
		case entity.ErrNotReservationHolder:
			return c.Status(fiber.StatusForbidden).JSON(ErrorResponse{
				Error:   "forbidden",
				Message: err.Error(),
			})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
				Error:   "internal_error",
				Message: "Failed to cancel reservation",
			})
		}
	}

	// Log success
	h.logger.Info().
		Int64("gameId", gameID).
		Str("wallet", req.WalletAddress).
		Msg("Reservation cancelled successfully")

	return c.SendStatus(fiber.StatusNoContent)
}
