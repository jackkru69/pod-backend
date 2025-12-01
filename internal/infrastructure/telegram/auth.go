package telegram

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

// AuthConfig holds Telegram bot authentication configuration.
type AuthConfig struct {
	BotToken string
	// MaxAge is the maximum age of initData in seconds (default: 86400 = 24 hours)
	MaxAge int64
}

// ValidateInitData validates Telegram Mini App initData using HMAC-SHA256 verification.
// Implements the validation algorithm from Telegram Mini Apps documentation.
// Returns user data if valid, error otherwise.
//
// Algorithm (per research.md §8):
// 1. Parse initData query string
// 2. Extract hash parameter
// 3. Create data_check_string from sorted key=value pairs (excluding hash)
// 4. Compute HMAC-SHA256 using bot token as key
// 5. Compare with provided hash
// 6. Verify auth_date is recent (within MaxAge)
func ValidateInitData(initData string, config AuthConfig) (map[string]string, error) {
	if initData == "" {
		return nil, fmt.Errorf("initData is empty")
	}

	// Parse query string
	values, err := url.ParseQuery(initData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse initData: %w", err)
	}

	// Extract hash
	providedHash := values.Get("hash")
	if providedHash == "" {
		return nil, fmt.Errorf("hash is missing from initData")
	}

	// Remove hash from values to create data_check_string
	values.Del("hash")

	// Create data_check_string: sorted key=value pairs joined with \n
	var pairs []string
	for key, vals := range values {
		if len(vals) > 0 {
			pairs = append(pairs, fmt.Sprintf("%s=%s", key, vals[0]))
		}
	}
	sort.Strings(pairs)
	dataCheckString := strings.Join(pairs, "\n")

	// Compute HMAC-SHA256
	// Key = HMAC-SHA256(data: "WebAppData", key: bot_token)
	secretKey := hmac.New(sha256.New, []byte("WebAppData"))
	secretKey.Write([]byte(config.BotToken))

	h := hmac.New(sha256.New, secretKey.Sum(nil))
	h.Write([]byte(dataCheckString))
	computedHash := hex.EncodeToString(h.Sum(nil))

	// Compare hashes
	if computedHash != providedHash {
		return nil, fmt.Errorf("hash verification failed")
	}

	// Verify auth_date is recent
	authDateStr := values.Get("auth_date")
	if authDateStr == "" {
		return nil, fmt.Errorf("auth_date is missing")
	}

	var authDate int64
	_, err = fmt.Sscanf(authDateStr, "%d", &authDate)
	if err != nil {
		return nil, fmt.Errorf("invalid auth_date format: %w", err)
	}

	maxAge := config.MaxAge
	if maxAge == 0 {
		maxAge = 86400 // 24 hours default
	}

	if time.Now().Unix()-authDate > maxAge {
		return nil, fmt.Errorf("initData is too old (auth_date: %d)", authDate)
	}

	// Convert to map for easy access
	result := make(map[string]string)
	for key, vals := range values {
		if len(vals) > 0 {
			result[key] = vals[0]
		}
	}

	return result, nil
}

// ParseUserData extracts user information from validated initData.
// Returns user_id, username, and other user fields.
func ParseUserData(data map[string]string) (userID int64, username string, err error) {
	userIDStr := data["user_id"]
	if userIDStr == "" {
		// Try parsing from user JSON field (Telegram format)
		userJSON := data["user"]
		if userJSON == "" {
			return 0, "", fmt.Errorf("user_id or user field is required")
		}
		// For simplicity, we'll require user_id to be present directly
		// Full JSON parsing can be added if needed
		return 0, "", fmt.Errorf("user_id not found in initData")
	}

	_, err = fmt.Sscanf(userIDStr, "%d", &userID)
	if err != nil {
		return 0, "", fmt.Errorf("invalid user_id format: %w", err)
	}

	username = data["username"]
	return userID, username, nil
}
