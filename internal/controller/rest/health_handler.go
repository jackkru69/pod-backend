package rest

import (
	"context"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/sony/gobreaker"
	"pod-backend/internal/entity"
	"pod-backend/internal/infrastructure/toncenter"
)

const (
	healthStatusHealthy         = "healthy"
	healthStatusDegraded        = "degraded"
	healthStatusUnhealthy       = "unhealthy"
	connectionConnected         = "connected"
	connectionDisconnected      = "disconnected"
	connectionNotConfigured     = "not_configured"
	eventSourceWebSocket        = "websocket"
	eventSourceHTTP             = "http"
	tonCenterRecovering         = "recovering"
	tonCenterCircuitOpen        = "circuit_breaker_open"
	parserRecoveryLive          = "live"
	parserRecoveryFallback      = "fallback_active"
	parserRecoveryRecovering    = "recovering"
	parserRecoveryStalled       = "stalled"
	parserRecoveryNotConfigured = "not_configured"
	healthCheckTimeout          = 2 * time.Second
)

// EventSourceProvider provides information about the current event source.
// T162: Interface for health check to report event source type.
type EventSourceProvider interface {
	GetSourceType() string
	IsConnected() bool
	GetLastProcessedLt() string
}

// SyncStateProvider provides the persisted blockchain checkpoint for health reporting.
type SyncStateProvider interface {
	Get(ctx context.Context) (*entity.BlockchainSyncState, error)
}

// HealthHandler handles health check requests
type HealthHandler struct {
	db                  *pgxpool.Pool
	logger              *zerolog.Logger
	tonCenterClient     *toncenter.Client
	eventSourceProvider EventSourceProvider
	syncStateProvider   SyncStateProvider
}

// NewHealthHandler creates a new HealthHandler
func NewHealthHandler(
	db *pgxpool.Pool,
	logger *zerolog.Logger,
	tonCenterClient *toncenter.Client,
	syncStateProvider SyncStateProvider,
) *HealthHandler {
	return &HealthHandler{
		db:                db,
		logger:            logger,
		tonCenterClient:   tonCenterClient,
		syncStateProvider: syncStateProvider,
	}
}

// SetEventSourceProvider sets the event source provider for health reporting.
// T162: Allows health check to report current event source type.
func (h *HealthHandler) SetEventSourceProvider(provider EventSourceProvider) {
	h.eventSourceProvider = provider
}

// GetHealth godoc
// @Summary Health check endpoint
// @Description Returns service health status including database connectivity, TON Center reachability, active event source, and parser checkpoint/recovery details
// @Tags health
// @Produce json
// @Success 200 {object} HealthResponse "Service is healthy"
// @Failure 503 {object} ErrorResponse "Service is unhealthy"
// @Router /api/v1/health [get]
func (h *HealthHandler) GetHealth(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.Context(), healthCheckTimeout)
	defer cancel()

	response := HealthResponse{
		Status:    healthStatusHealthy,
		Timestamp: time.Now(),
		Parser: ParserHealthResponse{
			Status:         healthStatusHealthy,
			RecoveryStatus: parserRecoveryLive,
		},
	}

	// Check database connection
	if h.db != nil {
		err := h.db.Ping(ctx)
		if err != nil {
			h.logger.Error().Err(err).Msg("Database health check failed")

			response.Status = healthStatusUnhealthy
			response.Database = connectionDisconnected

			return c.Status(fiber.StatusServiceUnavailable).JSON(response)
		}

		response.Database = connectionConnected
	} else {
		response.Database = connectionNotConfigured
	}

	// Check TON Center API circuit breaker state (T104, FR-019)
	if h.tonCenterClient != nil {
		cbState := h.tonCenterClient.GetCircuitBreakerState()
		switch cbState {
		case gobreaker.StateClosed:
			response.TonCenterAPI = connectionConnected
		case gobreaker.StateOpen:
			response.TonCenterAPI = tonCenterCircuitOpen
			response.Status = healthStatusDegraded
		case gobreaker.StateHalfOpen:
			response.TonCenterAPI = tonCenterRecovering
		}
	} else {
		response.TonCenterAPI = connectionNotConfigured
	}

	h.populateEventSourceHealth(&response)

	syncState := h.loadSyncState(ctx, &response)
	h.applySyncState(&response, syncState)
	h.applyLiveCheckpoint(&response)

	response.Parser.CurrentSourceType = response.EventSourceType
	response.Parser.Status, response.Parser.RecoveryStatus = deriveParserHealth(&response, syncState)
	response.Status = mergeHealthStatus(response.Status, response.Parser.Status)

	h.logger.Debug().
		Str("status", response.Status).
		Str("database", response.Database).
		Str("ton_center_api", response.TonCenterAPI).
		Str("event_source_type", response.EventSourceType).
		Str("event_source_status", response.EventSourceStatus).
		Str("parser_status", response.Parser.Status).
		Str("parser_recovery_status", response.Parser.RecoveryStatus).
		Str("last_processed_lt", response.Parser.LastProcessedLt).
		Msg("Health check completed")

	return c.JSON(response)
}

func (h *HealthHandler) populateEventSourceHealth(response *HealthResponse) {
	if h.eventSourceProvider == nil {
		response.EventSourceType = connectionNotConfigured
		response.EventSourceStatus = connectionNotConfigured

		return
	}

	response.EventSourceType = h.eventSourceProvider.GetSourceType()
	if h.eventSourceProvider.IsConnected() {
		response.EventSourceStatus = connectionConnected

		return
	}

	response.EventSourceStatus = connectionDisconnected
	if response.EventSourceType == eventSourceHTTP && response.Status == healthStatusHealthy {
		response.Status = healthStatusDegraded
	}
}

func (h *HealthHandler) loadSyncState(ctx context.Context, response *HealthResponse) *entity.BlockchainSyncState {
	if h.syncStateProvider == nil {
		return nil
	}

	state, err := h.syncStateProvider.Get(ctx)
	if err != nil {
		h.logger.Warn().Err(err).Msg("Failed to load blockchain sync state for health check")

		response.Parser.Status = healthStatusDegraded
		response.Parser.RecoveryStatus = parserRecoveryRecovering
		response.Status = mergeHealthStatus(response.Status, response.Parser.Status)

		return nil
	}

	return state
}

func (h *HealthHandler) applySyncState(response *HealthResponse, syncState *entity.BlockchainSyncState) {
	if syncState == nil {
		return
	}

	response.Parser.FallbackCount = syncState.FallbackCount
	response.Parser.LastFallbackAt = syncState.LastFallbackAt

	checkpointUpdatedAt := syncState.UpdatedAt.UTC()
	response.Parser.CheckpointUpdatedAt = &checkpointUpdatedAt

	if normalizedLt := normalizeLt(syncState.LastProcessedLt); normalizedLt != "" {
		response.Parser.LastProcessedLt = normalizedLt
	}

	if response.EventSourceType != connectionNotConfigured || strings.TrimSpace(syncState.EventSourceType) == "" {
		return
	}

	response.EventSourceType = syncState.EventSourceType
	if syncState.EventSourceType == eventSourceWebSocket {
		if syncState.WebSocketConnected {
			response.EventSourceStatus = connectionConnected
		} else {
			response.EventSourceStatus = connectionDisconnected
		}
	}
}

func (h *HealthHandler) applyLiveCheckpoint(response *HealthResponse) {
	if h.eventSourceProvider == nil {
		return
	}

	if normalizedLt := normalizeLt(h.eventSourceProvider.GetLastProcessedLt()); normalizedLt != "" {
		response.Parser.LastProcessedLt = normalizedLt
	}
}

func deriveParserHealth(response *HealthResponse, syncState *entity.BlockchainSyncState) (status string, recoveryStatus string) {
	switch {
	case isEventSourceStalled(response):
		return healthStatusUnhealthy, parserRecoveryStalled
	case isTonCenterRecovering(response):
		return healthStatusDegraded, parserRecoveryRecovering
	case isFallbackActive(response, syncState):
		return healthStatusDegraded, parserRecoveryFallback
	case response.EventSourceType == connectionNotConfigured || response.EventSourceStatus == connectionNotConfigured:
		return healthStatusDegraded, parserRecoveryNotConfigured
	default:
		return healthStatusHealthy, parserRecoveryLive
	}
}

func isEventSourceStalled(response *HealthResponse) bool {
	if response.EventSourceStatus != connectionDisconnected {
		return false
	}

	return response.EventSourceType == eventSourceWebSocket || response.EventSourceType == eventSourceHTTP
}

func isTonCenterRecovering(response *HealthResponse) bool {
	return response.TonCenterAPI == tonCenterCircuitOpen || response.TonCenterAPI == tonCenterRecovering
}

func isFallbackActive(response *HealthResponse, syncState *entity.BlockchainSyncState) bool {
	return response.EventSourceType == eventSourceHTTP && syncState != nil && syncState.FallbackCount > 0
}

func mergeHealthStatus(current, next string) string {
	if current == healthStatusUnhealthy || next == healthStatusUnhealthy {
		return healthStatusUnhealthy
	}

	if current == healthStatusDegraded || next == healthStatusDegraded {
		return healthStatusDegraded
	}

	return healthStatusHealthy
}

func normalizeLt(lt string) string {
	trimmed := strings.TrimSpace(lt)
	if trimmed == "" {
		return ""
	}

	return trimmed
}

// HealthResponse represents the health check response.
type HealthResponse struct {
	Status            string               `json:"status" enums:"healthy,degraded,unhealthy"`                                                 // Overall service status after merging database, TON Center, and parser state.
	Timestamp         time.Time            `json:"timestamp"`                                                                                 // Response timestamp in RFC3339 format.
	Database          string               `json:"database,omitempty" enums:"connected,disconnected,not_configured"`                          // PostgreSQL connectivity state.
	TonCenterAPI      string               `json:"ton_center_api,omitempty" enums:"connected,recovering,circuit_breaker_open,not_configured"` // TON Center client/circuit-breaker state.
	EventSourceType   string               `json:"event_source_type,omitempty" enums:"websocket,http,not_configured"`                         // Active blockchain ingestion source.
	EventSourceStatus string               `json:"event_source_status,omitempty" enums:"connected,disconnected,not_configured"`               // Connectivity state for the active event source.
	Parser            ParserHealthResponse `json:"parser"`                                                                                    // Parser checkpoint and recovery snapshot.
}

// ParserHealthResponse represents parser checkpoint and recovery health.
type ParserHealthResponse struct {
	Status              string     `json:"status" enums:"healthy,degraded,unhealthy"`                                                // Parser-specific health state.
	RecoveryStatus      string     `json:"recovery_status,omitempty" enums:"live,fallback_active,recovering,stalled,not_configured"` // Recovery mode derived from live and persisted sync state.
	LastProcessedLt     string     `json:"last_processed_lt,omitempty"`                                                              // Most recent successfully processed TON logical time.
	CheckpointUpdatedAt *time.Time `json:"checkpoint_updated_at,omitempty"`                                                          // Last persisted checkpoint update time in RFC3339 format.
	FallbackCount       int        `json:"fallback_count,omitempty"`                                                                 // Number of source fallback activations recorded in sync state.
	LastFallbackAt      *time.Time `json:"last_fallback_at,omitempty"`                                                               // Most recent fallback activation time in RFC3339 format.
	CurrentSourceType   string     `json:"current_source_type,omitempty" enums:"websocket,http,not_configured"`                      // Event source currently reflected by parser health.
}
