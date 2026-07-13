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

type BorrowUsecase struct {
	txnRepo         TransactionRepository
	auditRepo       AuditRepository
	idempotencyRepo IdempotencyRepository
	bookClient      BookServiceClient
	eventPublisher  StockEventPublisher
	maxActive       int
	loanDays        int
}

func NewBorrowUsecase(
	txnRepo TransactionRepository,
	auditRepo AuditRepository,
	idempotencyRepo IdempotencyRepository,
	bookClient BookServiceClient,
	maxActive int,
	loanDays int,
) *BorrowUsecase {
	return &BorrowUsecase{
		txnRepo:         txnRepo,
		auditRepo:       auditRepo,
		idempotencyRepo: idempotencyRepo,
		bookClient:      bookClient,
		maxActive:       maxActive,
		loanDays:        loanDays,
	}
}

func (uc *BorrowUsecase) SetEventPublisher(publisher StockEventPublisher) {
	uc.eventPublisher = publisher
}

type BorrowInput struct {
	UserID         string
	BookID         string
	IdempotencyKey string
}

type BorrowOutput struct {
	Transaction *domain.BorrowTransaction
}

func (uc *BorrowUsecase) Execute(ctx context.Context, input BorrowInput) (*BorrowOutput, error) {
	if input.IdempotencyKey != "" {
		requestHash := hashBorrowRequest(input.UserID, input.BookID, input.IdempotencyKey)
		isDuplicate, err := uc.idempotencyRepo.CheckOrCreate(ctx, "borrow", input.IdempotencyKey, requestHash)
		if err != nil {
			return nil, apperror.Conflict(err.Error())
		}
		if isDuplicate {
			rec, err := uc.idempotencyRepo.GetRecord(ctx, "borrow", input.IdempotencyKey)
			if err == nil && rec != nil {
				if rec.Status == "COMPLETED" && len(rec.ResponsePayload) > 0 {
					var tx domain.BorrowTransaction
					if err := json.Unmarshal(rec.ResponsePayload, &tx); err == nil {
						return &BorrowOutput{Transaction: &tx}, nil
					}
				}
			}
			return nil, apperror.Conflict("duplicate request")
		}
	}

	activeCount, err := uc.txnRepo.CountActiveByUser(ctx, input.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to count active borrows: %w", err)
	}
	if activeCount >= uc.maxActive {
		return nil, apperror.Conflictf("maximum %d active borrows reached", uc.maxActive)
	}

	now := time.Now()
	dueAt := now.AddDate(0, 0, uc.loanDays)
	ref := fmt.Sprintf("TXN-%d", now.UnixNano())
	bookSnapshot, err := uc.bookClient.GetBook(ctx, input.BookID)
	if err != nil {
		return nil, apperror.Conflictf("failed to load book: %v", err)
	}

	txn := domain.NewBorrowTransaction(
		uuid.New().String(),
		ref,
		input.UserID,
		input.BookID,
		now,
		dueAt,
	)
	txn.SetBookSnapshot(bookSnapshot)

	stockEventID, err := uc.bookClient.DecreaseStock(ctx, input.BookID, 1, txn.ID)
	if err != nil {
		return nil, apperror.Conflictf("failed to reserve stock: %v", err)
	}

	txn.StockEventID = &stockEventID

	if err := uc.txnRepo.CreateIfUserBelowActiveLimit(ctx, txn, uc.maxActive); err != nil {
		_, _ = uc.bookClient.IncreaseStock(context.Background(), input.BookID, 1, txn.ID)
		if errors.Is(err, domain.ErrActiveBorrowLimitReached) {
			return nil, apperror.Conflictf("maximum %d active borrows reached", uc.maxActive)
		}
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}

	audit := &domain.TransactionAudit{
		ID:            uuid.New().String(),
		TransactionID: txn.ID,
		ToStatus:      string(domain.TransactionActive),
		Reason:        "Book borrowed",
		CreatedAt:     now,
	}
	if err := uc.auditRepo.Create(ctx, audit); err != nil {
		logger.Warn("transaction audit creation failed",
			"transaction_id", txn.ID,
			"to_status", audit.ToStatus,
			"error", err.Error(),
		)
	}

	if uc.eventPublisher != nil {
		go func() {
			if err := uc.eventPublisher.PublishStockDecrease(context.Background(), txn.ID, txn.TransactionRef, txn.UserID, txn.BookID); err != nil {
				logger.Warn("async stock decrease publish failed",
					"transaction_id", txn.ID,
					"book_id", txn.BookID,
					"error", err.Error(),
				)
			}
		}()
	}

	if input.IdempotencyKey != "" {
		payload, err := json.Marshal(txn)
		if err == nil {
			_ = uc.idempotencyRepo.SaveResponse(ctx, "borrow", input.IdempotencyKey, payload)
		}
	}

	return &BorrowOutput{Transaction: txn}, nil
}

func hashBorrowRequest(userID, bookID, key string) string {
	data := fmt.Sprintf("%s:%s:%s", userID, bookID, key)
	h := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", h)
}
