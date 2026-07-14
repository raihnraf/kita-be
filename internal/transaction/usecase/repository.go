package usecase

import (
	"context"
	"time"

	domain "kita-be/internal/transaction/domain"
)

type TransactionRepository interface {
	Create(ctx context.Context, tx *domain.BorrowTransaction) error
	CreateIfUserBelowActiveLimit(ctx context.Context, tx *domain.BorrowTransaction, maxActive int) error
	CreateBorrowWithOutbox(ctx context.Context, tx *domain.BorrowTransaction, maxActive int, outbox *domain.StockEventOutbox) error
	EnqueueStockEvent(ctx context.Context, outbox *domain.StockEventOutbox) error
	SkipOutboxByTransactionID(ctx context.Context, transactionID string) error
	FindByID(ctx context.Context, id string) (*domain.BorrowTransaction, error)
	FindByRef(ctx context.Context, ref string) (*domain.BorrowTransaction, error)
	Update(ctx context.Context, tx *domain.BorrowTransaction) error
	UpdateStockEventID(ctx context.Context, id, stockEventID string) error
	ActivateBorrow(ctx context.Context, id, stockEventID string) error
	CancelBorrow(ctx context.Context, id string) error
	StartReturnWithOutbox(ctx context.Context, tx *domain.BorrowTransaction, outbox *domain.StockEventOutbox) error
	FinalizeReturn(ctx context.Context, id, stockEventID string) error
	RejectReturn(ctx context.Context, id string) error
	ReturnIfActive(ctx context.Context, tx *domain.BorrowTransaction) error
	ReturnIfActiveWithOutbox(ctx context.Context, tx *domain.BorrowTransaction, outbox *domain.StockEventOutbox) error
	FindActiveByUser(ctx context.Context, userID string) ([]domain.BorrowTransaction, error)
	CountActiveByUser(ctx context.Context, userID string) (int, error)
	GetHistory(ctx context.Context, userID string, page, perPage int) ([]domain.BorrowTransaction, int64, error)
	ListAll(ctx context.Context, page, perPage int) ([]domain.BorrowTransaction, int64, error)
	FindPendingOlderThan(ctx context.Context, threshold time.Time) ([]domain.BorrowTransaction, error)
	ReconcileCancelBorrow(ctx context.Context, id string, outbox *domain.StockEventOutbox) error
	RequeueStockCommand(ctx context.Context, transactionID, eventType string) error
}

type AuditRepository interface {
	Create(ctx context.Context, audit *domain.TransactionAudit) error
	FindByTransaction(ctx context.Context, txnID string) ([]domain.TransactionAudit, error)
}

type IdempotencyRepository interface {
	CheckOrCreate(ctx context.Context, scope, key, hash string) (bool, error)
	SaveResponse(ctx context.Context, scope, key string, payload []byte) error
	GetRecord(ctx context.Context, scope, key string) (*domain.IdempotencyRecord, error)
}
