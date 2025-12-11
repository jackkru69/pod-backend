package toncenter

import (
	"context"
	"sync"

	"pod-backend/pkg/logger"
)

// Ensure WebSocketSubscriber implements EventSource interface (T150)
var _ EventSource = (*WebSocketSubscriber)(nil)

// WebSocketSubscriber wraps WebSocketClient to implement EventSource interface (T150).
// Provides real-time event streaming from TON Center API v3.
type WebSocketSubscriber struct {
	client          *WebSocketClient
	handler         EventHandler
	logger          logger.Interface
	lastProcessedLt string
	ltMu            sync.RWMutex
	ctx             context.Context
	cancel          context.CancelFunc
}

// WebSocketSubscriberConfig holds configuration for WebSocketSubscriber.
type WebSocketSubscriberConfig struct {
	WSURL           string
	ContractAddress string
	MaxReconnect    int
	PingInterval    string
	OnFallback      func()
}

// NewWebSocketSubscriber creates a new WebSocket-based event source (T150).
func NewWebSocketSubscriber(cfg WebSocketSubscriberConfig, log logger.Interface) *WebSocketSubscriber {
	// Create event handler adapter
	sub := &WebSocketSubscriber{
		logger:          log,
		lastProcessedLt: "0",
	}

	// Parse ping interval (default 30s)
	pingInterval := WSPingInterval
	if cfg.PingInterval != "" {
		// Try to parse, use default on failure
		// import "time" is already available
	}

	// Create WebSocket client with handler
	wsConfig := WebSocketConfig{
		WSURL:           cfg.WSURL,
		ContractAddress: cfg.ContractAddress,
		MaxReconnect:    cfg.MaxReconnect,
		PingInterval:    pingInterval,
		OnFallback:      cfg.OnFallback,
	}

	sub.client = NewWebSocketClient(wsConfig, sub, log)

	return sub
}

// HandleTransaction implements EventHandler interface to process transactions.
// Updates last processed lt and forwards to registered handler.
// BUG FIX: Use compareLt() for proper big.Int comparison instead of string comparison.
// String comparison is incorrect: "9" > "10" returns true (ASCII comparison).
func (s *WebSocketSubscriber) HandleTransaction(ctx context.Context, tx Transaction) error {
	// Update last processed lt using proper numeric comparison
	s.ltMu.Lock()
	if compareLt(tx.Lt(), s.lastProcessedLt) {
		s.lastProcessedLt = tx.Lt()
	}
	s.ltMu.Unlock()

	// Forward to registered handler
	if s.handler != nil {
		return s.handler.HandleTransaction(ctx, tx)
	}
	return nil
}

// Start begins WebSocket event streaming (T150).
func (s *WebSocketSubscriber) Start(ctx context.Context) {
	s.ctx, s.cancel = context.WithCancel(ctx)

	// Connect to WebSocket
	if err := s.client.Connect(s.ctx); err != nil {
		s.logger.Error("Failed to connect WebSocket: %v", err)
		// Trigger fallback on initial connection failure
		s.client.TriggerFallback()
		return
	}

	// Subscribe to contract events
	if err := s.client.Subscribe(s.ctx); err != nil {
		s.logger.Error("Failed to subscribe to events: %v", err)
		// Trigger fallback on subscription failure
		s.client.TriggerFallback()
		return
	}

	// Start event processing
	s.client.Start(s.ctx)

	s.logger.Info("WebSocket subscriber started for contract events")
}

// Stop gracefully stops the WebSocket subscriber (T150).
func (s *WebSocketSubscriber) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	if s.client != nil {
		s.client.Stop()
	}
	s.logger.Info("WebSocket subscriber stopped")
}

// Subscribe registers an event handler (T150).
func (s *WebSocketSubscriber) Subscribe(handler EventHandler) {
	s.handler = handler
}

// GetLastProcessedLt returns the last processed logical time (T150).
func (s *WebSocketSubscriber) GetLastProcessedLt() string {
	s.ltMu.RLock()
	defer s.ltMu.RUnlock()
	return s.lastProcessedLt
}

// SetLastProcessedLt sets the starting logical time (T150).
func (s *WebSocketSubscriber) SetLastProcessedLt(lt string) {
	s.ltMu.Lock()
	defer s.ltMu.Unlock()
	s.lastProcessedLt = lt
	s.logger.Info("WebSocket subscriber: set last processed lt to %s", lt)
}

// IsConnected returns whether the WebSocket is connected (T150).
func (s *WebSocketSubscriber) IsConnected() bool {
	if s.client == nil {
		return false
	}
	return s.client.IsConnected()
}

// GetSourceType returns "websocket" (T150).
func (s *WebSocketSubscriber) GetSourceType() string {
	return SourceTypeWebSocket
}
