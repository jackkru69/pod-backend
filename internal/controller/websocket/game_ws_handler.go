package websocket

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"pod-backend/internal/entity"
	"pod-backend/internal/repository"
	"pod-backend/internal/usecase"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// GameWebSocketHandler handles WebSocket connections for game updates.
// `/ws/games` is a broadcast-only stream, while `/ws/games/:gameId` additionally
// accepts `sync_request` frames so reconnecting clients can fetch a fresh game
// snapshot without changing the broadcast contract.
type GameWebSocketHandler struct {
	gameRepo         repository.GameRepository
	broadcastUseCase *usecase.GameBroadcastUseCase
	pingInterval     time.Duration
	pongWait         time.Duration
}

// clientMessage documents the public client-to-server frame supported by the
// per-game WebSocket endpoint. Global subscriptions reject client-authored
// frames with an explicit `error` response.
type clientMessage struct {
	Type               string `json:"type"`
	LastEventTimestamp string `json:"last_event_timestamp,omitempty"`
}

// syncResponseMessage is emitted for per-game `sync_request` reconciliation.
// Like broadcast payloads, it includes a top-level server timestamp. The nested
// game object is the same additive REST/read-model shape, so remediated config
// and protocol fields may appear without changing the frame type contract.
type syncResponseMessage struct {
	Type      string       `json:"type"`
	Timestamp string       `json:"timestamp"`
	Game      *entity.Game `json:"game"`
}

type errorMessage struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

const (
	websocketGlobalRoute          = "/ws/games"
	websocketPerGameRoute         = "/ws/games/:gameId"
	websocketMessageSyncRequest   = "sync_request"
	websocketMessageSyncResponse  = "sync_response"
	websocketMessageError         = "error"
	websocketErrorInvalidJSON     = "invalid_message_json"
	websocketErrorUnsupportedType = "unsupported_message_type"
	websocketErrorSyncUnavailable = "sync_unavailable"

	websocketPingInterval       = 30 * time.Second
	websocketPongWait           = 60 * time.Second
	websocketControlWriteWindow = 10 * time.Second
)

func newServerTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func parseClientMessage(message []byte) (*clientMessage, error) {
	var msg clientMessage
	if err := json.Unmarshal(message, &msg); err != nil {
		return nil, err
	}

	return &msg, nil
}

func newSyncResponseMessage(game *entity.Game) syncResponseMessage {
	return syncResponseMessage{
		Type:      websocketMessageSyncResponse,
		Timestamp: newServerTimestamp(),
		Game:      game,
	}
}

func newErrorMessage(code, message string) errorMessage {
	return errorMessage{
		Type:      websocketMessageError,
		Timestamp: newServerTimestamp(),
		Code:      code,
		Message:   message,
	}
}

func writeServerMessage(c *websocket.Conn, payload interface{}) error {
	message, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	return c.WriteMessage(websocket.TextMessage, message)
}

func sendWebSocketError(conn *websocket.Conn, code, message string) error {
	return writeServerMessage(conn, newErrorMessage(code, message))
}

func isTextWebSocketMessage(messageType int) bool {
	return messageType == websocket.TextMessage
}

func (h *GameWebSocketHandler) prepareConnection(
	conn *websocket.Conn,
	onPong func() error,
) (*time.Ticker, chan struct{}, error) {
	conn.SetPongHandler(func(_ string) error {
		return onPong()
	})

	if err := conn.SetReadDeadline(time.Now().Add(h.pongWait)); err != nil {
		return nil, nil, err
	}

	ticker := time.NewTicker(h.pingInterval)
	done := make(chan struct{})

	return ticker, done, nil
}

func (h *GameWebSocketHandler) openGameConnection(ctx context.Context, conn *websocket.Conn) (gameID int64, clientID string, ok bool) {
	gameID, ok = conn.Locals("gameID").(int64)
	if !ok {
		log.Error().Msg("Missing gameID in WebSocket connection locals")
		conn.Close()
		return 0, "", false
	}

	clientID = uuid.New().String()
	h.broadcastUseCase.Subscribe(ctx, gameID, clientID, conn)

	log.Info().
		Int64("game_id", gameID).
		Str("client_id", clientID).
		Str("remote_addr", conn.RemoteAddr().String()).
		Msg("WebSocket connection established")

	return gameID, clientID, true
}

func (h *GameWebSocketHandler) openGlobalConnection(ctx context.Context, conn *websocket.Conn) string {
	clientID := uuid.New().String()
	h.broadcastUseCase.Subscribe(ctx, 0, clientID, conn)

	log.Info().
		Str("client_id", clientID).
		Str("remote_addr", conn.RemoteAddr().String()).
		Msg("Global WebSocket connection established")

	return clientID
}

func startPingLoop(
	conn *websocket.Conn,
	ticker *time.Ticker,
	done chan struct{},
	onPingFailure func(error),
	onPingSuccess func(),
) {
	go func() {
		defer close(done)

		for {
			select {
			case <-ticker.C:
				if err := conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(websocketControlWriteWindow)); err != nil {
					onPingFailure(err)
					return
				}

				onPingSuccess()
			case <-done:
				return
			}
		}
	}()
}

func readConnectionMessages(
	conn *websocket.Conn,
	onMessage func(messageType int, message []byte),
	onUnexpectedClose func(error),
	onNormalClose func(),
) {
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				onUnexpectedClose(err)
			} else {
				onNormalClose()
			}

			return
		}

		onMessage(messageType, message)
	}
}

// NewGameWebSocketHandler creates a new WebSocket handler
func NewGameWebSocketHandler(
	gameRepo repository.GameRepository,
	broadcastUseCase *usecase.GameBroadcastUseCase,
) *GameWebSocketHandler {
	return &GameWebSocketHandler{
		gameRepo:         gameRepo,
		broadcastUseCase: broadcastUseCase,
		pingInterval:     websocketPingInterval, // Send ping every 30 seconds
		pongWait:         websocketPongWait,     // Wait up to 60 seconds for pong
	}
}

// UpgradeCheck validates WebSocket upgrade requests
func (h *GameWebSocketHandler) UpgradeCheck(c *fiber.Ctx) error {
	// Validate WebSocket upgrade headers
	if !websocket.IsWebSocketUpgrade(c) {
		return fiber.ErrUpgradeRequired
	}

	// Validate game ID parameter
	gameIDStr := c.Params("gameId")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil || gameID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid game ID",
		})
	}

	// Verify game exists
	ctx := c.Context()
	game, err := h.gameRepo.GetByID(ctx, gameID)
	if err != nil {
		log.Warn().
			Err(err).
			Int64("game_id", gameID).
			Msg("Game not found for WebSocket connection")
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "game not found",
		})
	}

	// Store game ID in locals for handler
	c.Locals("gameID", game.GameID)

	return c.Next()
}

// HandleConnection handles WebSocket connection lifecycle
func (h *GameWebSocketHandler) HandleConnection(c *websocket.Conn) {
	ctx := context.Background()
	gameID, clientID, ok := h.openGameConnection(ctx, c)
	if !ok {
		return
	}
	defer h.broadcastUseCase.Unsubscribe(ctx, gameID, clientID)

	ticker, done, err := h.prepareConnection(c, func() error {
		log.Debug().
			Int64("game_id", gameID).
			Str("client_id", clientID).
			Msg("Received pong from client")

		return c.SetReadDeadline(time.Now().Add(h.pongWait))
	})
	if err != nil {
		log.Error().
			Err(err).
			Int64("game_id", gameID).
			Str("client_id", clientID).
			Msg("Failed to set initial read deadline")
		return
	}
	defer ticker.Stop()

	startPingLoop(
		c,
		ticker,
		done,
		func(err error) {
			log.Warn().
				Err(err).
				Int64("game_id", gameID).
				Str("client_id", clientID).
				Msg("Failed to send ping, closing connection")
		},
		func() {
			log.Debug().
				Int64("game_id", gameID).
				Str("client_id", clientID).
				Msg("Sent ping to client")
		},
	)

	readConnectionMessages(c, func(messageType int, message []byte) {
		h.handleClientMessage(ctx, c, gameID, clientID, messageType, message)
	},
		func(err error) {
			log.Warn().
				Err(err).
				Int64("game_id", gameID).
				Str("client_id", clientID).
				Msg("WebSocket connection closed unexpectedly")
		},
		func() {
			log.Info().
				Int64("game_id", gameID).
				Str("client_id", clientID).
				Msg("WebSocket connection closed normally")
		})

	log.Info().
		Int64("game_id", gameID).
		Str("client_id", clientID).
		Msg("WebSocket connection handler completed")
}

// handleClientMessage processes messages received from client
func (h *GameWebSocketHandler) handleClientMessage(ctx context.Context, conn *websocket.Conn, gameID int64, clientID string, messageType int, message []byte) {
	if !isTextWebSocketMessage(messageType) {
		log.Debug().
			Int64("game_id", gameID).
			Str("client_id", clientID).
			Int("message_type", messageType).
			Msg("Ignoring non-text WebSocket client message")
		return
	}

	clientMsg, err := parseClientMessage(message)
	if err != nil {
		h.logParseError(gameID, clientID, err)
		h.writeErrorMessage(conn, gameID, clientID, websocketErrorInvalidJSON, "message must be valid JSON")
		return
	}

	switch clientMsg.Type {
	case websocketMessageSyncRequest:
		h.handleSyncRequest(ctx, conn, gameID, clientID, clientMsg)
	default:
		log.Debug().
			Int64("game_id", gameID).
			Str("client_id", clientID).
			Str("message_type", clientMsg.Type).
			Msg("Received unsupported WebSocket client message")
		h.writeErrorMessage(conn, gameID, clientID, websocketErrorUnsupportedType, "unsupported websocket message type")
	}
}

// RegisterRoutes registers the public WebSocket surface used by the frontend:
// `/ws/games` stays broadcast-only, while `/ws/games/:gameId` supports the
// same server envelopes plus `sync_request` -> `sync_response` reconciliation.
func (h *GameWebSocketHandler) RegisterRoutes(app *fiber.App) {
	// Global WebSocket endpoint for all game updates: /ws/games
	app.Get(websocketGlobalRoute, h.GlobalUpgradeCheck, websocket.New(h.HandleGlobalConnection))

	// Game-specific WebSocket upgrade endpoint: /ws/games/:gameId
	app.Get(websocketPerGameRoute, h.UpgradeCheck, websocket.New(h.HandleConnection))

	log.Info().
		Str("global_route", websocketGlobalRoute).
		Str("per_game_route", websocketPerGameRoute).
		Msg("WebSocket routes registered")
}

// GlobalUpgradeCheck validates WebSocket upgrade requests for global subscription
func (h *GameWebSocketHandler) GlobalUpgradeCheck(c *fiber.Ctx) error {
	// Validate WebSocket upgrade headers
	if !websocket.IsWebSocketUpgrade(c) {
		return fiber.ErrUpgradeRequired
	}

	// No game ID validation needed for global subscription
	return c.Next()
}

// HandleGlobalConnection handles WebSocket connection for global game updates
func (h *GameWebSocketHandler) HandleGlobalConnection(c *websocket.Conn) {
	ctx := context.Background()
	clientID := h.openGlobalConnection(ctx, c)

	defer h.broadcastUseCase.Unsubscribe(ctx, 0, clientID)

	ticker, done, err := h.prepareConnection(c, func() error {
		log.Debug().
			Str("client_id", clientID).
			Msg("Received pong from global client")

		return c.SetReadDeadline(time.Now().Add(h.pongWait))
	})
	if err != nil {
		log.Error().
			Err(err).
			Str("client_id", clientID).
			Msg("Failed to set initial read deadline for global connection")
		return
	}
	defer ticker.Stop()

	startPingLoop(
		c,
		ticker,
		done,
		func(err error) {
			log.Warn().
				Err(err).
				Str("client_id", clientID).
				Msg("Failed to send ping to global client, closing connection")
		},
		func() {
			log.Debug().
				Str("client_id", clientID).
				Msg("Sent ping to global client")
		},
	)

	readConnectionMessages(c, func(messageType int, message []byte) {
		h.handleGlobalClientMessage(c, clientID, messageType, message)
	},
		func(err error) {
			log.Warn().
				Err(err).
				Str("client_id", clientID).
				Msg("Global WebSocket connection closed unexpectedly")
		},
		func() {
			log.Info().
				Str("client_id", clientID).
				Msg("Global WebSocket connection closed normally")
		})

	log.Info().
		Str("client_id", clientID).
		Msg("Global WebSocket connection handler completed")
}

// handleGlobalClientMessage processes messages received from global WebSocket client
func (h *GameWebSocketHandler) handleGlobalClientMessage(conn *websocket.Conn, clientID string, messageType int, message []byte) {
	if !isTextWebSocketMessage(messageType) {
		log.Debug().
			Str("client_id", clientID).
			Int("message_type", messageType).
			Msg("Ignoring non-text global WebSocket client message")
		return
	}

	clientMsg, err := parseClientMessage(message)
	if err != nil {
		log.Warn().
			Err(err).
			Str("client_id", clientID).
			Msg("Failed to parse global WebSocket client message")
		if writeErr := sendWebSocketError(conn, websocketErrorInvalidJSON, "message must be valid JSON"); writeErr != nil {
			log.Warn().
				Err(writeErr).
				Str("client_id", clientID).
				Msg("Failed to send global WebSocket error message")
		}
		return
	}

	log.Debug().
		Str("client_id", clientID).
		Str("message_type", clientMsg.Type).
		Msg("Received unsupported message from global WebSocket client")
	if err := sendWebSocketError(conn, websocketErrorUnsupportedType, "global websocket is broadcast-only"); err != nil {
		log.Warn().
			Err(err).
			Str("client_id", clientID).
			Msg("Failed to send global unsupported-message error")
	}
}

func (h *GameWebSocketHandler) handleSyncRequest(ctx context.Context, conn *websocket.Conn, gameID int64, clientID string, clientMsg *clientMessage) {
	game, err := h.gameRepo.GetByID(ctx, gameID)
	if err != nil || game == nil {
		log.Warn().
			Err(err).
			Int64("game_id", gameID).
			Str("client_id", clientID).
			Msg("Failed to load game for WebSocket sync response")
		h.writeErrorMessage(conn, gameID, clientID, websocketErrorSyncUnavailable, "unable to load current game state")
		return
	}

	if err := writeServerMessage(conn, newSyncResponseMessage(game)); err != nil {
		log.Warn().
			Err(err).
			Int64("game_id", gameID).
			Str("client_id", clientID).
			Msg("Failed to send WebSocket sync response")
		return
	}

	log.Debug().
		Int64("game_id", gameID).
		Str("client_id", clientID).
		Str("last_event_timestamp", clientMsg.LastEventTimestamp).
		Msg("Sent WebSocket sync response")
}

func (h *GameWebSocketHandler) logParseError(gameID int64, clientID string, err error) {
	log.Warn().
		Err(err).
		Int64("game_id", gameID).
		Str("client_id", clientID).
		Msg("Failed to parse WebSocket client message")
}

func (h *GameWebSocketHandler) writeErrorMessage(conn *websocket.Conn, gameID int64, clientID, code, message string) {
	if err := sendWebSocketError(conn, code, message); err != nil {
		log.Warn().
			Err(err).
			Int64("game_id", gameID).
			Str("client_id", clientID).
			Str("error_code", code).
			Msg("Failed to send WebSocket error message")
	}
}

// GetConnectionCount returns the number of active WebSocket connections
func (h *GameWebSocketHandler) GetConnectionCount() int {
	return h.broadcastUseCase.GetActiveConnectionCount()
}

// Shutdown gracefully closes all WebSocket connections
func (h *GameWebSocketHandler) Shutdown() error {
	log.Info().Msg("Shutting down WebSocket handler")
	return h.broadcastUseCase.CloseAllConnections(context.Background())
}
