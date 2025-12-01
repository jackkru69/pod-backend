package ratelimit

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ulule/limiter/v3"
	"github.com/ulule/limiter/v3/drivers/store/memory"
)

// Config holds rate limiter configuration.
type Config struct {
	// Rate limit per user (e.g., "100-M" for 100 requests per minute)
	Rate string
	// Header name to extract user identifier (default: "X-Telegram-User-Id")
	UserIDHeader string
}

// DefaultConfig returns default rate limiter configuration (FR-018).
// 100 requests per minute per user as per research.md.
func DefaultConfig() Config {
	return Config{
		Rate:         "100-M", // 100 requests per minute
		UserIDHeader: "X-Telegram-User-Id",
	}
}

// New creates a new rate limiter middleware.
// Implements FR-018: per-user rate limiting to prevent abuse.
func New(config ...Config) fiber.Handler {
	cfg := DefaultConfig()
	if len(config) > 0 {
		cfg = config[0]
	}

	// Parse rate limit
	rate, err := limiter.NewRateFromFormatted(cfg.Rate)
	if err != nil {
		panic(fmt.Sprintf("invalid rate limit format: %v", err))
	}

	// Create in-memory store
	store := memory.NewStore()

	// Create limiter instance
	instance := limiter.New(store, rate)

	return func(c *fiber.Ctx) error {
		// Skip rate limiting for health check and documentation
		if c.Path() == "/health" || c.Path() == "/metrics" ||
			c.Path() == "/swagger" || c.Path() == "/api/docs" ||
			len(c.Path()) > 8 && c.Path()[:8] == "/swagger" {
			return c.Next()
		}

		// Extract user identifier from header
		userID := c.Get(cfg.UserIDHeader)
		if userID == "" {
			// If no user ID, use IP address as fallback
			userID = c.IP()
		}

		// Get rate limit context for this user
		limiterCtx, err := instance.Get(c.Context(), userID)
		if err != nil {
			// Log error but don't block request
			c.Locals("rate_limit_error", err.Error())
			return c.Next()
		}

		// Set rate limit headers
		c.Set("X-RateLimit-Limit", fmt.Sprintf("%d", limiterCtx.Limit))
		c.Set("X-RateLimit-Remaining", fmt.Sprintf("%d", limiterCtx.Remaining))
		c.Set("X-RateLimit-Reset", fmt.Sprintf("%d", limiterCtx.Reset))

		// Check if rate limit exceeded
		if limiterCtx.Reached {
			// Log rate limit violation (FR-018: structured logging)
			c.Locals("rate_limit_exceeded", true)
			c.Locals("user_identifier", userID)

			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "rate_limit_exceeded",
				"message": fmt.Sprintf(
					"Rate limit exceeded. Try again in %s",
					time.Until(time.Unix(limiterCtx.Reset, 0)).Round(time.Second),
				),
				"retry_after": limiterCtx.Reset,
			})
		}

		return c.Next()
	}
}
