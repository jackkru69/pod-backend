package load_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

// Load test for WebSocket connections (T133)
// Verify SC-007: Support 100+ concurrent connections

const (
	// Test configuration
	targetConnections = 100
	testDuration      = 30 * time.Second
	wsURL             = "ws://localhost:3000/ws/games/1"
)

// TestWebSocketLoad_100Connections tests 100 concurrent WebSocket connections
func TestWebSocketLoad_100Connections(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	t.Logf("Starting WebSocket load test: %d connections for %v", targetConnections, testDuration)

	var (
		successfulConnections int64
		failedConnections     int64
		messagesReceived      int64
		activeConnections     int64
	)

	// Metrics tracking
	connectionTimes := make([]time.Duration, 0, targetConnections)
	var metricsMu sync.Mutex

	// Wait group for all goroutines
	var wg sync.WaitGroup

	// Context for test duration
	testCtx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()

	// Create connections concurrently
	for i := 0; i < targetConnections; i++ {
		wg.Add(1)
		go func(connID int) {
			defer wg.Done()

			startTime := time.Now()

			// Connect to WebSocket
			conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				atomic.AddInt64(&failedConnections, 1)
				t.Logf("Connection %d failed: %v", connID, err)
				return
			}
			defer conn.Close()

			connectionTime := time.Since(startTime)
			metricsMu.Lock()
			connectionTimes = append(connectionTimes, connectionTime)
			metricsMu.Unlock()

			atomic.AddInt64(&successfulConnections, 1)
			atomic.AddInt64(&activeConnections, 1)
			defer atomic.AddInt64(&activeConnections, -1)

			// Read messages until context is done
			conn.SetReadDeadline(time.Now().Add(testDuration))

			for {
				select {
				case <-testCtx.Done():
					return
				default:
					_, message, err := conn.ReadMessage()
					if err != nil {
						// Connection closed or error - exit gracefully
						return
					}

					if len(message) > 0 {
						atomic.AddInt64(&messagesReceived, 1)
					}
				}
			}
		}(i)

		// Stagger connection attempts slightly to avoid thundering herd
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for test duration
	<-testCtx.Done()

	// Give connections time to cleanup
	time.Sleep(1 * time.Second)

	// Wait for all goroutines to finish
	wg.Wait()

	// Calculate metrics
	var totalConnectionTime time.Duration
	for _, ct := range connectionTimes {
		totalConnectionTime += ct
	}

	avgConnectionTime := time.Duration(0)
	if len(connectionTimes) > 0 {
		avgConnectionTime = totalConnectionTime / time.Duration(len(connectionTimes))
	}

	// Report results
	t.Logf("=== WebSocket Load Test Results ===")
	t.Logf("Target connections: %d", targetConnections)
	t.Logf("Successful connections: %d", successfulConnections)
	t.Logf("Failed connections: %d", failedConnections)
	t.Logf("Messages received: %d", messagesReceived)
	t.Logf("Average connection time: %v", avgConnectionTime)
	t.Logf("Test duration: %v", testDuration)

	// Assertions for SC-007
	assert.GreaterOrEqual(t, successfulConnections, int64(100),
		"SC-007: Must support at least 100 concurrent connections")

	assert.LessOrEqual(t, failedConnections, int64(5),
		"Failed connections should be less than 5%")

	assert.Less(t, avgConnectionTime, 1*time.Second,
		"Average connection time should be under 1 second")

	// Success criteria
	successRate := float64(successfulConnections) / float64(targetConnections) * 100
	t.Logf("Success rate: %.2f%%", successRate)

	assert.GreaterOrEqual(t, successRate, 95.0,
		"Success rate should be at least 95%")
}

// TestWebSocketLoad_MessageBroadcast tests message broadcasting under load
func TestWebSocketLoad_MessageBroadcast(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	const (
		connections       = 50
		broadcastInterval = 1 * time.Second
		testDuration      = 10 * time.Second
	)

	t.Logf("Testing WebSocket broadcast to %d connections", connections)

	var (
		connectedClients int64
		totalMessages    int64
	)

	var wg sync.WaitGroup
	testCtx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()

	// Create multiple WebSocket connections
	for i := 0; i < connections; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()

			conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				t.Logf("Client %d failed to connect: %v", clientID, err)
				return
			}
			defer conn.Close()

			atomic.AddInt64(&connectedClients, 1)

			conn.SetReadDeadline(time.Now().Add(testDuration))

			messagesForClient := 0
			for {
				select {
				case <-testCtx.Done():
					t.Logf("Client %d received %d messages", clientID, messagesForClient)
					atomic.AddInt64(&totalMessages, int64(messagesForClient))
					return
				default:
					_, _, err := conn.ReadMessage()
					if err != nil {
						return
					}
					messagesForClient++
				}
			}
		}(i)

		time.Sleep(20 * time.Millisecond)
	}

	// Wait for test to complete
	<-testCtx.Done()
	time.Sleep(500 * time.Millisecond)
	wg.Wait()

	t.Logf("=== Broadcast Test Results ===")
	t.Logf("Connected clients: %d/%d", connectedClients, connections)
	t.Logf("Total messages received: %d", totalMessages)

	if connectedClients > 0 {
		avgMessagesPerClient := float64(totalMessages) / float64(connectedClients)
		t.Logf("Average messages per client: %.2f", avgMessagesPerClient)
	}

	assert.GreaterOrEqual(t, connectedClients, int64(45),
		"At least 90% of clients should connect successfully")
}

// TestWebSocketLoad_Reconnection tests connection stability
func TestWebSocketLoad_Reconnection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	const (
		connections = 20
		cycles      = 3
	)

	t.Logf("Testing WebSocket reconnection stability: %d connections x %d cycles", connections, cycles)

	var successfulReconnections int64

	for cycle := 0; cycle < cycles; cycle++ {
		t.Logf("Cycle %d/%d", cycle+1, cycles)

		var wg sync.WaitGroup

		for i := 0; i < connections; i++ {
			wg.Add(1)
			go func(connID int) {
				defer wg.Done()

				// Connect
				conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
				if err != nil {
					return
				}

				// Hold connection briefly
				time.Sleep(500 * time.Millisecond)

				// Close
				conn.Close()

				atomic.AddInt64(&successfulReconnections, 1)
			}(i)

			time.Sleep(10 * time.Millisecond)
		}

		wg.Wait()

		// Brief pause between cycles
		time.Sleep(1 * time.Second)
	}

	expectedReconnections := int64(connections * cycles)
	successRate := float64(successfulReconnections) / float64(expectedReconnections) * 100

	t.Logf("=== Reconnection Test Results ===")
	t.Logf("Expected reconnections: %d", expectedReconnections)
	t.Logf("Successful reconnections: %d", successfulReconnections)
	t.Logf("Success rate: %.2f%%", successRate)

	assert.GreaterOrEqual(t, successRate, 95.0,
		"Reconnection success rate should be at least 95%")
}
