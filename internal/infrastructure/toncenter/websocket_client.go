package toncenter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"pod-backend/pkg/logger"
)

// WebSocket constants (T141, T143)
const (
	// Reconnection backoff sequence: 1s, 2s, 4s, 8s, 16s max (per research.md)
	WSBackoffInitial = 1 * time.Second
	WSBackoffMax     = 16 * time.Second

	// Connection health monitoring (T143)
	WSPingInterval     = 30 * time.Second
	WSPongTimeout      = 10 * time.Second
	WSWriteWait        = 10 * time.Second
	WSReadBufferSize   = 1024
	WSWriteBufferSize  = 1024
	WSMaxReconnectWait = 60 * time.Second
)

// WebSocketClient implements real-time event streaming from TON Center API v3.
// Provides <2s latency for blockchain events (SC-001, SC-002).
type WebSocketClient struct {
	wsURL            string
	contractAddress  string
	logger           logger.Interface
	conn             *websocket.Conn
	connMu           sync.RWMutex
	isConnected      atomic.Bool
	subscribed       atomic.Bool
	stopped          atomic.Bool // Guard against double Stop()
	disconnecting    atomic.Bool // Guard against concurrent disconnect (deadlock fix)
	subscriptionID   string
	handler          EventHandler
	stopCh           chan struct{}
	reconnectAttempt int
	maxReconnect     int
	pingInterval     time.Duration
	lastPong         time.Time
	lastPongMu       sync.RWMutex
	onFallback       func() // Callback when fallback to HTTP is needed
}

// WebSocketConfig holds configuration for WebSocket client.
type WebSocketConfig struct {
	WSURL           string
	ContractAddress string
	MaxReconnect    int           // Max reconnection attempts before fallback
	PingInterval    time.Duration // Health check ping interval
	OnFallback      func()        // Called when max reconnects exceeded
}

// NewWebSocketClient creates a new TON Center WebSocket client (T139).
func NewWebSocketClient(cfg WebSocketConfig, handler EventHandler, log logger.Interface) *WebSocketClient {
	pingInterval := cfg.PingInterval
	if pingInterval == 0 {
		pingInterval = WSPingInterval
	}

	maxReconnect := cfg.MaxReconnect
	if maxReconnect == 0 {
		maxReconnect = 10
	}

	return &WebSocketClient{
		wsURL:           cfg.WSURL,
		contractAddress: cfg.ContractAddress,
		logger:          log,
		handler:         handler,
		stopCh:          make(chan struct{}),
		maxReconnect:    maxReconnect,
		pingInterval:    pingInterval,
		onFallback:      cfg.OnFallback,
	}
}

// Connect establishes WebSocket connection to TON Center API v3 (T139).
func (c *WebSocketClient) Connect(ctx context.Context) error {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.isConnected.Load() {
		return nil // Already connected
	}

	// Parse and validate WebSocket URL
	u, err := url.Parse(c.wsURL)
	if err != nil {
		return fmt.Errorf("invalid WebSocket URL: %w", err)
	}

	c.logger.Info("Connecting to TON Center WebSocket: %s", u.String())

	// Create WebSocket connection with custom dialer
	dialer := websocket.Dialer{
		ReadBufferSize:   WSReadBufferSize,
		WriteBufferSize:  WSWriteBufferSize,
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		return fmt.Errorf("WebSocket dial failed: %w", err)
	}

	c.conn = conn
	c.isConnected.Store(true)
	c.reconnectAttempt = 0
	c.lastPong = time.Now()

	// Set up pong handler for health monitoring (T143)
	conn.SetPongHandler(func(appData string) error {
		c.lastPongMu.Lock()
		c.lastPong = time.Now()
		c.lastPongMu.Unlock()
		c.logger.Debug("Received pong from TON Center WebSocket")
		return nil
	})

	c.logger.Info("Connected to TON Center WebSocket successfully")
	return nil
}

// Subscribe subscribes to transaction events for the contract address (T142).
func (c *WebSocketClient) Subscribe(ctx context.Context) error {
	c.connMu.RLock()
	conn := c.conn
	c.connMu.RUnlock()

	if conn == nil || !c.isConnected.Load() {
		return fmt.Errorf("not connected to WebSocket")
	}

	// TON Center API v3 WebSocket subscription message format
	// JSON-RPC 2.0 style subscription request
	subscribeMsg := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "subscribe",
		"params": map[string]interface{}{
			"account": c.contractAddress,
		},
	}

	c.connMu.Lock()
	conn.SetWriteDeadline(time.Now().Add(WSWriteWait))
	err := conn.WriteJSON(subscribeMsg)
	c.connMu.Unlock()

	if err != nil {
		return fmt.Errorf("failed to send subscription: %w", err)
	}

	c.logger.Info("Subscribed to transactions for contract: %s", c.contractAddress)
	c.subscribed.Store(true)
	return nil
}

// Start begins listening for WebSocket messages and handling events.
func (c *WebSocketClient) Start(ctx context.Context) {
	// Start ping goroutine for health monitoring (T143)
	go c.pingLoop(ctx)

	// Start message reading loop
	go c.readLoop(ctx)

	c.logger.Info("WebSocket client started for contract: %s", c.contractAddress)
}

// readLoop continuously reads messages from WebSocket (T140).
func (c *WebSocketClient) readLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			c.logger.Info("WebSocket read loop stopped: context cancelled")
			return
		case <-c.stopCh:
			c.logger.Info("WebSocket read loop stopped: stop signal received")
			return
		default:
			if !c.isConnected.Load() {
				// Attempt reconnection
				if err := c.reconnect(ctx); err != nil {
					c.logger.Error("Reconnection failed: %v", err)
					time.Sleep(time.Second)
					continue
				}
			}

			msg, err := c.readMessage()
			if err != nil {
				c.logger.Warn("WebSocket read error: %v", err)
				c.handleDisconnect()
				continue
			}

			if err := c.processMessage(ctx, msg); err != nil {
				c.logger.Error("Failed to process WebSocket message: %v", err)
			}
		}
	}
}

// readMessage reads a single message from WebSocket connection (T140).
func (c *WebSocketClient) readMessage() ([]byte, error) {
	c.connMu.RLock()
	conn := c.conn
	c.connMu.RUnlock()

	if conn == nil {
		return nil, fmt.Errorf("connection is nil")
	}

	_, msg, err := conn.ReadMessage()
	if err != nil {
		return nil, err
	}

	return msg, nil
}

// WebSocketMessage represents a message from TON Center WebSocket API (T140).
type WebSocketMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *WSError        `json:"error,omitempty"`
}

// WSError represents a WebSocket error response.
type WSError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// TransactionNotification represents a transaction event from WebSocket.
type TransactionNotification struct {
	Subscription string      `json:"subscription"`
	Result       Transaction `json:"result"`
}

// processMessage parses and handles a WebSocket message (T140).
func (c *WebSocketClient) processMessage(ctx context.Context, data []byte) error {
	var msg WebSocketMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return fmt.Errorf("failed to parse WebSocket message: %w", err)
	}

	// Handle error responses
	if msg.Error != nil {
		return fmt.Errorf("WebSocket error [%d]: %s", msg.Error.Code, msg.Error.Message)
	}

	// Handle subscription confirmation
	if msg.Result != nil && msg.Method == "" {
		var result struct {
			Subscription string `json:"subscription"`
		}
		if err := json.Unmarshal(msg.Result, &result); err == nil && result.Subscription != "" {
			c.subscriptionID = result.Subscription
			c.logger.Info("Subscription confirmed with ID: %s", c.subscriptionID)
			return nil
		}
	}

	// Handle transaction notification
	if msg.Method == "subscription" && msg.Params != nil {
		var notification TransactionNotification
		if err := json.Unmarshal(msg.Params, &notification); err != nil {
			return fmt.Errorf("failed to parse transaction notification: %w", err)
		}

		c.logger.Debug("Received transaction notification: hash=%s lt=%s",
			notification.Result.Hash(), notification.Result.Lt())

		// Process through event handler
		if c.handler != nil {
			if err := c.handler.HandleTransaction(ctx, notification.Result); err != nil {
				c.logger.Error("Failed to handle transaction: %v", err)
			}
		}
	}

	return nil
}

// pingLoop sends periodic pings for connection health monitoring (T143).
func (c *WebSocketClient) pingLoop(ctx context.Context) {
	ticker := time.NewTicker(c.pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		case <-ticker.C:
			if !c.isConnected.Load() {
				continue
			}

			c.connMu.Lock()
			if c.conn != nil {
				c.conn.SetWriteDeadline(time.Now().Add(WSWriteWait))
				if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					c.logger.Warn("Failed to send ping: %v", err)
					c.connMu.Unlock()
					c.handleDisconnect()
					continue
				}
			}
			c.connMu.Unlock()

			// Check for stale connection (no pong received within timeout)
			c.lastPongMu.RLock()
			lastPong := c.lastPong
			c.lastPongMu.RUnlock()

			if time.Since(lastPong) > c.pingInterval+WSPongTimeout {
				c.logger.Warn("Connection appears stale (no pong for %v), reconnecting",
					time.Since(lastPong))
				c.handleDisconnect()
			}
		}
	}
}

// handleDisconnect handles connection loss and triggers reconnection (T141).
// Uses atomic flag to prevent concurrent disconnect attempts which could cause deadlock.
func (c *WebSocketClient) handleDisconnect() {
	// Only allow one goroutine to handle disconnect at a time
	// This prevents deadlock when both readLoop and pingLoop try to disconnect simultaneously
	if !c.disconnecting.CompareAndSwap(false, true) {
		c.logger.Debug("Disconnect already in progress, skipping")
		return
	}
	defer c.disconnecting.Store(false)

	c.connMu.Lock()
	defer c.connMu.Unlock()

	if !c.isConnected.Load() {
		return // Already disconnected
	}

	c.isConnected.Store(false)
	c.subscribed.Store(false)

	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}

	c.logger.Warn("WebSocket disconnected, will attempt reconnection")
}

// reconnect attempts to reconnect with exponential backoff (T141).
func (c *WebSocketClient) reconnect(ctx context.Context) error {
	c.reconnectAttempt++

	if c.reconnectAttempt > c.maxReconnect {
		c.logger.Error("Max reconnection attempts (%d) exceeded, triggering fallback", c.maxReconnect)
		if c.onFallback != nil {
			c.onFallback()
		}
		return fmt.Errorf("max reconnection attempts exceeded")
	}

	// Calculate backoff duration with exponential growth (T141)
	backoff := WSBackoffInitial
	for i := 1; i < c.reconnectAttempt; i++ {
		backoff *= 2
		if backoff > WSBackoffMax {
			backoff = WSBackoffMax
			break
		}
	}

	c.logger.Info("Reconnection attempt %d/%d, waiting %v", c.reconnectAttempt, c.maxReconnect, backoff)
	time.Sleep(backoff)

	// Attempt to connect
	if err := c.Connect(ctx); err != nil {
		return err
	}

	// Re-subscribe after reconnection
	if err := c.Subscribe(ctx); err != nil {
		c.logger.Error("Failed to re-subscribe after reconnect: %v", err)
		c.handleDisconnect()
		return err
	}

	c.logger.Info("Reconnected and re-subscribed successfully")
	return nil
}

// Stop gracefully stops the WebSocket client.
func (c *WebSocketClient) Stop() {
	// Guard against double Stop() calls
	if c.stopped.Swap(true) {
		return // Already stopped
	}

	close(c.stopCh)

	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.conn != nil {
		// Send close message
		c.conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		c.conn.Close()
		c.conn = nil
	}

	c.isConnected.Store(false)
	c.subscribed.Store(false)
	c.logger.Info("WebSocket client stopped")
}

// TriggerFallback manually triggers the fallback callback if set.
// Used when initial connection or subscription fails (T158).
func (c *WebSocketClient) TriggerFallback() {
	if c.onFallback != nil {
		c.onFallback()
	}
}

// IsConnected returns whether the WebSocket is currently connected (T143).
func (c *WebSocketClient) IsConnected() bool {
	return c.isConnected.Load()
}

// IsSubscribed returns whether the client has an active subscription (T142).
func (c *WebSocketClient) IsSubscribed() bool {
	return c.subscribed.Load()
}

// GetSubscriptionID returns the current subscription ID (T142).
func (c *WebSocketClient) GetSubscriptionID() string {
	return c.subscriptionID
}

// GetLastPongTime returns when the last pong was received (T143).
func (c *WebSocketClient) GetLastPongTime() time.Time {
	c.lastPongMu.RLock()
	defer c.lastPongMu.RUnlock()
	return c.lastPong
}
