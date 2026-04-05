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
	MessageTypeGameStateUpdate     = "game_state_update"

	// GlobalGameID is used for subscribers who want to receive all game updates
	GlobalGameID = int64(0)
)

const websocketWriteDeadline = 10 * time.Second

// GameEventType represents the type of blockchain event that triggered the update
type GameEventType string

const (
	GameEventTypeInitialized         GameEventType = "game_initialized"
	GameEventTypeStarted             GameEventType = "game_started"
	GameEventTypeFinished            GameEventType = "game_finished"
	GameEventTypeCancelled           GameEventType = "game_cancelled"
	GameEventTypeDraw                GameEventType = "draw"
	GameEventTypeSecretOpened        GameEventType = "secret_opened"
	GameEventTypeInsufficientBalance GameEventType = "insufficient_balance"
)

// ReservationCreatedEvent is sent when a game is reserved
type ReservationCreatedEvent struct {
	Type       string `json:"type"`
	Timestamp  string `json:"timestamp"`
	GameID     int64  `json:"game_id"`
	ReservedBy string `json:"reserved_by"`
	ExpiresAt  string `json:"expires_at"` // ISO 8601
}

// ReservationReleasedEvent is sent when a reservation is released
type ReservationReleasedEvent struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	GameID    int64  `json:"game_id"`
	Reason    string `json:"reason"` // "expired", "cancelled", "joined"
}

type GameStateUpdateEvent struct {
	Type      string       `json:"type"`
	Timestamp string       `json:"timestamp"`
	EventType string       `json:"event_type,omitempty"`
	Data      *entity.Game `json:"data"`
}

func websocketTimestamp(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
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
func (uc *GameBroadcastUseCase) Subscribe(_ context.Context, gameID int64, clientID string, conn WebSocketConn) {
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
func (uc *GameBroadcastUseCase) Unsubscribe(_ context.Context, gameID int64, clientID string) {
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
// Deprecated: Use BroadcastGameUpdateWithEvent for new code
func (uc *GameBroadcastUseCase) BroadcastGameUpdate(ctx context.Context, game *entity.Game) error {
	return uc.BroadcastGameUpdateWithEvent(ctx, game, "")
}

// BroadcastGameUpdateWithEvent sends a game update with event type to all subscribers.
// RACE CONDITION FIX: Deep copy subscriber maps while holding lock to prevent
// concurrent modification during iteration.
func (uc *GameBroadcastUseCase) BroadcastGameUpdateWithEvent(_ context.Context, game *entity.Game, eventType GameEventType) error {
	allSubscribers, globalClientIDs := uc.snapshotSubscribers(game.GameID)
	if len(allSubscribers) == 0 {
		log.Debug().
			Int64("game_id", game.GameID).
			Msg("No subscribers for game update")
		return nil
	}

	// Create game update message matching OpenAPI spec with event_type
	msgData := GameStateUpdateEvent{
		Type:      MessageTypeGameStateUpdate,
		Timestamp: websocketTimestamp(time.Now()),
		Data:      game,
	}
	if eventType != "" {
		msgData.EventType = string(eventType)
	}
	message, err := json.Marshal(msgData)
	if err != nil {
		log.Error().
			Err(err).
			Int64("game_id", game.GameID).
			Msg("Failed to serialize game update")
		return fmt.Errorf("failed to serialize game update: %w", err)
	}

	failedGameClients, failedGlobalClients := uc.broadcastSerializedMessage(
		game.GameID,
		message,
		allSubscribers,
		globalClientIDs,
		"Failed to send game update to client",
		"Game update sent to client",
	)

	uc.cleanupFailedConnections(game.GameID, failedGameClients, failedGlobalClients)

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
		Int("total_subscribers", len(allSubscribers)).
		Int("global_subscribers", len(globalClientIDs)).
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
func (uc *GameBroadcastUseCase) CloseAllConnections(_ context.Context) error {
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
		Timestamp:  websocketTimestamp(time.Now()),
		GameID:     reservation.GameID,
		ReservedBy: reservation.WalletAddress,
		ExpiresAt:  reservation.ExpiresAt.Format(time.RFC3339),
	}

	return uc.broadcastEvent(ctx, reservation.GameID, event)
}

// BroadcastReservationReleased sends a reservation released event to all subscribers of that game
func (uc *GameBroadcastUseCase) BroadcastReservationReleased(ctx context.Context, gameID int64, reason string) error {
	event := ReservationReleasedEvent{
		Type:      MessageTypeReservationReleased,
		Timestamp: websocketTimestamp(time.Now()),
		GameID:    gameID,
		Reason:    reason,
	}

	return uc.broadcastEvent(ctx, gameID, event)
}

// broadcastEvent sends an arbitrary event to all subscribers of a game AND global subscribers.
// RACE CONDITION FIX: Deep copy subscriber maps while holding lock.
func (uc *GameBroadcastUseCase) broadcastEvent(_ context.Context, gameID int64, event interface{}) error {
	allSubscribers, globalClientIDs := uc.snapshotSubscribers(gameID)
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

	failedGameClients, failedGlobalClients := uc.broadcastSerializedMessage(
		gameID,
		message,
		allSubscribers,
		globalClientIDs,
		"Failed to send event to client",
		"Event sent to client",
	)

	uc.cleanupFailedConnections(gameID, failedGameClients, failedGlobalClients)

	totalFailed := len(failedGameClients) + len(failedGlobalClients)
	if totalFailed > 0 {
		log.Info().
			Int64("game_id", gameID).
			Int("failed_count", totalFailed).
			Msg("Cleaned up failed WebSocket connections")
	}

	log.Debug().
		Int64("game_id", gameID).
		Int("total_subscribers", len(allSubscribers)).
		Int("global_subscribers", len(globalClientIDs)).
		Int("failed", totalFailed).
		Msg("Event broadcast completed")

	return nil
}

func (uc *GameBroadcastUseCase) snapshotSubscribers(gameID int64) (allSubscribers map[string]*subscriber, globalClientIDs map[string]bool) {
	uc.mu.RLock()
	defer uc.mu.RUnlock()

	allSubscribers = make(map[string]*subscriber)

	if gameSubscribers := uc.gameSubscribers[gameID]; gameSubscribers != nil {
		for clientID, sub := range gameSubscribers {
			allSubscribers[clientID] = sub
		}
	}

	globalClientIDs = make(map[string]bool)

	if globalSubscribers := uc.gameSubscribers[GlobalGameID]; globalSubscribers != nil {
		for clientID, sub := range globalSubscribers {
			allSubscribers[clientID] = sub
			globalClientIDs[clientID] = true
		}
	}

	return allSubscribers, globalClientIDs
}

func (uc *GameBroadcastUseCase) broadcastSerializedMessage(
	gameID int64,
	message []byte,
	allSubscribers map[string]*subscriber,
	globalClientIDs map[string]bool,
	writeFailureLog, writeSuccessLog string,
) (failedGameClients, failedGlobalClients []string) {
	for clientID, sub := range allSubscribers {
		if err := sub.conn.SetWriteDeadline(time.Now().Add(websocketWriteDeadline)); err != nil {
			log.Warn().
				Err(err).
				Int64("game_id", gameID).
				Str("client_id", clientID).
				Msg("Failed to set write deadline")

			if globalClientIDs[clientID] {
				failedGlobalClients = append(failedGlobalClients, clientID)
			} else {
				failedGameClients = append(failedGameClients, clientID)
			}

			continue
		}

		if err := sub.conn.WriteMessage(websocket.TextMessage, message); err != nil {
			log.Warn().
				Err(err).
				Int64("game_id", gameID).
				Str("client_id", clientID).
				Msg(writeFailureLog)

			if globalClientIDs[clientID] {
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
			Msg(writeSuccessLog)
	}

	return failedGameClients, failedGlobalClients
}

func (uc *GameBroadcastUseCase) cleanupFailedConnections(gameID int64, failedGameClients, failedGlobalClients []string) {
	if len(failedGameClients) == 0 && len(failedGlobalClients) == 0 {
		return
	}

	uc.mu.Lock()
	defer uc.mu.Unlock()

	for _, clientID := range failedGameClients {
		if sub, exists := uc.gameSubscribers[gameID][clientID]; exists {
			sub.conn.Close()
			delete(uc.gameSubscribers[gameID], clientID)
			uc.activeConnections--
		}
	}

	if len(uc.gameSubscribers[gameID]) == 0 {
		delete(uc.gameSubscribers, gameID)
	}

	for _, clientID := range failedGlobalClients {
		if sub, exists := uc.gameSubscribers[GlobalGameID][clientID]; exists {
			sub.conn.Close()
			delete(uc.gameSubscribers[GlobalGameID], clientID)
			uc.activeConnections--
		}
	}
}
