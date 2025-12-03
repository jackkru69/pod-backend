package load_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
)

// Load test for WebSocket connections (T133)
// Verify SC-007: Support 100+ concurrent connections

const (
	// Test configuration
	targetConnections = 100
	testDuration      = 30 * time.Second
	wsURL             = "ws://localhost:8090/ws/games/1"
	testGameID        = 999999 // Use high ID to avoid conflicts
)

// getTestDB returns a database connection for test setup/cleanup
func getTestDB(t *testing.T) *sql.DB {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://user:myAwEsOm3pa55@w0rd@localhost:5433/db?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	return db
}

// setupTestGame creates a test game for WebSocket connections
func setupTestGame(t *testing.T, db *sql.DB, gameID int64) {
	// First ensure test user exists
	userQuery := `
		INSERT INTO users (telegram_user_id, telegram_username, wallet_address, created_at, updated_at)
		VALUES (12345678, 'test_user', 'EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2', NOW(), NOW())
		ON CONFLICT (wallet_address) DO NOTHING
	`
	_, _ = db.Exec(userQuery) // Ignore error if user exists

	txHash := fmt.Sprintf("test_tx_%d", gameID)
	query := `
		INSERT INTO games (
			game_id, status, player_one_address, bet_amount, player_one_choice,
			service_fee_numerator, referrer_fee_numerator, waiting_timeout_seconds,
			lowest_bid_allowed, highest_bid_allowed, fee_receiver_address, init_tx_hash, created_at
		)
		VALUES ($1, 1, 'EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2', 1000000000, 1,
			500, 50, 3600, 100000000, 10000000000, 'EQDtFpEwcFAEcRe5mLVh2N6C0x-_hJEM7W61_JLnSF74p4q2', 
			$2, NOW())
		ON CONFLICT (game_id) DO UPDATE SET status = 1
	`
	_, err := db.Exec(query, gameID, txHash)
	if err != nil {
		t.Fatalf("Failed to create test game: %v", err)
	}
	t.Logf("Created test game with ID %d", gameID)
}

// cleanupTestGame removes the test game
func cleanupTestGame(t *testing.T, db *sql.DB, gameID int64) {
	// First delete related events
	_, _ = db.Exec("DELETE FROM game_events WHERE game_id = $1", gameID)
	// Then delete the game
	_, err := db.Exec("DELETE FROM games WHERE game_id = $1", gameID)
	if err != nil {
		t.Logf("Warning: Failed to cleanup test game: %v", err)
	} else {
		t.Logf("Cleaned up test game with ID %d", gameID)
	}
}

// getWSURL returns WebSocket URL for a specific game ID
func getWSURL(gameID int64) string {
	return fmt.Sprintf("ws://localhost:8090/ws/games/%d", gameID)
}

// TestWebSocketLoad_100Connections tests 100 concurrent WebSocket connections
func TestWebSocketLoad_100Connections(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	// Setup test game
	db := getTestDB(t)
	defer db.Close()
	setupTestGame(t, db, testGameID)
	defer cleanupTestGame(t, db, testGameID)

	wsURLForTest := getWSURL(testGameID)
	t.Logf("Starting WebSocket load test: %d connections for %v to %s", targetConnections, testDuration, wsURLForTest)

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
			conn, _, err := websocket.DefaultDialer.Dial(wsURLForTest, nil)
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
		broadcastGameID   = 999998
	)

	// Setup test game
	db := getTestDB(t)
	defer db.Close()
	setupTestGame(t, db, broadcastGameID)
	defer cleanupTestGame(t, db, broadcastGameID)

	wsURLForTest := getWSURL(broadcastGameID)
	t.Logf("Testing WebSocket broadcast to %d connections at %s", connections, wsURLForTest)

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

			conn, _, err := websocket.DefaultDialer.Dial(wsURLForTest, nil)
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
		connections  = 20
		cycles       = 3
		reconnGameID = 999997
	)

	// Setup test game
	db := getTestDB(t)
	defer db.Close()
	setupTestGame(t, db, reconnGameID)
	defer cleanupTestGame(t, db, reconnGameID)

	wsURLForTest := getWSURL(reconnGameID)
	t.Logf("Testing WebSocket reconnection stability: %d connections x %d cycles to %s", connections, cycles, wsURLForTest)

	var successfulReconnections int64

	for cycle := 0; cycle < cycles; cycle++ {
		t.Logf("Cycle %d/%d", cycle+1, cycles)

		var wg sync.WaitGroup

		for i := 0; i < connections; i++ {
			wg.Add(1)
			go func(connID int) {
				defer wg.Done()

				// Connect
				conn, _, err := websocket.DefaultDialer.Dial(wsURLForTest, nil)
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
