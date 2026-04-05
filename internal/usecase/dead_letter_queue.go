package usecase

import (
	"context"
	"encoding/json"
	"time"

	"pod-backend/internal/entity"
	"pod-backend/internal/infrastructure/toncenter"
	"pod-backend/internal/repository"
	"pod-backend/pkg/logger"
)

const (
	defaultDLQBaseBackoff = 5 * time.Minute
	defaultDLQMaxRetries  = 3
)

// DLQStoreOutcome describes how a failed transaction was handled by the DLQ.
type DLQStoreOutcome string

const (
	DLQStoreOutcomeQueued         DLQStoreOutcome = "queued"
	DLQStoreOutcomeAlreadyPending DLQStoreOutcome = "already_pending"
)

// StoreFailedTransactionResult captures the explicit DLQ outcome for a failed transaction.
type StoreFailedTransactionResult struct {
	Outcome         DLQStoreOutcome     `json:"outcome"`
	EntryID         int64               `json:"entry_id,omitempty"`
	Status          entity.DLQStatus    `json:"status"`
	ErrorType       entity.DLQErrorType `json:"error_type"`
	RetryCount      int                 `json:"retry_count"`
	MaxRetries      int                 `json:"max_retries"`
	NextRetryAt     *time.Time          `json:"next_retry_at,omitempty"`
	ResolutionNotes string              `json:"resolution_notes,omitempty"`
}

// DeadLetterQueueUseCase handles failed transaction storage and retry (Issue #6).
type DeadLetterQueueUseCase struct {
	dlqRepo     repository.DeadLetterQueueRepository
	logger      logger.Interface
	baseBackoff time.Duration
}

// DeadLetterQueueObservability captures queue backlog and retry posture in one place.
type DeadLetterQueueObservability struct {
	BaseBackoff time.Duration        `json:"base_backoff"`
	Stats       *repository.DLQStats `json:"stats"`
}

// NewDeadLetterQueueUseCase creates a new DLQ use case.
func NewDeadLetterQueueUseCase(
	dlqRepo repository.DeadLetterQueueRepository,
	log logger.Interface,
) *DeadLetterQueueUseCase {
	return &DeadLetterQueueUseCase{
		dlqRepo:     dlqRepo,
		logger:      log,
		baseBackoff: defaultDLQBaseBackoff,
	}
}

// SetBaseBackoff sets the base backoff duration for retry scheduling.
func (uc *DeadLetterQueueUseCase) SetBaseBackoff(d time.Duration) {
	uc.baseBackoff = d
}

// StoreFailedTransaction stores a failed blockchain transaction in the DLQ.
// Called when transaction parsing or persistence fails.
func (uc *DeadLetterQueueUseCase) StoreFailedTransaction(
	ctx context.Context,
	tx toncenter.Transaction,
	err error,
	errorType entity.DLQErrorType,
) (*StoreFailedTransactionResult, error) {
	// Serialize transaction data
	rawData, marshalErr := serializeTransaction(tx)
	if marshalErr != nil {
		uc.logger.Error("Failed to serialize transaction for DLQ: tx=%s, error=%v",
			tx.Hash(), marshalErr)
		rawData = "{\"error\": \"serialization failed\"}"
	}

	entry := &entity.DeadLetterEntry{
		TransactionHash: tx.Hash(),
		TransactionLt:   tx.Lt(),
		RawData:         rawData,
		ErrorMessage:    err.Error(),
		ErrorType:       errorType,
		RetryCount:      0,
		MaxRetries:      defaultDLQMaxRetries,
		Status:          entity.DLQStatusPending,
		ResolutionNotes: pendingDLQResolutionNotes(errorType),
	}

	// Schedule first retry
	entry.ScheduleNextRetry(uc.baseBackoff)

	if createErr := uc.dlqRepo.Create(ctx, entry); createErr != nil {
		uc.logger.Error("Failed to store transaction in DLQ: tx=%s, error=%v",
			tx.Hash(), createErr)
		return nil, createErr
	}

	if entry.ID == 0 {
		existingEntry, getErr := uc.dlqRepo.GetByTransactionHash(ctx, tx.Hash(), tx.Lt())
		if getErr != nil {
			return nil, getErr
		}
		if existingEntry != nil {
			uc.logger.Warn(
				"Transaction already pending in DLQ: tx=%s, lt=%s, error_type=%s, retry_count=%d/%d, next_retry_at=%v",
				tx.Hash(),
				tx.Lt(),
				existingEntry.ErrorType,
				existingEntry.RetryCount,
				existingEntry.MaxRetries,
				existingEntry.NextRetryAt,
			)
			return newStoreFailedTransactionResult(DLQStoreOutcomeAlreadyPending, existingEntry), nil
		}

		uc.logger.Warn("Transaction already pending in DLQ: tx=%s, lt=%s, error_type=%s",
			tx.Hash(), tx.Lt(), errorType)
		return newStoreFailedTransactionResult(DLQStoreOutcomeAlreadyPending, entry), nil
	}

	uc.logger.Warn(
		"Transaction stored in DLQ: tx=%s, lt=%s, error_type=%s, retry_count=%d/%d, next_retry_at=%v, error=%s",
		tx.Hash(),
		tx.Lt(),
		errorType,
		entry.RetryCount,
		entry.MaxRetries,
		entry.NextRetryAt,
		err.Error(),
	)

	return newStoreFailedTransactionResult(DLQStoreOutcomeQueued, entry), nil
}

// GetPendingForRetry retrieves entries that are ready for retry.
func (uc *DeadLetterQueueUseCase) GetPendingForRetry(ctx context.Context, limit int) ([]*entity.DeadLetterEntry, error) {
	return uc.dlqRepo.GetPendingForRetry(ctx, limit)
}

// MarkRetryAttempt marks an entry as being retried and schedules next retry.
func (uc *DeadLetterQueueUseCase) MarkRetryAttempt(ctx context.Context, entry *entity.DeadLetterEntry) error {
	entry.MarkRetrying()
	entry.ScheduleNextRetry(uc.baseBackoff)

	if err := uc.dlqRepo.Update(ctx, entry); err != nil {
		return err
	}

	uc.logger.Info("DLQ entry retry attempt: id=%d, tx=%s, attempt=%d/%d",
		entry.ID, entry.TransactionHash, entry.RetryCount, entry.MaxRetries)

	return nil
}

// MarkResolved marks an entry as successfully resolved.
func (uc *DeadLetterQueueUseCase) MarkResolved(ctx context.Context, id int64, notes string) error {
	if err := uc.dlqRepo.UpdateStatus(ctx, id, entity.DLQStatusResolved, notes); err != nil {
		return err
	}

	uc.logger.Info("DLQ entry resolved: id=%d, notes=%s", id, notes)
	return nil
}

// MarkFailed marks an entry as permanently failed.
func (uc *DeadLetterQueueUseCase) MarkFailed(ctx context.Context, id int64, notes string) error {
	if err := uc.dlqRepo.UpdateStatus(ctx, id, entity.DLQStatusFailed, notes); err != nil {
		return err
	}

	uc.logger.Warn("DLQ entry permanently failed: id=%d, notes=%s", id, notes)
	return nil
}

// GetStats returns DLQ statistics for monitoring.
func (uc *DeadLetterQueueUseCase) GetStats(ctx context.Context) (*repository.DLQStats, error) {
	return uc.dlqRepo.GetStats(ctx)
}

// GetObservability returns queue backlog, retry posture, and backoff configuration.
func (uc *DeadLetterQueueUseCase) GetObservability(ctx context.Context) (*DeadLetterQueueObservability, error) {
	stats, err := uc.dlqRepo.GetStats(ctx)
	if err != nil {
		return nil, err
	}

	return &DeadLetterQueueObservability{
		BaseBackoff: uc.baseBackoff,
		Stats:       stats,
	}, nil
}

// GetByStatus retrieves entries by status with pagination.
func (uc *DeadLetterQueueUseCase) GetByStatus(
	ctx context.Context,
	status entity.DLQStatus,
	limit, offset int,
) ([]*entity.DeadLetterEntry, error) {
	return uc.dlqRepo.GetByStatus(ctx, status, limit, offset)
}

// GetByID retrieves a single DLQ entry by ID.
func (uc *DeadLetterQueueUseCase) GetByID(ctx context.Context, id int64) (*entity.DeadLetterEntry, error) {
	return uc.dlqRepo.GetByID(ctx, id)
}

// CheckDuplicate checks if a transaction is already in the DLQ.
func (uc *DeadLetterQueueUseCase) CheckDuplicate(ctx context.Context, hash, lt string) (bool, error) {
	entry, err := uc.dlqRepo.GetByTransactionHash(ctx, hash, lt)
	if err != nil {
		return false, err
	}
	return entry != nil, nil
}

// serializeTransaction converts a transaction to JSON for storage.
func serializeTransaction(tx toncenter.Transaction) (string, error) {
	data := map[string]interface{}{
		"hash":        tx.Hash(),
		"lt":          tx.Lt(),
		"utime":       tx.Utime,
		"data":        tx.Data,
		"fee":         tx.Fee,
		"storage_fee": tx.StorageFee,
		"other_fee":   tx.OtherFee,
	}

	// Add in_msg if available (store as raw JSON)
	if tx.InMsg != nil {
		data["in_msg"] = tx.InMsg
	}

	// Add out_msgs if available
	if tx.OutMsgs != nil {
		data["out_msgs"] = tx.OutMsgs
	}

	bytes, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func pendingDLQResolutionNotes(errorType entity.DLQErrorType) string {
	switch errorType {
	case entity.DLQErrorTypeParse:
		return "queued after parse failure"
	case entity.DLQErrorTypeValidation:
		return "queued after validation failure"
	case entity.DLQErrorTypePersistence:
		return "queued after persistence retries exhausted"
	default:
		return "queued after unknown failure"
	}
}

func newStoreFailedTransactionResult(
	outcome DLQStoreOutcome,
	entry *entity.DeadLetterEntry,
) *StoreFailedTransactionResult {
	result := &StoreFailedTransactionResult{
		Outcome:         outcome,
		Status:          entity.DLQStatusPending,
		ErrorType:       entity.DLQErrorTypeUnknown,
		ResolutionNotes: "",
	}
	if entry == nil {
		return result
	}

	result.EntryID = entry.ID
	result.Status = entry.Status
	result.ErrorType = entry.ErrorType
	result.RetryCount = entry.RetryCount
	result.MaxRetries = entry.MaxRetries
	result.ResolutionNotes = entry.ResolutionNotes
	if entry.NextRetryAt != nil {
		nextRetryAt := *entry.NextRetryAt
		result.NextRetryAt = &nextRetryAt
	}

	return result
}
