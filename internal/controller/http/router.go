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
	"pod-backend/internal/infrastructure/ratelimit"
	"pod-backend/internal/infrastructure/telegram"
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

	// Register custom validators (FR-011, T107)
	middleware.RegisterCustomValidators()

	// CORS middleware (must be applied BEFORE routes)
	corsConfig := corsmw.Config{
		AllowedOrigins: cfg.GameBackend.CORSAllowedOrigins,
	}
	app.Use(corsmw.New(corsConfig))

	// Options
	app.Use(middleware.Logger(l))
	app.Use(middleware.Recovery(l))

	// Rate limiting (FR-018: 100 req/min per user)
	// Skips /health, /metrics, /swagger automatically
	app.Use(ratelimit.New())

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
	userManagementUC := usecase.NewUserManagementUseCase(userRepo)

	// Initialize handlers
	gameHandler := rest.NewGameHandler(gameQueryUC, &zerologger)
	userHandler := rest.NewUserHandler(userManagementUC, gameQueryUC, zerologger)
	// TODO: Pass actual TON Center client when blockchain polling is integrated
	healthHandler := rest.NewHealthHandler(pg.Pool, &zerologger, nil)

	// Telegram auth middleware (optional for now - will be required when TMA is integrated)
	// For now, user endpoints are accessible without authentication to enable testing
	telegramAuth := telegram.OptionalAuthMiddleware(telegram.AuthConfig{
		BotToken: cfg.GameBackend.TelegramBotToken,
		MaxAge:   86400, // 24 hours
	})

	// API v1 routes
	apiV1Group := app.Group("/api/v1")
	{
		// Legacy translation routes
		v1.NewTranslationRoutes(apiV1Group, t, l)

		// Game routes
		apiV1Group.Get("/games", gameHandler.ListGames)
		apiV1Group.Get("/games/:gameId", gameHandler.GetGameByID)

		// User routes (FR-003: User profiles, FR-006: Game history, FR-021: Referral stats)
		// Uses optional auth for now - will switch to required auth when TMA is ready
		userRoutes := apiV1Group.Group("/users", telegramAuth)
		{
			userRoutes.Get("/:address", userHandler.GetUserProfile)
			userRoutes.Get("/:address/history", userHandler.GetUserGameHistory)
			userRoutes.Get("/:address/referrals", userHandler.GetReferralStats)
		}

		// Health route
		apiV1Group.Get("/health", healthHandler.GetHealth)
	}
}
