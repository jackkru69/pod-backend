package toncenter

import (
	"context"
	"fmt"
	"sync"
	"time"

	"pod-backend/pkg/logger"
)

// EventSourceFactory creates and manages event sources based on configuration (T151).
// Supports automatic fallback from WebSocket to HTTP polling.
type EventSourceFactory struct {
	config          FactoryConfig
	logger          logger.Interface
	httpClient      *Client
	currentSource   EventSource
	sourceMu        sync.RWMutex
	onFallback      func() // Called when fallback occurs
	fallbackHandler func() // Internal fallback trigger
}

// FactoryConfig holds configuration for EventSourceFactory.
type FactoryConfig struct {
	// HTTP Polling Configuration
	V2BaseURL             string
	ContractAddress       string
	CircuitBreakerMaxFail int
	CircuitBreakerTimeout time.Duration
	HTTPTimeout           time.Duration

	// WebSocket Configuration
	V3WSURL         string
	EnableWebSocket bool
	EventSourceType string // "websocket" or "http"
	MaxReconnect    int
	PingInterval    time.Duration

	// Callbacks
	OnFallback func() // Called when fallback from WebSocket to HTTP occurs
}

// NewEventSourceFactory creates a new event source factory (T151).
func NewEventSourceFactory(cfg FactoryConfig, log logger.Interface) *EventSourceFactory {
	// Create HTTP client for both HTTP polling and as fallback
	httpClient := NewClient(ClientConfig{
		V2BaseURL:             cfg.V2BaseURL,
		ContractAddress:       cfg.ContractAddress,
		CircuitBreakerMaxFail: cfg.CircuitBreakerMaxFail,
		CircuitBreakerTimeout: cfg.CircuitBreakerTimeout,
		HTTPTimeout:           cfg.HTTPTimeout,
	})

	factory := &EventSourceFactory{
		config:     cfg,
		logger:     log,
		httpClient: httpClient,
		onFallback: cfg.OnFallback,
	}

	return factory
}

// CreateEventSource creates an event source based on configuration (T151).
// If EnableWebSocket is true and EventSourceType is "websocket", creates WebSocket subscriber.
// Otherwise, creates HTTP poller.
func (f *EventSourceFactory) CreateEventSource(handler EventHandler) (EventSource, error) {
	f.sourceMu.Lock()
	defer f.sourceMu.Unlock()

	// Create appropriate event source based on configuration
	if f.config.EnableWebSocket && f.config.EventSourceType == SourceTypeWebSocket {
		f.logger.Info("Creating WebSocket event source for contract: %s", f.config.ContractAddress)

		// Create fallback handler
		fallbackTriggered := false
		fallbackHandler := func() {
			if fallbackTriggered {
				return // Prevent multiple fallbacks
			}
			fallbackTriggered = true
			f.logger.Warn("WebSocket fallback triggered, switching to HTTP polling")
			f.fallbackToHTTP(handler)
		}

		subscriber := NewWebSocketSubscriber(WebSocketSubscriberConfig{
			WSURL:           f.config.V3WSURL,
			ContractAddress: f.config.ContractAddress,
			MaxReconnect:    f.config.MaxReconnect,
			OnFallback:      fallbackHandler,
		}, f.logger)

		subscriber.Subscribe(handler)
		f.currentSource = subscriber

		return subscriber, nil
	}

	// Default to HTTP polling
	f.logger.Info("Creating HTTP polling event source for contract: %s", f.config.ContractAddress)
	return f.createHTTPPoller(handler), nil
}

// createHTTPPoller creates an HTTP polling event source.
func (f *EventSourceFactory) createHTTPPoller(handler EventHandler) EventSource {
	poller := NewPoller(f.httpClient, handler, f.logger, 0)
	f.currentSource = poller
	return poller
}

// fallbackToHTTP switches from WebSocket to HTTP polling (T151).
func (f *EventSourceFactory) fallbackToHTTP(handler EventHandler) {
	f.sourceMu.Lock()
	defer f.sourceMu.Unlock()

	// Stop current source if running
	if f.currentSource != nil {
		// Get last processed lt before stopping
		lastLt := f.currentSource.GetLastProcessedLt()
		f.currentSource.Stop()

		// Create HTTP poller
		f.logger.Info("Falling back to HTTP polling from lt: %s", lastLt)
		poller := f.createHTTPPoller(handler)
		poller.SetLastProcessedLt(lastLt)

		// Start the poller
		ctx := context.Background()
		poller.Start(ctx)
	}

	// Notify callback if set
	if f.onFallback != nil {
		go f.onFallback()
	}
}

// GetCurrentSource returns the currently active event source (T151).
func (f *EventSourceFactory) GetCurrentSource() EventSource {
	f.sourceMu.RLock()
	defer f.sourceMu.RUnlock()
	return f.currentSource
}

// GetCurrentSourceType returns the type of the current event source (T151).
func (f *EventSourceFactory) GetCurrentSourceType() string {
	f.sourceMu.RLock()
	defer f.sourceMu.RUnlock()

	if f.currentSource == nil {
		return ""
	}
	return f.currentSource.GetSourceType()
}

// IsConnected returns whether the current event source is connected (T151).
func (f *EventSourceFactory) IsConnected() bool {
	f.sourceMu.RLock()
	defer f.sourceMu.RUnlock()

	if f.currentSource == nil {
		return false
	}
	return f.currentSource.IsConnected()
}

// GetHTTPClient returns the underlying HTTP client (T151).
// Useful for health checks and circuit breaker status.
func (f *EventSourceFactory) GetHTTPClient() *Client {
	return f.httpClient
}

// SwitchToWebSocket attempts to switch from HTTP to WebSocket (T151).
// Returns error if WebSocket is not enabled or connection fails.
func (f *EventSourceFactory) SwitchToWebSocket(ctx context.Context, handler EventHandler) error {
	if !f.config.EnableWebSocket {
		return fmt.Errorf("WebSocket is not enabled in configuration")
	}

	f.sourceMu.Lock()
	defer f.sourceMu.Unlock()

	// Get current lt if source exists
	var lastLt string
	if f.currentSource != nil {
		lastLt = f.currentSource.GetLastProcessedLt()
		f.currentSource.Stop()
	}

	f.logger.Info("Switching to WebSocket event source from lt: %s", lastLt)

	// Create WebSocket subscriber
	fallbackTriggered := false
	fallbackHandler := func() {
		if fallbackTriggered {
			return
		}
		fallbackTriggered = true
		f.fallbackToHTTP(handler)
	}

	subscriber := NewWebSocketSubscriber(WebSocketSubscriberConfig{
		WSURL:           f.config.V3WSURL,
		ContractAddress: f.config.ContractAddress,
		MaxReconnect:    f.config.MaxReconnect,
		OnFallback:      fallbackHandler,
	}, f.logger)

	subscriber.Subscribe(handler)
	subscriber.SetLastProcessedLt(lastLt)
	f.currentSource = subscriber

	// Start the WebSocket subscriber
	subscriber.Start(ctx)

	return nil
}

// SwitchToHTTP switches from current source to HTTP polling (T151).
func (f *EventSourceFactory) SwitchToHTTP(ctx context.Context, handler EventHandler) {
	f.sourceMu.Lock()
	defer f.sourceMu.Unlock()

	// Get current lt if source exists
	var lastLt string
	if f.currentSource != nil {
		lastLt = f.currentSource.GetLastProcessedLt()
		f.currentSource.Stop()
	}

	f.logger.Info("Switching to HTTP polling event source from lt: %s", lastLt)

	// Create HTTP poller
	poller := f.createHTTPPoller(handler)
	poller.SetLastProcessedLt(lastLt)

	// Start the poller
	poller.Start(ctx)
}
