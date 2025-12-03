package websocket

import (
	"context"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"pod-backend/internal/repository"
	"pod-backend/internal/usecase"
)

// GameWebSocketHandler handles WebSocket connections for game updates
type GameWebSocketHandler struct {
	gameRepo         repository.GameRepository
	broadcastUseCase *usecase.GameBroadcastUseCase
	pingInterval     time.Duration
	pongWait         time.Duration
}

// NewGameWebSocketHandler creates a new WebSocket handler
func NewGameWebSocketHandler(
	gameRepo repository.GameRepository,
	broadcastUseCase *usecase.GameBroadcastUseCase,
) *GameWebSocketHandler {
	return &GameWebSocketHandler{
		gameRepo:         gameRepo,
		broadcastUseCase: broadcastUseCase,
		pingInterval:     30 * time.Second, // Send ping every 30 seconds
		pongWait:         60 * time.Second, // Wait up to 60 seconds for pong
	}
}

// UpgradeCheck validates WebSocket upgrade requests
func (h *GameWebSocketHandler) UpgradeCheck(c *fiber.Ctx) error {
	// Validate WebSocket upgrade headers
	if !websocket.IsWebSocketUpgrade(c) {
		return fiber.ErrUpgradeRequired
	}

	// Validate game ID parameter
	gameIDStr := c.Params("id")
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

	// Get game ID from fiber context (set by UpgradeCheck)
	gameID, ok := c.Locals("gameID").(int64)
	if !ok {
		log.Error().Msg("Missing gameID in WebSocket connection locals")
		c.Close()
		return
	}

	// Generate unique client ID
	clientID := uuid.New().String()

	// Subscribe to game updates
	h.broadcastUseCase.Subscribe(ctx, gameID, clientID, c)
	defer h.broadcastUseCase.Unsubscribe(ctx, gameID, clientID)

	log.Info().
		Int64("game_id", gameID).
		Str("client_id", clientID).
		Str("remote_addr", c.RemoteAddr().String()).
		Msg("WebSocket connection established")

	// Set up ping/pong handlers
	c.SetPongHandler(func(appData string) error {
		log.Debug().
			Int64("game_id", gameID).
			Str("client_id", clientID).
			Msg("Received pong from client")

		// Extend read deadline on pong
		return c.SetReadDeadline(time.Now().Add(h.pongWait))
	})

	// Set initial read deadline
	if err := c.SetReadDeadline(time.Now().Add(h.pongWait)); err != nil {
		log.Error().
			Err(err).
			Int64("game_id", gameID).
			Str("client_id", clientID).
			Msg("Failed to set initial read deadline")
		return
	}

	// Start ping ticker
	ticker := time.NewTicker(h.pingInterval)
	defer ticker.Stop()

	// Channel to signal connection closure
	done := make(chan struct{})

	// Goroutine to send periodic pings
	go func() {
		defer close(done)
		for {
			select {
			case <-ticker.C:
				// Send ping
				if err := c.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(10*time.Second)); err != nil {
					log.Warn().
						Err(err).
						Int64("game_id", gameID).
						Str("client_id", clientID).
						Msg("Failed to send ping, closing connection")
					return
				}
				log.Debug().
					Int64("game_id", gameID).
					Str("client_id", clientID).
					Msg("Sent ping to client")
			case <-done:
				return
			}
		}
	}()

	// Read loop (to detect client disconnection and handle pongs)
	for {
		messageType, message, err := c.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Warn().
					Err(err).
					Int64("game_id", gameID).
					Str("client_id", clientID).
					Msg("WebSocket connection closed unexpectedly")
			} else {
				log.Info().
					Int64("game_id", gameID).
					Str("client_id", clientID).
					Msg("WebSocket connection closed normally")
			}
			break
		}

		// Handle client messages (if any)
		h.handleClientMessage(gameID, clientID, messageType, message)
	}

	log.Info().
		Int64("game_id", gameID).
		Str("client_id", clientID).
		Msg("WebSocket connection handler completed")
}

// handleClientMessage processes messages received from client
func (h *GameWebSocketHandler) handleClientMessage(gameID int64, clientID string, messageType int, message []byte) {
	// For now, we only broadcast server -> client
	// Client -> server messages can be added here if needed (e.g., manual refresh request)
	log.Debug().
		Int64("game_id", gameID).
		Str("client_id", clientID).
		Int("message_type", messageType).
		Int("message_size", len(message)).
		Msg("Received message from WebSocket client (no action)")
}

// RegisterRoutes registers WebSocket routes with Fiber app
func (h *GameWebSocketHandler) RegisterRoutes(app *fiber.App) {
	// WebSocket upgrade endpoint: /ws/games/:id
	app.Get("/ws/games/:id", h.UpgradeCheck, websocket.New(h.HandleConnection))

	log.Info().Msg("WebSocket routes registered: /ws/games/:id")
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
