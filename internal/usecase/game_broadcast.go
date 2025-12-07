package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gofiber/websocket/v2"
	"github.com/rs/zerolog/log"

	"pod-backend/internal/entity"
)

// WebSocket message types for reservation events
const (
	MessageTypeReservationCreated  = "reservation_created"
	MessageTypeReservationReleased = "reservation_released"
	MessageTypeGameStateUpdate          = "game_state_update"

	// GlobalGameID is used for subscribers who want to receive all game updates
	GlobalGameID = int64(0)
)

// ReservationCreatedEvent is sent when a game is reserved
type ReservationCreatedEvent struct {
	Type       string `json:"type"`
	GameID     int64  `json:"game_id"`
	ReservedBy string `json:"reserved_by"`
	ExpiresAt  string `json:"expires_at"` // ISO 8601
}

// ReservationReleasedEvent is sent when a reservation is released
type ReservationReleasedEvent struct {
	Type   string `json:"type"`
	GameID int64  `json:"game_id"`
	Reason string `json:"reason"` // "expired", "cancelled", "joined"
}

// WebSocketConn is an interface for WebSocket connections to enable testing
type WebSocketConn interface {
	WriteMessage(messageType int, data []byte) error
	Close() error
	SetWriteDeadline(t time.Time) error
}

// subscriber represents a WebSocket client subscribed to a game
type subscriber struct {
	clientID string
	conn     WebSocketConn
}

// GameBroadcastUseCase handles WebSocket broadcasting of game updates using hub pattern
type GameBroadcastUseCase struct {
	// gameSubscribers maps gameID -> map[clientID]subscriber
	gameSubscribers map[int64]map[string]*subscriber
	mu              sync.RWMutex

	// Metrics
	activeConnections int
}

// NewGameBroadcastUseCase creates a new GameBroadcastUseCase instance
func NewGameBroadcastUseCase() *GameBroadcastUseCase {
	return &GameBroadcastUseCase{
		gameSubscribers:   make(map[int64]map[string]*subscriber),
		activeConnections: 0,
	}
}

// Subscribe adds a WebSocket connection to receive updates for a specific game
// Use gameID = GlobalGameID (0) to subscribe to all game updates
func (uc *GameBroadcastUseCase) Subscribe(ctx context.Context, gameID int64, clientID string, conn WebSocketConn) {
	uc.mu.Lock()
	defer uc.mu.Unlock()

	// Initialize game subscribers map if not exists
	if uc.gameSubscribers[gameID] == nil {
		uc.gameSubscribers[gameID] = make(map[string]*subscriber)
	}

	// Add subscriber
	uc.gameSubscribers[gameID][clientID] = &subscriber{
		clientID: clientID,
		conn:     conn,
	}

	uc.activeConnections++

	if gameID == GlobalGameID {
		log.Info().
			Str("client_id", clientID).
			Int("total_connections", uc.activeConnections).
			Msg("WebSocket client subscribed to ALL games")
	} else {
		log.Info().
			Int64("game_id", gameID).
			Str("client_id", clientID).
			Int("total_connections", uc.activeConnections).
			Msg("WebSocket client subscribed to game")
	}
}

// Unsubscribe removes a WebSocket connection from game updates
func (uc *GameBroadcastUseCase) Unsubscribe(ctx context.Context, gameID int64, clientID string) {
	uc.mu.Lock()
	defer uc.mu.Unlock()

	if uc.gameSubscribers[gameID] == nil {
		return
	}

	if _, exists := uc.gameSubscribers[gameID][clientID]; exists {
		delete(uc.gameSubscribers[gameID], clientID)
		uc.activeConnections--

		// Clean up empty game map
		if len(uc.gameSubscribers[gameID]) == 0 {
			delete(uc.gameSubscribers, gameID)
		}

		log.Info().
			Int64("game_id", gameID).
			Str("client_id", clientID).
			Int("total_connections", uc.activeConnections).
			Msg("WebSocket client unsubscribed from game")
	}
}

// BroadcastGameUpdate sends a game update to all subscribers of that game AND global subscribers
func (uc *GameBroadcastUseCase) BroadcastGameUpdate(ctx context.Context, game *entity.Game) error {
	uc.mu.RLock()
	gameSubscribers := uc.gameSubscribers[game.GameID]
	globalSubscribers := uc.gameSubscribers[GlobalGameID]
	uc.mu.RUnlock()

	// Merge subscribers: game-specific + global
	allSubscribers := make(map[string]*subscriber)
	for k, v := range gameSubscribers {
		allSubscribers[k] = v
	}
	for k, v := range globalSubscribers {
		allSubscribers[k] = v
	}

	if len(allSubscribers) == 0 {
		log.Debug().
			Int64("game_id", game.GameID).
			Msg("No subscribers for game update")
		return nil
	}

	// Create game update message matching OpenAPI spec
	message, err := json.Marshal(map[string]interface{}{
		"type": MessageTypeGameStateUpdate,
		"data": game,
	})
	if err != nil {
		log.Error().
			Err(err).
			Int64("game_id", game.GameID).
			Msg("Failed to serialize game update")
		return fmt.Errorf("failed to serialize game update: %w", err)
	}

	// Track failed connections for cleanup
	var failedGameClients []string
	var failedGlobalClients []string

	// Broadcast to all subscribers
	for clientID, sub := range allSubscribers {
		// Set write deadline to prevent slow clients from blocking
		if err := sub.conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
			log.Warn().
				Err(err).
				Int64("game_id", game.GameID).
				Str("client_id", clientID).
				Msg("Failed to set write deadline")
			// Check if this is a global or game-specific subscriber
			if _, isGlobal := globalSubscribers[clientID]; isGlobal {
				failedGlobalClients = append(failedGlobalClients, clientID)
			} else {
				failedGameClients = append(failedGameClients, clientID)
			}
			continue
		}

		// Send message
		if err := sub.conn.WriteMessage(websocket.TextMessage, message); err != nil {
			log.Warn().
				Err(err).
				Int64("game_id", game.GameID).
				Str("client_id", clientID).
				Msg("Failed to send game update to client")
			if _, isGlobal := globalSubscribers[clientID]; isGlobal {
				failedGlobalClients = append(failedGlobalClients, clientID)
			} else {
				failedGameClients = append(failedGameClients, clientID)
			}
			continue
		}

		log.Debug().
			Int64("game_id", game.GameID).
			Str("client_id", clientID).
			Int("message_size", len(message)).
			Msg("Game update sent to client")
	}

	// Clean up failed game-specific connections
	if len(failedGameClients) > 0 {
		uc.mu.Lock()
		for _, clientID := range failedGameClients {
			if sub, exists := uc.gameSubscribers[game.GameID][clientID]; exists {
				sub.conn.Close()
				delete(uc.gameSubscribers[game.GameID], clientID)
				uc.activeConnections--
			}
		}
		// Clean up empty game map
		if len(uc.gameSubscribers[game.GameID]) == 0 {
			delete(uc.gameSubscribers, game.GameID)
		}
		uc.mu.Unlock()
	}

	// Clean up failed global connections
	if len(failedGlobalClients) > 0 {
		uc.mu.Lock()
		for _, clientID := range failedGlobalClients {
			if sub, exists := uc.gameSubscribers[GlobalGameID][clientID]; exists {
				sub.conn.Close()
				delete(uc.gameSubscribers[GlobalGameID], clientID)
				uc.activeConnections--
			}
		}
		uc.mu.Unlock()
	}

	totalFailed := len(failedGameClients) + len(failedGlobalClients)
	if totalFailed > 0 {
		log.Info().
			Int64("game_id", game.GameID).
			Int("failed_count", totalFailed).
			Msg("Cleaned up failed WebSocket connections")
	}

	log.Info().
		Int64("game_id", game.GameID).
		Int("status", game.Status).
		Int("game_subscribers", len(gameSubscribers)).
		Int("global_subscribers", len(globalSubscribers)).
		Int("failed", totalFailed).
		Msg("Game update broadcast completed")

	return nil
}

// GetActiveConnectionCount returns the number of active WebSocket connections
func (uc *GameBroadcastUseCase) GetActiveConnectionCount() int {
	uc.mu.RLock()
	defer uc.mu.RUnlock()
	return uc.activeConnections
}

// GetGameSubscriberCount returns the number of subscribers for a specific game
func (uc *GameBroadcastUseCase) GetGameSubscriberCount(gameID int64) int {
	uc.mu.RLock()
	defer uc.mu.RUnlock()

	if subscribers, exists := uc.gameSubscribers[gameID]; exists {
		return len(subscribers)
	}
	return 0
}

// CloseAllConnections closes all active WebSocket connections (for graceful shutdown)
func (uc *GameBroadcastUseCase) CloseAllConnections(ctx context.Context) error {
	uc.mu.Lock()
	defer uc.mu.Unlock()

	totalClosed := 0

	for gameID, subscribers := range uc.gameSubscribers {
		for clientID, sub := range subscribers {
			if err := sub.conn.Close(); err != nil {
				log.Warn().
					Err(err).
					Int64("game_id", gameID).
					Str("client_id", clientID).
					Msg("Error closing WebSocket connection during shutdown")
			} else {
				totalClosed++
			}
		}
	}

	// Clear all subscriptions
	uc.gameSubscribers = make(map[int64]map[string]*subscriber)
	uc.activeConnections = 0

	log.Info().
		Int("connections_closed", totalClosed).
		Msg("All WebSocket connections closed for graceful shutdown")

	return nil
}

// BroadcastReservationCreated sends a reservation created event to all subscribers of that game
func (uc *GameBroadcastUseCase) BroadcastReservationCreated(ctx context.Context, reservation *entity.GameReservation) error {
	event := ReservationCreatedEvent{
		Type:       MessageTypeReservationCreated,
		GameID:     reservation.GameID,
		ReservedBy: reservation.WalletAddress,
		ExpiresAt:  reservation.ExpiresAt.Format(time.RFC3339),
	}

	return uc.broadcastEvent(ctx, reservation.GameID, event)
}

// BroadcastReservationReleased sends a reservation released event to all subscribers of that game
func (uc *GameBroadcastUseCase) BroadcastReservationReleased(ctx context.Context, gameID int64, reason string) error {
	event := ReservationReleasedEvent{
		Type:   MessageTypeReservationReleased,
		GameID: gameID,
		Reason: reason,
	}

	return uc.broadcastEvent(ctx, gameID, event)
}

// broadcastEvent sends an arbitrary event to all subscribers of a game AND global subscribers
func (uc *GameBroadcastUseCase) broadcastEvent(ctx context.Context, gameID int64, event interface{}) error {
	uc.mu.RLock()
	gameSubscribers := uc.gameSubscribers[gameID]
	globalSubscribers := uc.gameSubscribers[GlobalGameID]
	uc.mu.RUnlock()

	// Merge subscribers: game-specific + global
	allSubscribers := make(map[string]*subscriber)
	for k, v := range gameSubscribers {
		allSubscribers[k] = v
	}
	for k, v := range globalSubscribers {
		allSubscribers[k] = v
	}

	if len(allSubscribers) == 0 {
		log.Debug().
			Int64("game_id", gameID).
			Msg("No subscribers for event broadcast")
		return nil
	}

	// Serialize event to JSON
	message, err := json.Marshal(event)
	if err != nil {
		log.Error().
			Err(err).
			Int64("game_id", gameID).
			Msg("Failed to serialize event")
		return fmt.Errorf("failed to serialize event: %w", err)
	}

	// Track failed connections for cleanup
	var failedGameClients []string
	var failedGlobalClients []string

	// Broadcast to all subscribers
	for clientID, sub := range allSubscribers {
		// Set write deadline to prevent slow clients from blocking
		if err := sub.conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
			log.Warn().
				Err(err).
				Int64("game_id", gameID).
				Str("client_id", clientID).
				Msg("Failed to set write deadline")
			if _, isGlobal := globalSubscribers[clientID]; isGlobal {
				failedGlobalClients = append(failedGlobalClients, clientID)
			} else {
				failedGameClients = append(failedGameClients, clientID)
			}
			continue
		}

		// Send message
		if err := sub.conn.WriteMessage(websocket.TextMessage, message); err != nil {
			log.Warn().
				Err(err).
				Int64("game_id", gameID).
				Str("client_id", clientID).
				Msg("Failed to send event to client")
			if _, isGlobal := globalSubscribers[clientID]; isGlobal {
				failedGlobalClients = append(failedGlobalClients, clientID)
			} else {
				failedGameClients = append(failedGameClients, clientID)
			}
			continue
		}

		log.Debug().
			Int64("game_id", gameID).
			Str("client_id", clientID).
			Int("message_size", len(message)).
			Msg("Event sent to client")
	}

	// Clean up failed game-specific connections
	if len(failedGameClients) > 0 {
		uc.mu.Lock()
		for _, clientID := range failedGameClients {
			if sub, exists := uc.gameSubscribers[gameID][clientID]; exists {
				sub.conn.Close()
				delete(uc.gameSubscribers[gameID], clientID)
				uc.activeConnections--
			}
		}
		// Clean up empty game map
		if len(uc.gameSubscribers[gameID]) == 0 {
			delete(uc.gameSubscribers, gameID)
		}
		uc.mu.Unlock()
	}

	// Clean up failed global connections
	if len(failedGlobalClients) > 0 {
		uc.mu.Lock()
		for _, clientID := range failedGlobalClients {
			if sub, exists := uc.gameSubscribers[GlobalGameID][clientID]; exists {
				sub.conn.Close()
				delete(uc.gameSubscribers[GlobalGameID], clientID)
				uc.activeConnections--
			}
		}
		uc.mu.Unlock()
	}

	totalFailed := len(failedGameClients) + len(failedGlobalClients)
	if totalFailed > 0 {
		log.Info().
			Int64("game_id", gameID).
			Int("failed_count", totalFailed).
			Msg("Cleaned up failed WebSocket connections")
	}

	log.Debug().
		Int64("game_id", gameID).
		Int("game_subscribers", len(gameSubscribers)).
		Int("global_subscribers", len(globalSubscribers)).
		Int("failed", totalFailed).
		Msg("Event broadcast completed")

	return nil
}
