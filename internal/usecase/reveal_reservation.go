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

// RevealReservationUseCase handles reveal-phase reservation business logic
// (spec 005-reveal-reservation). The implementation mirrors ReservationUseCase
// but is intentionally separate so the join and reveal lifecycles never
// collide and can evolve independently.
//
// Storage is in-memory + RWMutex; persistence is intentionally out of scope
// (Constitution II — advisory off-chain layer, safe to lose on restart).
type RevealReservationUseCase struct {
	// gameID -> reservation
	reservations map[int64]*entity.RevealReservation

	// normalized wallet -> []gameID
	walletReservations map[string][]int64

	maxPerWallet    int
	timeout         time.Duration
	cleanupInterval time.Duration

	mu sync.RWMutex

	gameRepo    repository.GameRepository
	broadcastUC *GameBroadcastUseCase
	metrics     *metrics.RevealReservationMetrics

	stopCleanup chan struct{}
	stopOnce    sync.Once
}

// RevealReservationConfig holds configuration for the reveal-reservation use case.
type RevealReservationConfig struct {
	MaxPerWallet           int
	TimeoutSeconds         int
	CleanupIntervalSeconds int
}

// NewRevealReservationUseCase creates a new RevealReservationUseCase.
func NewRevealReservationUseCase(
	gameRepo repository.GameRepository,
	broadcastUC *GameBroadcastUseCase,
	cfg RevealReservationConfig,
) *RevealReservationUseCase {
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 90 * time.Second // default 90s — slightly longer than join (60s) to absorb wallet UX delays
	}

	maxPerWallet := cfg.MaxPerWallet
	if maxPerWallet <= 0 {
		maxPerWallet = 5
	}

	cleanup := time.Duration(cfg.CleanupIntervalSeconds) * time.Second
	if cleanup <= 0 {
		cleanup = 5 * time.Second
	}

	return &RevealReservationUseCase{
		reservations:       make(map[int64]*entity.RevealReservation),
		walletReservations: make(map[string][]int64),
		maxPerWallet:       maxPerWallet,
		timeout:            timeout,
		cleanupInterval:    cleanup,
		gameRepo:           gameRepo,
		broadcastUC:        broadcastUC,
		stopCleanup:        make(chan struct{}),
	}
}

// SetMetrics wires Prometheus metrics. Optional.
func (uc *RevealReservationUseCase) SetMetrics(m *metrics.RevealReservationMetrics) {
	uc.metrics = m
}

// Reserve creates a reveal reservation for a single game (FR-001..003).
func (uc *RevealReservationUseCase) Reserve(ctx context.Context, gameID int64, walletAddress string) (*entity.RevealReservation, error) {
	res, err := uc.ReserveBatch(ctx, []int64{gameID}, walletAddress)
	if err != nil {
		return nil, err
	}
	if len(res) == 0 {
		return nil, entity.ErrRevealReservationNotFound
	}
	return res[0], nil
}

// ReserveBatch creates reveal reservations for a set of games atomically (FR-004).
// All-or-nothing: if any game cannot be reserved, no reservation is created.
func (uc *RevealReservationUseCase) ReserveBatch(ctx context.Context, gameIDs []int64, walletAddress string) ([]*entity.RevealReservation, error) {
	if len(gameIDs) == 0 {
		return nil, entity.ErrRevealNotAvailable
	}

	normalized := normalizeReservationWallet(walletAddress)
	if normalized == "" {
		return nil, entity.ErrNotAPlayer
	}

	// 1. Pre-flight: validate every game is in waiting_for_open_bids and the caller is a participant.
	// Done outside the lock to avoid holding it during DB calls.
	for _, gameID := range gameIDs {
		game, err := uc.gameRepo.GetByID(ctx, gameID)
		if err != nil {
			return nil, err
		}
		if game == nil {
			return nil, entity.ErrRevealNotAvailable
		}
		if game.Status != entity.GameStatusWaitingForOpenBids {
			return nil, entity.ErrRevealNotAvailable
		}
		if !isGameParticipant(game, normalized) {
			return nil, entity.ErrNotAPlayer
		}
	}

	// 2. Atomic check + insert under a single lock acquisition.
	uc.mu.Lock()

	// Check capacity for the wallet (only count active).
	walletGameIDs := uc.walletReservations[normalized]
	activeCount := 0
	for _, gid := range walletGameIDs {
		if r, ok := uc.reservations[gid]; ok && r.IsActive() {
			activeCount++
		}
	}
	if activeCount+len(gameIDs) > uc.maxPerWallet {
		uc.mu.Unlock()
		return nil, entity.ErrTooManyRevealReservations
	}

	// Check none of the requested ids is already reserved by anyone.
	for _, gameID := range gameIDs {
		if existing, ok := uc.reservations[gameID]; ok && existing.IsActive() {
			uc.mu.Unlock()
			return nil, entity.ErrRevealAlreadyReserved
		}
	}

	now := time.Now()
	created := make([]*entity.RevealReservation, 0, len(gameIDs))
	for _, gameID := range gameIDs {
		r := &entity.RevealReservation{
			GameID:        gameID,
			WalletAddress: walletAddress,
			CreatedAt:     now,
			ExpiresAt:     now.Add(uc.timeout),
			Status:        entity.RevealReservationStatusActive,
		}
		uc.reservations[gameID] = r
		uc.walletReservations[normalized] = append(uc.walletReservations[normalized], gameID)
		created = append(created, r)
	}

	uc.mu.Unlock()

	for _, r := range created {
		log.Info().
			Int64("game_id", r.GameID).
			Str("wallet", r.WalletAddress).
			Time("expires_at", r.ExpiresAt).
			Msg("Reveal reservation created")

		if uc.metrics != nil {
			uc.metrics.RecordCreated()
		}
		if uc.broadcastUC != nil {
			uc.broadcastUC.BroadcastRevealReservationCreated(ctx, r)
		}
	}

	return created, nil
}

// Get returns the active reveal reservation for a game, if any.
func (uc *RevealReservationUseCase) Get(_ context.Context, gameID int64) (*entity.RevealReservation, error) {
	uc.mu.RLock()
	r, ok := uc.reservations[gameID]
	uc.mu.RUnlock()
	if !ok || !r.IsActive() {
		return nil, nil
	}
	return r, nil
}

// ListByWallet returns all active reveal reservations for a wallet.
func (uc *RevealReservationUseCase) ListByWallet(_ context.Context, walletAddress string) ([]*entity.RevealReservation, error) {
	normalized := normalizeReservationWallet(walletAddress)

	uc.mu.RLock()
	gameIDs := uc.walletReservations[normalized]
	out := make([]*entity.RevealReservation, 0, len(gameIDs))
	for _, gid := range gameIDs {
		if r, ok := uc.reservations[gid]; ok && r.IsActive() {
			out = append(out, r)
		}
	}
	uc.mu.RUnlock()

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

// Cancel releases a single reveal reservation. Only the holder may cancel.
func (uc *RevealReservationUseCase) Cancel(ctx context.Context, gameID int64, walletAddress string) error {
	normalized := normalizeReservationWallet(walletAddress)

	uc.mu.Lock()
	r, ok := uc.reservations[gameID]
	if !ok || !r.IsActive() {
		uc.mu.Unlock()
		return entity.ErrRevealReservationNotFound
	}
	if !sameReservationWallet(r.WalletAddress, normalized) {
		uc.mu.Unlock()
		return entity.ErrNotRevealReservationHolder
	}
	r.Status = entity.RevealReservationStatusReleased
	uc.removeFromRevealWalletIndex(r.WalletAddress, gameID)
	uc.mu.Unlock()

	log.Info().Int64("game_id", gameID).Str("wallet", walletAddress).Msg("Reveal reservation cancelled")
	if uc.metrics != nil {
		uc.metrics.RecordCancelled()
	}
	if uc.broadcastUC != nil {
		uc.broadcastUC.BroadcastRevealReservationReleased(ctx, gameID, "cancelled")
	}
	return nil
}

// ReleaseOnTerminal releases a reveal reservation when the on-chain game
// reached a terminal status (ended/paid/cancelled). Idempotent. Called from
// the persistence use case after the DB write of the terminal status has
// committed (so a crash between event ingest and DB commit does not release a
// reservation that the rest of the system still considers active).
func (uc *RevealReservationUseCase) ReleaseOnTerminal(ctx context.Context, gameID int64) {
	uc.mu.Lock()
	r, ok := uc.reservations[gameID]
	if !ok {
		uc.mu.Unlock()
		return
	}
	if !r.IsActive() {
		uc.mu.Unlock()
		return
	}
	r.Status = entity.RevealReservationStatusReleased
	uc.removeFromRevealWalletIndex(r.WalletAddress, gameID)
	uc.mu.Unlock()

	log.Info().Int64("game_id", gameID).Str("wallet", r.WalletAddress).Msg("Reveal reservation released on terminal on-chain status")
	if uc.metrics != nil {
		uc.metrics.RecordRevealed()
	}
	if uc.broadcastUC != nil {
		uc.broadcastUC.BroadcastRevealReservationReleased(ctx, gameID, "revealed")
	}
}

// StartCleanupLoop runs the background expiration goroutine.
func (uc *RevealReservationUseCase) StartCleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(uc.cleanupInterval)
	go func() {
		for {
			select {
			case <-ticker.C:
				uc.cleanupExpired(ctx)
			case <-uc.stopCleanup:
				ticker.Stop()
				log.Info().Msg("Reveal reservation cleanup loop stopped")
				return
			case <-ctx.Done():
				ticker.Stop()
				log.Info().Msg("Reveal reservation cleanup loop stopped (context cancelled)")
				return
			}
		}
	}()
	log.Info().Dur("interval", uc.cleanupInterval).Msg("Reveal reservation cleanup loop started")
}

// StopCleanupLoop stops the background goroutine. Safe to call multiple times.
func (uc *RevealReservationUseCase) StopCleanupLoop() {
	uc.stopOnce.Do(func() {
		close(uc.stopCleanup)
	})
}

// GetActiveCount returns the number of currently active reveal reservations.
func (uc *RevealReservationUseCase) GetActiveCount() int {
	uc.mu.RLock()
	defer uc.mu.RUnlock()
	count := 0
	for _, r := range uc.reservations {
		if r.IsActive() {
			count++
		}
	}
	return count
}

func (uc *RevealReservationUseCase) cleanupExpired(ctx context.Context) {
	uc.mu.Lock()

	var expired []int64
	for gameID, r := range uc.reservations {
		if r.Status == entity.RevealReservationStatusActive && r.IsExpired() {
			r.Status = entity.RevealReservationStatusExpired
			uc.removeFromRevealWalletIndex(r.WalletAddress, gameID)
			expired = append(expired, gameID)
		}
		if !r.IsActive() {
			// Reclaim memory after status flip.
			delete(uc.reservations, gameID)
		}
	}

	uc.mu.Unlock()

	for _, gameID := range expired {
		if uc.metrics != nil {
			uc.metrics.RecordExpired()
		}
		if uc.broadcastUC != nil {
			uc.broadcastUC.BroadcastRevealReservationReleased(ctx, gameID, "expired")
		}
	}

	if len(expired) > 0 {
		log.Info().Int("count", len(expired)).Msg("Cleaned up expired reveal reservations")
	}
}

// removeFromRevealWalletIndex removes a gameID from the wallet's list. Caller MUST hold uc.mu.
func (uc *RevealReservationUseCase) removeFromRevealWalletIndex(walletAddress string, gameID int64) {
	key := normalizeReservationWallet(walletAddress)
	ids := uc.walletReservations[key]
	for i, gid := range ids {
		if gid == gameID {
			uc.walletReservations[key] = append(ids[:i], ids[i+1:]...)
			break
		}
	}
	if len(uc.walletReservations[key]) == 0 {
		delete(uc.walletReservations, key)
	}
}

// isGameParticipant returns true if the normalized wallet is player one or two of the game.
func isGameParticipant(game *entity.Game, normalizedWallet string) bool {
	if sameReservationWallet(game.PlayerOneAddress, normalizedWallet) {
		return true
	}
	if game.PlayerTwoAddress != nil && sameReservationWallet(*game.PlayerTwoAddress, normalizedWallet) {
		return true
	}
	return false
}
