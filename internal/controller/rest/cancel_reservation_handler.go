package rest

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
	"pod-backend/internal/entity"
	"pod-backend/internal/usecase"
)

// CancelReservationHandler exposes creator-only cancel-coordination endpoints.
type CancelReservationHandler struct {
	uc     *usecase.CancelReservationUseCase
	logger *zerolog.Logger
}

// NewCancelReservationHandler creates a new cancel-reservation handler.
func NewCancelReservationHandler(uc *usecase.CancelReservationUseCase, logger *zerolog.Logger) *CancelReservationHandler {
	return &CancelReservationHandler{uc: uc, logger: logger}
}

// CancelReserveRequest is the payload for single-game cancel reservation.
type CancelReserveRequest struct {
	WalletAddress string `json:"wallet_address" validate:"required"`
}

// CancelReservationResponse is the public shape of a cancel reservation.
type CancelReservationResponse struct {
	GameID        int64  `json:"game_id"`
	WalletAddress string `json:"wallet_address"`
	CreatedAt     string `json:"created_at"`
	ExpiresAt     string `json:"expires_at"`
	Status        string `json:"status"`
}

// CancelReservationEnvelope wraps a single cancel reservation.
type CancelReservationEnvelope struct {
	Reservation CancelReservationResponse `json:"reservation"`
}

// CancelReservationListResponse wraps multiple cancel reservations.
type CancelReservationListResponse struct {
	Reservations []CancelReservationResponse `json:"reservations"`
}

func toCancelReservationResponse(r *entity.CancelReservation) CancelReservationResponse {
	return CancelReservationResponse{
		GameID:        r.GameID,
		WalletAddress: r.WalletAddress,
		CreatedAt:     r.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		ExpiresAt:     r.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
		Status:        r.Status,
	}
}

func cancelReservationStatus(err error) (int, string) {
	switch err {
	case entity.ErrCancelAlreadyReserved:
		return fiber.StatusConflict, "cancel_already_reserved"
	case entity.ErrTooManyCancelReservations:
		return fiber.StatusConflict, "too_many_cancel_reservations"
	case entity.ErrCancelNotAvailable:
		return fiber.StatusNotFound, "cancel_not_available"
	case entity.ErrNotGameCreator:
		return fiber.StatusForbidden, "not_game_creator"
	case entity.ErrCancelReservationNotFound:
		return fiber.StatusNotFound, "not_found"
	case entity.ErrNotCancelReservationHolder:
		return fiber.StatusForbidden, "not_cancel_reservation_holder"
	case entity.ErrGameAlreadyReserved:
		return fiber.StatusConflict, "game_already_reserved"
	default:
		return fiber.StatusInternalServerError, "internal_error"
	}
}

// ReserveCancel godoc
// @Summary      Reserve cancel coordination for a waiting game
// @Description  Advisory creator-only lock that blocks conflicting join attempts while the creator confirms a cancel transaction.
// @Tags         cancel-reservations
// @Accept       json
// @Produce      json
// @Param        gameId   path  int                   true "Game ID" minimum(1)
// @Param        request  body  CancelReserveRequest  true "Cancel reservation request"
// @Success      201  {object}  CancelReservationEnvelope
// @Failure      400  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      409  {object}  ErrorResponse
// @Router       /api/v1/games/{gameId}/cancel-reserve [post]
//
//nolint:dupl // Mirrors the established reveal-reservation handler flow with cancel-specific errors and docs.
func (h *CancelReservationHandler) ReserveCancel(c *fiber.Ctx) error {
	gameID, err := strconv.ParseInt(c.Params("gameId"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "bad_request", Message: "Invalid game ID format"})
	}

	var req CancelReserveRequest
	if err := c.BodyParser(&req); err != nil || req.WalletAddress == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "bad_request", Message: "wallet_address is required"})
	}

	reservation, err := h.uc.Reserve(c.Context(), gameID, req.WalletAddress)
	if err != nil {
		status, code := cancelReservationStatus(err)
		h.logger.Warn().Err(err).Int64("gameId", gameID).Str("wallet", req.WalletAddress).Msg("Failed to reserve cancel")
		return c.Status(status).JSON(ErrorResponse{Error: code, Message: err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(CancelReservationEnvelope{Reservation: toCancelReservationResponse(reservation)})
}

// GetCancelReservation godoc
// @Summary      Get current cancel reservation for a game
// @Tags         cancel-reservations
// @Produce      json
// @Param        gameId  path  int  true  "Game ID" minimum(1)
// @Success      200  {object}  CancelReservationEnvelope
// @Success      204  "No cancel reservation exists"
// @Failure      400  {object}  ErrorResponse
// @Router       /api/v1/games/{gameId}/cancel-reservation [get]
//
//nolint:dupl // Mirrors the reveal-reservation read/list/cancel endpoints to preserve the additive public contract shape.
func (h *CancelReservationHandler) GetCancelReservation(c *fiber.Ctx) error {
	gameID, err := strconv.ParseInt(c.Params("gameId"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "bad_request", Message: "Invalid game ID format"})
	}

	reservation, err := h.uc.Get(c.Context(), gameID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: "internal_error", Message: "Failed to get cancel reservation"})
	}
	if reservation == nil {
		return c.SendStatus(fiber.StatusNoContent)
	}

	return c.JSON(CancelReservationEnvelope{Reservation: toCancelReservationResponse(reservation)})
}

// ListCancelReservationsByWallet godoc
// @Summary      List active cancel reservations for a wallet
// @Tags         cancel-reservations
// @Produce      json
// @Param        wallet  query  string  true  "Wallet address"
// @Success      200  {object}  CancelReservationListResponse
// @Failure      400  {object}  ErrorResponse
// @Router       /api/v1/cancel-reservations [get]
func (h *CancelReservationHandler) ListCancelReservationsByWallet(c *fiber.Ctx) error {
	wallet := c.Query("wallet")
	if wallet == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "bad_request", Message: "wallet query parameter is required"})
	}

	list, err := h.uc.ListByWallet(c.Context(), wallet)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: "internal_error", Message: "Failed to list cancel reservations"})
	}

	response := CancelReservationListResponse{Reservations: make([]CancelReservationResponse, 0, len(list))}
	for _, reservation := range list {
		response.Reservations = append(response.Reservations, toCancelReservationResponse(reservation))
	}
	return c.JSON(response)
}

// CancelCancelReservation godoc
// @Summary      Cancel a cancel reservation (holder-only)
// @Tags         cancel-reservations
// @Accept       json
// @Param        gameId   path  int                   true "Game ID" minimum(1)
// @Param        request  body  CancelReserveRequest  true "Cancellation request with wallet address"
// @Success      204  "Cancelled"
// @Failure      400  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /api/v1/games/{gameId}/cancel-reserve [delete]
func (h *CancelReservationHandler) CancelCancelReservation(c *fiber.Ctx) error {
	gameID, err := strconv.ParseInt(c.Params("gameId"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "bad_request", Message: "Invalid game ID format"})
	}

	var req CancelReserveRequest
	if err := c.BodyParser(&req); err != nil || req.WalletAddress == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "bad_request", Message: "wallet_address is required"})
	}

	if err := h.uc.Cancel(c.Context(), gameID, req.WalletAddress); err != nil {
		status, code := cancelReservationStatus(err)
		return c.Status(status).JSON(ErrorResponse{Error: code, Message: err.Error()})
	}

	return c.SendStatus(fiber.StatusNoContent)
}
