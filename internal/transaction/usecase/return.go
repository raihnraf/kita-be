package usecase

import (
	"context"
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
	bookClient      BookServiceClient
	eventPublisher  StockEventPublisher
	fineCalculator  *FineCalculator
}

func NewReturnUsecase(
	txnRepo TransactionRepository,
	auditRepo AuditRepository,
	idempotencyRepo IdempotencyRepository,
	bookClient BookServiceClient,
	fineCalculator *FineCalculator,
) *ReturnUsecase {
	return &ReturnUsecase{
		txnRepo:         txnRepo,
		auditRepo:       auditRepo,
		idempotencyRepo: idempotencyRepo,
		bookClient:      bookClient,
		fineCalculator:  fineCalculator,
	}
}

func (uc *ReturnUsecase) SetEventPublisher(publisher StockEventPublisher) {
	uc.eventPublisher = publisher
}

type ReturnInput struct {
	TransactionID string
	UserID        string
}

type ReturnOutput struct {
	Transaction *domain.BorrowTransaction
}

func (uc *ReturnUsecase) Execute(ctx context.Context, input ReturnInput) (*ReturnOutput, error) {
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
		txn.Status = domain.TransactionReturnedLate
		txn.LateDays = lateDays
		txn.FineAmountCents = fineAmountCents
	} else {
		txn.Status = domain.TransactionReturned
	}

	stockEventID, err := uc.bookClient.IncreaseStock(ctx, txn.BookID, 1, txn.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to return stock: %w", err)
	}

	txn.StockEventID = &stockEventID
	txn.UpdatedAt = now

	if err := uc.txnRepo.ReturnIfActive(ctx, txn); err != nil {
		if errors.Is(err, domain.ErrTransactionNotActive) {
			return nil, apperror.Conflict("transaction is not active")
		}
		return nil, fmt.Errorf("failed to update transaction: %w", err)
	}

	audit := &domain.TransactionAudit{
		ID:            uuid.New().String(),
		TransactionID: txn.ID,
		FromStatus:    &fromStatus,
		ToStatus:      string(txn.Status),
		Reason:        "Book returned",
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

	if uc.eventPublisher != nil {
		go func() {
			if err := uc.eventPublisher.PublishStockIncrease(context.Background(), txn.ID, txn.TransactionRef, txn.UserID, txn.BookID); err != nil {
				logger.Warn("async stock increase publish failed",
					"transaction_id", txn.ID,
					"book_id", txn.BookID,
					"error", err.Error(),
				)
			}
		}()
	}

	return &ReturnOutput{Transaction: txn}, nil
}
