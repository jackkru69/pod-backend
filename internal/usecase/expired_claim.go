package usecase

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"pod-backend/internal/entity"
	"pod-backend/internal/infrastructure/metrics"
	"pod-backend/internal/repository"
)

// ExpiredClaimUseCase coordinates advisory expired-follow-up claims for games
// that remain in the expired-attention queue.
type ExpiredClaimUseCase struct {
	claims       map[int64]*entity.ExpiredClaim
	walletClaims map[string][]int64

	maxPerWallet    int
	timeout         time.Duration
	cleanupInterval time.Duration

	mu sync.RWMutex

	gameRepo    repository.GameRepository
	broadcastUC *GameBroadcastUseCase
	metrics     *metrics.ExpiredClaimMetrics

	stopCleanup chan struct{}
	stopOnce    sync.Once
}

// ExpiredClaimConfig holds configuration for expired-follow-up claims.
type ExpiredClaimConfig struct {
	MaxPerWallet           int
	TimeoutSeconds         int
	CleanupIntervalSeconds int
}

// NewExpiredClaimUseCase creates a new ExpiredClaimUseCase.
func NewExpiredClaimUseCase(
	gameRepo repository.GameRepository,
	broadcastUC *GameBroadcastUseCase,
	cfg ExpiredClaimConfig,
) *ExpiredClaimUseCase {
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}

	maxPerWallet := cfg.MaxPerWallet
	if maxPerWallet <= 0 {
		maxPerWallet = 5
	}

	cleanup := time.Duration(cfg.CleanupIntervalSeconds) * time.Second
	if cleanup <= 0 {
		cleanup = 5 * time.Second
	}

	return &ExpiredClaimUseCase{
		claims:          make(map[int64]*entity.ExpiredClaim),
		walletClaims:    make(map[string][]int64),
		maxPerWallet:    maxPerWallet,
		timeout:         timeout,
		cleanupInterval: cleanup,
		gameRepo:        gameRepo,
		broadcastUC:     broadcastUC,
		stopCleanup:     make(chan struct{}),
	}
}

// SetMetrics wires Prometheus metrics. Optional.
func (uc *ExpiredClaimUseCase) SetMetrics(m *metrics.ExpiredClaimMetrics) {
	uc.metrics = m
}

// Claim creates or resumes an expired-follow-up claim for a game.
func (uc *ExpiredClaimUseCase) Claim(ctx context.Context, gameID int64, walletAddress string) (*entity.ExpiredClaim, error) {
	normalized := normalizeReservationWallet(walletAddress)
	if normalized == "" {
		return nil, entity.ErrExpiredClaimWalletRequired
	}

	game, err := uc.gameRepo.GetByID(ctx, gameID)
	if err != nil {
		return nil, err
	}
	if game == nil || game.Status != entity.GameStatusEnded {
		return nil, entity.ErrExpiredClaimNotAvailable
	}
	if !isGameParticipant(game, normalized) {
		return nil, entity.ErrNotExpiredClaimParticipant
	}

	uc.mu.Lock()
	if existing, ok := uc.claims[gameID]; ok && existing.IsActive() {
		uc.mu.Unlock()
		if sameReservationWallet(existing.WalletAddress, normalized) {
			return existing, nil
		}
		return nil, entity.ErrExpiredClaimAlreadyClaimed
	}

	walletGameIDs := uc.walletClaims[normalized]
	activeCount := 0
	for _, gid := range walletGameIDs {
		if claim, ok := uc.claims[gid]; ok && claim.IsActive() {
			activeCount++
		}
	}
	if activeCount >= uc.maxPerWallet {
		uc.mu.Unlock()
		return nil, entity.ErrTooManyExpiredClaims
	}

	now := time.Now()
	claim := &entity.ExpiredClaim{
		GameID:        gameID,
		WalletAddress: walletAddress,
		CreatedAt:     now,
		ExpiresAt:     now.Add(uc.timeout),
		Status:        entity.ExpiredClaimStatusActive,
	}

	uc.claims[gameID] = claim
	uc.walletClaims[normalized] = append(uc.walletClaims[normalized], gameID)
	uc.mu.Unlock()

	log.Info().
		Int64("game_id", gameID).
		Str("wallet", walletAddress).
		Time("expires_at", claim.ExpiresAt).
		Msg("Expired follow-up claim created")

	if uc.metrics != nil {
		uc.metrics.RecordCreated()
	}
	if uc.broadcastUC != nil {
		uc.broadcastUC.BroadcastExpiredClaimCreated(ctx, claim)
	}

	return claim, nil
}

// Get returns the active expired-follow-up claim for a game, if any.
func (uc *ExpiredClaimUseCase) Get(ctx context.Context, gameID int64) (*entity.ExpiredClaim, error) {
	claim := uc.peekActiveClaim(gameID)
	if claim == nil {
		return nil, nil
	}

	isActionable, err := uc.isClaimGameActionable(ctx, gameID)
	if err != nil {
		return nil, err
	}
	if !isActionable {
		uc.ReleaseOnResolved(ctx, gameID)
		return nil, nil
	}

	return claim, nil
}

// ListByWallet returns all active expired-follow-up claims for a wallet.
func (uc *ExpiredClaimUseCase) ListByWallet(ctx context.Context, walletAddress string) ([]*entity.ExpiredClaim, error) {
	normalized := normalizeReservationWallet(walletAddress)
	if normalized == "" {
		return []*entity.ExpiredClaim{}, nil
	}

	uc.mu.RLock()
	gameIDs := uc.walletClaims[normalized]
	claims := make([]*entity.ExpiredClaim, 0, len(gameIDs))
	for _, gameID := range gameIDs {
		if claim, ok := uc.claims[gameID]; ok && claim.IsActive() {
			claims = append(claims, claim)
		}
	}
	uc.mu.RUnlock()

	result := make([]*entity.ExpiredClaim, 0, len(claims))
	for _, claim := range claims {
		isActionable, err := uc.isClaimGameActionable(ctx, claim.GameID)
		if err != nil {
			return nil, err
		}
		if !isActionable {
			uc.ReleaseOnResolved(ctx, claim.GameID)
			continue
		}
		result = append(result, claim)
	}

	sort.SliceStable(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})

	return result, nil
}

// Release cancels an expired-follow-up claim. Only the holder may release it.
func (uc *ExpiredClaimUseCase) Release(ctx context.Context, gameID int64, walletAddress string) error {
	normalized := normalizeReservationWallet(walletAddress)
	if normalized == "" {
		return entity.ErrExpiredClaimWalletRequired
	}

	uc.mu.Lock()
	claim, ok := uc.claims[gameID]
	if !ok || !claim.IsActive() {
		uc.mu.Unlock()
		return entity.ErrExpiredClaimNotFound
	}
	if !sameReservationWallet(claim.WalletAddress, normalized) {
		uc.mu.Unlock()
		return entity.ErrNotExpiredClaimHolder
	}

	claim.Status = entity.ExpiredClaimStatusReleased
	uc.removeFromWalletIndex(claim.WalletAddress, gameID)
	uc.mu.Unlock()

	log.Info().
		Int64("game_id", gameID).
		Str("wallet", walletAddress).
		Msg("Expired follow-up claim cancelled")

	if uc.metrics != nil {
		uc.metrics.RecordCancelled()
	}
	if uc.broadcastUC != nil {
		uc.broadcastUC.BroadcastExpiredClaimReleased(ctx, gameID, "cancelled")
	}

	return nil
}

// ReleaseOnResolved releases an expired-follow-up claim because the game is no
// longer actionable in the expired-attention queue.
func (uc *ExpiredClaimUseCase) ReleaseOnResolved(ctx context.Context, gameID int64) {
	uc.releaseWithReason(ctx, gameID, "resolved", func() {
		if uc.metrics != nil {
			uc.metrics.RecordResolved()
		}
	})
}

// StartCleanupLoop runs the background expiration goroutine.
func (uc *ExpiredClaimUseCase) StartCleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(uc.cleanupInterval)
	go func() {
		for {
			select {
			case <-ticker.C:
				uc.CleanupExpired(ctx)
			case <-uc.stopCleanup:
				ticker.Stop()
				log.Info().Msg("Expired-claim cleanup loop stopped")
				return
			case <-ctx.Done():
				ticker.Stop()
				log.Info().Msg("Expired-claim cleanup loop stopped (context cancelled)")
				return
			}
		}
	}()

	log.Info().Dur("interval", uc.cleanupInterval).Msg("Expired-claim cleanup loop started")
}

// StopCleanupLoop stops the background cleanup goroutine.
func (uc *ExpiredClaimUseCase) StopCleanupLoop() {
	uc.stopOnce.Do(func() {
		close(uc.stopCleanup)
	})
}

// CleanupExpired removes expired claims and broadcasts updates.
func (uc *ExpiredClaimUseCase) CleanupExpired(ctx context.Context) {
	uc.mu.Lock()

	var expiredGameIDs []int64
	for gameID, claim := range uc.claims {
		if claim.Status == entity.ExpiredClaimStatusActive && claim.IsExpired() {
			claim.Status = entity.ExpiredClaimStatusExpired
			uc.removeFromWalletIndex(claim.WalletAddress, gameID)
			expiredGameIDs = append(expiredGameIDs, gameID)
		}
		if !claim.IsActive() {
			delete(uc.claims, gameID)
		}
	}

	uc.mu.Unlock()

	for _, gameID := range expiredGameIDs {
		if uc.metrics != nil {
			uc.metrics.RecordExpired()
		}
		if uc.broadcastUC != nil {
			uc.broadcastUC.BroadcastExpiredClaimReleased(ctx, gameID, "expired")
		}
	}

	if len(expiredGameIDs) > 0 {
		log.Info().Int("count", len(expiredGameIDs)).Msg("Cleaned up expired follow-up claims")
	}
}

// GetActiveCount returns the number of currently active expired claims.
func (uc *ExpiredClaimUseCase) GetActiveCount() int {
	uc.mu.RLock()
	defer uc.mu.RUnlock()

	count := 0
	for _, claim := range uc.claims {
		if claim.IsActive() {
			count++
		}
	}
	return count
}

func (uc *ExpiredClaimUseCase) peekActiveClaim(gameID int64) *entity.ExpiredClaim {
	uc.mu.RLock()
	defer uc.mu.RUnlock()

	claim, ok := uc.claims[gameID]
	if !ok || !claim.IsActive() {
		return nil
	}

	return claim
}

func (uc *ExpiredClaimUseCase) isClaimGameActionable(ctx context.Context, gameID int64) (bool, error) {
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

	return game.Status == entity.GameStatusEnded, nil
}

func (uc *ExpiredClaimUseCase) releaseWithReason(
	ctx context.Context,
	gameID int64,
	reason string,
	metricsHook func(),
) {
	uc.mu.Lock()
	claim, ok := uc.claims[gameID]
	if !ok || !claim.IsActive() {
		uc.mu.Unlock()
		return
	}

	claim.Status = entity.ExpiredClaimStatusReleased
	uc.removeFromWalletIndex(claim.WalletAddress, gameID)
	uc.mu.Unlock()

	if metricsHook != nil {
		metricsHook()
	}
	if uc.broadcastUC != nil {
		uc.broadcastUC.BroadcastExpiredClaimReleased(ctx, gameID, reason)
	}
}

func (uc *ExpiredClaimUseCase) removeFromWalletIndex(walletAddress string, gameID int64) {
	key := normalizeReservationWallet(walletAddress)
	ids := uc.walletClaims[key]
	for i, gid := range ids {
		if gid == gameID {
			uc.walletClaims[key] = append(ids[:i], ids[i+1:]...)
			break
		}
	}
	if len(uc.walletClaims[key]) == 0 {
		delete(uc.walletClaims, key)
	}
}
