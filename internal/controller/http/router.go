// Package http implements routing paths. Each service in own file.
package http

import (
	"net/http"

	"github.com/ansrivas/fiberprometheus/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/swagger"
	"github.com/rs/zerolog"

	"pod-backend/config"
	_ "pod-backend/docs" // Swagger docs.
	"pod-backend/internal/controller/http/middleware"
	v1 "pod-backend/internal/controller/http/v1"
	"pod-backend/internal/controller/rest"
	corsmw "pod-backend/internal/infrastructure/cors"
	postgresrepo "pod-backend/internal/repository/postgres"
	"pod-backend/internal/usecase"
	"pod-backend/pkg/logger"
	"pod-backend/pkg/postgres"
)

// NewRouter creates and configures the HTTP router
// Swagger spec:
// @title       POD Game Backend API
// @description Backend service for TON blockchain gambling game with real-time WebSocket updates
// @version     1.0.0
// @host        localhost:3000
// @BasePath    /api/v1
// @schemes     http https
//
// @contact.name   POD Game Team
//
// @license.name  BUSL-1.1
//
// @securityDefinitions.apikey TelegramAuth
// @in header
// @name X-Telegram-Init-Data
// @description Telegram Mini App authentication via initData string
func NewRouter(app *fiber.App, cfg *config.Config, t usecase.Translation, l logger.Interface, pg *postgres.Postgres) {
	// Create zerolog logger
	zerologger := zerolog.New(nil).Level(zerolog.InfoLevel)

	// CORS middleware (must be applied BEFORE routes)
	corsConfig := corsmw.Config{
		AllowedOrigins: cfg.GameBackend.CORSAllowedOrigins,
	}
	app.Use(corsmw.New(corsConfig))

	// Options
	app.Use(middleware.Logger(l))
	app.Use(middleware.Recovery(l))

	// Prometheus metrics
	if cfg.Metrics.Enabled {
		prometheus := fiberprometheus.New("pod-game-backend")
		prometheus.RegisterAt(app, "/metrics")
		app.Use(prometheus.Middleware)
	}

	// Swagger
	if cfg.Swagger.Enabled {
		app.Get("/swagger/*", swagger.HandlerDefault)
	}

	// K8s probe
	app.Get("/healthz", func(ctx *fiber.Ctx) error { return ctx.SendStatus(http.StatusOK) })

	// Initialize repositories
	userRepo := postgresrepo.NewUserRepository(pg)
	gameRepo := postgresrepo.NewGameRepository(pg)

	// Initialize use cases
	gameQueryUC := usecase.NewGameQueryUseCase(gameRepo)

	// Initialize handlers
	gameHandler := rest.NewGameHandler(gameQueryUC, &zerologger)
	healthHandler := rest.NewHealthHandler(pg.Pool, &zerologger)

	// API v1 routes
	apiV1Group := app.Group("/api/v1")
	{
		// Legacy translation routes
		v1.NewTranslationRoutes(apiV1Group, t, l)

		// Game routes
		apiV1Group.Get("/games", gameHandler.ListGames)
		apiV1Group.Get("/games/:gameId", gameHandler.GetGameByID)

		// Health route
		apiV1Group.Get("/health", healthHandler.GetHealth)
	}

	// Suppress unused variable warnings (remove when used)
	_ = userRepo
}
