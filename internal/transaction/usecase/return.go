package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"kita-be/internal/platform/apperror"
	"kita-be/internal/platform/logger"
	domain "kita-be/internal/transaction/domain"
)

type ReturnUsecase struct {
	txnRepo         TransactionRepository
	auditRepo       AuditRepository
	idempotencyRepo IdempotencyRepository
	fineCalculator  *FineCalculator
}

func NewReturnUsecase(
	txnRepo TransactionRepository,
	auditRepo AuditRepository,
	idempotencyRepo IdempotencyRepository,
	fineCalculator *FineCalculator,
) *ReturnUsecase {
	return &ReturnUsecase{
		txnRepo:         txnRepo,
		auditRepo:       auditRepo,
		idempotencyRepo: idempotencyRepo,
		fineCalculator:  fineCalculator,
	}
}

type ReturnInput struct {
	TransactionID  string
	UserID         string
	IdempotencyKey string
}

type ReturnOutput struct {
	Transaction *domain.BorrowTransaction
}

func (uc *ReturnUsecase) Execute(ctx context.Context, input ReturnInput) (*ReturnOutput, error) {
	if input.IdempotencyKey != "" {
		requestHash := hashReturnRequest(input.UserID, input.TransactionID, input.IdempotencyKey)
		isDuplicate, err := uc.idempotencyRepo.CheckOrCreate(ctx, "return", input.IdempotencyKey, requestHash)
		if err != nil {
			logger.Warn("return idempotency check failed", "user_id", input.UserID, "transaction_id", input.TransactionID, "error", err.Error())
			return nil, apperror.Conflict("idempotency key conflicts with another request")
		}
		if isDuplicate {
			rec, err := uc.idempotencyRepo.GetRecord(ctx, "return", input.IdempotencyKey)
			if err == nil && rec != nil {
				if rec.Status == "COMPLETED" && len(rec.ResponsePayload) > 0 {
					var tx domain.BorrowTransaction
					if err := json.Unmarshal(rec.ResponsePayload, &tx); err == nil {
						return &ReturnOutput{Transaction: &tx}, nil
					}
				}
			}
			return nil, apperror.Conflict("duplicate request")
		}
	}

	txn, err := uc.txnRepo.FindByID(ctx, input.TransactionID)
	if err != nil {
		return nil, apperror.NotFound("transaction not found")
	}

	if !txn.IsActive() {
		return nil, apperror.Conflict("transaction is not active")
	}

	if !txn.BelongsTo(input.UserID) {
		return nil, apperror.Forbidden("transaction does not belong to this user")
	}

	now := time.Now()
	fromStatus := string(txn.Status)
	txn.ReturnedAt = &now

	lateDays, fineAmountCents := uc.fineCalculator.Calculate(txn.DueAt, now)

	if lateDays > 0 {
		txn.LateDays = lateDays
		txn.FineAmountCents = fineAmountCents
	}
	finalStatus := domain.TransactionReturned
	if lateDays > 0 {
		finalStatus = domain.TransactionReturnedLate
	}
	txn.Status = domain.TransactionReturnPending
	txn.StockEventID = nil

	txn.UpdatedAt = now

	outbox := domain.NewStockEventOutbox(uuid.New().String(), "INCREASE", txn)
	if err := uc.txnRepo.StartReturnWithOutbox(ctx, txn, outbox); err != nil {
		if errors.Is(err, domain.ErrTransactionNotActive) {
			return nil, apperror.Conflict("transaction is not active")
		}
		return nil, fmt.Errorf("failed to start return transaction: %w", err)
	}

	audit := &domain.TransactionAudit{
		ID:            uuid.New().String(),
		TransactionID: txn.ID,
		FromStatus:    &fromStatus,
		ToStatus:      string(domain.TransactionReturnPending),
		Reason:        fmt.Sprintf("Return requested; waiting for stock restore before %s", finalStatus),
		CreatedAt:     now,
	}
	if err := uc.auditRepo.Create(ctx, audit); err != nil {
		logger.Warn("transaction audit creation failed",
			"transaction_id", txn.ID,
			"from_status", fromStatus,
			"to_status", audit.ToStatus,
			"error", err.Error(),
		)
	}

	if input.IdempotencyKey != "" {
		payload, err := json.Marshal(txn)
		if err == nil {
			_ = uc.idempotencyRepo.SaveResponse(ctx, "return", input.IdempotencyKey, payload)
		}
	}

	return &ReturnOutput{Transaction: txn}, nil
}

func hashReturnRequest(userID, transactionID, key string) string {
	data := fmt.Sprintf("%s:%s:%s", userID, transactionID, key)
	h := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", h)
}
