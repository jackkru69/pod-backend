package usecase

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/xssnick/tonutils-go/address"
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
	gameRepo            repository.GameRepository
	broadcastUC         *GameBroadcastUseCase
	cancelReservationUC *CancelReservationUseCase
	metrics             *metrics.ReservationMetrics // T049: Prometheus metrics

	// Cleanup control
	stopCleanup chan struct{}
	stopOnce    sync.Once // Prevents double-close panic
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

// SetCancelReservationUseCase wires creator-side cancel coordination into join
// reservation validation so stale join attempts are rejected while cancellation
// is in progress.
func (uc *ReservationUseCase) SetCancelReservationUseCase(cancelUC *CancelReservationUseCase) {
	uc.cancelReservationUC = cancelUC
}

// Reserve creates a new reservation for a game.
// Returns error if:
// - Game is already reserved (ErrGameAlreadyReserved)
// - Wallet has too many reservations (ErrTooManyReservations)
// - Wallet owns the game (ErrCannotReserveOwnGame)
// - Game is not in waiting_for_opponent status (ErrGameNotAvailable)
func (uc *ReservationUseCase) Reserve(ctx context.Context, gameID int64, walletAddress string) (*entity.GameReservation, error) {
	normalizedWalletAddress := normalizeReservationWallet(walletAddress)

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
	if sameReservationWallet(game.PlayerOneAddress, normalizedWalletAddress) {
		return nil, entity.ErrCannotReserveOwnGame
	}

	if uc.cancelReservationUC != nil {
		cancelReservation, getErr := uc.cancelReservationUC.Get(ctx, gameID)
		if getErr != nil {
			return nil, getErr
		}
		if cancelReservation != nil && cancelReservation.IsActive() {
			if uc.metrics != nil {
				uc.metrics.RecordError("cancel_pending")
			}
			return nil, entity.ErrGameCancellationPending
		}
	}

	// Lock for atomic reservation creation
	uc.mu.Lock()

	// Check if game is already reserved
	if existing, ok := uc.reservations[gameID]; ok && existing.IsActive() {
		uc.mu.Unlock()
		return nil, entity.ErrGameAlreadyReserved
	}

	// Check wallet reservation limit (FR-010)
	walletGameIDs := uc.walletReservations[normalizedWalletAddress]
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
	uc.walletReservations[normalizedWalletAddress] = append(uc.walletReservations[normalizedWalletAddress], gameID)

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
	reservation, ok := uc.reservations[gameID]
	uc.mu.RUnlock()
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
	gameIDs := uc.walletReservations[normalizeReservationWallet(walletAddress)]
	reservations := make([]*entity.GameReservation, 0, len(gameIDs))

	for _, gameID := range gameIDs {
		if res, ok := uc.reservations[gameID]; ok && res.IsActive() {
			reservations = append(reservations, res)
		}
	}
	uc.mu.RUnlock()

	result := make([]*entity.GameReservation, 0, len(reservations))
	for _, reservation := range reservations {
		isAvailable, err := uc.isReservationGameAvailable(ctx, reservation.GameID)
		if err != nil {
			return nil, err
		}

		if !isAvailable {
			uc.releaseStaleReservation(reservation.GameID)
			continue
		}

		result = append(result, reservation)
	}

	sort.SliceStable(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})

	return result, nil
}

// Cancel releases a reservation.
// Only the reservation holder can cancel.
func (uc *ReservationUseCase) Cancel(ctx context.Context, gameID int64, walletAddress string) error {
	normalizedWalletAddress := normalizeReservationWallet(walletAddress)
	uc.mu.Lock()

	reservation, ok := uc.reservations[gameID]
	if !ok || !reservation.IsActive() {
		uc.mu.Unlock()
		return entity.ErrReservationNotFound
	}

	// Check ownership
	if !sameReservationWallet(reservation.WalletAddress, normalizedWalletAddress) {
		uc.mu.Unlock()
		return entity.ErrNotReservationHolder
	}

	// Release reservation
	reservation.Status = entity.ReservationStatusReleased
	uc.removeFromWalletIndex(reservation.WalletAddress, gameID)

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

// StopCleanupLoop stops the background cleanup goroutine.
// Safe to call multiple times - uses sync.Once to prevent panic on double close.
func (uc *ReservationUseCase) StopCleanupLoop() {
	uc.stopOnce.Do(func() {
		close(uc.stopCleanup)
		log.Info().Msg("Reservation cleanup loop stop signal sent")
	})
}

// CleanupExpired removes expired reservations and broadcasts updates.
// Also cleans up old non-active reservations to prevent memory leaks.
// Exported for testing purposes.
func (uc *ReservationUseCase) CleanupExpired(ctx context.Context) {
	uc.mu.Lock()

	var expiredGames []int64
	var expiredReservations []*entity.GameReservation
	var staleGameIDs []int64

	for gameID, reservation := range uc.reservations {
		if reservation.Status == entity.ReservationStatusActive && reservation.IsExpired() {
			// Mark as expired and collect for broadcast
			reservation.Status = entity.ReservationStatusExpired
			uc.removeFromWalletIndex(reservation.WalletAddress, gameID)
			expiredGames = append(expiredGames, gameID)
			expiredReservations = append(expiredReservations, reservation)
		}

		// Collect non-active reservations for cleanup (memory leak fix)
		// Keep expired reservations for a short time for debugging, then remove
		if !reservation.IsActive() {
			staleGameIDs = append(staleGameIDs, gameID)
		}
	}

	// Remove stale reservations from memory to prevent leak
	for _, gameID := range staleGameIDs {
		delete(uc.reservations, gameID)
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
	walletKey := normalizeReservationWallet(walletAddress)
	gameIDs := uc.walletReservations[walletKey]
	for i, gid := range gameIDs {
		if gid == gameID {
			uc.walletReservations[walletKey] = append(gameIDs[:i], gameIDs[i+1:]...)
			break
		}
	}
	// Clean up empty wallet entries
	if len(uc.walletReservations[walletKey]) == 0 {
		delete(uc.walletReservations, walletKey)
	}
}

func (uc *ReservationUseCase) isReservationGameAvailable(ctx context.Context, gameID int64) (bool, error) {
	if uc.gameRepo == nil {
		return true, nil
	}

	game, err := uc.gameRepo.GetByID(ctx, gameID)
	if err != nil {
		return false, err
	}
	if game == nil {
		return false, nil
	}

	return game.Status == entity.GameStatusWaitingForOpponent, nil
}

func (uc *ReservationUseCase) releaseStaleReservation(gameID int64) {
	uc.mu.Lock()
	defer uc.mu.Unlock()

	reservation, ok := uc.reservations[gameID]
	if !ok {
		return
	}

	reservation.Status = entity.ReservationStatusReleased
	uc.removeFromWalletIndex(reservation.WalletAddress, gameID)
	delete(uc.reservations, gameID)
}

func normalizeReservationWallet(walletAddress string) string {
	trimmedWalletAddress := strings.TrimSpace(walletAddress)
	if trimmedWalletAddress == "" {
		return ""
	}

	parsedWalletAddress, err := address.ParseAddr(trimmedWalletAddress)
	if err != nil {
		return strings.ToLower(trimmedWalletAddress)
	}

	return parsedWalletAddress.StringRaw()
}

func sameReservationWallet(walletAddress string, normalizedWalletAddress string) bool {
	if normalizedWalletAddress == "" {
		return normalizeReservationWallet(walletAddress) == ""
	}

	return normalizeReservationWallet(walletAddress) == normalizedWalletAddress
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
