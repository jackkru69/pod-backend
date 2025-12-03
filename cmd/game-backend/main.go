package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/swagger"

	_ "pod-backend/docs" // Swagger docs

	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/valyala/fasthttp/fasthttpadaptor"

	"pod-backend/config"
	blockchainctrl "pod-backend/internal/controller/blockchain"
	"pod-backend/internal/controller/rest"
	websocketctrl "pod-backend/internal/controller/websocket"
	"pod-backend/internal/infrastructure/metrics"
	"pod-backend/internal/infrastructure/toncenter"
	repopg "pod-backend/internal/repository/postgres"
	"pod-backend/internal/usecase"
	pkglogger "pod-backend/pkg/logger"
	"pod-backend/pkg/postgres"
)

// Prometheus metrics (T059, T121)
// Note: HTTP metrics are defined in internal/infrastructure/metrics/middleware.go
var (
	wsActiveConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "websocket_active_connections",
		Help: "Number of active WebSocket connections",
	})

	// Database connection pool metrics (T121)
	dbConnectionsActive = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "db_connections_active",
		Help: "Number of active database connections in use",
	})

	dbConnectionsIdle = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "db_connections_idle",
		Help: "Number of idle database connections in pool",
	})

	dbConnectionsTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "db_connections_total",
		Help: "Total number of database connections in pool",
	})
)

func main() {
	// Initialize structured logger (T060)
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	log.Info().Msg("Starting Game Backend Service")

	// Load .env file (ignore error if file doesn't exist)
	if err := godotenv.Load(); err != nil {
		log.Warn().Msg("No .env file found, using environment variables")
	}

	// Load configuration from environment
	appCfg, err := config.NewConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	// Initialize structured logger with config level
	l := pkglogger.New(appCfg.Log.Level)

	// Initialize PostgreSQL connection pool
	pg, err := postgres.New(appCfg.PG.URL, postgres.MaxPoolSize(appCfg.PG.PoolMax))
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize database")
	}
	defer pg.Close()

	log.Info().Msg("Database connection pool initialized")

	// Initialize repositories
	gameRepo := repopg.NewGameRepository(pg)
	userRepo := repopg.NewUserRepository(pg)
	eventRepo := repopg.NewGameEventRepository(pg)
	syncStateRepo := repopg.NewBlockchainSyncStateRepository(pg)

	// Initialize use cases
	gameQueryUC := usecase.NewGameQueryUseCase(gameRepo)
	gameBroadcastUC := usecase.NewGameBroadcastUseCase()
	userManagementUC := usecase.NewUserManagementUseCase(userRepo)

	// Initialize TON Center client for blockchain monitoring and health checks
	circuitBreakerTimeout, err := time.ParseDuration(appCfg.GameBackend.CircuitBreakerTimeout)
	if err != nil {
		circuitBreakerTimeout = 60 * time.Second
		log.Warn().Err(err).Msg("Failed to parse circuit breaker timeout, using default 60s")
	}

	var tonClient *toncenter.Client
	if appCfg.GameBackend.TONGameContractAddr != "" {
		tonClient = toncenter.NewClient(toncenter.ClientConfig{
			V2BaseURL:             appCfg.GameBackend.TONCenterV2URL,
			ContractAddress:       appCfg.GameBackend.TONGameContractAddr,
			CircuitBreakerMaxFail: appCfg.GameBackend.CircuitBreakerMaxFail,
			CircuitBreakerTimeout: circuitBreakerTimeout,
			HTTPTimeout:           30 * time.Second,
		})
		log.Info().Str("contract", appCfg.GameBackend.TONGameContractAddr).Msg("TON Center client initialized")
	}

	// Initialize blockchain persistence use case (T093)
	gamePersistenceUC := usecase.NewGamePersistenceUseCase(gameRepo, eventRepo, userRepo)
	gamePersistenceUC.SetBroadcastUseCase(gameBroadcastUC) // Wire WebSocket broadcasting

	// Initialize blockchain metrics (T097)
	blockchainMetrics := metrics.NewBlockchainMetrics()

	// Initialize blockchain event handler (T094, T095)
	blockchainHandler, err := blockchainctrl.NewTONEventHandler(appCfg, gamePersistenceUC, l)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize blockchain event handler")
	}

	// Load last processed lt from database to resume from saved state
	ctx := context.Background()
	lastLt, err := syncStateRepo.GetLastProcessedLt(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to load last processed lt from database, starting from 0")
		lastLt = "0"
	}
	if lastLt != "0" {
		log.Info().Str("lt", lastLt).Msg("Resuming blockchain subscription from saved state")
		blockchainHandler.SetLastProcessedLt(lastLt)
	}

	// Set callback to persist lt updates to database
	blockchainHandler.SetOnLtUpdated(func(lt string) {
		if err := syncStateRepo.UpdateLastProcessedLt(ctx, lt); err != nil {
			log.Error().Err(err).Str("lt", lt).Msg("Failed to persist last processed lt")
		}
	})

	// Wire metrics into blockchain subscriber (T097)
	blockchainHandler.SetMetrics(blockchainMetrics)

	// Initialize Fiber app
	app := fiber.New(fiber.Config{
		AppName:               "Game Backend v1.0",
		DisableStartupMessage: false,
		ErrorHandler:          customErrorHandler,
		ReadTimeout:           10 * time.Second,
		WriteTimeout:          10 * time.Second,
		IdleTimeout:           120 * time.Second,
	})

	// Middleware
	app.Use(recover.New())
	app.Use(logger.New())

	// CORS middleware (FR-027)
	app.Use(cors.New(cors.Config{
		AllowOrigins:     appCfg.GameBackend.CORSAllowedOrigins,
		AllowMethods:     "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders:     "Origin,Content-Type,Accept,Authorization,X-Telegram-Init-Data",
		AllowCredentials: true,
		MaxAge:           3600,
	}))

	// Prometheus metrics middleware
	app.Use(metrics.New())

	// Health check endpoint
	healthHandler := rest.NewHealthHandler(pg.Pool, &log.Logger, tonClient)
	// T162: Wire up event source provider for health reporting
	healthHandler.SetEventSourceProvider(blockchainHandler)
	app.Get("/health", healthHandler.GetHealth)

	// Prometheus metrics endpoint
	app.Get("/metrics", func(c *fiber.Ctx) error {
		handler := fasthttpadaptor.NewFastHTTPHandler(promhttp.Handler())
		handler(c.Context())
		return nil
	})

	// Swagger documentation (FR-024, FR-025)
	if appCfg.Swagger.Enabled {
		app.Get("/swagger/*", swagger.HandlerDefault)
		log.Info().Msg("Swagger UI available at /swagger/index.html")
	}

	// REST API routes
	apiV1 := app.Group("/api/v1")

	// Game endpoints
	gameHandler := rest.NewGameHandler(gameQueryUC, &log.Logger)
	apiV1.Get("/games", gameHandler.ListGames)
	apiV1.Get("/games/:gameId", gameHandler.GetGameByID)

	// User endpoints (T074)
	userHandler := rest.NewUserHandler(userManagementUC, gameQueryUC, log.Logger)
	apiV1.Get("/users/:address", userHandler.GetUserProfile)
	apiV1.Get("/users/:address/history", userHandler.GetUserGameHistory)
	apiV1.Get("/users/:address/referrals", userHandler.GetReferralStats)

	// WebSocket routes (T058)
	wsHandler := websocketctrl.NewGameWebSocketHandler(gameRepo, gameBroadcastUC)
	wsHandler.RegisterRoutes(app)

	log.Info().Msg("All routes registered successfully")

	// Start WebSocket connection count updater (T059)
	go updateWebSocketMetrics(gameBroadcastUC)

	// Start database connection pool metrics updater (T121)
	go updateDatabaseMetrics(pg)

	// Start blockchain event subscription (T095)
	if err := blockchainHandler.Start(); err != nil {
		log.Fatal().Err(err).Msg("Failed to start blockchain event handler")
	}
	log.Info().Msg("Blockchain event subscription started")

	// Graceful shutdown handler (T061)
	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, os.Interrupt, syscall.SIGTERM)

	// Start server in goroutine
	go func() {
		addr := fmt.Sprintf(":%s", appCfg.HTTP.Port)
		log.Info().Str("address", addr).Msg("Starting HTTP server")
		if err := app.Listen(addr); err != nil {
			log.Fatal().Err(err).Msg("Failed to start server")
		}
	}()

	// Wait for shutdown signal
	sig := <-shutdownChan
	log.Info().Str("signal", sig.String()).Msg("Shutdown signal received")

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop blockchain event subscription (T095)
	log.Info().Msg("Stopping blockchain event subscription")
	if err := blockchainHandler.Stop(); err != nil {
		log.Error().Err(err).Msg("Error during blockchain handler shutdown")
	}

	// Close all WebSocket connections (T061)
	log.Info().Msg("Closing all WebSocket connections")
	if err := wsHandler.Shutdown(); err != nil {
		log.Error().Err(err).Msg("Error during WebSocket shutdown")
	}

	// Shutdown HTTP server
	log.Info().Msg("Shutting down HTTP server")
	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("Error during server shutdown")
	}

	log.Info().Msg("Game Backend Service stopped gracefully")
}

// customErrorHandler handles Fiber errors
func customErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	message := "Internal Server Error"

	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
		message = e.Message
	}

	log.Error().
		Err(err).
		Int("status", code).
		Str("path", c.Path()).
		Str("method", c.Method()).
		Msg("Request error")

	return c.Status(code).JSON(fiber.Map{
		"error":   http.StatusText(code),
		"message": message,
	})
}

// prometheusMiddleware is now provided by internal/infrastructure/metrics package
// See metrics.New() for implementation

// updateWebSocketMetrics periodically updates WebSocket connection count metric (T059)
func updateWebSocketMetrics(broadcastUC *usecase.GameBroadcastUseCase) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		count := broadcastUC.GetActiveConnectionCount()
		wsActiveConnections.Set(float64(count))

		log.Debug().
			Int("active_connections", count).
			Msg("Updated WebSocket metrics")
	}
}

// updateDatabaseMetrics periodically updates database connection pool metrics (T121)
func updateDatabaseMetrics(pg *postgres.Postgres) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		stats := pg.Pool.Stat()

		// Update Prometheus metrics
		dbConnectionsActive.Set(float64(stats.AcquiredConns()))
		dbConnectionsIdle.Set(float64(stats.IdleConns()))
		dbConnectionsTotal.Set(float64(stats.TotalConns()))

		log.Debug().
			Int32("active", stats.AcquiredConns()).
			Int32("idle", stats.IdleConns()).
			Int32("total", stats.TotalConns()).
			Int32("max", stats.MaxConns()).
			Msg("Updated database connection pool metrics")
	}
}
