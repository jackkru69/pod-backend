package rest

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"

	"pod-backend/internal/entity"
	"pod-backend/internal/usecase"
)

// GameHandler handles HTTP requests for game endpoints
type GameHandler struct {
	gameQueryUC *usecase.GameQueryUseCase
	logger      *zerolog.Logger
}

// NewGameHandler creates a new GameHandler
func NewGameHandler(gameQueryUC *usecase.GameQueryUseCase, logger *zerolog.Logger) *GameHandler {
	return &GameHandler{
		gameQueryUC: gameQueryUC,
		logger:      logger,
	}
}

// ListGames godoc
// @Summary Get available games
// @Description Retrieve list of games filtered by status with pagination support
// @Tags games
// @Accept json
// @Produce json
// @Param status query int false "Filter by game status (0-4)" default(1) Enums(0, 1, 2, 3, 4)
// @Param limit query int false "Maximum number of results" default(20) minimum(1) maximum(100)
// @Param offset query int false "Number of results to skip" default(0) minimum(0)
// @Success 200 {object} GameListResponse "List of games"
// @Failure 400 {object} ErrorResponse "Invalid request parameters"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /api/v1/games [get]
func (h *GameHandler) ListGames(c *fiber.Ctx) error {
	// Parse query parameters with defaults
	status := c.QueryInt("status", entity.GameStatusWaitingForOpponent)
	limit := c.QueryInt("limit", 20)
	offset := c.QueryInt("offset", 0)

	// Log request
	h.logger.Info().
		Int("status", status).
		Int("limit", limit).
		Int("offset", offset).
		Msg("ListGames request")

	// Call use case
	games, err := h.gameQueryUC.ListGames(c.Context(), status, limit, offset)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to list games")
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "bad_request",
			Message: err.Error(),
		})
	}

	// Count total (for now, return length of results - in production this would be a separate count query)
	total := len(games)

	// Log success
	h.logger.Debug().
		Int("total", total).
		Int("returned", len(games)).
		Msg("ListGames successful")

	return c.JSON(GameListResponse{
		Games:  games,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	})
}

// GetGameByID godoc
// @Summary Get game details
// @Description Retrieve detailed information for a specific game by ID
// @Tags games
// @Accept json
// @Produce json
// @Param gameId path int true "Game ID" minimum(1)
// @Success 200 {object} entity.Game "Game details"
// @Failure 400 {object} ErrorResponse "Invalid game ID"
// @Failure 404 {object} ErrorResponse "Game not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /api/v1/games/{gameId} [get]
func (h *GameHandler) GetGameByID(c *fiber.Ctx) error {
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
	h.logger.Info().
		Int64("gameId", gameID).
		Msg("GetGameByID request")

	// Call use case
	game, err := h.gameQueryUC.GetGameByID(c.Context(), gameID)
	if err != nil {
		// Check if game not found or other error
		if err.Error() == "game not found" {
			h.logger.Debug().
				Int64("gameId", gameID).
				Msg("Game not found")
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error:   "not_found",
				Message: "Game not found",
			})
		}

		h.logger.Error().
			Err(err).
			Int64("gameId", gameID).
			Msg("Failed to get game")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "internal_error",
			Message: "Failed to retrieve game",
		})
	}

	// Log success
	h.logger.Debug().
		Int64("gameId", gameID).
		Msg("GetGameByID successful")

	return c.JSON(game)
}

// GameListResponse represents the response for list games endpoint
type GameListResponse struct {
	Games  []*entity.Game `json:"games"`
	Total  int            `json:"total"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string                 `json:"error"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}
