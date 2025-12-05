package usecase

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"pod-backend/internal/entity"
	"pod-backend/internal/infrastructure/metrics"
	"pod-backend/internal/repository"
)

// ReservationUseCase handles game reservation business logic.
// Uses in-memory storage with mutex protection for thread safety.
type ReservationUseCase struct {
	// Primary index: gameID -> reservation
	reservations map[int64]*entity.GameReservation

	// Secondary index: walletAddress -> []gameID (for limit checking)
	walletReservations map[string][]int64

	// Configuration
	maxPerWallet    int
	timeout         time.Duration
	cleanupInterval time.Duration

	// Thread safety
	mu sync.RWMutex

	// Dependencies
	gameRepo    repository.GameRepository
	broadcastUC *GameBroadcastUseCase
	metrics     *metrics.ReservationMetrics // T049: Prometheus metrics

	// Cleanup control
	stopCleanup chan struct{}
}

// ReservationConfig holds configuration for the reservation use case
type ReservationConfig struct {
	MaxPerWallet           int
	TimeoutSeconds         int
	CleanupIntervalSeconds int
}

// NewReservationUseCase creates a new ReservationUseCase instance
func NewReservationUseCase(
	gameRepo repository.GameRepository,
	broadcastUC *GameBroadcastUseCase,
	cfg ReservationConfig,
) *ReservationUseCase {
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second // default 60s
	}

	maxPerWallet := cfg.MaxPerWallet
	if maxPerWallet <= 0 {
		maxPerWallet = 3 // default 3
	}

	cleanupInterval := time.Duration(cfg.CleanupIntervalSeconds) * time.Second
	if cleanupInterval <= 0 {
		cleanupInterval = 5 * time.Second // default 5s
	}

	return &ReservationUseCase{
		reservations:       make(map[int64]*entity.GameReservation),
		walletReservations: make(map[string][]int64),
		maxPerWallet:       maxPerWallet,
		timeout:            timeout,
		cleanupInterval:    cleanupInterval,
		gameRepo:           gameRepo,
		broadcastUC:        broadcastUC,
		metrics:            nil, // Set via SetMetrics
		stopCleanup:        make(chan struct{}),
	}
}

// SetMetrics sets the Prometheus metrics collector (T049).
// This is optional - if not set, metrics collection is disabled.
func (uc *ReservationUseCase) SetMetrics(m *metrics.ReservationMetrics) {
	uc.metrics = m
}

// Reserve creates a new reservation for a game.
// Returns error if:
// - Game is already reserved (ErrGameAlreadyReserved)
// - Wallet has too many reservations (ErrTooManyReservations)
// - Wallet owns the game (ErrCannotReserveOwnGame)
// - Game is not in waiting_for_opponent status (ErrGameNotAvailable)
func (uc *ReservationUseCase) Reserve(ctx context.Context, gameID int64, walletAddress string) (*entity.GameReservation, error) {
	// First, check game existence and status (outside lock to avoid holding lock during DB call)
	game, err := uc.gameRepo.GetByID(ctx, gameID)
	if err != nil {
		return nil, err
	}
	if game == nil {
		return nil, entity.ErrGameNotAvailable
	}

	// Check game status
	if game.Status != entity.GameStatusWaitingForOpponent {
		return nil, entity.ErrGameNotAvailable
	}

	// Check if player owns the game (FR-011)
	if game.PlayerOneAddress == walletAddress {
		return nil, entity.ErrCannotReserveOwnGame
	}

	// Lock for atomic reservation creation
	uc.mu.Lock()

	// Check if game is already reserved
	if existing, ok := uc.reservations[gameID]; ok && existing.IsActive() {
		uc.mu.Unlock()
		return nil, entity.ErrGameAlreadyReserved
	}

	// Check wallet reservation limit (FR-010)
	walletGameIDs := uc.walletReservations[walletAddress]
	activeCount := 0
	for _, gid := range walletGameIDs {
		if res, ok := uc.reservations[gid]; ok && res.IsActive() {
			activeCount++
		}
	}
	if activeCount >= uc.maxPerWallet {
		uc.mu.Unlock()
		return nil, entity.ErrTooManyReservations
	}

	// Create reservation
	now := time.Now()
	reservation := &entity.GameReservation{
		GameID:        gameID,
		WalletAddress: walletAddress,
		CreatedAt:     now,
		ExpiresAt:     now.Add(uc.timeout),
		Status:        entity.ReservationStatusActive,
	}

	// Store reservation
	uc.reservations[gameID] = reservation

	// Update wallet index
	uc.walletReservations[walletAddress] = append(uc.walletReservations[walletAddress], gameID)

	// Release lock before broadcasting (to avoid deadlock)
	uc.mu.Unlock()

	log.Info().
		Int64("game_id", gameID).
		Str("wallet", walletAddress).
		Time("expires_at", reservation.ExpiresAt).
		Msg("Game reserved")

	// Record metrics (T049)
	if uc.metrics != nil {
		uc.metrics.RecordCreated()
	}

	// Broadcast reservation created event (T042)
	if uc.broadcastUC != nil {
		uc.broadcastUC.BroadcastReservationCreated(ctx, reservation)
	}

	// Re-lock is not needed since we're returning
	return reservation, nil
}

// GetReservation returns the current reservation for a game, if any
func (uc *ReservationUseCase) GetReservation(ctx context.Context, gameID int64) (*entity.GameReservation, error) {
	uc.mu.RLock()
	defer uc.mu.RUnlock()

	reservation, ok := uc.reservations[gameID]
	if !ok {
		return nil, nil // No reservation exists
	}

	// Return nil if expired (cleanup will remove it)
	if !reservation.IsActive() {
		return nil, nil
	}

	return reservation, nil
}

// ListByWallet returns all active reservations for a wallet
func (uc *ReservationUseCase) ListByWallet(ctx context.Context, walletAddress string) ([]*entity.GameReservation, error) {
	uc.mu.RLock()
	defer uc.mu.RUnlock()

	gameIDs := uc.walletReservations[walletAddress]
	result := make([]*entity.GameReservation, 0, len(gameIDs))

	for _, gameID := range gameIDs {
		if res, ok := uc.reservations[gameID]; ok && res.IsActive() {
			result = append(result, res)
		}
	}

	return result, nil
}

// Cancel releases a reservation.
// Only the reservation holder can cancel.
func (uc *ReservationUseCase) Cancel(ctx context.Context, gameID int64, walletAddress string) error {
	uc.mu.Lock()

	reservation, ok := uc.reservations[gameID]
	if !ok || !reservation.IsActive() {
		uc.mu.Unlock()
		return entity.ErrReservationNotFound
	}

	// Check ownership
	if reservation.WalletAddress != walletAddress {
		uc.mu.Unlock()
		return entity.ErrNotReservationHolder
	}

	// Release reservation
	reservation.Status = entity.ReservationStatusReleased
	uc.removeFromWalletIndex(walletAddress, gameID)

	// Release lock before broadcasting (T043)
	uc.mu.Unlock()

	log.Info().
		Int64("game_id", gameID).
		Str("wallet", walletAddress).
		Msg("Reservation cancelled")

	// Record metrics (T049)
	if uc.metrics != nil {
		uc.metrics.RecordCancelled()
	}

	// Broadcast reservation released event
	if uc.broadcastUC != nil {
		uc.broadcastUC.BroadcastReservationReleased(ctx, gameID, "cancelled")
	}

	return nil
}

// ReleaseOnJoin releases a reservation when a player successfully joins via blockchain.
// Called by blockchain subscriber when game_joined event is received.
func (uc *ReservationUseCase) ReleaseOnJoin(ctx context.Context, gameID int64) {
	uc.mu.Lock()

	reservation, ok := uc.reservations[gameID]
	if !ok {
		uc.mu.Unlock()
		return // No reservation to release
	}

	if reservation.IsActive() {
		reservation.Status = entity.ReservationStatusReleased
		uc.removeFromWalletIndex(reservation.WalletAddress, gameID)

		// Release lock before broadcasting (T043)
		uc.mu.Unlock()

		log.Info().
			Int64("game_id", gameID).
			Str("wallet", reservation.WalletAddress).
			Msg("Reservation released on blockchain join")

		// Record metrics (T049)
		if uc.metrics != nil {
			uc.metrics.RecordJoined()
		}

		// Broadcast reservation released event
		if uc.broadcastUC != nil {
			uc.broadcastUC.BroadcastReservationReleased(ctx, gameID, "joined")
		}
	} else {
		uc.mu.Unlock()
	}
}

// StartCleanupLoop starts the background goroutine that cleans up expired reservations
func (uc *ReservationUseCase) StartCleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(uc.cleanupInterval)
	go func() {
		for {
			select {
			case <-ticker.C:
				uc.CleanupExpired(ctx)
			case <-uc.stopCleanup:
				ticker.Stop()
				log.Info().Msg("Reservation cleanup loop stopped")
				return
			case <-ctx.Done():
				ticker.Stop()
				log.Info().Msg("Reservation cleanup loop stopped (context cancelled)")
				return
			}
		}
	}()

	log.Info().
		Dur("interval", uc.cleanupInterval).
		Msg("Reservation cleanup loop started")
}

// StopCleanupLoop stops the background cleanup goroutine
func (uc *ReservationUseCase) StopCleanupLoop() {
	close(uc.stopCleanup)
}

// CleanupExpired removes expired reservations and broadcasts updates
// Exported for testing purposes
func (uc *ReservationUseCase) CleanupExpired(ctx context.Context) {
	uc.mu.Lock()

	var expiredGames []int64
	var expiredReservations []*entity.GameReservation

	for gameID, reservation := range uc.reservations {
		if reservation.Status == entity.ReservationStatusActive && reservation.IsExpired() {
			reservation.Status = entity.ReservationStatusExpired
			uc.removeFromWalletIndex(reservation.WalletAddress, gameID)
			expiredGames = append(expiredGames, gameID)
			expiredReservations = append(expiredReservations, reservation)
		}
	}

	uc.mu.Unlock()

	// Broadcast expired reservations and record metrics (outside lock)
	for i, gameID := range expiredGames {
		if uc.broadcastUC != nil {
			uc.broadcastUC.BroadcastReservationReleased(ctx, gameID, "expired")
		}
		// Record metrics (T049)
		if uc.metrics != nil {
			uc.metrics.RecordExpired()
		}
		log.Debug().
			Int64("game_id", gameID).
			Str("wallet", expiredReservations[i].WalletAddress).
			Msg("Reservation expired")
	}

	if len(expiredGames) > 0 {
		log.Info().
			Int("count", len(expiredGames)).
			Msg("Cleaned up expired reservations")
	}
}

// removeFromWalletIndex removes a gameID from the wallet's reservation list
// Must be called with lock held
func (uc *ReservationUseCase) removeFromWalletIndex(walletAddress string, gameID int64) {
	gameIDs := uc.walletReservations[walletAddress]
	for i, gid := range gameIDs {
		if gid == gameID {
			uc.walletReservations[walletAddress] = append(gameIDs[:i], gameIDs[i+1:]...)
			break
		}
	}
	// Clean up empty wallet entries
	if len(uc.walletReservations[walletAddress]) == 0 {
		delete(uc.walletReservations, walletAddress)
	}
}

// GetActiveCount returns the number of active reservations (for metrics)
func (uc *ReservationUseCase) GetActiveCount() int {
	uc.mu.RLock()
	defer uc.mu.RUnlock()

	count := 0
	for _, res := range uc.reservations {
		if res.IsActive() {
			count++
		}
	}
	return count
}
