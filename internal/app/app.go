// Package app configures and runs application.
package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"pod-backend/config"
	blockchainctrl "pod-backend/internal/controller/blockchain"
	"pod-backend/internal/controller/http"
	websocketctrl "pod-backend/internal/controller/websocket"
	"pod-backend/internal/infrastructure/metrics"
	"pod-backend/internal/infrastructure/toncenter"
	repopg "pod-backend/internal/repository/postgres"
	"pod-backend/internal/usecase"
	"pod-backend/pkg/httpserver"
	"pod-backend/pkg/logger"
	"pod-backend/pkg/postgres"
)

// Prometheus metrics (T059, T121)
var (
	wsActiveConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "websocket_active_connections",
		Help: "Number of active WebSocket connections",
	})

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

// Run creates objects via constructors and starts the application.
func Run(cfg *config.Config) { //nolint: gocyclo,cyclop,funlen,gocritic,nolintlint
	// Initialize structured logger (T060)
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	log.Info().Msg("Starting Game Backend Service")

	l := logger.New(cfg.Log.Level)

	// Initialize PostgreSQL connection pool
	pg, err := postgres.New(cfg.PG.URL, postgres.MaxPoolSize(cfg.PG.PoolMax))
	if err != nil {
		l.Fatal(fmt.Errorf("app - Run - postgres.New: %w", err))
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

	// Initialize reservation use case
	reservationCfg := usecase.ReservationConfig{
		MaxPerWallet:           cfg.Reservation.MaxPerWallet,
		TimeoutSeconds:         cfg.Reservation.TimeoutSeconds,
		CleanupIntervalSeconds: cfg.Reservation.CleanupIntervalSeconds,
	}
	reservationUC := usecase.NewReservationUseCase(gameRepo, gameBroadcastUC, reservationCfg)
	reservationUC.StartCleanupLoop(context.Background())

	// Initialize TON Center client for blockchain monitoring and health checks
	circuitBreakerTimeout, err := time.ParseDuration(cfg.GameBackend.CircuitBreakerTimeout)
	if err != nil {
		circuitBreakerTimeout = 60 * time.Second
		log.Warn().Err(err).Msg("Failed to parse circuit breaker timeout, using default 60s")
	}

	var tonClient *toncenter.Client
	if cfg.GameBackend.TONGameContractAddr != "" {
		tonClient = toncenter.NewClient(toncenter.ClientConfig{
			V2BaseURL:             cfg.GameBackend.TONCenterV2URL,
			ContractAddress:       cfg.GameBackend.TONGameContractAddr,
			CircuitBreakerMaxFail: cfg.GameBackend.CircuitBreakerMaxFail,
			CircuitBreakerTimeout: circuitBreakerTimeout,
			HTTPTimeout:           30 * time.Second,
		})
		log.Info().Str("contract", cfg.GameBackend.TONGameContractAddr).Msg("TON Center client initialized")
	}

	// Initialize blockchain persistence use case (T093)
	gamePersistenceUC := usecase.NewGamePersistenceUseCase(gameRepo, eventRepo, userRepo)
	gamePersistenceUC.SetBroadcastUseCase(gameBroadcastUC) // Wire WebSocket broadcasting

	// Initialize blockchain metrics (T097)
	blockchainMetrics := metrics.NewBlockchainMetrics()

	// Initialize blockchain event handler (T094, T095)
	blockchainHandler, err := blockchainctrl.NewTONEventHandler(cfg, gamePersistenceUC, l)
	if err != nil {
		l.Fatal(fmt.Errorf("app - Run - blockchainctrl.NewTONEventHandler: %w", err))
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

	// Create router dependencies
	routerDeps := http.RouterDeps{
		Logger:            l,
		GameQueryUC:       gameQueryUC,
		ReservationUC:     reservationUC,
		UserManagementUC:  userManagementUC,
		BroadcastUC:       gameBroadcastUC,
		TONClient:         tonClient,
		BlockchainHandler: blockchainHandler,
		PG:                pg,
		GameRepo:          gameRepo,
	}

	// HTTP Server
	httpServer := httpserver.New(l, httpserver.Port(cfg.HTTP.Port))
	http.NewRouter(httpServer.App, cfg, routerDeps)

	// WebSocket routes (T058)
	wsHandler := websocketctrl.NewGameWebSocketHandler(gameRepo, gameBroadcastUC)
	wsHandler.RegisterRoutes(httpServer.App)

	log.Info().Msg("All routes registered successfully")

	// Start HTTP server
	httpServer.Start()

	// Start WebSocket connection count updater (T059)
	go updateWebSocketMetrics(gameBroadcastUC)

	// Start database connection pool metrics updater (T121)
	go updateDatabaseMetrics(pg)

	// Start blockchain event subscription (T095)
	if err := blockchainHandler.Start(); err != nil {
		l.Fatal(fmt.Errorf("app - Run - blockchainHandler.Start: %w", err))
	}
	log.Info().Msg("Blockchain event subscription started")

	// Waiting signal
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	select {
	case s := <-interrupt:
		l.Info("app - Run - signal: %s", s.String())
	case err = <-httpServer.Notify():
		l.Error(fmt.Errorf("app - Run - httpServer.Notify: %w", err))
	}

	// Graceful shutdown
	log.Info().Msg("Shutting down...")

	// Stop blockchain event subscription (T095)
	log.Info().Msg("Stopping blockchain event subscription")
	if err := blockchainHandler.Stop(); err != nil {
		l.Error(fmt.Errorf("app - Run - blockchainHandler.Stop: %w", err))
	}

	// Close all WebSocket connections (T061)
	log.Info().Msg("Closing all WebSocket connections")
	if err := wsHandler.Shutdown(); err != nil {
		l.Error(fmt.Errorf("app - Run - wsHandler.Shutdown: %w", err))
	}

	// Shutdown HTTP server
	log.Info().Msg("Shutting down HTTP server")
	if err := httpServer.Shutdown(); err != nil {
		l.Error(fmt.Errorf("app - Run - httpServer.Shutdown: %w", err))
	}

	log.Info().Msg("Game Backend Service stopped gracefully")
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
