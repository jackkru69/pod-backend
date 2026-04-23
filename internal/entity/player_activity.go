package entity

import "time"

// ActivityQueueKey identifies the player-facing queue that currently owns a
// game in the additive activity shell.
type ActivityQueueKey string

const (
	ActivityQueueJoinable         ActivityQueueKey = "joinable"
	ActivityQueueMyActive         ActivityQueueKey = "my-active"
	ActivityQueueRevealRequired   ActivityQueueKey = "reveal-required"
	ActivityQueueExpiredAttention ActivityQueueKey = "expired-attention"
	ActivityQueueHistory          ActivityQueueKey = "history"
)

// PlayerActivityNextAction describes the next valid or visible player action
// for a queue item.
type PlayerActivityNextAction string

const (
	ActivityNextActionJoin              PlayerActivityNextAction = "join"
	ActivityNextActionResumeJoin        PlayerActivityNextAction = "resume_join"
	ActivityNextActionWaitForJoinWindow PlayerActivityNextAction = "wait_for_join_window"
	ActivityNextActionResumeCancel      PlayerActivityNextAction = "resume_cancel"
	ActivityNextActionWaitForCancel     PlayerActivityNextAction = "wait_for_cancel_resolution"
	ActivityNextActionWaitForOpponent   PlayerActivityNextAction = "wait_for_opponent"
	ActivityNextActionReveal            PlayerActivityNextAction = "reveal"
	ActivityNextActionResumeReveal      PlayerActivityNextAction = "resume_reveal"
	ActivityNextActionWaitForReveal     PlayerActivityNextAction = "wait_for_reveal"
	ActivityNextActionReviewResult      PlayerActivityNextAction = "review_result"
	ActivityNextActionResumeReview      PlayerActivityNextAction = "resume_review_result"
	ActivityNextActionWaitForReview     PlayerActivityNextAction = "wait_for_review_window"
	ActivityNextActionBrowseHistory     PlayerActivityNextAction = "browse_history"
)

// ActionClaimType identifies the advisory off-chain claim type associated with
// a queue item.
type ActionClaimType string

const (
	ActionClaimTypeJoin            ActionClaimType = "join"
	ActionClaimTypeCancel          ActionClaimType = "cancel"
	ActionClaimTypeReveal          ActionClaimType = "reveal"
	ActionClaimTypeExpiredFollowUp ActionClaimType = "expired_follow_up"
)

// ActionClaimStatus mirrors the advisory reservation lifecycle.
type ActionClaimStatus string

const (
	ActionClaimStatusActive   ActionClaimStatus = "active"
	ActionClaimStatusReleased ActionClaimStatus = "released"
	ActionClaimStatusExpired  ActionClaimStatus = "expired"
)

// ActionClaim captures an advisory off-chain lock that affects a player-facing
// activity item.
type ActionClaim struct {
	ClaimType     ActionClaimType   `json:"claim_type"`
	GameID        int64             `json:"game_id"`
	HolderWallet  string            `json:"holder_wallet"`
	Status        ActionClaimStatus `json:"status"`
	CreatedAt     time.Time         `json:"created_at"`
	ExpiresAt     time.Time         `json:"expires_at"`
	ReleaseReason string            `json:"release_reason,omitempty"`
}

// PlayerActivityGame is the queue-oriented, player-relative view of a single
// indexed game.
type PlayerActivityGame struct {
	Game              *Game                    `json:"game"`
	QueueKey          ActivityQueueKey         `json:"queue_key"`
	NextAction        PlayerActivityNextAction `json:"next_action"`
	RequiresAttention bool                     `json:"requires_attention"`
	IsOwnedByPlayer   bool                     `json:"is_owned_by_player"`
	LatestActivityAt  time.Time                `json:"latest_activity_at"`
	ActiveClaims      []ActionClaim            `json:"active_claims,omitempty"`
}

// GameplayQueue is the public additive queue envelope returned by activity
// surfaces.
type GameplayQueue struct {
	Key               ActivityQueueKey      `json:"key"`
	Title             string                `json:"title"`
	AttentionRequired bool                  `json:"attention_required"`
	TotalCount        int                   `json:"total_count"`
	Items             []*PlayerActivityGame `json:"items"`
}

// PlayerActivitySummary contains cross-queue counts used by the queue shell.
type PlayerActivitySummary struct {
	WalletAddress         string     `json:"wallet_address,omitempty"`
	JoinableCount         int        `json:"joinable_count"`
	MyActiveCount         int        `json:"my_active_count"`
	RevealRequiredCount   int        `json:"reveal_required_count"`
	ExpiredAttentionCount int        `json:"expired_attention_count"`
	HistoryCount          int        `json:"history_count"`
	LastActivityAt        *time.Time `json:"last_activity_at,omitempty"`
}

// ActivityQueueTitle returns the player-facing label for a queue key.
func ActivityQueueTitle(queueKey ActivityQueueKey) string {
	switch queueKey {
	case ActivityQueueJoinable:
		return "Можно присоединиться"
	case ActivityQueueMyActive:
		return "Мои активные"
	case ActivityQueueRevealRequired:
		return "Нужно раскрыть"
	case ActivityQueueExpiredAttention:
		return "Требуют внимания"
	case ActivityQueueHistory:
		return "История"
	default:
		return "Активность"
	}
}

// QueueRequiresAttention indicates whether the queue represents items that are
// explicitly attention-seeking by default.
func QueueRequiresAttention(queueKey ActivityQueueKey) bool {
	switch queueKey {
	case ActivityQueueRevealRequired, ActivityQueueExpiredAttention:
		return true
	case ActivityQueueJoinable, ActivityQueueMyActive, ActivityQueueHistory:
		return false
	default:
		return false
	}
}

// IsValidActivityQueueKey validates known queue keys.
func IsValidActivityQueueKey(queueKey ActivityQueueKey) bool {
	switch queueKey {
	case ActivityQueueJoinable,
		ActivityQueueMyActive,
		ActivityQueueRevealRequired,
		ActivityQueueExpiredAttention,
		ActivityQueueHistory:
		return true
	default:
		return false
	}
}
