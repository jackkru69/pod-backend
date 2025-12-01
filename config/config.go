package config

import (
	"fmt"

	"github.com/caarlos0/env/v11"
)

type (
	// Config -.
	Config struct {
		App         App
		HTTP        HTTP
		Log         Log
		PG          PG
		GRPC        GRPC
		RMQ         RMQ
		NATS        NATS
		Metrics     Metrics
		Swagger     Swagger
		GameBackend GameBackend
	}

	// App -.
	App struct {
		Name    string `env:"APP_NAME,required"`
		Version string `env:"APP_VERSION,required"`
	}

	// HTTP -.
	HTTP struct {
		Port           string `env:"HTTP_PORT,required"`
		UsePreforkMode bool   `env:"HTTP_USE_PREFORK_MODE" envDefault:"false"`
	}

	// Log -.
	Log struct {
		Level string `env:"LOG_LEVEL,required"`
	}

	// PG -.
	PG struct {
		PoolMax int    `env:"PG_POOL_MAX,required"`
		URL     string `env:"PG_URL,required"`
	}

	// GRPC -.
	GRPC struct {
		Port string `env:"GRPC_PORT,required"`
	}

	// RMQ -.
	RMQ struct {
		ServerExchange string `env:"RMQ_RPC_SERVER,required"`
		ClientExchange string `env:"RMQ_RPC_CLIENT,required"`
		URL            string `env:"RMQ_URL,required"`
	}

	// NATS -.
	NATS struct {
		ServerExchange string `env:"NATS_RPC_SERVER,required"`
		URL            string `env:"NATS_URL,required"`
	}

	// Metrics -.
	Metrics struct {
		Enabled bool `env:"METRICS_ENABLED" envDefault:"true"`
	}

	// Swagger -.
	Swagger struct {
		Enabled bool `env:"SWAGGER_ENABLED" envDefault:"false"`
	}

	// GameBackend - Game backend specific configuration
	GameBackend struct {
		TONCenterV2URL        string `env:"TON_CENTER_V2_URL" envDefault:"http://localhost:8082"`
		TONCenterV3WSURL      string `env:"TON_CENTER_V3_WS_URL" envDefault:"ws://localhost:8081/api/v3/websocket"`
		TONGameContractAddr   string `env:"TON_GAME_CONTRACT_ADDRESS"`
		HTTPPort              string `env:"GAME_BACKEND_HTTP_PORT" envDefault:"3000"`
		Environment           string `env:"GAME_BACKEND_ENV" envDefault:"development"`
		RateLimitRequests     int    `env:"RATE_LIMIT_REQUESTS" envDefault:"100"`
		RateLimitWindow       string `env:"RATE_LIMIT_WINDOW" envDefault:"1m"`
		CircuitBreakerMaxFail int    `env:"CIRCUIT_BREAKER_MAX_FAILURES" envDefault:"5"`
		CircuitBreakerTimeout string `env:"CIRCUIT_BREAKER_TIMEOUT" envDefault:"60s"`
		TelegramBotToken      string `env:"TELEGRAM_BOT_TOKEN"`
		CORSAllowedOrigins    string `env:"CORS_ALLOWED_ORIGINS" envDefault:"http://localhost:3001"`
	}
)

// NewConfig returns app config.
func NewConfig() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("config error: %w", err)
	}

	return cfg, nil
}
