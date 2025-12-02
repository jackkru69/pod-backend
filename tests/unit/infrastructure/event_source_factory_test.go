package infrastructure_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pod-backend/internal/infrastructure/toncenter"
	"pod-backend/pkg/logger"
)

// factoryMockLogger implements logger.Interface for factory testing.
type factoryMockLogger struct {
	debugMsgs []string
	infoMsgs  []string
	warnMsgs  []string
	errorMsgs []string
	mu        sync.Mutex
}

func newFactoryMockLogger() *factoryMockLogger {
	return &factoryMockLogger{}
}

func (l *factoryMockLogger) Debug(message interface{}, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if s, ok := message.(string); ok {
		l.debugMsgs = append(l.debugMsgs, s)
	}
}

func (l *factoryMockLogger) Info(message string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.infoMsgs = append(l.infoMsgs, message)
}

func (l *factoryMockLogger) Warn(message string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warnMsgs = append(l.warnMsgs, message)
}

func (l *factoryMockLogger) Error(message interface{}, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if s, ok := message.(string); ok {
		l.errorMsgs = append(l.errorMsgs, s)
	}
}

func (l *factoryMockLogger) Fatal(message interface{}, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if s, ok := message.(string); ok {
		l.errorMsgs = append(l.errorMsgs, s)
	}
}

// factoryMockEventHandler implements toncenter.EventHandler for factory testing.
type factoryMockEventHandler struct {
	transactions []toncenter.Transaction
	mu           sync.Mutex
}

func newFactoryMockEventHandler() *factoryMockEventHandler {
	return &factoryMockEventHandler{}
}

func (h *factoryMockEventHandler) HandleTransaction(ctx context.Context, tx toncenter.Transaction) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.transactions = append(h.transactions, tx)
	return nil
}

// TestEventSourceFactory_CreateHTTPSource tests factory creating HTTP event source when WebSocket disabled (T156).
func TestEventSourceFactory_CreateHTTPSource(t *testing.T) {
	log := newFactoryMockLogger()

	// Create factory with WebSocket disabled
	factory := toncenter.NewEventSourceFactory(toncenter.FactoryConfig{
		V2BaseURL:             "http://localhost:8080/api/v2",
		ContractAddress:       "EQTest123",
		CircuitBreakerMaxFail: 5,
		CircuitBreakerTimeout: 60 * time.Second,
		HTTPTimeout:           30 * time.Second,
		EnableWebSocket:       false,
		EventSourceType:       toncenter.SourceTypeHTTP,
	}, log)

	// Create event source
	handler := newFactoryMockEventHandler()
	source, err := factory.CreateEventSource(handler)
	require.NoError(t, err)
	require.NotNil(t, source)

	// Verify it's HTTP source
	assert.Equal(t, toncenter.SourceTypeHTTP, source.GetSourceType())
}

// TestEventSourceFactory_CreateWebSocketSource tests factory creating WebSocket event source when enabled (T156).
func TestEventSourceFactory_CreateWebSocketSource(t *testing.T) {
	log := newFactoryMockLogger()

	// Create factory with WebSocket enabled
	factory := toncenter.NewEventSourceFactory(toncenter.FactoryConfig{
		V2BaseURL:             "http://localhost:8080/api/v2",
		ContractAddress:       "EQTest123",
		CircuitBreakerMaxFail: 5,
		CircuitBreakerTimeout: 60 * time.Second,
		HTTPTimeout:           30 * time.Second,
		V3WSURL:               "ws://localhost:8081/api/v3/websocket",
		EnableWebSocket:       true,
		EventSourceType:       toncenter.SourceTypeWebSocket,
		MaxReconnect:          5,
		PingInterval:          30 * time.Second,
	}, log)

	// Create event source
	handler := newFactoryMockEventHandler()
	source, err := factory.CreateEventSource(handler)
	require.NoError(t, err)
	require.NotNil(t, source)

	// Verify it's WebSocket source
	assert.Equal(t, toncenter.SourceTypeWebSocket, source.GetSourceType())
}

// TestEventSourceFactory_HTTPFallbackWhenWebSocketEnabled tests HTTP source creation when EventSourceType is http but WebSocket enabled (T156).
func TestEventSourceFactory_HTTPFallbackWhenWebSocketEnabled(t *testing.T) {
	log := newFactoryMockLogger()

	// Create factory with WebSocket enabled but EventSourceType set to http
	factory := toncenter.NewEventSourceFactory(toncenter.FactoryConfig{
		V2BaseURL:             "http://localhost:8080/api/v2",
		ContractAddress:       "EQTest123",
		CircuitBreakerMaxFail: 5,
		CircuitBreakerTimeout: 60 * time.Second,
		HTTPTimeout:           30 * time.Second,
		V3WSURL:               "ws://localhost:8081/api/v3/websocket",
		EnableWebSocket:       true,
		EventSourceType:       toncenter.SourceTypeHTTP, // Explicitly request HTTP
		MaxReconnect:          5,
		PingInterval:          30 * time.Second,
	}, log)

	// Create event source
	handler := newFactoryMockEventHandler()
	source, err := factory.CreateEventSource(handler)
	require.NoError(t, err)
	require.NotNil(t, source)

	// Should be HTTP because EventSourceType is "http"
	assert.Equal(t, toncenter.SourceTypeHTTP, source.GetSourceType())
}

// TestEventSourceFactory_GetCurrentSourceType tests GetCurrentSourceType method (T156).
func TestEventSourceFactory_GetCurrentSourceType(t *testing.T) {
	log := newFactoryMockLogger()

	// Create factory
	factory := toncenter.NewEventSourceFactory(toncenter.FactoryConfig{
		V2BaseURL:             "http://localhost:8080/api/v2",
		ContractAddress:       "EQTest123",
		CircuitBreakerMaxFail: 5,
		CircuitBreakerTimeout: 60 * time.Second,
		HTTPTimeout:           30 * time.Second,
		EnableWebSocket:       false,
		EventSourceType:       toncenter.SourceTypeHTTP,
	}, log)

	// Before creating source
	assert.Equal(t, "", factory.GetCurrentSourceType())

	// Create event source
	handler := newFactoryMockEventHandler()
	_, err := factory.CreateEventSource(handler)
	require.NoError(t, err)

	// After creating source
	assert.Equal(t, toncenter.SourceTypeHTTP, factory.GetCurrentSourceType())
}

// TestEventSourceFactory_GetCurrentSource tests GetCurrentSource method (T156).
func TestEventSourceFactory_GetCurrentSource(t *testing.T) {
	log := newFactoryMockLogger()

	// Create factory
	factory := toncenter.NewEventSourceFactory(toncenter.FactoryConfig{
		V2BaseURL:             "http://localhost:8080/api/v2",
		ContractAddress:       "EQTest123",
		CircuitBreakerMaxFail: 5,
		CircuitBreakerTimeout: 60 * time.Second,
		HTTPTimeout:           30 * time.Second,
		EnableWebSocket:       false,
		EventSourceType:       toncenter.SourceTypeHTTP,
	}, log)

	// Before creating source
	assert.Nil(t, factory.GetCurrentSource())

	// Create event source
	handler := newFactoryMockEventHandler()
	source, err := factory.CreateEventSource(handler)
	require.NoError(t, err)

	// After creating source
	currentSource := factory.GetCurrentSource()
	assert.NotNil(t, currentSource)
	assert.Equal(t, source, currentSource)
}

// TestEventSourceFactory_GetHTTPClient tests GetHTTPClient method (T156).
func TestEventSourceFactory_GetHTTPClient(t *testing.T) {
	log := newFactoryMockLogger()

	// Create factory
	factory := toncenter.NewEventSourceFactory(toncenter.FactoryConfig{
		V2BaseURL:             "http://localhost:8080/api/v2",
		ContractAddress:       "EQTest123",
		CircuitBreakerMaxFail: 5,
		CircuitBreakerTimeout: 60 * time.Second,
		HTTPTimeout:           30 * time.Second,
		EnableWebSocket:       false,
	}, log)

	// HTTP client should be available
	client := factory.GetHTTPClient()
	assert.NotNil(t, client)
}

// TestEventSourceFactory_ConfigValidation tests that factory handles various config combinations (T156).
func TestEventSourceFactory_ConfigValidation(t *testing.T) {
	log := newFactoryMockLogger()

	testCases := []struct {
		name           string
		config         toncenter.FactoryConfig
		expectedSource string
	}{
		{
			name: "WebSocket disabled with HTTP type",
			config: toncenter.FactoryConfig{
				V2BaseURL:       "http://localhost:8080/api/v2",
				ContractAddress: "EQTest123",
				EnableWebSocket: false,
				EventSourceType: toncenter.SourceTypeHTTP,
			},
			expectedSource: toncenter.SourceTypeHTTP,
		},
		{
			name: "WebSocket enabled with WebSocket type",
			config: toncenter.FactoryConfig{
				V2BaseURL:       "http://localhost:8080/api/v2",
				V3WSURL:         "ws://localhost:8081/api/v3/websocket",
				ContractAddress: "EQTest123",
				EnableWebSocket: true,
				EventSourceType: toncenter.SourceTypeWebSocket,
			},
			expectedSource: toncenter.SourceTypeWebSocket,
		},
		{
			name: "WebSocket enabled with HTTP type (explicit fallback)",
			config: toncenter.FactoryConfig{
				V2BaseURL:       "http://localhost:8080/api/v2",
				V3WSURL:         "ws://localhost:8081/api/v3/websocket",
				ContractAddress: "EQTest123",
				EnableWebSocket: true,
				EventSourceType: toncenter.SourceTypeHTTP,
			},
			expectedSource: toncenter.SourceTypeHTTP,
		},
		{
			name: "WebSocket disabled with WebSocket type (should default to HTTP)",
			config: toncenter.FactoryConfig{
				V2BaseURL:       "http://localhost:8080/api/v2",
				V3WSURL:         "ws://localhost:8081/api/v3/websocket",
				ContractAddress: "EQTest123",
				EnableWebSocket: false,
				EventSourceType: toncenter.SourceTypeWebSocket,
			},
			expectedSource: toncenter.SourceTypeHTTP, // WebSocket disabled, so HTTP
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			factory := toncenter.NewEventSourceFactory(tc.config, log)
			handler := newFactoryMockEventHandler()
			source, err := factory.CreateEventSource(handler)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedSource, source.GetSourceType())
		})
	}
}

// Ensure factoryMockLogger satisfies logger.Interface
var _ logger.Interface = (*factoryMockLogger)(nil)
