package usecase

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"pod-backend/internal/entity"
	"pod-backend/internal/repository"
)

const activityClaimCapacity = 3

// GameActivityConfig controls queue pagination defaults for the additive
// activity surfaces.
type GameActivityConfig struct {
	DefaultLimit int
	MaxLimit     int
}

// GameActivityUseCase provides queue-oriented game discovery without changing
// the contract-led game lifecycle semantics.
type GameActivityUseCase struct {
	gameRepo            repository.GameRepository
	reservationUC       *ReservationUseCase
	revealReservationUC *RevealReservationUseCase
	expiredClaimUC      *ExpiredClaimUseCase
	config              GameActivityConfig
}

// NewGameActivityUseCase creates a new queue-oriented activity use case.
func NewGameActivityUseCase(
	gameRepo repository.GameRepository,
	reservationUC *ReservationUseCase,
	revealReservationUC *RevealReservationUseCase,
	expiredClaimUC *ExpiredClaimUseCase,
	cfg GameActivityConfig,
) *GameActivityUseCase {
	if cfg.DefaultLimit <= 0 {
		cfg.DefaultLimit = 20
	}
	if cfg.MaxLimit <= 0 {
		cfg.MaxLimit = 100
	}

	return &GameActivityUseCase{
		gameRepo:            gameRepo,
		reservationUC:       reservationUC,
		revealReservationUC: revealReservationUC,
		expiredClaimUC:      expiredClaimUC,
		config:              cfg,
	}
}

// GetQueue returns the requested player-facing queue together with the summary
// counts used by the queue shell.
func (uc *GameActivityUseCase) GetQueue(
	ctx context.Context,
	queueKey entity.ActivityQueueKey,
	walletAddress string,
	limit int,
	offset int,
) (*entity.GameplayQueue, *entity.PlayerActivitySummary, error) {
	if !entity.IsValidActivityQueueKey(queueKey) {
		return nil, nil, entity.ErrInvalidActivityQueueKey
	}

	if offset < 0 {
		return nil, nil, entity.ErrInvalidActivityOffset
	}

	limit = uc.normalizeLimit(limit)
	summary, queues, err := uc.buildQueues(ctx, walletAddress)
	if err != nil {
		return nil, nil, err
	}

	items := queues[queueKey]
	pagedItems := paginateActivityItems(items, limit, offset)

	return &entity.GameplayQueue{
		Key:               queueKey,
		Title:             entity.ActivityQueueTitle(queueKey),
		AttentionRequired: entity.QueueRequiresAttention(queueKey),
		TotalCount:        len(items),
		Items:             pagedItems,
	}, summary, nil
}

// GetSummary returns the cross-queue counts for a specific wallet.
func (uc *GameActivityUseCase) GetSummary(
	ctx context.Context,
	walletAddress string,
) (*entity.PlayerActivitySummary, error) {
	if normalizeReservationWallet(walletAddress) == "" {
		return nil, entity.ErrActivityWalletRequired
	}

	summary, _, err := uc.buildQueues(ctx, walletAddress)
	if err != nil {
		return nil, err
	}

	return summary, nil
}

// Search returns activity items matching a wallet/opponent/game identifier
// query across the player's currently visible activity surfaces.
func (uc *GameActivityUseCase) Search(
	ctx context.Context,
	walletAddress string,
	rawQuery string,
	queueScope entity.ActivityQueueKey,
	limit int,
	offset int,
) ([]*entity.PlayerActivityGame, *entity.PlayerActivitySummary, int, error) {
	trimmedQuery := strings.TrimSpace(rawQuery)

	if offset < 0 {
		return nil, nil, 0, entity.ErrInvalidActivityOffset
	}

	if queueScope != "" && !entity.IsValidActivityQueueKey(queueScope) {
		return nil, nil, 0, entity.ErrInvalidActivityQueueKey
	}

	if trimmedQuery == "" {
		return []*entity.PlayerActivityGame{}, buildPlayerActivitySummary(walletAddress, newActivityQueueMap()), 0, nil
	}

	limit = uc.normalizeLimit(limit)
	summary, queues, err := uc.buildQueues(ctx, walletAddress)
	if err != nil {
		return nil, nil, 0, err
	}

	matchedItems := filterActivitySearchItems(queues, queueScope, trimmedQuery)
	return paginateActivityItems(matchedItems, limit, offset), summary, len(matchedItems), nil
}

func (uc *GameActivityUseCase) buildQueues(
	ctx context.Context,
	walletAddress string,
) (*entity.PlayerActivitySummary, map[entity.ActivityQueueKey][]*entity.PlayerActivityGame, error) {
	normalizedWallet := normalizeReservationWallet(walletAddress)
	queues := newActivityQueueMap()

	if err := uc.populateJoinableQueue(ctx, normalizedWallet, queues); err != nil {
		return nil, nil, err
	}

	if err := uc.populatePlayerQueues(ctx, walletAddress, normalizedWallet, queues); err != nil {
		return nil, nil, err
	}

	for queueKey := range queues {
		sortActivityItems(queues[queueKey])
	}

	summary := buildPlayerActivitySummary(walletAddress, queues)
	return summary, queues, nil
}

func newActivityQueueMap() map[entity.ActivityQueueKey][]*entity.PlayerActivityGame {
	return map[entity.ActivityQueueKey][]*entity.PlayerActivityGame{
		entity.ActivityQueueJoinable:         {},
		entity.ActivityQueueMyActive:         {},
		entity.ActivityQueueRevealRequired:   {},
		entity.ActivityQueueExpiredAttention: {},
		entity.ActivityQueueHistory:          {},
	}
}

func (uc *GameActivityUseCase) populateJoinableQueue(
	ctx context.Context,
	normalizedWallet string,
	queues map[entity.ActivityQueueKey][]*entity.PlayerActivityGame,
) error {
	joinableGames, err := uc.gameRepo.GetByStatus(ctx, entity.GameStatusWaitingForOpponent)
	if err != nil {
		return fmt.Errorf("load joinable games: %w", err)
	}

	for _, game := range joinableGames {
		if normalizedWallet != "" && belongsToWallet(game, normalizedWallet) {
			continue
		}

		item, buildErr := uc.buildJoinableItem(ctx, game, normalizedWallet)
		if buildErr != nil {
			return fmt.Errorf("classify joinable game %d: %w", game.GameID, buildErr)
		}

		queues[entity.ActivityQueueJoinable] = append(queues[entity.ActivityQueueJoinable], item)
	}

	return nil
}

func (uc *GameActivityUseCase) populatePlayerQueues(
	ctx context.Context,
	walletAddress string,
	normalizedWallet string,
	queues map[entity.ActivityQueueKey][]*entity.PlayerActivityGame,
) error {
	if normalizedWallet == "" {
		return nil
	}

	playerGames, err := uc.gameRepo.GetByPlayerAndStatuses(
		ctx,
		walletAddress,
		[]int{
			entity.GameStatusWaitingForOpponent,
			entity.GameStatusWaitingForOpenBids,
			entity.GameStatusEnded,
			entity.GameStatusPaid,
		},
	)
	if err != nil {
		return fmt.Errorf("load player activity games: %w", err)
	}

	for _, game := range playerGames {
		item, buildErr := uc.buildPlayerItem(ctx, game, normalizedWallet)
		if buildErr != nil {
			return fmt.Errorf("classify player game %d: %w", game.GameID, buildErr)
		}
		if item == nil {
			continue
		}

		queues[item.QueueKey] = append(queues[item.QueueKey], item)
	}

	return nil
}

func (uc *GameActivityUseCase) buildJoinableItem(
	ctx context.Context,
	game *entity.Game,
	normalizedWallet string,
) (*entity.PlayerActivityGame, error) {
	claims, err := uc.loadClaims(ctx, game.GameID)
	if err != nil {
		return nil, err
	}

	nextAction := entity.ActivityNextActionJoin
	requiresAttention := false

	if claim := findClaim(claims, entity.ActionClaimTypeJoin); claim != nil {
		if sameReservationWallet(claim.HolderWallet, normalizedWallet) {
			nextAction = entity.ActivityNextActionResumeJoin
			requiresAttention = true
		} else {
			nextAction = entity.ActivityNextActionWaitForJoinWindow
		}
	}

	return &entity.PlayerActivityGame{
		Game:              game,
		QueueKey:          entity.ActivityQueueJoinable,
		NextAction:        nextAction,
		RequiresAttention: requiresAttention,
		IsOwnedByPlayer:   false,
		LatestActivityAt:  latestGameActivity(game),
		ActiveClaims:      claims,
	}, nil
}

func (uc *GameActivityUseCase) buildPlayerItem(
	ctx context.Context,
	game *entity.Game,
	normalizedWallet string,
) (*entity.PlayerActivityGame, error) {
	claims, err := uc.loadClaims(ctx, game.GameID)
	if err != nil {
		return nil, err
	}

	item := &entity.PlayerActivityGame{
		Game:             game,
		IsOwnedByPlayer:  true,
		LatestActivityAt: latestGameActivity(game),
		ActiveClaims:     claims,
	}

	switch game.Status {
	case entity.GameStatusWaitingForOpponent:
		item.QueueKey = entity.ActivityQueueMyActive
		item.NextAction = entity.ActivityNextActionWaitForOpponent
		item.RequiresAttention = false
	case entity.GameStatusWaitingForOpenBids:
		if playerNeedsReveal(game, normalizedWallet) {
			item.QueueKey = entity.ActivityQueueRevealRequired
			item.NextAction, item.RequiresAttention = resolveRevealNextAction(claims, normalizedWallet)
			return item, nil
		}

		item.QueueKey = entity.ActivityQueueMyActive
		item.NextAction = entity.ActivityNextActionWaitForReveal
		item.RequiresAttention = false
	case entity.GameStatusEnded:
		item.QueueKey = entity.ActivityQueueExpiredAttention
		item.NextAction, item.RequiresAttention = resolveExpiredFollowUpNextAction(claims, normalizedWallet)
	case entity.GameStatusPaid:
		item.QueueKey = entity.ActivityQueueHistory
		item.NextAction = entity.ActivityNextActionBrowseHistory
		item.RequiresAttention = false
	default:
		return nil, nil
	}

	return item, nil
}

func (uc *GameActivityUseCase) loadClaims(ctx context.Context, gameID int64) ([]entity.ActionClaim, error) {
	claims := make([]entity.ActionClaim, 0, activityClaimCapacity)

	if uc.reservationUC != nil {
		reservation, err := uc.reservationUC.GetReservation(ctx, gameID)
		if err != nil {
			return nil, fmt.Errorf("get join reservation: %w", err)
		}
		if reservation != nil && reservation.IsActive() {
			claims = append(claims, entity.ActionClaim{
				ClaimType:    entity.ActionClaimTypeJoin,
				GameID:       reservation.GameID,
				HolderWallet: reservation.WalletAddress,
				Status:       entity.ActionClaimStatusActive,
				CreatedAt:    reservation.CreatedAt,
				ExpiresAt:    reservation.ExpiresAt,
			})
		}
	}

	if uc.revealReservationUC != nil {
		revealReservation, err := uc.revealReservationUC.Get(ctx, gameID)
		if err != nil {
			return nil, fmt.Errorf("get reveal reservation: %w", err)
		}
		if revealReservation != nil && revealReservation.IsActive() {
			claims = append(claims, entity.ActionClaim{
				ClaimType:    entity.ActionClaimTypeReveal,
				GameID:       revealReservation.GameID,
				HolderWallet: revealReservation.WalletAddress,
				Status:       entity.ActionClaimStatusActive,
				CreatedAt:    revealReservation.CreatedAt,
				ExpiresAt:    revealReservation.ExpiresAt,
			})
		}
	}

	if uc.expiredClaimUC != nil {
		expiredClaim, err := uc.expiredClaimUC.Get(ctx, gameID)
		if err != nil {
			return nil, fmt.Errorf("get expired follow-up claim: %w", err)
		}
		if expiredClaim != nil && expiredClaim.IsActive() {
			claims = append(claims, entity.ActionClaim{
				ClaimType:    entity.ActionClaimTypeExpiredFollowUp,
				GameID:       expiredClaim.GameID,
				HolderWallet: expiredClaim.WalletAddress,
				Status:       entity.ActionClaimStatusActive,
				CreatedAt:    expiredClaim.CreatedAt,
				ExpiresAt:    expiredClaim.ExpiresAt,
			})
		}
	}

	return claims, nil
}

func (uc *GameActivityUseCase) normalizeLimit(limit int) int {
	if limit <= 0 {
		return uc.config.DefaultLimit
	}
	if limit > uc.config.MaxLimit {
		return uc.config.MaxLimit
	}
	return limit
}

func buildPlayerActivitySummary(
	walletAddress string,
	queues map[entity.ActivityQueueKey][]*entity.PlayerActivityGame,
) *entity.PlayerActivitySummary {
	summary := &entity.PlayerActivitySummary{
		WalletAddress:         walletAddress,
		JoinableCount:         len(queues[entity.ActivityQueueJoinable]),
		MyActiveCount:         len(queues[entity.ActivityQueueMyActive]),
		RevealRequiredCount:   len(queues[entity.ActivityQueueRevealRequired]),
		ExpiredAttentionCount: len(queues[entity.ActivityQueueExpiredAttention]),
		HistoryCount:          len(queues[entity.ActivityQueueHistory]),
	}

	var latest *entity.PlayerActivityGame
	for _, items := range queues {
		for _, item := range items {
			if latest == nil || item.LatestActivityAt.After(latest.LatestActivityAt) {
				latest = item
			}
		}
	}

	if latest != nil {
		lastActivityAt := latest.LatestActivityAt
		summary.LastActivityAt = &lastActivityAt
	}

	return summary
}

func resolveRevealNextAction(
	claims []entity.ActionClaim,
	normalizedWallet string,
) (entity.PlayerActivityNextAction, bool) {
	claim := findClaim(claims, entity.ActionClaimTypeReveal)
	if claim == nil {
		return entity.ActivityNextActionReveal, true
	}

	if sameReservationWallet(claim.HolderWallet, normalizedWallet) {
		return entity.ActivityNextActionResumeReveal, true
	}

	return entity.ActivityNextActionWaitForReveal, false
}

func resolveExpiredFollowUpNextAction(
	claims []entity.ActionClaim,
	normalizedWallet string,
) (entity.PlayerActivityNextAction, bool) {
	claim := findClaim(claims, entity.ActionClaimTypeExpiredFollowUp)
	if claim == nil {
		return entity.ActivityNextActionReviewResult, true
	}

	if sameReservationWallet(claim.HolderWallet, normalizedWallet) {
		return entity.ActivityNextActionResumeReview, true
	}

	return entity.ActivityNextActionWaitForReview, false
}

func findClaim(
	claims []entity.ActionClaim,
	claimType entity.ActionClaimType,
) *entity.ActionClaim {
	for i := range claims {
		if claims[i].ClaimType == claimType {
			return &claims[i]
		}
	}

	return nil
}

func belongsToWallet(game *entity.Game, normalizedWallet string) bool {
	if game == nil {
		return false
	}

	if sameReservationWallet(game.PlayerOneAddress, normalizedWallet) {
		return true
	}

	return game.PlayerTwoAddress != nil && sameReservationWallet(*game.PlayerTwoAddress, normalizedWallet)
}

func playerNeedsReveal(game *entity.Game, normalizedWallet string) bool {
	if game == nil || game.Status != entity.GameStatusWaitingForOpenBids {
		return false
	}

	if sameReservationWallet(game.PlayerOneAddress, normalizedWallet) {
		return game.PlayerOneChoice == entity.CoinSideClosed || game.PlayerOneChoice == entity.CoinSideUnknown
	}

	if game.PlayerTwoAddress != nil && sameReservationWallet(*game.PlayerTwoAddress, normalizedWallet) {
		return game.PlayerTwoChoice == nil || *game.PlayerTwoChoice == entity.CoinSideClosed || *game.PlayerTwoChoice == entity.CoinSideUnknown
	}

	return false
}

func sortActivityItems(items []*entity.PlayerActivityGame) {
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].LatestActivityAt.After(items[j].LatestActivityAt)
	})
}

func paginateActivityItems(items []*entity.PlayerActivityGame, limit, offset int) []*entity.PlayerActivityGame {
	if offset >= len(items) {
		return []*entity.PlayerActivityGame{}
	}

	end := offset + limit
	if end > len(items) {
		end = len(items)
	}

	return items[offset:end]
}

func filterActivitySearchItems(
	queues map[entity.ActivityQueueKey][]*entity.PlayerActivityGame,
	queueScope entity.ActivityQueueKey,
	rawQuery string,
) []*entity.PlayerActivityGame {
	trimmedQuery := strings.TrimSpace(rawQuery)
	if trimmedQuery == "" {
		return []*entity.PlayerActivityGame{}
	}

	searchQueues := []entity.ActivityQueueKey{
		entity.ActivityQueueJoinable,
		entity.ActivityQueueMyActive,
		entity.ActivityQueueRevealRequired,
		entity.ActivityQueueExpiredAttention,
		entity.ActivityQueueHistory,
	}
	if queueScope != "" {
		searchQueues = []entity.ActivityQueueKey{queueScope}
	}

	matchedItems := make([]*entity.PlayerActivityGame, 0)
	for _, candidateQueue := range searchQueues {
		for _, item := range queues[candidateQueue] {
			if matchesActivitySearch(item, trimmedQuery) {
				matchedItems = append(matchedItems, item)
			}
		}
	}

	sortActivityItems(matchedItems)
	return matchedItems
}

func matchesActivitySearch(item *entity.PlayerActivityGame, query string) bool {
	if item == nil || item.Game == nil {
		return false
	}

	lowerQuery := strings.ToLower(query)
	if strings.Contains(strconv.FormatInt(item.Game.GameID, 10), lowerQuery) {
		return true
	}

	normalizedWalletQuery := normalizeReservationWallet(query)
	if normalizedWalletQuery != "" {
		if sameReservationWallet(item.Game.PlayerOneAddress, normalizedWalletQuery) {
			return true
		}
		if item.Game.PlayerTwoAddress != nil && sameReservationWallet(*item.Game.PlayerTwoAddress, normalizedWalletQuery) {
			return true
		}
	}

	for _, candidate := range activitySearchCandidates(item.Game) {
		if strings.Contains(strings.ToLower(candidate), lowerQuery) {
			return true
		}
	}

	return false
}

func activitySearchCandidates(game *entity.Game) []string {
	if game == nil {
		return nil
	}

	candidates := []string{
		game.PlayerOneAddress,
		normalizeReservationWallet(game.PlayerOneAddress),
		strconv.FormatInt(game.GameID, 10),
	}

	if game.PlayerTwoAddress != nil {
		candidates = append(candidates, *game.PlayerTwoAddress, normalizeReservationWallet(*game.PlayerTwoAddress))
	}

	return candidates
}
