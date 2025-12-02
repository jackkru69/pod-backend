package rest

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/sony/gobreaker"

	"pod-backend/internal/infrastructure/toncenter"
)

// HealthHandler handles health check requests
type HealthHandler struct {
	db              *pgxpool.Pool
	logger          *zerolog.Logger
	tonCenterClient *toncenter.Client
}

// NewHealthHandler creates a new HealthHandler
func NewHealthHandler(db *pgxpool.Pool, logger *zerolog.Logger, tonCenterClient *toncenter.Client) *HealthHandler {
	return &HealthHandler{
		db:              db,
		logger:          logger,
		tonCenterClient: tonCenterClient,
	}
}

// GetHealth godoc
// @Summary Health check endpoint
// @Description Returns service health status including database connectivity
// @Tags health
// @Produce json
// @Success 200 {object} HealthResponse "Service is healthy"
// @Failure 503 {object} ErrorResponse "Service is unhealthy"
// @Router /api/v1/health [get]
func (h *HealthHandler) GetHealth(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.Context(), 2*time.Second)
	defer cancel()

	response := HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now(),
	}

	// Check database connection
	if h.db != nil {
		err := h.db.Ping(ctx)
		if err != nil {
			h.logger.Error().Err(err).Msg("Database health check failed")
			response.Status = "unhealthy"
			response.Database = "disconnected"
			return c.Status(fiber.StatusServiceUnavailable).JSON(response)
		}
		response.Database = "connected"
	} else {
		response.Database = "not_configured"
	}

	// Check TON Center API circuit breaker state (T104, FR-019)
	if h.tonCenterClient != nil {
		cbState := h.tonCenterClient.GetCircuitBreakerState()
		switch cbState {
		case gobreaker.StateClosed:
			response.TonCenterAPI = "connected"
		case gobreaker.StateOpen:
			response.TonCenterAPI = "circuit_breaker_open"
			response.Status = "degraded" // Service is partially available
		case gobreaker.StateHalfOpen:
			response.TonCenterAPI = "recovering"
		}
	} else {
		response.TonCenterAPI = "not_configured"
	}

	h.logger.Debug().
		Str("status", response.Status).
		Str("database", response.Database).
		Str("ton_center_api", response.TonCenterAPI).
		Msg("Health check completed")

	return c.JSON(response)
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status       string    `json:"status" enums:"healthy,degraded,unhealthy"`
	Timestamp    time.Time `json:"timestamp"`
	Database     string    `json:"database,omitempty" enums:"connected,disconnected,not_configured"`
	TonCenterAPI string    `json:"ton_center_api,omitempty" enums:"connected,recovering,circuit_breaker_open,not_configured"`
}
