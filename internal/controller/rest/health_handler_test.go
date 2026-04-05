package rest

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pod-backend/internal/entity"
)

type testEventSourceProvider struct {
	sourceType      string
	connected       bool
	lastProcessedLt string
}

func (p testEventSourceProvider) GetSourceType() string {
	return p.sourceType
}

func (p testEventSourceProvider) IsConnected() bool {
	return p.connected
}

func (p testEventSourceProvider) GetLastProcessedLt() string {
	return p.lastProcessedLt
}

type testSyncStateProvider struct {
	state *entity.BlockchainSyncState
	err   error
}

func (p testSyncStateProvider) Get(context.Context) (*entity.BlockchainSyncState, error) {
	return p.state, p.err
}

func TestHealthHandler_GetHealth_FallbackActive(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	fallbackAt := time.Date(2026, 4, 5, 1, 0, 0, 0, time.UTC)
	checkpointUpdatedAt := time.Date(2026, 4, 5, 1, 2, 0, 0, time.UTC)

	handler := NewHealthHandler(nil, &logger, nil, testSyncStateProvider{
		state: &entity.BlockchainSyncState{
			EventSourceType: "http",
			LastProcessedLt: "101",
			FallbackCount:   2,
			LastFallbackAt:  &fallbackAt,
			UpdatedAt:       checkpointUpdatedAt,
		},
	})
	handler.SetEventSourceProvider(testEventSourceProvider{
		sourceType:      "http",
		connected:       true,
		lastProcessedLt: "105",
	})

	app := fiber.New()
	app.Get("/health", handler.GetHealth)

	req := httptest.NewRequest(http.MethodGet, "/health", http.NoBody)
	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var payload HealthResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))

	assert.Equal(t, "degraded", payload.Status)
	assert.Equal(t, "http", payload.EventSourceType)
	assert.Equal(t, "connected", payload.EventSourceStatus)
	assert.Equal(t, "degraded", payload.Parser.Status)
	assert.Equal(t, "fallback_active", payload.Parser.RecoveryStatus)
	assert.Equal(t, "105", payload.Parser.LastProcessedLt)
	assert.Equal(t, 2, payload.Parser.FallbackCount)
	require.NotNil(t, payload.Parser.LastFallbackAt)
	assert.True(t, payload.Parser.LastFallbackAt.Equal(fallbackAt))
	require.NotNil(t, payload.Parser.CheckpointUpdatedAt)
	assert.True(t, payload.Parser.CheckpointUpdatedAt.Equal(checkpointUpdatedAt))
}

func TestHealthHandler_GetHealth_WebsocketDisconnected(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	handler := NewHealthHandler(nil, &logger, nil, testSyncStateProvider{
		state: &entity.BlockchainSyncState{
			EventSourceType: "websocket",
			LastProcessedLt: "200",
			UpdatedAt:       time.Date(2026, 4, 5, 1, 4, 0, 0, time.UTC),
		},
	})
	handler.SetEventSourceProvider(testEventSourceProvider{
		sourceType:      "websocket",
		connected:       false,
		lastProcessedLt: "200",
	})

	app := fiber.New()
	app.Get("/health", handler.GetHealth)

	req := httptest.NewRequest(http.MethodGet, "/health", http.NoBody)
	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var payload HealthResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))

	assert.Equal(t, "unhealthy", payload.Status)
	assert.Equal(t, "unhealthy", payload.Parser.Status)
	assert.Equal(t, "stalled", payload.Parser.RecoveryStatus)
	assert.Equal(t, "websocket", payload.EventSourceType)
	assert.Equal(t, "disconnected", payload.EventSourceStatus)
}
