package telegram

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
)

// contextKey is the type for context keys to avoid collisions
type contextKey string

const (
	// UserIDKey is the context key for storing authenticated Telegram user ID
	UserIDKey contextKey = "telegram_user_id"
	// UsernameKey is the context key for storing authenticated Telegram username
	UsernameKey contextKey = "telegram_username"
)

// AuthMiddleware creates a Fiber middleware that validates Telegram Mini App authentication.
// Reads X-Telegram-Init-Data header, validates HMAC signature, and injects user data into context.
//
// Usage:
//
//	authMiddleware := telegram.AuthMiddleware(telegram.AuthConfig{
//	    BotToken: "your_bot_token",
//	    MaxAge:   86400,
//	})
//	app.Use("/api/v1/users", authMiddleware)
func AuthMiddleware(config AuthConfig) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Extract initData from header
		initData := c.Get("X-Telegram-Init-Data")
		if initData == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error":   "unauthorized",
				"message": "Missing X-Telegram-Init-Data header",
			})
		}

		// Validate initData
		data, err := ValidateInitData(initData, config)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error":   "unauthorized",
				"message": fmt.Sprintf("Invalid Telegram authentication: %v", err),
			})
		}

		// Extract user information
		userID, username, err := ParseUserData(data)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error":   "unauthorized",
				"message": fmt.Sprintf("Failed to parse user data: %v", err),
			})
		}

		// Inject user data into context
		c.Locals(UserIDKey, userID)
		c.Locals(UsernameKey, username)

		return c.Next()
	}
}

// OptionalAuthMiddleware is similar to AuthMiddleware but doesn't return error if auth fails.
// Instead, it just doesn't set user data in context. Useful for endpoints that work both
// authenticated and unauthenticated.
func OptionalAuthMiddleware(config AuthConfig) fiber.Handler {
	return func(c *fiber.Ctx) error {
		initData := c.Get("X-Telegram-Init-Data")
		if initData == "" {
			return c.Next()
		}

		data, err := ValidateInitData(initData, config)
		if err != nil {
			// Just continue without auth
			return c.Next()
		}

		userID, username, err := ParseUserData(data)
		if err != nil {
			return c.Next()
		}

		c.Locals(UserIDKey, userID)
		c.Locals(UsernameKey, username)

		return c.Next()
	}
}

// GetUserIDFromContext extracts Telegram user ID from Fiber context.
// Returns 0 if not authenticated.
func GetUserIDFromContext(c *fiber.Ctx) int64 {
	userID, ok := c.Locals(UserIDKey).(int64)
	if !ok {
		return 0
	}
	return userID
}

// GetUsernameFromContext extracts Telegram username from Fiber context.
// Returns empty string if not authenticated.
func GetUsernameFromContext(c *fiber.Ctx) string {
	username, ok := c.Locals(UsernameKey).(string)
	if !ok {
		return ""
	}
	return username
}
