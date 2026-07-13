package usecase

import (
	"context"

	domain "kita-be/internal/book/domain"
)

type BookRepository interface {
	List(ctx context.Context, input ListBooksInput) ([]domain.Book, int64, error)
	FindByID(ctx context.Context, id string) (*domain.Book, error)
	FindByISBN(ctx context.Context, isbn string) (*domain.Book, error)
	Create(ctx context.Context, book *domain.Book) error
	Update(ctx context.Context, book *domain.Book) error
	DecreaseStock(ctx context.Context, id string, qty int) error
	IncreaseStock(ctx context.Context, id string, qty int) error
	ApplyStockEvent(ctx context.Context, event *domain.BookStockEvent) (*domain.BookStockEvent, error)
	RecordStockEvent(ctx context.Context, event *domain.BookStockEvent) error
	FindStockEventByEventID(ctx context.Context, eventID string) (*domain.BookStockEvent, error)
	FindStockEventByTransactionID(ctx context.Context, txnID string, eventType string) (*domain.BookStockEvent, error)
}

type ListBooksInput struct {
	Search   string
	Category string
	Page     int
	PerPage  int
}
