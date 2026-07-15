package postgres_test

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	domain "kita-be/internal/book/domain"
	postgres "kita-be/internal/book/repository/postgres"
)

func TestBookRepositoryCreateAndFind(t *testing.T) {
	pool := newTestPoolForBook(t)
	repo := postgres.NewBookRepository(pool)
	ctx := context.Background()

	book := domain.NewBook(uuid.NewString(), "978-0-123456-78-9", "Test Book", "Test Author", 10)
	if err := repo.Create(ctx, book); err != nil {
		t.Fatalf("failed to create book: %v", err)
	}

	found, err := repo.FindByID(ctx, book.ID)
	if err != nil {
		t.Fatalf("failed to find book by ID: %v", err)
	}
	if found.ID != book.ID {
		t.Errorf("expected ID %s, got %s", book.ID, found.ID)
	}
	if found.ISBN != "978-0-123456-78-9" {
		t.Errorf("expected ISBN '978-0-123456-78-9', got %s", found.ISBN)
	}
	if found.AvailableStock != 10 {
		t.Errorf("expected AvailableStock 10, got %d", found.AvailableStock)
	}
}

func TestBookRepositoryFindByISBN(t *testing.T) {
	pool := newTestPoolForBook(t)
	repo := postgres.NewBookRepository(pool)
	ctx := context.Background()

	book := domain.NewBook(uuid.NewString(), "978-0-123456-78-9", "ISBN Test", "Author", 5)
	if err := repo.Create(ctx, book); err != nil {
		t.Fatalf("failed to create book: %v", err)
	}

	found, err := repo.FindByISBN(ctx, "978-0-123456-78-9")
	if err != nil {
		t.Fatalf("failed to find book by ISBN: %v", err)
	}
	if found.ID != book.ID {
		t.Errorf("expected ID %s, got %s", book.ID, found.ID)
	}
}

func TestBookRepositoryList(t *testing.T) {
	pool := newTestPoolForBook(t)
	repo := postgres.NewBookRepository(pool)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		book := domain.NewBook(uuid.NewString(), "978-0-123456-7"+string(rune('0'+i))+"-9", "Book "+string(rune('0'+i)), "Author", 5)
		if err := repo.Create(ctx, book); err != nil {
			t.Fatalf("failed to create book %d: %v", i, err)
		}
	}

	books, total, err := repo.List(ctx, domain.ListBooksInput{Page: 1, PerPage: 10})
	if err != nil {
		t.Fatalf("failed to list books: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total 5, got %d", total)
	}
	if len(books) != 5 {
		t.Errorf("expected 5 books, got %d", len(books))
	}
}

func TestBookRepositoryApplyStockEventIdempotency(t *testing.T) {
	pool := newTestPoolForBook(t)
	repo := postgres.NewBookRepository(pool)
	ctx := context.Background()

	book := domain.NewBook(uuid.NewString(), "978-0-123456-78-9", "Stock Test", "Author", 10)
	if err := repo.Create(ctx, book); err != nil {
		t.Fatalf("failed to create book: %v", err)
	}

	event := &domain.BookStockEvent{
		ID:            uuid.NewString(),
		EventID:       uuid.NewString(),
		BookID:        book.ID,
		TransactionID: "txn-1",
		EventType:     domain.StockEventDecrease,
		Quantity:      2,
		Status:        domain.StockEventPending,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	result1, err := repo.ApplyStockEvent(ctx, event)
	if err != nil {
		t.Fatalf("first ApplyStockEvent failed: %v", err)
	}
	if result1.ID != event.ID {
		t.Errorf("expected event ID %s, got %s", event.ID, result1.ID)
	}

	result2, err := repo.ApplyStockEvent(ctx, event)
	if err != nil {
		t.Fatalf("duplicate ApplyStockEvent failed: %v", err)
	}
	if result2.ID != event.ID {
		t.Errorf("expected duplicate to return same event ID %s, got %s", event.ID, result2.ID)
	}

	updated, err := repo.FindByID(ctx, book.ID)
	if err != nil {
		t.Fatalf("failed to find updated book: %v", err)
	}
	if updated.AvailableStock != 8 {
		t.Errorf("expected AvailableStock 8 after one decrease, got %d", updated.AvailableStock)
	}
}

func TestBookRepositoryApplyStockEventDuplicateTransactionID(t *testing.T) {
	pool := newTestPoolForBook(t)
	repo := postgres.NewBookRepository(pool)
	ctx := context.Background()

	book := domain.NewBook(uuid.NewString(), "978-0-123456-78-9", "Dup Txn Test", "Author", 10)
	if err := repo.Create(ctx, book); err != nil {
		t.Fatalf("failed to create book: %v", err)
	}

	event1 := &domain.BookStockEvent{
		ID:            uuid.NewString(),
		EventID:       uuid.NewString(),
		BookID:        book.ID,
		TransactionID: "txn-dup",
		EventType:     domain.StockEventDecrease,
		Quantity:      1,
		Status:        domain.StockEventPending,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	_, err := repo.ApplyStockEvent(ctx, event1)
	if err != nil {
		t.Fatalf("first ApplyStockEvent failed: %v", err)
	}

	event2 := &domain.BookStockEvent{
		ID:            uuid.NewString(),
		EventID:       uuid.NewString(),
		BookID:        book.ID,
		TransactionID: "txn-dup",
		EventType:     domain.StockEventDecrease,
		Quantity:      1,
		Status:        domain.StockEventPending,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	result2, err := repo.ApplyStockEvent(ctx, event2)
	if err != nil {
		t.Fatalf("duplicate transaction_id ApplyStockEvent failed: %v", err)
	}
	if result2.ID != event1.ID {
		t.Errorf("expected duplicate transaction_id to return original event ID %s, got %s", event1.ID, result2.ID)
	}

	updated, err := repo.FindByID(ctx, book.ID)
	if err != nil {
		t.Fatalf("failed to find updated book: %v", err)
	}
	if updated.AvailableStock != 9 {
		t.Errorf("expected AvailableStock 9 after one decrease, got %d", updated.AvailableStock)
	}
}

func TestBookRepositoryApplyStockEventConcurrency(t *testing.T) {
	pool := newTestPoolForBook(t)
	repo := postgres.NewBookRepository(pool)
	ctx := context.Background()

	book := domain.NewBook(uuid.NewString(), "978-0-123456-78-9", "Concurrent Test", "Author", 1)
	if err := repo.Create(ctx, book); err != nil {
		t.Fatalf("failed to create book: %v", err)
	}

	const workers = 5
	var wg sync.WaitGroup
	errs := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			event := &domain.BookStockEvent{
				ID:            uuid.NewString(),
				EventID:       uuid.NewString(),
				BookID:        book.ID,
				TransactionID: "txn-conc-" + string(rune('0'+workerID)),
				EventType:     domain.StockEventDecrease,
				Quantity:      1,
				Status:        domain.StockEventPending,
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
			}
			_, err := repo.ApplyStockEvent(ctx, event)
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)

	successCount := 0
	insufficientCount := 0
	for err := range errs {
		switch err {
		case nil:
			successCount++
		case domain.ErrInsufficientStock:
			insufficientCount++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}

	if successCount != 1 {
		t.Errorf("expected exactly 1 successful decrease, got %d", successCount)
	}
}

func newTestPoolForBook(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to connect to postgres: %v", err)
	}
	t.Cleanup(pool.Close)

	if _, err := pool.Exec(ctx, `DROP TABLE IF EXISTS book_stock_events; DROP TABLE IF EXISTS books;`); err != nil {
		t.Fatalf("failed to drop tables: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

		CREATE TABLE books (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			isbn VARCHAR(20) NOT NULL,
			title VARCHAR(500) NOT NULL,
			author VARCHAR(255) NOT NULL,
			publisher VARCHAR(255),
			category VARCHAR(100),
			description TEXT,
			total_stock INT NOT NULL DEFAULT 0,
			available_stock INT NOT NULL DEFAULT 0,
			status VARCHAR(50) NOT NULL DEFAULT 'AVAILABLE',
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			CONSTRAINT chk_available_stock CHECK (available_stock >= 0),
			CONSTRAINT chk_available_not_above_total CHECK (available_stock <= total_stock),
			CONSTRAINT chk_total_stock CHECK (total_stock >= 0)
		);

		CREATE UNIQUE INDEX idx_books_isbn ON books(isbn);

		CREATE TABLE book_stock_events (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			event_id UUID NOT NULL,
			book_id UUID NOT NULL,
			transaction_id UUID,
			event_type VARCHAR(20) NOT NULL,
			quantity INT NOT NULL DEFAULT 1,
			status VARCHAR(20) NOT NULL DEFAULT 'PENDING',
			error_message TEXT,
			processed_at TIMESTAMP WITH TIME ZONE,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
		);

		CREATE UNIQUE INDEX idx_book_stock_events_event_id ON book_stock_events(event_id);
		CREATE UNIQUE INDEX idx_book_stock_events_transaction_type ON book_stock_events(transaction_id, event_type);
	`); err != nil {
		t.Fatalf("failed to create tables: %v", err)
	}

	return pool
}
