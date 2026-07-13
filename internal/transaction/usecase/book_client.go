package usecase

import (
	"context"

	domain "kita-be/internal/transaction/domain"
)

type BookServiceClient interface {
	GetBook(ctx context.Context, bookID string) (*domain.BookSnapshot, error)
	DecreaseStock(ctx context.Context, bookID string, qty int, txnID string) (string, error)
	IncreaseStock(ctx context.Context, bookID string, qty int, txnID string) (string, error)
}

type StockEventPublisher interface {
	PublishStockDecrease(ctx context.Context, transactionID, transactionRef, userID, bookID string) error
	PublishStockIncrease(ctx context.Context, transactionID, transactionRef, userID, bookID string) error
}
