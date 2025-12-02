package usecase

import (
	"context"
	"encoding/json"
	"fmt"

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
		broadcastUC: nil, // Set via SetBroadcastUseCase
	}
}

// SetBroadcastUseCase sets the broadcast use case for real-time WebSocket updates (T093).
// This is optional - if not set, persistence works without broadcasting.
func (uc *GamePersistenceUseCase) SetBroadcastUseCase(broadcastUC *GameBroadcastUseCase) {
	uc.broadcastUC = broadcastUC
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
// Fetches the latest game state and broadcasts to all subscribers.
func (uc *GamePersistenceUseCase) broadcastGameUpdate(ctx context.Context, gameID int64) {
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

	// Trigger broadcast (errors are logged inside BroadcastGameUpdate)
	_ = uc.broadcastUC.BroadcastGameUpdate(ctx, game)
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

	// Serialize EventData to Payload for persistence
	if err := serializeEventData(event); err != nil {
		return err
	}

	// Persist event to audit trail
	if err := uc.eventRepo.Upsert(ctx, event); err != nil {
		return fmt.Errorf("failed to persist event: %w", err)
	}

	// Extract event data
	gameID, ok := event.EventData["game_id"].(int64)
	if !ok {
		return fmt.Errorf("missing or invalid game_id in event data")
	}

	playerOne, ok := event.EventData["player_one"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid player_one in event data")
	}

	betAmount, ok := event.EventData["bet_amount"].(int64)
	if !ok {
		return fmt.Errorf("missing or invalid bet_amount in event data")
	}

	playerOneChoice, ok := event.EventData["player_one_choice"].(int64)
	if !ok {
		return fmt.Errorf("missing or invalid player_one_choice in event data")
	}

	// Create game entity
	game := &entity.Game{
		GameID:           gameID,
		Status:           entity.GameStatusWaitingForOpponent,
		PlayerOneAddress: playerOne,
		PlayerOneChoice:  int(playerOneChoice),
		BetAmount:        betAmount,
		InitTxHash:       event.TransactionHash,
		CreatedAt:        event.Timestamp,
	}

	// Persist game (FR-001)
	if err := uc.gameRepo.Create(ctx, game); err != nil {
		return fmt.Errorf("failed to create game: %w", err)
	}

	// Broadcast game update to WebSocket subscribers (T093)
	uc.broadcastGameUpdate(ctx, gameID)

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

	// Serialize EventData to Payload
	if err := serializeEventData(event); err != nil {
		return err
	}

	// Persist event
	if err := uc.eventRepo.Upsert(ctx, event); err != nil {
		return fmt.Errorf("failed to persist event: %w", err)
	}

	// Extract event data
	playerTwo, ok := event.EventData["player_two"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid player_two in event data")
	}

	// Update game with player 2 (FR-008)
	if err := uc.gameRepo.JoinGame(ctx, event.GameID, playerTwo, event.TransactionHash); err != nil {
		return fmt.Errorf("failed to join game: %w", err)
	}

	// Broadcast game update to WebSocket subscribers (T093)
	uc.broadcastGameUpdate(ctx, event.GameID)

	return nil
}

// HandleGameFinished processes GameFinishedNotify event.
// Completes game with winner and payout, updates user statistics.
func (uc *GamePersistenceUseCase) HandleGameFinished(ctx context.Context, event *entity.GameEvent) error {
	// Validate event (FR-011)
	if err := event.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	if event.EventType != entity.EventTypeGameFinished {
		return fmt.Errorf("invalid event type: expected %s, got %s", entity.EventTypeGameFinished, event.EventType)
	}

	// Serialize EventData to Payload
	if err := serializeEventData(event); err != nil {
		return err
	}

	// Persist event
	if err := uc.eventRepo.Upsert(ctx, event); err != nil {
		return fmt.Errorf("failed to persist event: %w", err)
	}

	// Extract event data
	winner, ok := event.EventData["winner"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid winner in event data")
	}

	payout, ok := event.EventData["payout"].(int64)
	if !ok {
		return fmt.Errorf("missing or invalid payout in event data")
	}

	// Complete game (FR-012)
	if err := uc.gameRepo.CompleteGame(ctx, event.GameID, winner, payout, event.TransactionHash); err != nil {
		return fmt.Errorf("failed to complete game: %w", err)
	}

	// Update user statistics if userRepo is available
	if uc.userRepo != nil {
		// Get game to determine loser
		game, err := uc.gameRepo.GetByID(ctx, event.GameID)
		if err != nil {
			return fmt.Errorf("failed to get game for stats update: %w", err)
		}

		// Determine loser
		var loser string
		if game.PlayerOneAddress == winner {
			if game.PlayerTwoAddress != nil {
				loser = *game.PlayerTwoAddress
			}
		} else {
			loser = game.PlayerOneAddress
		}

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
		// Calculate referrer earnings based on bet amount and referrer fee
		var referrerAddress *string
		if game.PlayerOneAddress == winner && game.PlayerOneReferrer != nil {
			referrerAddress = game.PlayerOneReferrer
		} else if game.PlayerTwoAddress != nil && *game.PlayerTwoAddress == winner && game.PlayerTwoReferrer != nil {
			referrerAddress = game.PlayerTwoReferrer
		}

		if referrerAddress != nil && *referrerAddress != "" {
			// Calculate referrer earnings: (bet_amount * referrer_fee_numerator) / 10000
			// referrer_fee_numerator is in basis points (1/10000)
			referrerEarnings := (game.BetAmount * game.ReferrerFeeNumerator) / 10000

			if err := uc.userRepo.IncrementReferrals(ctx, *referrerAddress, referrerEarnings); err != nil {
				return fmt.Errorf("failed to update referrer stats: %w", err)
			}
		}
	}

	// Broadcast game update to WebSocket subscribers (T093)
	uc.broadcastGameUpdate(ctx, event.GameID)

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

	// Serialize EventData to Payload
	if err := serializeEventData(event); err != nil {
		return err
	}

	// Persist event
	if err := uc.eventRepo.Upsert(ctx, event); err != nil {
		return fmt.Errorf("failed to persist event: %w", err)
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
	uc.broadcastGameUpdate(ctx, event.GameID)

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

	// Serialize EventData to Payload
	if err := serializeEventData(event); err != nil {
		return err
	}

	// Persist event
	if err := uc.eventRepo.Upsert(ctx, event); err != nil {
		return fmt.Errorf("failed to persist event: %w", err)
	}

	// Cancel game
	if err := uc.gameRepo.CancelGame(ctx, event.GameID, event.TransactionHash); err != nil {
		return fmt.Errorf("failed to cancel game: %w", err)
	}

	// Broadcast game update to WebSocket subscribers (T093)
	uc.broadcastGameUpdate(ctx, event.GameID)

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

	// Serialize EventData to Payload
	if err := serializeEventData(event); err != nil {
		return err
	}

	// Persist event
	if err := uc.eventRepo.Upsert(ctx, event); err != nil {
		return fmt.Errorf("failed to persist event: %w", err)
	}

	// Extract event data
	playerAddress, ok := event.EventData["player"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid player in event data")
	}

	choice, ok := event.EventData["choice"].(int64)
	if !ok {
		return fmt.Errorf("missing or invalid choice in event data")
	}

	// Update game with revealed choice
	if err := uc.gameRepo.RevealChoice(ctx, event.GameID, playerAddress, int(choice), event.TransactionHash); err != nil {
		return fmt.Errorf("failed to reveal choice: %w", err)
	}

	// Broadcast game update to WebSocket subscribers (T093)
	uc.broadcastGameUpdate(ctx, event.GameID)

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
