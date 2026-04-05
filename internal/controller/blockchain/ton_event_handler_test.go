package blockchain

import (
	"testing"

	"pod-backend/config"
	"pod-backend/internal/entity"
	"pod-backend/internal/usecase"
	"pod-backend/pkg/logger"
)

func TestResolveInitialEventSourceType(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	cfg.GameBackend.BlockchainEventSource = "http"
	cfg.GameBackend.ResumeEventSource = true

	checkpoint := &entity.BlockchainSyncState{EventSourceType: "websocket"}
	if got := resolveInitialEventSourceType(cfg, checkpoint); got != "websocket" {
		t.Fatalf("resolveInitialEventSourceType() = %q, want %q", got, "websocket")
	}

	cfg.GameBackend.ResumeEventSource = false
	if got := resolveInitialEventSourceType(cfg, checkpoint); got != "http" {
		t.Fatalf("resolveInitialEventSourceType() with resume disabled = %q, want %q", got, "http")
	}
}

func TestNewTONEventHandlerResumesCheckpointLt(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	cfg.GameBackend.TONCenterV2URL = "http://localhost:8082"
	cfg.GameBackend.TONCenterV3WSURL = "ws://localhost:8081/api/v3/websocket"
	cfg.GameBackend.CircuitBreakerTimeout = "60s"
	cfg.GameBackend.WebSocketPingInterval = "30s"
	cfg.GameBackend.MinPollInterval = "5s"
	cfg.GameBackend.MaxPollInterval = "30s"
	cfg.GameBackend.BlockchainEventSource = "http"
	cfg.GameBackend.ResumeFromCheckpoint = true

	handler, err := NewTONEventHandler(
		cfg,
		usecase.NewGamePersistenceUseCase(nil, nil, nil),
		logger.New("error"),
		&entity.BlockchainSyncState{LastProcessedLt: "12345"},
	)
	if err != nil {
		t.Fatalf("NewTONEventHandler() error = %v", err)
	}

	if got := handler.GetLastProcessedLt(); got != "12345" {
		t.Fatalf("GetLastProcessedLt() = %q, want %q", got, "12345")
	}
}
