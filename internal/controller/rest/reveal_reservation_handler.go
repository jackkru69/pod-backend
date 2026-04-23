package rest

import (
	"strconv"

	"pod-backend/internal/entity"
	"pod-backend/internal/usecase"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
)

// RevealReservationHandler exposes the reveal-phase reservation endpoints
// (spec 005-reveal-reservation). Mirrors ReservationHandler but on a separate
// URL namespace so the join and reveal lifecycles never collide.
type RevealReservationHandler struct {
	uc     *usecase.RevealReservationUseCase
	logger *zerolog.Logger
}

// NewRevealReservationHandler creates a new handler.
func NewRevealReservationHandler(uc *usecase.RevealReservationUseCase, logger *zerolog.Logger) *RevealReservationHandler {
	return &RevealReservationHandler{uc: uc, logger: logger}
}

// RevealReserveRequest is the payload for single-game reveal reservation.
type RevealReserveRequest struct {
	WalletAddress string `json:"wallet_address" validate:"required"`
}

// RevealReserveBatchRequest is the payload for batched reveal reservation.
type RevealReserveBatchRequest struct {
	WalletAddress string  `json:"wallet_address" validate:"required"`
	GameIDs       []int64 `json:"game_ids"       validate:"required,min=1"`
}

// RevealReservationResponse is the public shape of a reveal reservation.
type RevealReservationResponse struct {
	GameID        int64  `json:"game_id"`
	WalletAddress string `json:"wallet_address"`
	CreatedAt     string `json:"created_at"`
	ExpiresAt     string `json:"expires_at"`
	Status        string `json:"status"`
}

// RevealReservationEnvelope wraps a single reveal reservation.
type RevealReservationEnvelope struct {
	Reservation RevealReservationResponse `json:"reservation"`
}

// RevealReservationListResponse wraps multiple reveal reservations.
type RevealReservationListResponse struct {
	Reservations []RevealReservationResponse `json:"reservations"`
}

func toRevealReservationResponse(r *entity.RevealReservation) RevealReservationResponse {
	return RevealReservationResponse{
		GameID:        r.GameID,
		WalletAddress: r.WalletAddress,
		CreatedAt:     r.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		ExpiresAt:     r.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
		Status:        r.Status,
	}
}

func revealReservationStatus(err error) (int, string) {
	switch err {
	case entity.ErrRevealAlreadyReserved:
		return fiber.StatusConflict, "reveal_already_reserved"
	case entity.ErrTooManyRevealReservations:
		return fiber.StatusConflict, "too_many_reveal_reservations"
	case entity.ErrRevealNotAvailable:
		return fiber.StatusNotFound, "reveal_not_available"
	case entity.ErrNotAPlayer:
		return fiber.StatusForbidden, "not_a_player"
	case entity.ErrRevealReservationNotFound:
		return fiber.StatusNotFound, "not_found"
	case entity.ErrNotRevealReservationHolder:
		return fiber.StatusForbidden, "not_reveal_reservation_holder"
	default:
		return fiber.StatusInternalServerError, "internal_error"
	}
}

// ReserveReveal godoc
// @Summary      Reserve the reveal slot for a game
// @Description  Off-chain advisory lock that prevents two concurrent callers from sending the same OpenBid for the same game.
// @Tags         reveal-reservations
// @Accept       json
// @Produce      json
// @Param        gameId   path  int                   true "Game ID" minimum(1)
// @Param        request  body  RevealReserveRequest  true "Reveal reservation request"
// @Success      201  {object}  RevealReservationEnvelope
// @Failure      400  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      409  {object}  ErrorResponse
// @Router       /api/v1/games/{gameId}/reveal-reserve [post]
//
//nolint:dupl // Mirrors the cancel-reservation handler by design to keep additive reservation-style APIs consistent.
func (h *RevealReservationHandler) ReserveReveal(c *fiber.Ctx) error {
	gameID, err := strconv.ParseInt(c.Params("gameId"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "bad_request", Message: "Invalid game ID format"})
	}

	var req RevealReserveRequest
	if err := c.BodyParser(&req); err != nil || req.WalletAddress == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "bad_request", Message: "wallet_address is required"})
	}

	r, err := h.uc.Reserve(c.Context(), gameID, req.WalletAddress)
	if err != nil {
		status, code := revealReservationStatus(err)
		h.logger.Warn().Err(err).Int64("gameId", gameID).Str("wallet", req.WalletAddress).Msg("Failed to reserve reveal")
		return c.Status(status).JSON(ErrorResponse{Error: code, Message: err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(RevealReservationEnvelope{Reservation: toRevealReservationResponse(r)})
}

// ReserveRevealBatch godoc
// @Summary      Reserve the reveal slot for a batch of games (atomic)
// @Description  All-or-nothing reservation. Useful for piggyback OpenBid that bundles several game IDs.
// @Tags         reveal-reservations
// @Accept       json
// @Produce      json
// @Param        request  body  RevealReserveBatchRequest  true "Batched reveal reservation request"
// @Success      201  {object}  RevealReservationListResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      409  {object}  ErrorResponse
// @Router       /api/v1/reveal-reserve [post]
func (h *RevealReservationHandler) ReserveRevealBatch(c *fiber.Ctx) error {
	var req RevealReserveBatchRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "bad_request", Message: "Invalid request body"})
	}
	if req.WalletAddress == "" || len(req.GameIDs) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "bad_request", Message: "wallet_address and game_ids are required"})
	}

	created, err := h.uc.ReserveBatch(c.Context(), req.GameIDs, req.WalletAddress)
	if err != nil {
		status, code := revealReservationStatus(err)
		h.logger.Warn().Err(err).Int("game_count", len(req.GameIDs)).Str("wallet", req.WalletAddress).Msg("Failed to reserve reveal batch")
		return c.Status(status).JSON(ErrorResponse{Error: code, Message: err.Error()})
	}

	resp := RevealReservationListResponse{Reservations: make([]RevealReservationResponse, 0, len(created))}
	for _, r := range created {
		resp.Reservations = append(resp.Reservations, toRevealReservationResponse(r))
	}
	return c.Status(fiber.StatusCreated).JSON(resp)
}

// GetRevealReservation godoc
// @Summary      Get current reveal reservation for a game
// @Tags         reveal-reservations
// @Produce      json
// @Param        gameId  path  int  true  "Game ID" minimum(1)
// @Success      200  {object}  RevealReservationEnvelope
// @Success      204  "No reveal reservation exists"
// @Failure      400  {object}  ErrorResponse
// @Router       /api/v1/games/{gameId}/reveal-reservation [get]
//
//nolint:dupl // Mirrors the cancel-reservation read/list/cancel endpoints to preserve the shared API shape.
func (h *RevealReservationHandler) GetRevealReservation(c *fiber.Ctx) error {
	gameID, err := strconv.ParseInt(c.Params("gameId"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "bad_request", Message: "Invalid game ID format"})
	}
	r, err := h.uc.Get(c.Context(), gameID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: "internal_error", Message: "Failed to get reveal reservation"})
	}
	if r == nil {
		return c.SendStatus(fiber.StatusNoContent)
	}
	return c.JSON(RevealReservationEnvelope{Reservation: toRevealReservationResponse(r)})
}

// ListRevealReservationsByWallet godoc
// @Summary      List active reveal reservations for a wallet
// @Tags         reveal-reservations
// @Produce      json
// @Param        wallet  query  string  true  "Wallet address"
// @Success      200  {object}  RevealReservationListResponse
// @Failure      400  {object}  ErrorResponse
// @Router       /api/v1/reveal-reservations [get]
func (h *RevealReservationHandler) ListRevealReservationsByWallet(c *fiber.Ctx) error {
	wallet := c.Query("wallet")
	if wallet == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "bad_request", Message: "wallet query parameter is required"})
	}
	list, err := h.uc.ListByWallet(c.Context(), wallet)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: "internal_error", Message: "Failed to list reveal reservations"})
	}
	resp := RevealReservationListResponse{Reservations: make([]RevealReservationResponse, 0, len(list))}
	for _, r := range list {
		resp.Reservations = append(resp.Reservations, toRevealReservationResponse(r))
	}
	return c.JSON(resp)
}

// CancelRevealReservation godoc
// @Summary      Cancel a reveal reservation (holder-only)
// @Tags         reveal-reservations
// @Accept       json
// @Param        gameId   path  int                   true "Game ID" minimum(1)
// @Param        request  body  RevealReserveRequest  true "Cancellation request with wallet address"
// @Success      204  "Cancelled"
// @Failure      400  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /api/v1/games/{gameId}/reveal-reserve [delete]
func (h *RevealReservationHandler) CancelRevealReservation(c *fiber.Ctx) error {
	gameID, err := strconv.ParseInt(c.Params("gameId"), 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "bad_request", Message: "Invalid game ID format"})
	}
	var req RevealReserveRequest
	if err := c.BodyParser(&req); err != nil || req.WalletAddress == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "bad_request", Message: "wallet_address is required"})
	}
	if err := h.uc.Cancel(c.Context(), gameID, req.WalletAddress); err != nil {
		status, code := revealReservationStatus(err)
		return c.Status(status).JSON(ErrorResponse{Error: code, Message: err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}
