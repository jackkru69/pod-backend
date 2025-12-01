package cors

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

// Config holds CORS middleware configuration
type Config struct {
	// AllowedOrigins is a comma-separated list of allowed origins
	// Example: "https://t.me,http://localhost:3000"
	AllowedOrigins string
}

// New creates a new CORS middleware with the given configuration
func New(cfg Config) fiber.Handler {
	// Parse allowed origins from comma-separated string
	origins := parseOrigins(cfg.AllowedOrigins)

	// Default to allowing all origins if none specified
	if len(origins) == 0 {
		origins = []string{"*"}
	}

	return cors.New(cors.Config{
		AllowOrigins: strings.Join(origins, ","),
		AllowMethods: strings.Join([]string{
			fiber.MethodGet,
			fiber.MethodPost,
			fiber.MethodPut,
			fiber.MethodDelete,
			fiber.MethodPatch,
			fiber.MethodOptions,
		}, ","),
		AllowHeaders: strings.Join([]string{
			"Origin",
			"Content-Type",
			"Accept",
			"Authorization",
			"X-Telegram-Init-Data",
		}, ","),
		AllowCredentials: true,
		MaxAge:           86400, // 24 hours
	})
}

// parseOrigins parses a comma-separated list of origins
func parseOrigins(originsStr string) []string {
	if originsStr == "" {
		return []string{}
	}

	parts := strings.Split(originsStr, ",")
	origins := make([]string, 0, len(parts))

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			origins = append(origins, trimmed)
		}
	}

	return origins
}
