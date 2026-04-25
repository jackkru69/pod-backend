package rest

import (
	"errors"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
	"pod-backend/internal/entity"
	"pod-backend/internal/usecase"
)

const (
	defaultActivityResponseLimit = 20
	maxActivityResponseLimit     = 100
)

// GameActivityHandler exposes additive queue-oriented activity surfaces.
type GameActivityHandler struct {
	uc             *usecase.GameActivityUseCase
	expiredClaimUC *usecase.ExpiredClaimUseCase
	logger         *zerolog.Logger
}

// NewGameActivityHandler creates a new activity handler.
func NewGameActivityHandler(
	uc *usecase.GameActivityUseCase,
	expiredClaimUC *usecase.ExpiredClaimUseCase,
	logger *zerolog.Logger,
) *GameActivityHandler {
	return &GameActivityHandler{uc: uc, expiredClaimUC: expiredClaimUC, logger: logger}
}

// GameActivityQueueResponse is the public queue envelope returned by the
// additive activity surfaces.
type GameActivityQueueResponse struct {
	QueueKey          entity.ActivityQueueKey       `json:"queue_key"`
	Title             string                        `json:"title"`
	AttentionRequired bool                          `json:"attention_required"`
	Items             []*entity.PlayerActivityGame  `json:"items"`
	Summary           *entity.PlayerActivitySummary `json:"summary"`
	Total             int                           `json:"total"`
	Limit             int                           `json:"limit"`
	Offset            int                           `json:"offset"`
}

// SearchActivityPlaceholderResponse communicates that the additive search
// contract is scaffolded but intentionally deferred to the later user story.
type SearchActivityPlaceholderResponse struct {
	Items   []any  `json:"items"`
	Message string `json:"message"`
}

// GameActivitySearchResponse is the public additive search response.
type GameActivitySearchResponse struct {
	Query      string                        `json:"query"`
	QueueScope string                        `json:"queue_scope,omitempty"`
	Items      []*entity.PlayerActivityGame  `json:"items"`
	Summary    *entity.PlayerActivitySummary `json:"summary"`
	Total      int                           `json:"total"`
	Limit      int                           `json:"limit"`
	Offset     int                           `json:"offset"`
	Message    string                        `json:"message,omitempty"`
}

// ExpiredClaimRequest is the payload for create/release expired-follow-up claims.
type ExpiredClaimRequest struct {
	WalletAddress string `json:"wallet_address" validate:"required"`
}

// ExpiredClaimResponse is the public shape of an expired-follow-up claim.
type ExpiredClaimResponse struct {
	GameID        int64  `json:"game_id"`
	WalletAddress string `json:"wallet_address"`
	CreatedAt     string `json:"created_at"`
	ExpiresAt     string `json:"expires_at"`
	Status        string `json:"status"`
}

// ExpiredClaimEnvelope wraps a single expired-follow-up claim.
type ExpiredClaimEnvelope struct {
	Claim ExpiredClaimResponse `json:"claim"`
}

func toExpiredClaimResponse(claim *entity.ExpiredClaim) ExpiredClaimResponse {
	return ExpiredClaimResponse{
		GameID:        claim.GameID,
		WalletAddress: claim.WalletAddress,
		CreatedAt:     claim.CreatedAt.Format(time.RFC3339),
		ExpiresAt:     claim.ExpiresAt.Format(time.RFC3339),
		Status:        claim.Status,
	}
}

func parseActivityGameID(c *fiber.Ctx) (int64, error) {
	gameID, err := strconv.ParseInt(c.Params("gameId"), 10, 64)
	if err != nil {
		return 0, err
	}

	return gameID, nil
}

func parseExpiredClaimRequest(c *fiber.Ctx) (ExpiredClaimRequest, error) {
	var req ExpiredClaimRequest
	if err := c.BodyParser(&req); err != nil {
		return ExpiredClaimRequest{}, err
	}
	if req.WalletAddress == "" {
		return ExpiredClaimRequest{}, entity.ErrExpiredClaimWalletRequired
	}

	return req, nil
}

func createExpiredClaimErrorResponse(err error) (int, ErrorResponse) {
	switch {
	case errors.Is(err, entity.ErrExpiredClaimWalletRequired):
		return fiber.StatusBadRequest, ErrorResponse{Error: "bad_request", Message: err.Error()}
	case errors.Is(err, entity.ErrExpiredClaimAlreadyClaimed):
		return fiber.StatusConflict, ErrorResponse{Error: "expired_claim_already_claimed", Message: err.Error()}
	case errors.Is(err, entity.ErrTooManyExpiredClaims):
		return fiber.StatusConflict, ErrorResponse{Error: "too_many_expired_claims", Message: err.Error()}
	case errors.Is(err, entity.ErrNotExpiredClaimParticipant):
		return fiber.StatusForbidden, ErrorResponse{Error: "not_a_participant", Message: err.Error()}
	case errors.Is(err, entity.ErrExpiredClaimNotAvailable):
		return fiber.StatusNotFound, ErrorResponse{Error: "not_found", Message: err.Error()}
	default:
		return fiber.StatusInternalServerError, ErrorResponse{Error: "internal_error", Message: "Failed to create expired claim"}
	}
}

func releaseExpiredClaimErrorResponse(err error) (int, ErrorResponse) {
	switch {
	case errors.Is(err, entity.ErrExpiredClaimWalletRequired):
		return fiber.StatusBadRequest, ErrorResponse{Error: "bad_request", Message: err.Error()}
	case errors.Is(err, entity.ErrExpiredClaimNotFound):
		return fiber.StatusNotFound, ErrorResponse{Error: "not_found", Message: err.Error()}
	case errors.Is(err, entity.ErrNotExpiredClaimHolder):
		return fiber.StatusForbidden, ErrorResponse{Error: "forbidden", Message: err.Error()}
	default:
		return fiber.StatusInternalServerError, ErrorResponse{Error: "internal_error", Message: "Failed to release expired claim"}
	}
}

// ListQueue godoc
// @Summary      Get a player-facing activity queue
// @Description  Returns queue-oriented activity items for joinable, my-active, reveal-required, expired-attention, or history views.
// @Tags         games
// @Accept       json
// @Produce      json
// @Param        queueKey  path   string  true   "Queue key" Enums(joinable,my-active,reveal-required,expired-attention,history)
// @Param        wallet    query  string  false  "Optional wallet address used for player-relative queue classification"
// @Param        limit     query  int     false  "Maximum number of results" default(20) minimum(1) maximum(100)
// @Param        offset    query  int     false  "Number of results to skip" default(0) minimum(0)
// @Success      200  {object}  GameActivityQueueResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /api/v1/games/activity/{queueKey} [get]
func (h *GameActivityHandler) ListQueue(c *fiber.Ctx) error {
	queueKey := entity.ActivityQueueKey(c.Params("queueKey"))
	walletAddress := c.Query("wallet")
	limit := c.QueryInt("limit", 0)
	offset := c.QueryInt("offset", 0)

	queue, summary, err := h.uc.GetQueue(c.Context(), queueKey, walletAddress, limit, offset)
	if err != nil {
		h.logger.Warn().Err(err).Str("queue_key", string(queueKey)).Msg("Failed to load activity queue")
		status := fiber.StatusInternalServerError

		if errors.Is(err, entity.ErrInvalidActivityQueueKey) || errors.Is(err, entity.ErrInvalidActivityOffset) {
			status = fiber.StatusBadRequest
		}

		return c.Status(status).JSON(ErrorResponse{
			Error:   "bad_request",
			Message: err.Error(),
		})
	}

	responseLimit := limit
	if responseLimit <= 0 {
		responseLimit = defaultActivityResponseLimit
	}

	if responseLimit > maxActivityResponseLimit {
		responseLimit = maxActivityResponseLimit
	}

	return c.JSON(GameActivityQueueResponse{
		QueueKey:          queue.Key,
		Title:             queue.Title,
		AttentionRequired: queue.AttentionRequired,
		Items:             queue.Items,
		Summary:           summary,
		Total:             queue.TotalCount,
		Limit:             responseLimit,
		Offset:            offset,
	})
}

// SearchActivity godoc
// @Summary      Search activity surfaces
// @Description  Searches queue-oriented activity items by wallet, opponent, or game identifier.
// @Tags         games
// @Accept       json
// @Produce      json
// @Param        q        query  string  false  "Search term"
// @Param        wallet   query  string  false  "Optional wallet address"
// @Param        queue    query  string  false  "Optional queue scope"
// @Param        limit    query  int     false  "Maximum number of results" default(20) minimum(1) maximum(100)
// @Param        offset   query  int     false  "Number of results to skip" default(0) minimum(0)
// @Success      200  {object}  GameActivitySearchResponse
// @Failure      400  {object}  ErrorResponse
// @Router       /api/v1/games/activity/search [get]
func (h *GameActivityHandler) SearchActivity(c *fiber.Ctx) error {
	query := c.Query("q")
	walletAddress := c.Query("wallet")
	queueScope := entity.ActivityQueueKey(c.Query("queue"))
	limit := c.QueryInt("limit", 0)
	offset := c.QueryInt("offset", 0)

	items, summary, total, err := h.uc.Search(c.Context(), walletAddress, query, queueScope, limit, offset)
	if err != nil {
		h.logger.Warn().Err(err).Str("query", query).Str("queue_scope", string(queueScope)).Msg("Failed to search activity")
		status := fiber.StatusInternalServerError

		if errors.Is(err, entity.ErrInvalidActivityQueueKey) || errors.Is(err, entity.ErrInvalidActivityOffset) {
			status = fiber.StatusBadRequest
		}

		return c.Status(status).JSON(ErrorResponse{
			Error:   "bad_request",
			Message: err.Error(),
		})
	}

	responseLimit := limit
	if responseLimit <= 0 {
		responseLimit = defaultActivityResponseLimit
	}
	if responseLimit > maxActivityResponseLimit {
		responseLimit = maxActivityResponseLimit
	}

	message := ""
	if query == "" {
		message = "Enter a wallet, opponent, or game ID to search activity."
	} else if total == 0 {
		message = "No matching activity found."
	}

	response := GameActivitySearchResponse{
		Query:   query,
		Items:   items,
		Summary: summary,
		Total:   total,
		Limit:   responseLimit,
		Offset:  offset,
		Message: message,
	}
	if queueScope != "" {
		response.QueueScope = string(queueScope)
	}

	return c.JSON(response)
}

// GetUserActivitySummary godoc
// @Summary      Get queue summary for a wallet
// @Description  Returns the additive queue counts used by the activity shell for the specified wallet.
// @Tags         users
// @Accept       json
// @Produce      json
// @Param        address  path  string  true  "TON wallet address"
// @Success      200  {object}  entity.PlayerActivitySummary
// @Failure      400  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /api/v1/users/{address}/activity-summary [get]
func (h *GameActivityHandler) GetUserActivitySummary(c *fiber.Ctx) error {
	walletAddress := c.Params("address")
	summary, err := h.uc.GetSummary(c.Context(), walletAddress)
	if err != nil {
		h.logger.Warn().Err(err).Str("wallet", walletAddress).Msg("Failed to load activity summary")
		status := fiber.StatusInternalServerError

		if errors.Is(err, entity.ErrActivityWalletRequired) {
			status = fiber.StatusBadRequest
		}

		return c.Status(status).JSON(ErrorResponse{
			Error:   "bad_request",
			Message: err.Error(),
		})
	}

	return c.JSON(summary)
}

// CreateExpiredClaim godoc
// @Summary      Create expired-follow-up claim
// @Description  Creates or resumes an additive expired-follow-up claim for an ended game that still requires attention.
// @Tags         games
// @Accept       json
// @Produce      json
// @Param        gameId  path  int  true  "Game ID" minimum(1)
// @Param        request  body  ExpiredClaimRequest  true  "Expired claim request"
// @Success      201  {object}  ExpiredClaimEnvelope
// @Failure      400  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      409  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /api/v1/games/{gameId}/expired-claim [post]
func (h *GameActivityHandler) CreateExpiredClaim(c *fiber.Ctx) error {
	gameID, err := parseActivityGameID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid game ID format",
		})
	}

	req, err := parseExpiredClaimRequest(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "bad_request",
			Message: "wallet_address is required",
		})
	}

	claim, err := h.expiredClaimUC.Claim(c.Context(), gameID, req.WalletAddress)
	if err != nil {
		h.logger.Warn().Err(err).Int64("game_id", gameID).Str("wallet", req.WalletAddress).Msg("Failed to create expired claim")
		status, response := createExpiredClaimErrorResponse(err)
		return c.Status(status).JSON(response)
	}

	return c.Status(fiber.StatusCreated).JSON(ExpiredClaimEnvelope{Claim: toExpiredClaimResponse(claim)})
}

// GetExpiredClaim godoc
// @Summary      Get expired-follow-up claim
// @Description  Returns the current additive expired-follow-up claim for the specified game.
// @Tags         games
// @Accept       json
// @Produce      json
// @Param        gameId  path  int  true  "Game ID" minimum(1)
// @Success      200  {object}  ExpiredClaimEnvelope
// @Success      204  "No expired-follow-up claim exists"
// @Failure      400  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /api/v1/games/{gameId}/expired-claim [get]
func (h *GameActivityHandler) GetExpiredClaim(c *fiber.Ctx) error {
	gameID, err := parseActivityGameID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid game ID format",
		})
	}

	claim, err := h.expiredClaimUC.Get(c.Context(), gameID)
	if err != nil {
		h.logger.Error().Err(err).Int64("game_id", gameID).Msg("Failed to get expired claim")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to get expired claim",
		})
	}
	if claim == nil {
		return c.SendStatus(fiber.StatusNoContent)
	}

	return c.JSON(ExpiredClaimEnvelope{Claim: toExpiredClaimResponse(claim)})
}

// DeleteExpiredClaim godoc
// @Summary      Release expired-follow-up claim
// @Description  Releases the current expired-follow-up claim held by the requesting wallet.
// @Tags         games
// @Accept       json
// @Produce      json
// @Param        gameId  path  int  true  "Game ID" minimum(1)
// @Param        request  body  ExpiredClaimRequest  true  "Expired claim release request"
// @Success      204  "Claim released"
// @Failure      400  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /api/v1/games/{gameId}/expired-claim [delete]
func (h *GameActivityHandler) DeleteExpiredClaim(c *fiber.Ctx) error {
	gameID, err := parseActivityGameID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "bad_request",
			Message: "Invalid game ID format",
		})
	}

	req, err := parseExpiredClaimRequest(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "bad_request",
			Message: "wallet_address is required",
		})
	}

	err = h.expiredClaimUC.Release(c.Context(), gameID, req.WalletAddress)
	if err != nil {
		h.logger.Warn().Err(err).Int64("game_id", gameID).Str("wallet", req.WalletAddress).Msg("Failed to release expired claim")
		status, response := releaseExpiredClaimErrorResponse(err)
		return c.Status(status).JSON(response)
	}

	return c.SendStatus(fiber.StatusNoContent)
}
