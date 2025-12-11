package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"

	"pod-backend/internal/entity"
	"pod-backend/internal/repository"
)

// GamePersistenceUseCase handles persisting blockchain game events to database.
// Supports FR-001 (persist blockchain events), FR-008 (update game status),
// FR-011 (validate blockchain data), FR-012 (store outcomes).
// T093: Integrated with GameBroadcastUseCase for real-time WebSocket updates.
type GamePersistenceUseCase struct {
	gameRepo    repository.GameRepository
	eventRepo   repository.GameEventRepository
	userRepo    repository.UserRepository
	txManager   repository.TxManager  // Optional: for transactional operations
	broadcastUC *GameBroadcastUseCase // Optional: nil when WebSocket not enabled
}

// NewGamePersistenceUseCase creates a new game persistence use case.
func NewGamePersistenceUseCase(
	gameRepo repository.GameRepository,
	eventRepo repository.GameEventRepository,
	userRepo repository.UserRepository,
) *GamePersistenceUseCase {
	return &GamePersistenceUseCase{
		gameRepo:    gameRepo,
		eventRepo:   eventRepo,
		userRepo:    userRepo,
		txManager:   nil, // Set via SetTxManager
		broadcastUC: nil, // Set via SetBroadcastUseCase
	}
}

// SetTxManager sets the transaction manager for atomic multi-repository operations.
// This is optional - if not set, operations execute without transactions (legacy behavior).
func (uc *GamePersistenceUseCase) SetTxManager(txManager repository.TxManager) {
	uc.txManager = txManager
}

// SetBroadcastUseCase sets the broadcast use case for real-time WebSocket updates (T093).
// This is optional - if not set, persistence works without broadcasting.
func (uc *GamePersistenceUseCase) SetBroadcastUseCase(broadcastUC *GameBroadcastUseCase) {
	uc.broadcastUC = broadcastUC
}

// extractInt64 extracts an int64 from event data, handling both int64 and float64 types.
// JSON unmarshaling produces float64 for numbers, so we need to handle both.
func extractInt64(data map[string]interface{}, key string) (int64, error) {
	val, ok := data[key]
	if !ok {
		return 0, fmt.Errorf("missing %s", key)
	}

	switch v := val.(type) {
	case int64:
		return v, nil
	case float64:
		return int64(v), nil
	case int:
		return int64(v), nil
	case uint8:
		return int64(v), nil
	case uint16:
		return int64(v), nil
	case uint32:
		return int64(v), nil
	case uint64:
		if v > math.MaxInt64 {
			return 0, fmt.Errorf("%s value too large: %d", key, v)
		}
		return int64(v), nil
	case string:
		// Also handle string numbers (e.g., bet_amount from TON)
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid %s: %w", key, err)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("invalid type for %s: %T", key, val)
	}
}

// extractString extracts a string from event data.
func extractString(data map[string]interface{}, key string) (string, error) {
	val, ok := data[key]
	if !ok {
		return "", fmt.Errorf("missing %s", key)
	}
	str, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("invalid type for %s: expected string, got %T", key, val)
	}
	return str, nil
}

// serializeEventData converts EventData to JSON string for Payload field.
func serializeEventData(event *entity.GameEvent) error {
	if event.Payload != "" {
		// Payload already set, no need to serialize
		return nil
	}

	if event.EventData != nil {
		payloadBytes, err := json.Marshal(event.EventData)
		if err != nil {
			return fmt.Errorf("failed to serialize event data: %w", err)
		}
		event.Payload = string(payloadBytes)
	}

	return nil
}

// broadcastGameUpdate triggers WebSocket broadcast if broadcast use case is configured (T093).
// Fetches the latest game state and broadcasts to all subscribers with event type.
func (uc *GamePersistenceUseCase) broadcastGameUpdate(ctx context.Context, gameID int64, eventType GameEventType) {
	if uc.broadcastUC == nil {
		// Broadcasting not enabled
		return
	}

	// Fetch latest game state
	game, err := uc.gameRepo.GetByID(ctx, gameID)
	if err != nil {
		// Log error but don't fail the persistence operation
		// Broadcast failure shouldn't break blockchain event processing
		return
	}

	// Trigger broadcast with event type (errors are logged inside BroadcastGameUpdateWithEvent)
	_ = uc.broadcastUC.BroadcastGameUpdateWithEvent(ctx, game, eventType)
}

// HandleGameInitialized processes GameInitializedNotify event.
// Creates a new game record with status WAITING_FOR_OPPONENT (1).
func (uc *GamePersistenceUseCase) HandleGameInitialized(ctx context.Context, event *entity.GameEvent) error {
	// Validate event (FR-011)
	if err := event.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	if event.EventType != entity.EventTypeGameInitialized {
		return fmt.Errorf("invalid event type: expected %s, got %s", entity.EventTypeGameInitialized, event.EventType)
	}

	// Extract event data first (before creating game)
	gameID, err := extractInt64(event.EventData, "game_id")
	if err != nil {
		return fmt.Errorf("invalid event data: %w", err)
	}

	playerOne, err := extractString(event.EventData, "player_one")
	if err != nil {
		return fmt.Errorf("invalid event data: %w", err)
	}

	betAmount, err := extractInt64(event.EventData, "bet_amount")
	if err != nil {
		return fmt.Errorf("invalid event data: %w", err)
	}

	// player_one_choice is optional - may not be present in GameInitializedNotify
	// (in TON contract, the choice is hidden in the secret hash)
	playerOneChoice, _ := extractInt64(event.EventData, "player_one_choice")

	// Ensure player_one user exists (for FK constraint satisfaction)
	// This creates a minimal "blockchain-only" user if they don't exist yet
	if err := uc.userRepo.EnsureUserByWallet(ctx, playerOne); err != nil {
		return fmt.Errorf("failed to ensure player one user: %w", err)
	}

	// Create game entity
	game := &entity.Game{
		GameID:           gameID,
		Status:           entity.GameStatusWaitingForOpponent,
		PlayerOneAddress: playerOne,
		PlayerOneChoice:  int(playerOneChoice), // Will be 0 if not present
		BetAmount:        betAmount,
		InitTxHash:       event.TransactionHash,
		CreatedAt:        event.Timestamp,
	}

	// CRITICAL: Persist game FIRST (FK constraint requirement)
	// game_events table has FK to games.game_id, so game must exist before event
	// Use CreateOrIgnore for idempotent event processing
	_, err = uc.gameRepo.CreateOrIgnore(ctx, game)
	if err != nil {
		return fmt.Errorf("failed to create game: %w", err)
	}

	// If game wasn't created (already exists), this might be a duplicate event
	// We still need to check if THIS specific event was already processed
	// (different events can reference the same game_id)

	// Serialize EventData to Payload for persistence
	if err := serializeEventData(event); err != nil {
		return err
	}

	// Now persist event - game_id FK constraint is satisfied
	// If it's a duplicate (same game_id, tx_hash, event_type),
	// Upsert will skip it via ON CONFLICT DO NOTHING
	if err := uc.eventRepo.Upsert(ctx, event); err != nil {
		return fmt.Errorf("failed to persist event: %w", err)
	}

	// If event.ID is 0, it means the event was a duplicate and wasn't inserted
	// Event already processed - idempotent, safe to return
	if event.ID == 0 {
		return nil
	}

	// Broadcast game update to WebSocket subscribers (T093)
	uc.broadcastGameUpdate(ctx, gameID, GameEventTypeInitialized)

	return nil
}

// HandleGameStarted processes GameStartedNotify event.
// Updates game with player 2 information and status WAITING_FOR_OPEN_BIDS (2).
func (uc *GamePersistenceUseCase) HandleGameStarted(ctx context.Context, event *entity.GameEvent) error {
	// Validate event (FR-011)
	if err := event.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	if event.EventType != entity.EventTypeGameStarted {
		return fmt.Errorf("invalid event type: expected %s, got %s", entity.EventTypeGameStarted, event.EventType)
	}

	// Extract event data
	playerTwo, err := extractString(event.EventData, "player_two")
	if err != nil {
		return fmt.Errorf("invalid event data: %w", err)
	}

	// Verify game exists (should be created by GameInitialized event)
	// This is a safety check - normally GameInitialized comes first
	_, err = uc.gameRepo.GetByID(ctx, event.GameID)
	if err != nil {
		return fmt.Errorf("game %d not found (GameInitialized may not have been processed yet): %w", event.GameID, err)
	}

	// Serialize EventData to Payload
	if err := serializeEventData(event); err != nil {
		return err
	}

	// Check for duplicate event before modifying game state
	if err := uc.eventRepo.Upsert(ctx, event); err != nil {
		return fmt.Errorf("failed to persist event: %w", err)
	}

	// If event.ID is 0, the event was a duplicate - skip game update
	if event.ID == 0 {
		return nil // Event already processed - idempotent
	}

	// Ensure player_two user exists (for FK constraint satisfaction)
	// This creates a minimal "blockchain-only" user if they don't exist yet
	if err := uc.userRepo.EnsureUserByWallet(ctx, playerTwo); err != nil {
		return fmt.Errorf("failed to ensure player two user: %w", err)
	}

	// Update game with player 2 (FR-008)
	if err := uc.gameRepo.JoinGame(ctx, event.GameID, playerTwo, event.TransactionHash); err != nil {
		return fmt.Errorf("failed to join game: %w", err)
	}

	// Broadcast game update to WebSocket subscribers (T093)
	uc.broadcastGameUpdate(ctx, event.GameID, GameEventTypeStarted)

	return nil
}

// HandleGameFinished processes GameFinishedNotify event.
// Completes game with winner and payout, updates user statistics.
// Uses database transaction to ensure atomicity of game completion and stats updates.
func (uc *GamePersistenceUseCase) HandleGameFinished(ctx context.Context, event *entity.GameEvent) error {
	// Validate event (FR-011)
	if err := event.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	if event.EventType != entity.EventTypeGameFinished {
		return fmt.Errorf("invalid event type: expected %s, got %s", entity.EventTypeGameFinished, event.EventType)
	}

	// Verify game exists (should be created by GameInitialized event)
	game, err := uc.gameRepo.GetByID(ctx, event.GameID)
	if err != nil {
		return fmt.Errorf("game %d not found (GameInitialized may not have been processed yet): %w", event.GameID, err)
	}

	// Serialize EventData to Payload FIRST
	if err := serializeEventData(event); err != nil {
		return err
	}

	// Extract event data before transaction
	winner, err := extractString(event.EventData, "winner")
	if err != nil {
		return fmt.Errorf("invalid event data: %w", err)
	}

	// Payout is optional, default to 0 if not provided
	payout, _ := extractInt64(event.EventData, "payout")

	// Determine loser before transaction
	var loser string
	if game.PlayerOneAddress == winner {
		if game.PlayerTwoAddress != nil {
			loser = *game.PlayerTwoAddress
		}
	} else {
		loser = game.PlayerOneAddress
	}

	// Determine referrer before transaction
	var referrerAddress *string
	var referrerEarnings int64
	if game.PlayerOneAddress == winner && game.PlayerOneReferrer != nil {
		referrerAddress = game.PlayerOneReferrer
	} else if game.PlayerTwoAddress != nil && *game.PlayerTwoAddress == winner && game.PlayerTwoReferrer != nil {
		referrerAddress = game.PlayerTwoReferrer
	}
	if referrerAddress != nil && *referrerAddress != "" {
		// Calculate referrer earnings: (bet_amount * referrer_fee_numerator) / 10000
		// referrer_fee_numerator is in basis points (1/10000)
		referrerEarnings = (game.BetAmount * game.ReferrerFeeNumerator) / 10000
	}

	// If TxManager is available, use transaction for atomicity
	if uc.txManager != nil {
		err = uc.txManager.WithTx(ctx, func(tx repository.Querier) error {
			// Check for duplicate event within transaction
			if err := uc.eventRepo.UpsertWithQuerier(ctx, tx, event); err != nil {
				return fmt.Errorf("failed to persist event: %w", err)
			}

			// If event.ID is 0, the event was a duplicate - rollback
			if event.ID == 0 {
				return nil // Will commit empty transaction
			}

			// Complete game (FR-012)
			if err := uc.gameRepo.CompleteGameWithQuerier(ctx, tx, event.GameID, winner, payout, event.TransactionHash); err != nil {
				return fmt.Errorf("failed to complete game: %w", err)
			}

			// Update user statistics if userRepo is available
			if uc.userRepo != nil {
				// Update winner stats
				if err := uc.userRepo.IncrementGamesPlayedWithQuerier(ctx, tx, winner); err != nil {
					return fmt.Errorf("failed to increment games played for winner: %w", err)
				}
				if err := uc.userRepo.IncrementWinsWithQuerier(ctx, tx, winner); err != nil {
					return fmt.Errorf("failed to increment wins: %w", err)
				}

				// Update loser stats
				if loser != "" {
					if err := uc.userRepo.IncrementGamesPlayedWithQuerier(ctx, tx, loser); err != nil {
						return fmt.Errorf("failed to increment games played for loser: %w", err)
					}
					if err := uc.userRepo.IncrementLossesWithQuerier(ctx, tx, loser); err != nil {
						return fmt.Errorf("failed to increment losses: %w", err)
					}
				}

				// Update referrer statistics (FR-020, FR-021, T091)
				if referrerAddress != nil && *referrerAddress != "" && referrerEarnings > 0 {
					if err := uc.userRepo.IncrementReferralsWithQuerier(ctx, tx, *referrerAddress, referrerEarnings); err != nil {
						return fmt.Errorf("failed to update referrer stats: %w", err)
					}
				}
			}

			return nil
		})
		if err != nil {
			return err
		}
	} else {
		// Legacy non-transactional behavior for backward compatibility
		// Check for duplicate event before modifying game state
		if err := uc.eventRepo.Upsert(ctx, event); err != nil {
			return fmt.Errorf("failed to persist event: %w", err)
		}

		// If event.ID is 0, the event was a duplicate - skip
		if event.ID == 0 {
			return nil // Event already processed
		}

		// Complete game (FR-012)
		if err := uc.gameRepo.CompleteGame(ctx, event.GameID, winner, payout, event.TransactionHash); err != nil {
			return fmt.Errorf("failed to complete game: %w", err)
		}

		// Update user statistics if userRepo is available
		if uc.userRepo != nil {
			// Update statistics
			if err := uc.userRepo.IncrementGamesPlayed(ctx, winner); err != nil {
				return fmt.Errorf("failed to increment games played for winner: %w", err)
			}
			if err := uc.userRepo.IncrementWins(ctx, winner); err != nil {
				return fmt.Errorf("failed to increment wins: %w", err)
			}

			if loser != "" {
				if err := uc.userRepo.IncrementGamesPlayed(ctx, loser); err != nil {
					return fmt.Errorf("failed to increment games played for loser: %w", err)
				}
				if err := uc.userRepo.IncrementLosses(ctx, loser); err != nil {
					return fmt.Errorf("failed to increment losses: %w", err)
				}
			}

			// Update referrer statistics (FR-020, FR-021, T091)
			if referrerAddress != nil && *referrerAddress != "" && referrerEarnings > 0 {
				if err := uc.userRepo.IncrementReferrals(ctx, *referrerAddress, referrerEarnings); err != nil {
					return fmt.Errorf("failed to update referrer stats: %w", err)
				}
			}
		}
	}

	// Only broadcast if event was actually processed (not a duplicate)
	if event.ID != 0 {
		// Broadcast game update to WebSocket subscribers (T093)
		uc.broadcastGameUpdate(ctx, event.GameID, GameEventTypeFinished)
	}

	return nil
}

// HandleDraw processes DrawNotify event.
// Completes game with no winner (draw outcome).
func (uc *GamePersistenceUseCase) HandleDraw(ctx context.Context, event *entity.GameEvent) error {
	// Validate event (FR-011)
	if err := event.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	if event.EventType != entity.EventTypeDraw {
		return fmt.Errorf("invalid event type: expected %s, got %s", entity.EventTypeDraw, event.EventType)
	}

	// Verify game exists (should be created by GameInitialized event)
	_, err := uc.gameRepo.GetByID(ctx, event.GameID)
	if err != nil {
		return fmt.Errorf("game %d not found (GameInitialized may not have been processed yet): %w", event.GameID, err)
	}

	// Serialize EventData to Payload
	if err := serializeEventData(event); err != nil {
		return err
	}

	// Check for duplicate event
	if err := uc.eventRepo.Upsert(ctx, event); err != nil {
		return fmt.Errorf("failed to persist event: %w", err)
	}

	// If event.ID is 0, the event was a duplicate - skip
	if event.ID == 0 {
		return nil
	}

	// Complete game with no winner (FR-012)
	if err := uc.gameRepo.CompleteGame(ctx, event.GameID, "", 0, event.TransactionHash); err != nil {
		return fmt.Errorf("failed to complete game (draw): %w", err)
	}

	// Update user statistics (both players participated, no winner/loser)
	if uc.userRepo != nil {
		game, err := uc.gameRepo.GetByID(ctx, event.GameID)
		if err != nil {
			return fmt.Errorf("failed to get game for stats update: %w", err)
		}

		// Both players get games_played incremented
		if err := uc.userRepo.IncrementGamesPlayed(ctx, game.PlayerOneAddress); err != nil {
			return fmt.Errorf("failed to increment games played for player 1: %w", err)
		}

		if game.PlayerTwoAddress != nil {
			if err := uc.userRepo.IncrementGamesPlayed(ctx, *game.PlayerTwoAddress); err != nil {
				return fmt.Errorf("failed to increment games played for player 2: %w", err)
			}
		}
	}

	// Broadcast game update to WebSocket subscribers (T093)
	uc.broadcastGameUpdate(ctx, event.GameID, GameEventTypeDraw)

	return nil
}

// HandleGameCancelled processes GameCancelledNotify event.
// Marks game as cancelled (status ENDED with no winner).
func (uc *GamePersistenceUseCase) HandleGameCancelled(ctx context.Context, event *entity.GameEvent) error {
	// Validate event (FR-011)
	if err := event.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	if event.EventType != entity.EventTypeGameCancelled {
		return fmt.Errorf("invalid event type: expected %s, got %s", entity.EventTypeGameCancelled, event.EventType)
	}

	// Verify game exists (should be created by GameInitialized event)
	_, err := uc.gameRepo.GetByID(ctx, event.GameID)
	if err != nil {
		return fmt.Errorf("game %d not found (GameInitialized may not have been processed yet): %w", event.GameID, err)
	}

	// Serialize EventData to Payload
	if err := serializeEventData(event); err != nil {
		return err
	}

	// Check for duplicate event
	if err := uc.eventRepo.Upsert(ctx, event); err != nil {
		return fmt.Errorf("failed to persist event: %w", err)
	}

	// If event.ID is 0, the event was a duplicate - skip
	if event.ID == 0 {
		return nil
	}

	// Cancel game
	if err := uc.gameRepo.CancelGame(ctx, event.GameID, event.TransactionHash); err != nil {
		return fmt.Errorf("failed to cancel game: %w", err)
	}

	// Broadcast game update to WebSocket subscribers (T093)
	uc.broadcastGameUpdate(ctx, event.GameID, GameEventTypeCancelled)

	return nil
}

// HandleSecretOpened processes SecretOpenedNotify event.
// Updates game with revealed choice for a player.
func (uc *GamePersistenceUseCase) HandleSecretOpened(ctx context.Context, event *entity.GameEvent) error {
	// Validate event (FR-011)
	if err := event.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	if event.EventType != entity.EventTypeSecretOpened {
		return fmt.Errorf("invalid event type: expected %s, got %s", entity.EventTypeSecretOpened, event.EventType)
	}

	// Verify game exists (should be created by GameInitialized event)
	_, err := uc.gameRepo.GetByID(ctx, event.GameID)
	if err != nil {
		return fmt.Errorf("game %d not found (GameInitialized may not have been processed yet): %w", event.GameID, err)
	}

	// Serialize EventData to Payload
	if err := serializeEventData(event); err != nil {
		return err
	}

	// Check for duplicate event
	if err := uc.eventRepo.Upsert(ctx, event); err != nil {
		return fmt.Errorf("failed to persist event: %w", err)
	}

	// If event.ID is 0, the event was a duplicate - skip
	if event.ID == 0 {
		return nil
	}

	// Extract event data
	playerAddress, err := extractString(event.EventData, "player")
	if err != nil {
		return fmt.Errorf("invalid event data: %w", err)
	}

	// Use coin_side key as defined in SecretOpenedNotify message in smart contract
	coinSide, err := extractInt64(event.EventData, "coin_side")
	if err != nil {
		return fmt.Errorf("invalid event data: %w", err)
	}

	// Update game with revealed choice
	if err := uc.gameRepo.RevealChoice(ctx, event.GameID, playerAddress, int(coinSide), event.TransactionHash); err != nil {
		return fmt.Errorf("failed to reveal choice: %w", err)
	}

	// Broadcast game update to WebSocket subscribers (T093)
	uc.broadcastGameUpdate(ctx, event.GameID, GameEventTypeSecretOpened)

	return nil
}

// HandleInsufficientBalance processes InsufficientBalanceNotify event.
// Logs the error but doesn't modify game state.
func (uc *GamePersistenceUseCase) HandleInsufficientBalance(ctx context.Context, event *entity.GameEvent) error {
	// Validate event (FR-011)
	if err := event.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	if event.EventType != entity.EventTypeInsufficientBalance {
		return fmt.Errorf("invalid event type: expected %s, got %s", entity.EventTypeInsufficientBalance, event.EventType)
	}

	// Verify game exists (should be created by GameInitialized event)
	_, err := uc.gameRepo.GetByID(ctx, event.GameID)
	if err != nil {
		return fmt.Errorf("game %d not found (GameInitialized may not have been processed yet): %w", event.GameID, err)
	}

	// Serialize EventData to Payload
	if err := serializeEventData(event); err != nil {
		return err
	}

	// Persist event (audit trail only, no game state change)
	if err := uc.eventRepo.Upsert(ctx, event); err != nil {
		return fmt.Errorf("failed to persist event: %w", err)
	}

	// Note: No broadcast for InsufficientBalance events as game state doesn't change
	// This is an audit-only event

	return nil
}
