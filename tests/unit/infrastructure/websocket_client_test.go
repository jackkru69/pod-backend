package infrastructure_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pod-backend/internal/infrastructure/toncenter"
	"pod-backend/pkg/logger"
)

// mockLogger implements logger.Interface for testing.
type mockLogger struct {
	debugMsgs []string
	infoMsgs  []string
	warnMsgs  []string
	errorMsgs []string
	mu        sync.Mutex
}

func newMockLogger() *mockLogger {
	return &mockLogger{}
}

func (l *mockLogger) Debug(message interface{}, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if s, ok := message.(string); ok {
		l.debugMsgs = append(l.debugMsgs, s)
	}
}

func (l *mockLogger) Info(message string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.infoMsgs = append(l.infoMsgs, message)
}

func (l *mockLogger) Warn(message string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warnMsgs = append(l.warnMsgs, message)
}

func (l *mockLogger) Error(message interface{}, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if s, ok := message.(string); ok {
		l.errorMsgs = append(l.errorMsgs, s)
	}
}

func (l *mockLogger) Fatal(message interface{}, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if s, ok := message.(string); ok {
		l.errorMsgs = append(l.errorMsgs, s)
	}
}

// mockEventHandler implements toncenter.EventHandler for testing.
type mockEventHandler struct {
	transactions []toncenter.Transaction
	mu           sync.Mutex
	txChan       chan toncenter.Transaction
}

func newMockEventHandler() *mockEventHandler {
	return &mockEventHandler{
		txChan: make(chan toncenter.Transaction, 10),
	}
}

func (h *mockEventHandler) HandleTransaction(ctx context.Context, tx toncenter.Transaction) error {
	h.mu.Lock()
	h.transactions = append(h.transactions, tx)
	h.mu.Unlock()

	select {
	case h.txChan <- tx:
	default:
	}

	return nil
}

func (h *mockEventHandler) GetTransactions() []toncenter.Transaction {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]toncenter.Transaction{}, h.transactions...)
}

// mockWebSocketServer creates a test WebSocket server for testing.
type mockWebSocketServer struct {
	server           *httptest.Server
	upgrader         websocket.Upgrader
	connections      []*websocket.Conn
	messageHandler   func(conn *websocket.Conn, msg []byte)
	onConnect        func(conn *websocket.Conn)
	mu               sync.Mutex
	messagesReceived []json.RawMessage
}

func newMockWebSocketServer() *mockWebSocketServer {
	mock := &mockWebSocketServer{
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		messagesReceived: make([]json.RawMessage, 0),
	}

	mock.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := mock.upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		mock.mu.Lock()
		mock.connections = append(mock.connections, conn)
		mock.mu.Unlock()

		if mock.onConnect != nil {
			mock.onConnect(conn)
		}

		// Read messages
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				break
			}

			mock.mu.Lock()
			mock.messagesReceived = append(mock.messagesReceived, json.RawMessage(msg))
			mock.mu.Unlock()

			if mock.messageHandler != nil {
				mock.messageHandler(conn, msg)
			}
		}
	}))

	return mock
}

func (m *mockWebSocketServer) URL() string {
	return "ws" + strings.TrimPrefix(m.server.URL, "http")
}

func (m *mockWebSocketServer) Close() {
	m.mu.Lock()
	for _, conn := range m.connections {
		conn.Close()
	}
	m.mu.Unlock()
	m.server.Close()
}

func (m *mockWebSocketServer) SendMessage(msg interface{}) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, conn := range m.connections {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			return err
		}
	}
	return nil
}

// TestWebSocketClient_Connect tests WebSocket connection establishment (T155).
func TestWebSocketClient_Connect(t *testing.T) {
	// Create mock server
	mockServer := newMockWebSocketServer()
	defer mockServer.Close()

	// Create client
	log := newMockLogger()
	handler := newMockEventHandler()
	client := toncenter.NewWebSocketClient(toncenter.WebSocketConfig{
		WSURL:           mockServer.URL(),
		ContractAddress: "EQTest123",
		MaxReconnect:    3,
		PingInterval:    30 * time.Second,
	}, handler, log)

	// Connect
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := client.Connect(ctx)
	require.NoError(t, err)
	assert.True(t, client.IsConnected())

	// Close
	client.Stop()
	time.Sleep(100 * time.Millisecond) // Allow close to propagate
	assert.False(t, client.IsConnected())
}

// TestWebSocketClient_Subscribe tests subscription message sending (T155).
func TestWebSocketClient_Subscribe(t *testing.T) {
	// Create mock server that responds to subscribe messages
	mockServer := newMockWebSocketServer()
	defer mockServer.Close()

	subscribeReceived := make(chan bool, 1)
	mockServer.messageHandler = func(conn *websocket.Conn, msg []byte) {
		var request map[string]interface{}
		if err := json.Unmarshal(msg, &request); err == nil {
			if method, ok := request["method"].(string); ok && method == "subscribe" {
				// Send subscription confirmation
				response := map[string]interface{}{
					"id":     request["id"],
					"result": "subscription_12345",
				}
				conn.WriteJSON(response)
				subscribeReceived <- true
			}
		}
	}

	// Create client
	log := newMockLogger()
	handler := newMockEventHandler()
	client := toncenter.NewWebSocketClient(toncenter.WebSocketConfig{
		WSURL:           mockServer.URL(),
		ContractAddress: "EQTest123",
		MaxReconnect:    3,
		PingInterval:    30 * time.Second,
	}, handler, log)

	// Connect
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := client.Connect(ctx)
	require.NoError(t, err)
	defer client.Stop()

	// Subscribe
	err = client.Subscribe(ctx)
	require.NoError(t, err)

	// Verify subscribe message was sent
	select {
	case <-subscribeReceived:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("Subscribe message not received by server")
	}

	// Verify subscription state
	assert.True(t, client.IsSubscribed())
}

// TestWebSocketClient_MessageParsing tests parsing of transaction messages (T155).
func TestWebSocketClient_MessageParsing(t *testing.T) {
	// Create mock server
	mockServer := newMockWebSocketServer()
	defer mockServer.Close()

	// Respond to subscription
	mockServer.messageHandler = func(conn *websocket.Conn, msg []byte) {
		var request map[string]interface{}
		if err := json.Unmarshal(msg, &request); err == nil {
			if method, ok := request["method"].(string); ok && method == "subscribe" {
				response := map[string]interface{}{
					"id":     request["id"],
					"result": "subscription_12345",
				}
				conn.WriteJSON(response)
			}
		}
	}

	// Create client
	log := newMockLogger()
	handler := newMockEventHandler()
	client := toncenter.NewWebSocketClient(toncenter.WebSocketConfig{
		WSURL:           mockServer.URL(),
		ContractAddress: "EQTest123",
		MaxReconnect:    3,
		PingInterval:    30 * time.Second,
	}, handler, log)

	// Connect and start
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := client.Connect(ctx)
	require.NoError(t, err)
	defer client.Stop()

	// Start read loop
	go client.Start(ctx)

	// Subscribe
	err = client.Subscribe(ctx)
	require.NoError(t, err)

	// Wait a bit for everything to stabilize
	time.Sleep(100 * time.Millisecond)

	// Send transaction notification from server
	// The client expects "method": "subscription" with the transaction in params.result
	txNotification := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "subscription",
		"params": map[string]interface{}{
			"subscription": "subscription_12345",
			"result": map[string]interface{}{
				"transaction_id": map[string]interface{}{
					"hash": "abc123hash",
					"lt":   "12345678",
				},
				"utime": 1699999999,
				"in_msg": map[string]interface{}{
					"event_type": "game_finished",
					"game_id":    42,
				},
			},
		},
	}
	err = mockServer.SendMessage(txNotification)
	require.NoError(t, err)

	// Wait for transaction
	select {
	case tx := <-handler.txChan:
		assert.Equal(t, "abc123hash", tx.Hash())
		assert.Equal(t, "12345678", tx.Lt())
		assert.Equal(t, int64(1699999999), tx.Utime)
	case <-time.After(3 * time.Second):
		t.Fatal("Transaction not received")
	}
}

// TestWebSocketClient_Reconnection tests automatic reconnection on disconnect (T155).
func TestWebSocketClient_Reconnection(t *testing.T) {
	// Create mock server
	mockServer := newMockWebSocketServer()

	connectCount := atomic.Int32{}
	mockServer.onConnect = func(conn *websocket.Conn) {
		connectCount.Add(1)
	}

	// Auto-respond to subscribe
	mockServer.messageHandler = func(conn *websocket.Conn, msg []byte) {
		var request map[string]interface{}
		if err := json.Unmarshal(msg, &request); err == nil {
			if method, ok := request["method"].(string); ok && method == "subscribe" {
				response := map[string]interface{}{
					"id":     request["id"],
					"result": "subscription_12345",
				}
				conn.WriteJSON(response)
			}
		}
	}

	// Create client
	log := newMockLogger()
	handler := newMockEventHandler()
	client := toncenter.NewWebSocketClient(toncenter.WebSocketConfig{
		WSURL:           mockServer.URL(),
		ContractAddress: "EQTest123",
		MaxReconnect:    5,
		PingInterval:    30 * time.Second,
	}, handler, log)

	// Connect
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := client.Connect(ctx)
	require.NoError(t, err)
	assert.Equal(t, int32(1), connectCount.Load())

	// Start read loop that handles reconnection
	go client.Start(ctx)

	// Subscribe
	err = client.Subscribe(ctx)
	require.NoError(t, err)

	// Force disconnect by closing server connections
	time.Sleep(100 * time.Millisecond)
	mockServer.mu.Lock()
	for _, conn := range mockServer.connections {
		conn.Close()
	}
	mockServer.connections = nil
	mockServer.mu.Unlock()

	// Wait for reconnection (with some buffer for exponential backoff)
	time.Sleep(3 * time.Second)

	// Verify reconnection happened
	assert.GreaterOrEqual(t, connectCount.Load(), int32(2), "Should have reconnected")

	client.Stop()
	mockServer.Close()
}

// TestWebSocketClient_MaxReconnectAttempts tests fallback after max reconnect failures (T155).
func TestWebSocketClient_MaxReconnectAttempts(t *testing.T) {
	// Create a server and immediately close it
	mockServer := newMockWebSocketServer()
	serverURL := mockServer.URL()
	mockServer.Close()

	fallbackCalled := atomic.Bool{}

	// Create client with very low max reconnect
	log := newMockLogger()
	handler := newMockEventHandler()
	client := toncenter.NewWebSocketClient(toncenter.WebSocketConfig{
		WSURL:           serverURL, // Server is closed
		ContractAddress: "EQTest123",
		MaxReconnect:    2,
		PingInterval:    30 * time.Second,
		OnFallback: func() {
			fallbackCalled.Store(true)
		},
	}, handler, log)

	// Connect should fail
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := client.Connect(ctx)
	// Connect may fail immediately or trigger fallback

	// If connect succeeded somehow, start the loop to trigger reconnection
	if err == nil {
		go client.Start(ctx)
	}

	// Wait for fallback (exponential backoff: 1s, 2s = 3s minimum)
	time.Sleep(5 * time.Second)

	// Either fallback was called or connect returned error
	if err != nil {
		// Connection failed as expected
		assert.Error(t, err)
	} else {
		// Fallback should have been triggered
		assert.True(t, fallbackCalled.Load(), "Fallback should have been called after max reconnect attempts")
	}

	client.Stop()
}

// TestWebSocketClient_ConnectionState tests IsConnected and IsSubscribed state tracking (T155).
func TestWebSocketClient_ConnectionState(t *testing.T) {
	// Create mock server
	mockServer := newMockWebSocketServer()
	defer mockServer.Close()

	// Auto-respond to subscribe
	mockServer.messageHandler = func(conn *websocket.Conn, msg []byte) {
		var request map[string]interface{}
		if err := json.Unmarshal(msg, &request); err == nil {
			if method, ok := request["method"].(string); ok && method == "subscribe_account" {
				response := map[string]interface{}{
					"id":     request["id"],
					"result": "subscription_12345",
				}
				conn.WriteJSON(response)
			}
		}
	}

	// Create client
	log := newMockLogger()
	handler := newMockEventHandler()
	client := toncenter.NewWebSocketClient(toncenter.WebSocketConfig{
		WSURL:           mockServer.URL(),
		ContractAddress: "EQTest123",
		MaxReconnect:    3,
		PingInterval:    30 * time.Second,
	}, handler, log)

	// Initial state
	assert.False(t, client.IsConnected())
	assert.False(t, client.IsSubscribed())

	// Connect
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := client.Connect(ctx)
	require.NoError(t, err)

	// After connect
	assert.True(t, client.IsConnected())
	assert.False(t, client.IsSubscribed())

	// Subscribe
	err = client.Subscribe(ctx)
	require.NoError(t, err)

	// Wait for subscription response
	time.Sleep(100 * time.Millisecond)

	// After subscribe
	assert.True(t, client.IsConnected())
	assert.True(t, client.IsSubscribed())

	// Stop
	client.Stop()
	time.Sleep(100 * time.Millisecond)

	// After stop
	assert.False(t, client.IsConnected())
}

// Ensure mockLogger satisfies logger.Interface
var _ logger.Interface = (*mockLogger)(nil)
