// Package http implements routing paths. Each service in own file.
package http

import (
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/swagger"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/valyala/fasthttp/fasthttpadaptor"
	"pod-backend/config"
	_ "pod-backend/docs" // Swagger docs.
	blockchainctrl "pod-backend/internal/controller/blockchain"
	"pod-backend/internal/controller/http/middleware"
	"pod-backend/internal/controller/rest"
	corsmw "pod-backend/internal/infrastructure/cors"
	"pod-backend/internal/infrastructure/metrics"
	"pod-backend/internal/infrastructure/ratelimit"
	"pod-backend/internal/infrastructure/toncenter"
	"pod-backend/internal/repository"
	"pod-backend/internal/usecase"
	"pod-backend/pkg/logger"
	"pod-backend/pkg/postgres"
)

// RouterDeps contains all dependencies needed for the router
type RouterDeps struct {
	Logger              logger.Interface
	GameQueryUC         *usecase.GameQueryUseCase
	ReservationUC       *usecase.ReservationUseCase
	RevealReservationUC *usecase.RevealReservationUseCase
	UserManagementUC    *usecase.UserManagementUseCase
	BroadcastUC         *usecase.GameBroadcastUseCase
	TONClient           *toncenter.Client
	BlockchainHandler   *blockchainctrl.TONEventHandler
	SyncStateRepo       repository.BlockchainSyncStateRepository
	PG                  *postgres.Postgres
	GameRepo            repository.GameRepository
}

// NewRouter creates and configures the HTTP router
// Swagger spec:
// @title       POD Game Backend API
// @description Backend service for TON blockchain gambling game with real-time WebSocket updates
// @version     1.0.0
// @host        localhost:8090
// @BasePath    /
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
func NewRouter(app *fiber.App, cfg *config.Config, deps RouterDeps) {
	// Create zerolog logger for handlers
	zerologger := zerolog.New(nil).Level(zerolog.InfoLevel)

	// Register custom validators (FR-011, T107)
	middleware.RegisterCustomValidators()

	// CORS middleware (must be applied BEFORE routes)
	corsConfig := corsmw.Config{
		AllowedOrigins: cfg.GameBackend.CORSAllowedOrigins,
	}
	app.Use(corsmw.New(corsConfig))

	// Options
	app.Use(middleware.Logger(deps.Logger))
	app.Use(middleware.Recovery(deps.Logger))

	// Rate limiting (FR-018: 100 req/min per user)
	// Skips /health, /metrics, /swagger automatically
	app.Use(ratelimit.New())

	// Prometheus metrics middleware
	app.Use(metrics.New())

	// Prometheus metrics endpoint
	app.Get("/metrics", func(c *fiber.Ctx) error {
		handler := fasthttpadaptor.NewFastHTTPHandler(promhttp.Handler())
		handler(c.Context())
		return nil
	})

	// Swagger
	if cfg.Swagger.Enabled {
		app.Get("/swagger/*", swagger.HandlerDefault)
	}

	// K8s probe
	app.Get("/healthz", func(ctx *fiber.Ctx) error { return ctx.SendStatus(http.StatusOK) })

	// Health check endpoint
	healthHandler := rest.NewHealthHandler(deps.PG.Pool, &zerologger, deps.TONClient, deps.SyncStateRepo)
	// T162: Wire up event source provider for health reporting
	if deps.BlockchainHandler != nil {
		healthHandler.SetEventSourceProvider(deps.BlockchainHandler)
	}
	app.Get("/health", healthHandler.GetHealth)

	// API v1 routes
	apiV1Group := app.Group("/api/v1")
	{
		// Game routes
		gameHandler := rest.NewGameHandler(deps.GameQueryUC, deps.ReservationUC, &zerologger)
		apiV1Group.Get("/games", gameHandler.ListGames)
		apiV1Group.Get("/games/:gameId", gameHandler.GetGameByID)

		// Reservation endpoints
		reservationHandler := rest.NewReservationHandler(deps.ReservationUC, &zerologger)
		apiV1Group.Post("/games/:gameId/reserve", reservationHandler.ReserveGame)
		apiV1Group.Get("/games/:gameId/reservation", reservationHandler.GetReservation)
		apiV1Group.Delete("/games/:gameId/reserve", reservationHandler.CancelReservation)
		apiV1Group.Get("/reservations", reservationHandler.ListReservationsByWallet)

		// Reveal-phase reservation endpoints (spec 005-reveal-reservation)
		if deps.RevealReservationUC != nil {
			revealHandler := rest.NewRevealReservationHandler(deps.RevealReservationUC, &zerologger)
			apiV1Group.Post("/games/:gameId/reveal-reserve", revealHandler.ReserveReveal)
			apiV1Group.Get("/games/:gameId/reveal-reservation", revealHandler.GetRevealReservation)
			apiV1Group.Delete("/games/:gameId/reveal-reserve", revealHandler.CancelRevealReservation)
			apiV1Group.Post("/reveal-reserve", revealHandler.ReserveRevealBatch)
			apiV1Group.Get("/reveal-reservations", revealHandler.ListRevealReservationsByWallet)
		}

		// User routes (FR-003: User profiles, FR-006: Game history, FR-021: Referral stats)
		userHandler := rest.NewUserHandler(deps.UserManagementUC, deps.GameQueryUC, zerologger)
		apiV1Group.Get("/users/:address", userHandler.GetUserProfile)
		apiV1Group.Get("/users/:address/history", userHandler.GetUserGameHistory)
		apiV1Group.Get("/users/:address/referrals", userHandler.GetReferralStats)

		// Health route (API v1)
		apiV1Group.Get("/health", healthHandler.GetHealth)
	}
}
