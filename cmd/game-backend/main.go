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
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/valyala/fasthttp/fasthttpadaptor"

	"pod-backend/internal/controller/rest"
	websocketctrl "pod-backend/internal/controller/websocket"
	repopg "pod-backend/internal/repository/postgres"
	"pod-backend/internal/usecase"
	"pod-backend/pkg/postgres"
)

// Prometheus metrics (T059)
var (
	wsActiveConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "websocket_active_connections",
		Help: "Number of active WebSocket connections",
	})

	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
)

func main() {
	// Initialize structured logger (T060)
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	log.Info().Msg("Starting Game Backend Service")

	// Load configuration from environment
	cfg := loadConfig()

	// Initialize PostgreSQL connection pool
	pg, err := initDatabase(cfg.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize database")
	}
	defer pg.Close()

	log.Info().Msg("Database connection pool initialized")

	// Initialize repositories
	gameRepo := repopg.NewGameRepository(pg)
	userRepo := repopg.NewUserRepository(pg)
	_ = gameRepo // Will be used for queries

	// Initialize use cases
	gameQueryUC := usecase.NewGameQueryUseCase(gameRepo)
	gameBroadcastUC := usecase.NewGameBroadcastUseCase()
	userManagementUC := usecase.NewUserManagementUseCase(userRepo)

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
		AllowOrigins:     cfg.CORSAllowedOrigins,
		AllowMethods:     "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders:     "Origin,Content-Type,Accept,Authorization,X-Telegram-Init-Data",
		AllowCredentials: true,
		MaxAge:           3600,
	}))

	// Prometheus metrics middleware
	app.Use(prometheusMiddleware)

	// Health check endpoint
	healthHandler := rest.NewHealthHandler(pg.Pool, &log.Logger)
	app.Get("/health", healthHandler.GetHealth)

	// Prometheus metrics endpoint
	app.Get("/metrics", func(c *fiber.Ctx) error {
		handler := fasthttpadaptor.NewFastHTTPHandler(promhttp.Handler())
		handler(c.Context())
		return nil
	})

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

	// Graceful shutdown handler (T061)
	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, os.Interrupt, syscall.SIGTERM)

	// Start server in goroutine
	go func() {
		addr := fmt.Sprintf(":%s", cfg.Port)
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

// Config holds application configuration
type Config struct {
	Port               string
	DatabaseURL        string
	CORSAllowedOrigins string
}

// loadConfig loads configuration from environment variables
func loadConfig() *Config {
	return &Config{
		Port:               getEnv("PORT", "3000"),
		DatabaseURL:        getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/gamedb?sslmode=disable"),
		CORSAllowedOrigins: getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000,http://localhost:5173"),
	}
}

// getEnv gets environment variable with default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// initDatabase initializes PostgreSQL connection using pkg/postgres
func initDatabase(dbURL string) (*postgres.Postgres, error) {
	pg, err := postgres.New(dbURL, postgres.MaxPoolSize(25))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize postgres: %w", err)
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := pg.Pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return pg, nil
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

// prometheusMiddleware tracks HTTP request metrics
func prometheusMiddleware(c *fiber.Ctx) error {
	start := time.Now()
	path := c.Path()
	method := c.Method()

	// Process request
	err := c.Next()

	// Record metrics
	duration := time.Since(start).Seconds()
	status := fmt.Sprintf("%d", c.Response().StatusCode())

	httpRequestsTotal.WithLabelValues(method, path, status).Inc()
	httpRequestDuration.WithLabelValues(method, path).Observe(duration)

	return err
}

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
