package usecase

import (
	"context"
	"sort"
	"sync"
	"time"

	"pod-backend/internal/entity"
	"pod-backend/internal/infrastructure/metrics"
	"pod-backend/internal/repository"

	"github.com/rs/zerolog/log"
)

// CancelReservationUseCase handles creator-only cancel coordination for waiting
// games. Storage is intentionally in-memory and advisory only.
type CancelReservationUseCase struct {
	reservations map[int64]*entity.CancelReservation

	walletReservations map[string][]int64

	maxPerWallet    int
	timeout         time.Duration
	cleanupInterval time.Duration

	mu sync.RWMutex

	gameRepo      repository.GameRepository
	reservationUC *ReservationUseCase
	broadcastUC   *GameBroadcastUseCase
	metrics       *metrics.CancelReservationMetrics
	stopCleanup   chan struct{}
	stopOnce      sync.Once
}

// CancelReservationConfig holds configuration for cancel coordination.
type CancelReservationConfig struct {
	MaxPerWallet           int
	TimeoutSeconds         int
	CleanupIntervalSeconds int
}

// NewCancelReservationUseCase creates a new CancelReservationUseCase.
func NewCancelReservationUseCase(
	gameRepo repository.GameRepository,
	reservationUC *ReservationUseCase,
	broadcastUC *GameBroadcastUseCase,
	cfg CancelReservationConfig,
) *CancelReservationUseCase {
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	maxPerWallet := cfg.MaxPerWallet
	if maxPerWallet <= 0 {
		maxPerWallet = 3
	}

	cleanupInterval := time.Duration(cfg.CleanupIntervalSeconds) * time.Second
	if cleanupInterval <= 0 {
		cleanupInterval = 5 * time.Second
	}

	return &CancelReservationUseCase{
		reservations:       make(map[int64]*entity.CancelReservation),
		walletReservations: make(map[string][]int64),
		maxPerWallet:       maxPerWallet,
		timeout:            timeout,
		cleanupInterval:    cleanupInterval,
		gameRepo:           gameRepo,
		reservationUC:      reservationUC,
		broadcastUC:        broadcastUC,
		stopCleanup:        make(chan struct{}),
	}
}

// SetMetrics wires Prometheus metrics. Optional.
func (uc *CancelReservationUseCase) SetMetrics(m *metrics.CancelReservationMetrics) {
	uc.metrics = m
}

// Reserve creates a cancel reservation for a waiting game. Only the game
// creator may reserve cancel, and the flow is blocked if a competing join
// reservation already exists.
func (uc *CancelReservationUseCase) Reserve(ctx context.Context, gameID int64, walletAddress string) (*entity.CancelReservation, error) {
	normalizedWallet := normalizeReservationWallet(walletAddress)

	game, err := uc.gameRepo.GetByID(ctx, gameID)
	if err != nil {
		return nil, err
	}
	if game == nil || game.Status != entity.GameStatusWaitingForOpponent {
		uc.recordError("cancel_not_available")
		return nil, entity.ErrCancelNotAvailable
	}

	if !sameReservationWallet(game.PlayerOneAddress, normalizedWallet) {
		uc.recordError("not_game_creator")
		return nil, entity.ErrNotGameCreator
	}

	if uc.reservationUC != nil {
		joinReservation, getErr := uc.reservationUC.GetReservation(ctx, gameID)
		if getErr != nil {
			return nil, getErr
		}
		if joinReservation != nil && joinReservation.IsActive() {
			uc.recordError("join_conflict")
			return nil, entity.ErrGameAlreadyReserved
		}
	}

	uc.mu.Lock()
	defer uc.mu.Unlock()

	if existing, ok := uc.reservations[gameID]; ok && existing.IsActive() {
		uc.recordError("already_reserved")
		return nil, entity.ErrCancelAlreadyReserved
	}

	walletGameIDs := uc.walletReservations[normalizedWallet]
	activeCount := 0
	for _, gid := range walletGameIDs {
		if reservation, ok := uc.reservations[gid]; ok && reservation.IsActive() {
			activeCount++
		}
	}
	if activeCount >= uc.maxPerWallet {
		uc.recordError("too_many_reservations")
		return nil, entity.ErrTooManyCancelReservations
	}

	now := time.Now()
	reservation := &entity.CancelReservation{
		GameID:        gameID,
		WalletAddress: walletAddress,
		CreatedAt:     now,
		ExpiresAt:     now.Add(uc.timeout),
		Status:        entity.CancelReservationStatusActive,
	}

	uc.reservations[gameID] = reservation
	uc.walletReservations[normalizedWallet] = append(uc.walletReservations[normalizedWallet], gameID)

	log.Info().
		Int64("game_id", gameID).
		Str("wallet", walletAddress).
		Time("expires_at", reservation.ExpiresAt).
		Msg("Cancel reservation created")

	if uc.metrics != nil {
		uc.metrics.RecordCreated()
	}
	if uc.broadcastUC != nil {
		uc.broadcastUC.BroadcastCancelReservationCreated(ctx, reservation)
	}

	return reservation, nil
}

// Get returns the active cancel reservation for a game, if any.
func (uc *CancelReservationUseCase) Get(_ context.Context, gameID int64) (*entity.CancelReservation, error) {
	uc.mu.RLock()
	reservation, ok := uc.reservations[gameID]
	uc.mu.RUnlock()
	if !ok || !reservation.IsActive() {
		return nil, nil
	}
	return reservation, nil
}

// ListByWallet returns all active cancel reservations for a wallet.
func (uc *CancelReservationUseCase) ListByWallet(ctx context.Context, walletAddress string) ([]*entity.CancelReservation, error) {
	normalizedWallet := normalizeReservationWallet(walletAddress)

	uc.mu.RLock()
	gameIDs := uc.walletReservations[normalizedWallet]
	reservations := make([]*entity.CancelReservation, 0, len(gameIDs))
	for _, gid := range gameIDs {
		if reservation, ok := uc.reservations[gid]; ok && reservation.IsActive() {
			reservations = append(reservations, reservation)
		}
	}
	uc.mu.RUnlock()

	result := make([]*entity.CancelReservation, 0, len(reservations))
	for _, reservation := range reservations {
		available, err := uc.isCancelableGameAvailable(ctx, reservation.GameID)
		if err != nil {
			return nil, err
		}
		if !available {
			uc.releaseStaleReservation(ctx, reservation.GameID, "resolved")
			continue
		}
		result = append(result, reservation)
	}

	sort.SliceStable(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})

	return result, nil
}

// Cancel releases a cancel reservation. Only the holder may cancel it.
func (uc *CancelReservationUseCase) Cancel(ctx context.Context, gameID int64, walletAddress string) error {
	normalizedWallet := normalizeReservationWallet(walletAddress)

	uc.mu.Lock()
	reservation, ok := uc.reservations[gameID]
	if !ok || !reservation.IsActive() {
		uc.mu.Unlock()
		uc.recordError("not_found")
		return entity.ErrCancelReservationNotFound
	}
	if !sameReservationWallet(reservation.WalletAddress, normalizedWallet) {
		uc.mu.Unlock()
		uc.recordError("not_holder")
		return entity.ErrNotCancelReservationHolder
	}

	uc.removeFromWalletIndex(reservation.WalletAddress, gameID)
	delete(uc.reservations, gameID)
	uc.mu.Unlock()

	log.Info().
		Int64("game_id", gameID).
		Str("wallet", walletAddress).
		Msg("Cancel reservation cancelled")

	if uc.metrics != nil {
		uc.metrics.RecordCancelled()
	}
	if uc.broadcastUC != nil {
		uc.broadcastUC.BroadcastCancelReservationReleased(ctx, gameID, "cancelled")
	}

	return nil
}

// ReleaseOnUnavailable releases a cancel reservation after authoritative game
// progress proves the waiting-game cancel flow is no longer applicable.
func (uc *CancelReservationUseCase) ReleaseOnUnavailable(ctx context.Context, gameID int64, reason string) {
	uc.mu.Lock()
	reservation, ok := uc.reservations[gameID]
	if !ok || !reservation.IsActive() {
		uc.mu.Unlock()
		return
	}

	uc.removeFromWalletIndex(reservation.WalletAddress, gameID)
	delete(uc.reservations, gameID)
	uc.mu.Unlock()

	releaseReason := reason
	if releaseReason == "" {
		releaseReason = "resolved"
	}

	log.Info().
		Int64("game_id", gameID).
		Str("wallet", reservation.WalletAddress).
		Str("reason", releaseReason).
		Msg("Cancel reservation released on authoritative progress")

	if uc.metrics != nil {
		uc.metrics.RecordResolved()
	}
	if uc.broadcastUC != nil {
		uc.broadcastUC.BroadcastCancelReservationReleased(ctx, gameID, releaseReason)
	}
}

// StartCleanupLoop runs the background expiration goroutine.
func (uc *CancelReservationUseCase) StartCleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(uc.cleanupInterval)
	go func() {
		for {
			select {
			case <-ticker.C:
				uc.CleanupExpired(ctx)
			case <-uc.stopCleanup:
				ticker.Stop()
				log.Info().Msg("Cancel reservation cleanup loop stopped")
				return
			case <-ctx.Done():
				ticker.Stop()
				log.Info().Msg("Cancel reservation cleanup loop stopped (context cancelled)")
				return
			}
		}
	}()

	log.Info().Dur("interval", uc.cleanupInterval).Msg("Cancel reservation cleanup loop started")
}

// StopCleanupLoop stops the background cleanup goroutine. Safe to call more than once.
func (uc *CancelReservationUseCase) StopCleanupLoop() {
	uc.stopOnce.Do(func() {
		close(uc.stopCleanup)
	})
}

// CleanupExpired releases expired cancel reservations.
func (uc *CancelReservationUseCase) CleanupExpired(ctx context.Context) {
	uc.mu.Lock()
	expired := make([]int64, 0)
	for gameID, reservation := range uc.reservations {
		if reservation.Status == entity.CancelReservationStatusActive && reservation.IsExpired() {
			uc.removeFromWalletIndex(reservation.WalletAddress, gameID)
			delete(uc.reservations, gameID)
			expired = append(expired, gameID)
		}
	}
	uc.mu.Unlock()

	for _, gameID := range expired {
		if uc.metrics != nil {
			uc.metrics.RecordExpired()
		}
		if uc.broadcastUC != nil {
			uc.broadcastUC.BroadcastCancelReservationReleased(ctx, gameID, "expired")
		}
	}

	if len(expired) > 0 {
		log.Info().Int("count", len(expired)).Msg("Cleaned up expired cancel reservations")
	}
}

// GetActiveCount returns the number of active cancel reservations.
func (uc *CancelReservationUseCase) GetActiveCount() int {
	uc.mu.RLock()
	defer uc.mu.RUnlock()
	count := 0
	for _, reservation := range uc.reservations {
		if reservation.IsActive() {
			count++
		}
	}
	return count
}

func (uc *CancelReservationUseCase) removeFromWalletIndex(walletAddress string, gameID int64) {
	walletKey := normalizeReservationWallet(walletAddress)
	gameIDs := uc.walletReservations[walletKey]
	for i, gid := range gameIDs {
		if gid == gameID {
			uc.walletReservations[walletKey] = append(gameIDs[:i], gameIDs[i+1:]...)
			break
		}
	}
	if len(uc.walletReservations[walletKey]) == 0 {
		delete(uc.walletReservations, walletKey)
	}
}

func (uc *CancelReservationUseCase) isCancelableGameAvailable(ctx context.Context, gameID int64) (bool, error) {
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

func (uc *CancelReservationUseCase) releaseStaleReservation(ctx context.Context, gameID int64, reason string) {
	uc.mu.Lock()
	reservation, ok := uc.reservations[gameID]
	if !ok {
		uc.mu.Unlock()
		return
	}

	uc.removeFromWalletIndex(reservation.WalletAddress, gameID)
	delete(uc.reservations, gameID)
	uc.mu.Unlock()

	if uc.metrics != nil {
		uc.metrics.RecordResolved()
	}
	if uc.broadcastUC != nil {
		uc.broadcastUC.BroadcastCancelReservationReleased(ctx, gameID, reason)
	}
}

func (uc *CancelReservationUseCase) recordError(errorType string) {
	if uc.metrics != nil {
		uc.metrics.RecordError(errorType)
	}
}
