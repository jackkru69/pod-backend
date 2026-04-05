package usecase_test

import (
	"context"
	"time"

	"github.com/stretchr/testify/mock"
	"pod-backend/internal/entity"
	"pod-backend/internal/repository"
)

// Shared test mocks for all usecase tests

// MockGameRepository implements repository.GameRepository for testing
type MockGameRepository struct {
	mock.Mock
}

func (m *MockGameRepository) Create(ctx context.Context, game *entity.Game) error {
	args := m.Called(ctx, game)
	return args.Error(0)
}

func (m *MockGameRepository) GetByID(ctx context.Context, gameID int64) (*entity.Game, error) {
	args := m.Called(ctx, gameID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.Game), args.Error(1)
}

func (m *MockGameRepository) GetByStatus(ctx context.Context, status int) ([]*entity.Game, error) {
	args := m.Called(ctx, status)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*entity.Game), args.Error(1)
}

func (m *MockGameRepository) GetAvailableGames(ctx context.Context) ([]*entity.Game, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*entity.Game), args.Error(1)
}

func (m *MockGameRepository) GetByPlayerAddress(ctx context.Context, address string) ([]*entity.Game, error) {
	args := m.Called(ctx, address)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*entity.Game), args.Error(1)
}

func (m *MockGameRepository) GetByPlayer(ctx context.Context, address string) ([]*entity.Game, error) {
	return m.GetByPlayerAddress(ctx, address)
}

func (m *MockGameRepository) Update(ctx context.Context, game *entity.Game) error {
	args := m.Called(ctx, game)
	return args.Error(0)
}

func (m *MockGameRepository) UpdateStatus(ctx context.Context, gameID int64, status int) error {
	args := m.Called(ctx, gameID, status)
	return args.Error(0)
}

func (m *MockGameRepository) JoinGame(ctx context.Context, gameID int64, playerTwoAddress string, joinTxHash string, joinedAt time.Time) error {
	args := m.Called(ctx, gameID, playerTwoAddress, joinTxHash, joinedAt)
	return args.Error(0)
}

func (m *MockGameRepository) CompleteGame(ctx context.Context, gameID int64, winner string, payout int64, finishTxHash string, completedAt time.Time) error {
	args := m.Called(ctx, gameID, winner, payout, finishTxHash, completedAt)
	return args.Error(0)
}

func (m *MockGameRepository) CancelGame(ctx context.Context, gameID int64, cancelTxHash string, completedAt time.Time) error {
	args := m.Called(ctx, gameID, cancelTxHash, completedAt)
	return args.Error(0)
}

func (m *MockGameRepository) RevealChoice(ctx context.Context, gameID int64, playerAddress string, choice int, revealTxHash string, revealedAt time.Time) error {
	args := m.Called(ctx, gameID, playerAddress, choice, revealTxHash, revealedAt)
	return args.Error(0)
}

func (m *MockGameRepository) DeleteOlderThan(ctx context.Context, olderThanDate string) (int64, error) {
	args := m.Called(ctx, olderThanDate)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockGameRepository) Exists(ctx context.Context, gameID int64) (bool, error) {
	args := m.Called(ctx, gameID)
	return args.Bool(0), args.Error(1)
}

func (m *MockGameRepository) CreateOrIgnore(ctx context.Context, game *entity.Game) (bool, error) {
	args := m.Called(ctx, game)
	return args.Bool(0), args.Error(1)
}

func (m *MockGameRepository) CompleteGameWithQuerier(ctx context.Context, q repository.Querier, gameID int64, winner string, payout int64, finishTxHash string, completedAt time.Time) error {
	args := m.Called(ctx, q, gameID, winner, payout, finishTxHash, completedAt)
	return args.Error(0)
}

// MockUserRepository implements repository.UserRepository for testing
type MockUserRepository struct {
	mock.Mock
}

func (m *MockUserRepository) Create(ctx context.Context, user *entity.User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

func (m *MockUserRepository) CreateOrUpdate(ctx context.Context, user *entity.User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

func (m *MockUserRepository) EnsureUserByWallet(ctx context.Context, walletAddress string) error {
	args := m.Called(ctx, walletAddress)
	return args.Error(0)
}

func (m *MockUserRepository) GetByID(ctx context.Context, userID int64) (*entity.User, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.User), args.Error(1)
}

func (m *MockUserRepository) GetByWalletAddress(ctx context.Context, walletAddress string) (*entity.User, error) {
	args := m.Called(ctx, walletAddress)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.User), args.Error(1)
}

func (m *MockUserRepository) GetByWallet(ctx context.Context, walletAddress string) (*entity.User, error) {
	args := m.Called(ctx, walletAddress)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.User), args.Error(1)
}

func (m *MockUserRepository) GetByTelegramUserID(ctx context.Context, telegramUserID int64) ([]*entity.User, error) {
	args := m.Called(ctx, telegramUserID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*entity.User), args.Error(1)
}

func (m *MockUserRepository) Update(ctx context.Context, user *entity.User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

func (m *MockUserRepository) IncrementGamesPlayed(ctx context.Context, walletAddress string) error {
	args := m.Called(ctx, walletAddress)
	return args.Error(0)
}

func (m *MockUserRepository) IncrementGamesPlayedWithQuerier(ctx context.Context, q repository.Querier, walletAddress string) error {
	args := m.Called(ctx, q, walletAddress)
	return args.Error(0)
}

func (m *MockUserRepository) IncrementWins(ctx context.Context, walletAddress string) error {
	args := m.Called(ctx, walletAddress)
	return args.Error(0)
}

func (m *MockUserRepository) IncrementWinsWithQuerier(ctx context.Context, q repository.Querier, walletAddress string) error {
	args := m.Called(ctx, q, walletAddress)
	return args.Error(0)
}

func (m *MockUserRepository) IncrementLosses(ctx context.Context, walletAddress string) error {
	args := m.Called(ctx, walletAddress)
	return args.Error(0)
}

func (m *MockUserRepository) IncrementLossesWithQuerier(ctx context.Context, q repository.Querier, walletAddress string) error {
	args := m.Called(ctx, q, walletAddress)
	return args.Error(0)
}

func (m *MockUserRepository) IncrementReferrals(ctx context.Context, walletAddress string, earningsNanotons int64) error {
	args := m.Called(ctx, walletAddress, earningsNanotons)
	return args.Error(0)
}

func (m *MockUserRepository) IncrementReferralsWithQuerier(ctx context.Context, q repository.Querier, walletAddress string, earningsNanotons int64) error {
	args := m.Called(ctx, q, walletAddress, earningsNanotons)
	return args.Error(0)
}

func (m *MockUserRepository) GetReferralStats(ctx context.Context, walletAddress string) (*entity.ReferralStats, error) {
	args := m.Called(ctx, walletAddress)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.ReferralStats), args.Error(1)
}

func (m *MockUserRepository) DeleteOlderThan(ctx context.Context, olderThanDate string) (int64, error) {
	args := m.Called(ctx, olderThanDate)
	return args.Get(0).(int64), args.Error(1)
}

// MockGameEventRepository implements repository.GameEventRepository for testing
type MockGameEventRepository struct {
	mock.Mock
}

func (m *MockGameEventRepository) Create(ctx context.Context, event *entity.GameEvent) error {
	args := m.Called(ctx, event)
	return args.Error(0)
}

func (m *MockGameEventRepository) Upsert(ctx context.Context, event *entity.GameEvent) error {
	args := m.Called(ctx, event)
	// Note: To simulate duplicate (ON CONFLICT DO NOTHING), use Run() in test to set event.ID=0
	// The default behavior sets event.ID=1 to simulate successful insert
	// But if Run() was used and set event.ID to something, we respect that
	return args.Error(0)
}

func (m *MockGameEventRepository) UpsertWithQuerier(ctx context.Context, q repository.Querier, event *entity.GameEvent) error {
	args := m.Called(ctx, q, event)
	// Same as above - Run() can override the default behavior
	return args.Error(0)
}

func (m *MockGameEventRepository) GetByGameID(ctx context.Context, gameID int64) ([]*entity.GameEvent, error) {
	args := m.Called(ctx, gameID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*entity.GameEvent), args.Error(1)
}

func (m *MockGameEventRepository) GetByTransactionHash(ctx context.Context, txHash string) (*entity.GameEvent, error) {
	args := m.Called(ctx, txHash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.GameEvent), args.Error(1)
}

func (m *MockGameEventRepository) GetByEventType(ctx context.Context, eventType string) ([]*entity.GameEvent, error) {
	args := m.Called(ctx, eventType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*entity.GameEvent), args.Error(1)
}

func (m *MockGameEventRepository) GetByBlockRange(ctx context.Context, startBlock, endBlock int64) ([]*entity.GameEvent, error) {
	args := m.Called(ctx, startBlock, endBlock)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*entity.GameEvent), args.Error(1)
}

func (m *MockGameEventRepository) Exists(ctx context.Context, gameID int64, txHash string, eventType string) (bool, error) {
	args := m.Called(ctx, gameID, txHash, eventType)
	return args.Bool(0), args.Error(1)
}

func (m *MockGameEventRepository) ExistsByTxHash(ctx context.Context, txHash string) (bool, error) {
	args := m.Called(ctx, txHash)
	return args.Bool(0), args.Error(1)
}

func (m *MockGameEventRepository) GetLatestByGameID(ctx context.Context, gameID int64) (*entity.GameEvent, error) {
	args := m.Called(ctx, gameID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.GameEvent), args.Error(1)
}

// MockDeadLetterQueueRepository implements repository.DeadLetterQueueRepository for testing.
type MockDeadLetterQueueRepository struct {
	mock.Mock
}

func (m *MockDeadLetterQueueRepository) Create(ctx context.Context, entry *entity.DeadLetterEntry) error {
	args := m.Called(ctx, entry)
	return args.Error(0)
}

func (m *MockDeadLetterQueueRepository) GetByID(ctx context.Context, id int64) (*entity.DeadLetterEntry, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.DeadLetterEntry), args.Error(1)
}

func (m *MockDeadLetterQueueRepository) GetByTransactionHash(ctx context.Context, hash, lt string) (*entity.DeadLetterEntry, error) {
	args := m.Called(ctx, hash, lt)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.DeadLetterEntry), args.Error(1)
}

func (m *MockDeadLetterQueueRepository) GetPendingForRetry(ctx context.Context, limit int) ([]*entity.DeadLetterEntry, error) {
	args := m.Called(ctx, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*entity.DeadLetterEntry), args.Error(1)
}

func (m *MockDeadLetterQueueRepository) GetByStatus(ctx context.Context, status entity.DLQStatus, limit, offset int) ([]*entity.DeadLetterEntry, error) {
	args := m.Called(ctx, status, limit, offset)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*entity.DeadLetterEntry), args.Error(1)
}

func (m *MockDeadLetterQueueRepository) Update(ctx context.Context, entry *entity.DeadLetterEntry) error {
	args := m.Called(ctx, entry)
	return args.Error(0)
}

func (m *MockDeadLetterQueueRepository) UpdateStatus(ctx context.Context, id int64, status entity.DLQStatus, notes string) error {
	args := m.Called(ctx, id, status, notes)
	return args.Error(0)
}

func (m *MockDeadLetterQueueRepository) IncrementRetryCount(ctx context.Context, id int64, nextRetryAt time.Time) error {
	args := m.Called(ctx, id, nextRetryAt)
	return args.Error(0)
}

func (m *MockDeadLetterQueueRepository) Delete(ctx context.Context, id int64) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockDeadLetterQueueRepository) CountByStatus(ctx context.Context, status entity.DLQStatus) (int64, error) {
	args := m.Called(ctx, status)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockDeadLetterQueueRepository) GetStats(ctx context.Context) (*repository.DLQStats, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*repository.DLQStats), args.Error(1)
}
