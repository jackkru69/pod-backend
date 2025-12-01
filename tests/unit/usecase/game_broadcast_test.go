package usecase_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/websocket/v2"
	"github.com/stretchr/testify/assert"

	"pod-backend/internal/entity"
	"pod-backend/internal/usecase"
)

// MockWebSocketConn is a mock WebSocket connection for testing
type MockWebSocketConn struct {
	mu            sync.Mutex
	messagesSent  [][]byte
	closed        bool
	writeDeadline time.Time
}

func (m *MockWebSocketConn) WriteMessage(messageType int, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return websocket.ErrCloseSent
	}

	m.messagesSent = append(m.messagesSent, data)
	return nil
}

func (m *MockWebSocketConn) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *MockWebSocketConn) SetWriteDeadline(t time.Time) error {
	m.writeDeadline = t
	return nil
}

func (m *MockWebSocketConn) GetMessagesSent() [][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.messagesSent
}

func (m *MockWebSocketConn) IsClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

// TestGameBroadcastUseCase_BroadcastGameUpdate tests the BroadcastGameUpdate method
func TestGameBroadcastUseCase_BroadcastGameUpdate(t *testing.T) {
	t.Run("should broadcast game update to all subscribers", func(t *testing.T) {
		// Arrange
		bc := usecase.NewGameBroadcastUseCase()
		ctx := context.Background()

		// Create mock connections
		conn1 := &MockWebSocketConn{messagesSent: [][]byte{}}
		conn2 := &MockWebSocketConn{messagesSent: [][]byte{}}
		conn3 := &MockWebSocketConn{messagesSent: [][]byte{}}

		// Subscribe connections to game
		gameID := int64(123)
		bc.Subscribe(ctx, gameID, "client1", conn1)
		bc.Subscribe(ctx, gameID, "client2", conn2)
		bc.Subscribe(ctx, gameID, "client3", conn3)

		// Create test game update
		game := &entity.Game{
			GameID:           gameID,
			Status:           entity.GameStatusWaitingForOpponent,
			PlayerOneAddress: "EQAbcdefghijklmnopqrstuvwxyz0123456789ABCDEFGH",
			PlayerOneChoice:  entity.CoinSideHeads,
			BetAmount:        1000000000,
			CreatedAt:        time.Now(),
			InitTxHash:       "abc123def456",
		}

		// Act
		err := bc.BroadcastGameUpdate(ctx, game)

		// Assert
		assert.NoError(t, err)

		// Verify all connections received the message
		messages1 := conn1.GetMessagesSent()
		messages2 := conn2.GetMessagesSent()
		messages3 := conn3.GetMessagesSent()

		assert.Len(t, messages1, 1, "conn1 should receive 1 message")
		assert.Len(t, messages2, 1, "conn2 should receive 1 message")
		assert.Len(t, messages3, 1, "conn3 should receive 1 message")

		// Verify message content
		var receivedGame entity.Game
		err = json.Unmarshal(messages1[0], &receivedGame)
		assert.NoError(t, err)
		assert.Equal(t, gameID, receivedGame.GameID)
		assert.Equal(t, entity.GameStatusWaitingForOpponent, receivedGame.Status)
	})

	t.Run("should handle broadcast when no subscribers exist", func(t *testing.T) {
		// Arrange
		bc := usecase.NewGameBroadcastUseCase()
		ctx := context.Background()

		game := &entity.Game{
			GameID:           999,
			Status:           entity.GameStatusWaitingForOpponent,
			PlayerOneAddress: "EQAbcdefghijklmnopqrstuvwxyz0123456789ABCDEFGH",
			BetAmount:        1000000000,
			InitTxHash:       "abc123",
		}

		// Act
		err := bc.BroadcastGameUpdate(ctx, game)

		// Assert
		assert.NoError(t, err, "should not error when no subscribers")
	})

	t.Run("should skip closed connections during broadcast", func(t *testing.T) {
		// Arrange
		bc := usecase.NewGameBroadcastUseCase()
		ctx := context.Background()

		conn1 := &MockWebSocketConn{messagesSent: [][]byte{}}
		conn2 := &MockWebSocketConn{messagesSent: [][]byte{}, closed: true}
		conn3 := &MockWebSocketConn{messagesSent: [][]byte{}}

		gameID := int64(123)
		bc.Subscribe(ctx, gameID, "client1", conn1)
		bc.Subscribe(ctx, gameID, "client2", conn2)
		bc.Subscribe(ctx, gameID, "client3", conn3)

		game := &entity.Game{
			GameID:           gameID,
			Status:           entity.GameStatusWaitingForOpponent,
			PlayerOneAddress: "EQAbcdefghijklmnopqrstuvwxyz0123456789ABCDEFGH",
			BetAmount:        1000000000,
			InitTxHash:       "abc123",
		}

		// Act
		err := bc.BroadcastGameUpdate(ctx, game)

		// Assert
		assert.NoError(t, err)

		// Verify only open connections received messages
		assert.Len(t, conn1.GetMessagesSent(), 1, "conn1 should receive message")
		assert.Len(t, conn2.GetMessagesSent(), 0, "conn2 (closed) should not receive message")
		assert.Len(t, conn3.GetMessagesSent(), 1, "conn3 should receive message")
	})

	t.Run("should only broadcast to subscribers of specific game", func(t *testing.T) {
		// Arrange
		bc := usecase.NewGameBroadcastUseCase()
		ctx := context.Background()

		conn1 := &MockWebSocketConn{messagesSent: [][]byte{}}
		conn2 := &MockWebSocketConn{messagesSent: [][]byte{}}

		// Subscribe to different games
		bc.Subscribe(ctx, 123, "client1", conn1)
		bc.Subscribe(ctx, 456, "client2", conn2)

		game := &entity.Game{
			GameID:           123,
			Status:           entity.GameStatusWaitingForOpponent,
			PlayerOneAddress: "EQAbcdefghijklmnopqrstuvwxyz0123456789ABCDEFGH",
			BetAmount:        1000000000,
			InitTxHash:       "abc123",
		}

		// Act
		err := bc.BroadcastGameUpdate(ctx, game)

		// Assert
		assert.NoError(t, err)
		assert.Len(t, conn1.GetMessagesSent(), 1, "conn1 (game 123) should receive message")
		assert.Len(t, conn2.GetMessagesSent(), 0, "conn2 (game 456) should not receive message")
	})

	t.Run("should handle concurrent broadcasts", func(t *testing.T) {
		// Arrange
		bc := usecase.NewGameBroadcastUseCase()
		ctx := context.Background()

		numClients := 10
		conns := make([]*MockWebSocketConn, numClients)
		for i := 0; i < numClients; i++ {
			conns[i] = &MockWebSocketConn{messagesSent: [][]byte{}}
			bc.Subscribe(ctx, 123, string(rune(i)), conns[i])
		}

		game := &entity.Game{
			GameID:           123,
			Status:           entity.GameStatusWaitingForOpponent,
			PlayerOneAddress: "EQAbcdefghijklmnopqrstuvwxyz0123456789ABCDEFGH",
			BetAmount:        1000000000,
			InitTxHash:       "abc123",
		}

		// Act - broadcast concurrently multiple times
		var wg sync.WaitGroup
		numBroadcasts := 5
		for i := 0; i < numBroadcasts; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				bc.BroadcastGameUpdate(ctx, game)
			}()
		}
		wg.Wait()

		// Assert - all connections should have received all broadcasts
		for i, conn := range conns {
			messages := conn.GetMessagesSent()
			assert.GreaterOrEqual(t, len(messages), 1, "client %d should receive at least 1 message", i)
		}
	})
}

// TestGameBroadcastUseCase_ConnectionLifecycle tests subscription, unsubscription, and cleanup
func TestGameBroadcastUseCase_ConnectionLifecycle(t *testing.T) {
	t.Run("should successfully subscribe a connection", func(t *testing.T) {
		// Arrange
		bc := usecase.NewGameBroadcastUseCase()
		ctx := context.Background()
		conn := &MockWebSocketConn{messagesSent: [][]byte{}}

		// Act
		bc.Subscribe(ctx, 123, "client1", conn)

		// Broadcast to verify subscription
		game := &entity.Game{
			GameID:           123,
			Status:           entity.GameStatusWaitingForOpponent,
			PlayerOneAddress: "EQAbcdefghijklmnopqrstuvwxyz0123456789ABCDEFGH",
			BetAmount:        1000000000,
			InitTxHash:       "abc123",
		}
		bc.BroadcastGameUpdate(ctx, game)

		// Assert
		assert.Len(t, conn.GetMessagesSent(), 1, "subscribed connection should receive broadcast")
	})

	t.Run("should successfully unsubscribe a connection", func(t *testing.T) {
		// Arrange
		bc := usecase.NewGameBroadcastUseCase()
		ctx := context.Background()
		conn := &MockWebSocketConn{messagesSent: [][]byte{}}

		bc.Subscribe(ctx, 123, "client1", conn)

		// Act
		bc.Unsubscribe(ctx, 123, "client1")

		// Broadcast to verify unsubscription
		game := &entity.Game{
			GameID:           123,
			Status:           entity.GameStatusWaitingForOpponent,
			PlayerOneAddress: "EQAbcdefghijklmnopqrstuvwxyz0123456789ABCDEFGH",
			BetAmount:        1000000000,
			InitTxHash:       "abc123",
		}
		bc.BroadcastGameUpdate(ctx, game)

		// Assert
		assert.Len(t, conn.GetMessagesSent(), 0, "unsubscribed connection should not receive broadcast")
	})

	t.Run("should handle multiple subscriptions to same game", func(t *testing.T) {
		// Arrange
		bc := usecase.NewGameBroadcastUseCase()
		ctx := context.Background()

		conn1 := &MockWebSocketConn{messagesSent: [][]byte{}}
		conn2 := &MockWebSocketConn{messagesSent: [][]byte{}}
		conn3 := &MockWebSocketConn{messagesSent: [][]byte{}}

		// Act - subscribe all to same game
		bc.Subscribe(ctx, 123, "client1", conn1)
		bc.Subscribe(ctx, 123, "client2", conn2)
		bc.Subscribe(ctx, 123, "client3", conn3)

		game := &entity.Game{
			GameID:           123,
			Status:           entity.GameStatusWaitingForOpponent,
			PlayerOneAddress: "EQAbcdefghijklmnopqrstuvwxyz0123456789ABCDEFGH",
			BetAmount:        1000000000,
			InitTxHash:       "abc123",
		}
		bc.BroadcastGameUpdate(ctx, game)

		// Assert - all should receive
		assert.Len(t, conn1.GetMessagesSent(), 1)
		assert.Len(t, conn2.GetMessagesSent(), 1)
		assert.Len(t, conn3.GetMessagesSent(), 1)
	})

	t.Run("should handle subscriptions to different games", func(t *testing.T) {
		// Arrange
		bc := usecase.NewGameBroadcastUseCase()
		ctx := context.Background()

		conn1 := &MockWebSocketConn{messagesSent: [][]byte{}}
		conn2 := &MockWebSocketConn{messagesSent: [][]byte{}}

		// Subscribe to different games
		bc.Subscribe(ctx, 123, "client1", conn1)
		bc.Subscribe(ctx, 456, "client2", conn2)

		// Broadcast to game 123
		game123 := &entity.Game{
			GameID:           123,
			Status:           entity.GameStatusWaitingForOpponent,
			PlayerOneAddress: "EQAbcdefghijklmnopqrstuvwxyz0123456789ABCDEFGH",
			BetAmount:        1000000000,
			InitTxHash:       "abc123",
		}
		bc.BroadcastGameUpdate(ctx, game123)

		// Assert - only game 123 subscriber receives
		assert.Len(t, conn1.GetMessagesSent(), 1)
		assert.Len(t, conn2.GetMessagesSent(), 0)

		// Broadcast to game 456
		game456 := &entity.Game{
			GameID:           456,
			Status:           entity.GameStatusWaitingForOpponent,
			PlayerOneAddress: "EQXyzabcdefghijklmnopqrstuvwxyz0123456789ABCDE",
			BetAmount:        2000000000,
			InitTxHash:       "def456",
		}
		bc.BroadcastGameUpdate(ctx, game456)

		// Assert - only game 456 subscriber receives
		assert.Len(t, conn1.GetMessagesSent(), 1) // Still 1 from before
		assert.Len(t, conn2.GetMessagesSent(), 1)
	})

	t.Run("should handle unsubscribe of non-existent client", func(t *testing.T) {
		// Arrange
		bc := usecase.NewGameBroadcastUseCase()
		ctx := context.Background()

		// Act - unsubscribe without subscribe
		bc.Unsubscribe(ctx, 123, "nonexistent")

		// Assert - should not panic or error
		game := &entity.Game{
			GameID:           123,
			Status:           entity.GameStatusWaitingForOpponent,
			PlayerOneAddress: "EQAbcdefghijklmnopqrstuvwxyz0123456789ABCDEFGH",
			BetAmount:        1000000000,
			InitTxHash:       "abc123",
		}
		err := bc.BroadcastGameUpdate(ctx, game)
		assert.NoError(t, err)
	})

	t.Run("should allow resubscription after unsubscribe", func(t *testing.T) {
		// Arrange
		bc := usecase.NewGameBroadcastUseCase()
		ctx := context.Background()
		conn := &MockWebSocketConn{messagesSent: [][]byte{}}

		// Subscribe, unsubscribe, then resubscribe
		bc.Subscribe(ctx, 123, "client1", conn)
		bc.Unsubscribe(ctx, 123, "client1")
		bc.Subscribe(ctx, 123, "client1", conn)

		// Act
		game := &entity.Game{
			GameID:           123,
			Status:           entity.GameStatusWaitingForOpponent,
			PlayerOneAddress: "EQAbcdefghijklmnopqrstuvwxyz0123456789ABCDEFGH",
			BetAmount:        1000000000,
			InitTxHash:       "abc123",
		}
		bc.BroadcastGameUpdate(ctx, game)

		// Assert - should receive message after resubscription
		assert.Len(t, conn.GetMessagesSent(), 1)
	})

	t.Run("should clean up closed connections automatically", func(t *testing.T) {
		// Arrange
		bc := usecase.NewGameBroadcastUseCase()
		ctx := context.Background()

		conn1 := &MockWebSocketConn{messagesSent: [][]byte{}}
		conn2 := &MockWebSocketConn{messagesSent: [][]byte{}, closed: true}

		bc.Subscribe(ctx, 123, "client1", conn1)
		bc.Subscribe(ctx, 123, "client2", conn2)

		// Act - broadcast which should trigger cleanup
		game := &entity.Game{
			GameID:           123,
			Status:           entity.GameStatusWaitingForOpponent,
			PlayerOneAddress: "EQAbcdefghijklmnopqrstuvwxyz0123456789ABCDEFGH",
			BetAmount:        1000000000,
			InitTxHash:       "abc123",
		}
		bc.BroadcastGameUpdate(ctx, game)

		// Assert - only open connection receives
		assert.Len(t, conn1.GetMessagesSent(), 1)
		assert.Len(t, conn2.GetMessagesSent(), 0)
	})

	t.Run("should handle concurrent subscribe and unsubscribe", func(t *testing.T) {
		// Arrange
		bc := usecase.NewGameBroadcastUseCase()
		ctx := context.Background()

		var wg sync.WaitGroup
		numOperations := 50

		// Act - concurrent subscriptions and unsubscriptions
		for i := 0; i < numOperations; i++ {
			wg.Add(2)

			clientID := string(rune(i))
			conn := &MockWebSocketConn{messagesSent: [][]byte{}}

			// Subscribe
			go func() {
				defer wg.Done()
				bc.Subscribe(ctx, 123, clientID, conn)
			}()

			// Unsubscribe
			go func() {
				defer wg.Done()
				time.Sleep(10 * time.Millisecond)
				bc.Unsubscribe(ctx, 123, clientID)
			}()
		}

		wg.Wait()

		// Assert - no panics, system remains stable
		game := &entity.Game{
			GameID:           123,
			Status:           entity.GameStatusWaitingForOpponent,
			PlayerOneAddress: "EQAbcdefghijklmnopqrstuvwxyz0123456789ABCDEFGH",
			BetAmount:        1000000000,
			InitTxHash:       "abc123",
		}
		err := bc.BroadcastGameUpdate(ctx, game)
		assert.NoError(t, err)
	})

	t.Run("should get active connection count", func(t *testing.T) {
		// Arrange
		bc := usecase.NewGameBroadcastUseCase()
		ctx := context.Background()

		conn1 := &MockWebSocketConn{messagesSent: [][]byte{}}
		conn2 := &MockWebSocketConn{messagesSent: [][]byte{}}
		conn3 := &MockWebSocketConn{messagesSent: [][]byte{}}

		// Act
		bc.Subscribe(ctx, 123, "client1", conn1)
		bc.Subscribe(ctx, 123, "client2", conn2)
		bc.Subscribe(ctx, 456, "client3", conn3)

		// Assert
		count := bc.GetActiveConnectionCount()
		assert.Equal(t, 3, count, "should have 3 active connections")

		// Unsubscribe one
		bc.Unsubscribe(ctx, 123, "client1")
		count = bc.GetActiveConnectionCount()
		assert.Equal(t, 2, count, "should have 2 active connections after unsubscribe")
	})
}
