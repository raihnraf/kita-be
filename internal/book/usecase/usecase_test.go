package usecase_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"

	domain "kita-be/internal/book/domain"
	"kita-be/internal/book/usecase"
)

type fakeBookRepo struct {
	books       map[string]*domain.Book
	stockEvents map[string]*domain.BookStockEvent
}

func newFakeBookRepo() *fakeBookRepo {
	return &fakeBookRepo{
		books:       make(map[string]*domain.Book),
		stockEvents: make(map[string]*domain.BookStockEvent),
	}
}

func (r *fakeBookRepo) List(ctx context.Context, input usecase.ListBooksInput) ([]domain.Book, int64, error) {
	var result []domain.Book
	for _, b := range r.books {
		if input.Search != "" {
			search := input.Search
			matched := false
			if contains(b.Title, search) || contains(b.Author, search) || contains(b.ISBN, search) {
				matched = true
			}
			if b.Category != nil && contains(*b.Category, search) {
				matched = true
			}
			if !matched {
				continue
			}
		}
		if input.Category != "" {
			if b.Category == nil || *b.Category != input.Category {
				continue
			}
		}
		result = append(result, *b)
	}

	total := int64(len(result))
	start := (input.Page - 1) * input.PerPage
	if start >= len(result) {
		return []domain.Book{}, total, nil
	}
	end := start + input.PerPage
	if end > len(result) {
		end = len(result)
	}

	return result[start:end], total, nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func (r *fakeBookRepo) FindByID(ctx context.Context, id string) (*domain.Book, error) {
	b, ok := r.books[id]
	if !ok {
		return nil, fmt.Errorf("book not found")
	}
	return b, nil
}

func (r *fakeBookRepo) FindByISBN(ctx context.Context, isbn string) (*domain.Book, error) {
	for _, b := range r.books {
		if b.ISBN == isbn {
			return b, nil
		}
	}
	return nil, fmt.Errorf("book not found")
}

func (r *fakeBookRepo) Create(ctx context.Context, book *domain.Book) error {
	r.books[book.ID] = book
	return nil
}

func (r *fakeBookRepo) Update(ctx context.Context, book *domain.Book) error {
	r.books[book.ID] = book
	return nil
}

func (r *fakeBookRepo) DecreaseStock(ctx context.Context, id string, qty int) error {
	b, ok := r.books[id]
	if !ok {
		return fmt.Errorf("book not found")
	}
	return b.DecreaseStock(qty)
}

func (r *fakeBookRepo) IncreaseStock(ctx context.Context, id string, qty int) error {
	b, ok := r.books[id]
	if !ok {
		return fmt.Errorf("book not found")
	}
	return b.IncreaseStock(qty)
}

func (r *fakeBookRepo) ApplyStockEvent(ctx context.Context, event *domain.BookStockEvent) (*domain.BookStockEvent, error) {
	if existing, err := r.FindStockEventByEventID(ctx, event.EventID); err == nil {
		return existing, nil
	}
	if event.TransactionID != "" {
		if existing, err := r.FindStockEventByTransactionID(ctx, event.TransactionID, string(event.EventType)); err == nil {
			return existing, nil
		}
	}

	switch event.EventType {
	case domain.StockEventDecrease:
		if err := r.DecreaseStock(ctx, event.BookID, event.Quantity); err != nil {
			return nil, err
		}
	case domain.StockEventIncrease:
		if err := r.IncreaseStock(ctx, event.BookID, event.Quantity); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported stock event type")
	}

	r.stockEvents[event.ID] = event
	return event, nil
}

func (r *fakeBookRepo) RecordStockEvent(ctx context.Context, event *domain.BookStockEvent) error {
	r.stockEvents[event.ID] = event
	return nil
}

func (r *fakeBookRepo) FindStockEventByEventID(ctx context.Context, eventID string) (*domain.BookStockEvent, error) {
	for _, ev := range r.stockEvents {
		if ev.EventID == eventID {
			return ev, nil
		}
	}
	return nil, domain.ErrStockEventNotFound
}

func (r *fakeBookRepo) FindStockEventByTransactionID(ctx context.Context, txnID string, eventType string) (*domain.BookStockEvent, error) {
	for _, ev := range r.stockEvents {
		if ev.TransactionID == txnID && string(ev.EventType) == eventType {
			return ev, nil
		}
	}
	return nil, domain.ErrStockEventNotFound
}

func seedBook(repo *fakeBookRepo, isbn, title, author string, stock int) *domain.Book {
	book := domain.NewBook(uuid.New().String(), isbn, title, author, stock)
	repo.books[book.ID] = book
	return book
}

func TestListBooks(t *testing.T) {
	repo := newFakeBookRepo()
	seedBook(repo, "978-001", "Go Programming", "John Doe", 5)
	seedBook(repo, "978-002", "Fiber Web", "Jane Doe", 3)
	seedBook(repo, "978-003", "PostgreSQL Guide", "Bob Smith", 0)

	uc := usecase.NewListBooksUsecase(repo)
	output, err := uc.Execute(context.Background(), usecase.ListBooksInput{
		Page:    1,
		PerPage: 10,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(output.Books) != 3 {
		t.Errorf("expected 3 books, got %d", len(output.Books))
	}
	if output.Total != 3 {
		t.Errorf("expected total 3, got %d", output.Total)
	}
}

func TestListBooksSearch(t *testing.T) {
	repo := newFakeBookRepo()
	seedBook(repo, "978-001", "Go Programming", "John Doe", 5)
	seedBook(repo, "978-002", "Fiber Web", "Jane Doe", 3)

	uc := usecase.NewListBooksUsecase(repo)
	output, err := uc.Execute(context.Background(), usecase.ListBooksInput{
		Search:  "Go",
		Page:    1,
		PerPage: 10,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(output.Books) != 1 {
		t.Errorf("expected 1 book, got %d", len(output.Books))
	}
}

func TestListBooksPagination(t *testing.T) {
	repo := newFakeBookRepo()
	for i := 0; i < 5; i++ {
		seedBook(repo, fmt.Sprintf("978-%03d", i), fmt.Sprintf("Book %d", i), "Author", 5)
	}

	uc := usecase.NewListBooksUsecase(repo)
	output, err := uc.Execute(context.Background(), usecase.ListBooksInput{
		Page:    1,
		PerPage: 2,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(output.Books) != 2 {
		t.Errorf("expected 2 books on page 1, got %d", len(output.Books))
	}
	if output.Total != 5 {
		t.Errorf("expected total 5, got %d", output.Total)
	}
}

func TestGetBook(t *testing.T) {
	repo := newFakeBookRepo()
	book := seedBook(repo, "978-001", "Go Programming", "John Doe", 5)

	uc := usecase.NewGetBookUsecase(repo)
	result, err := uc.Execute(context.Background(), book.ID)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result.Title != "Go Programming" {
		t.Errorf("expected title Go Programming, got %s", result.Title)
	}
}

func TestGetBookNotFound(t *testing.T) {
	repo := newFakeBookRepo()

	uc := usecase.NewGetBookUsecase(repo)
	_, err := uc.Execute(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for not found book")
	}
}

func TestCreateBook(t *testing.T) {
	repo := newFakeBookRepo()

	uc := usecase.NewCreateBookUsecase(repo)
	book, err := uc.Execute(context.Background(), usecase.CreateBookInput{
		ISBN:       "978-001",
		Title:      "New Book",
		Author:     "Author",
		TotalStock: 3,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if book.ISBN != "978-001" {
		t.Errorf("expected ISBN 978-001, got %s", book.ISBN)
	}
	if book.TotalStock != 3 {
		t.Errorf("expected total_stock 3, got %d", book.TotalStock)
	}
}

func TestCreateBookDuplicateISBN(t *testing.T) {
	repo := newFakeBookRepo()
	seedBook(repo, "978-001", "Existing", "Author", 3)

	uc := usecase.NewCreateBookUsecase(repo)
	_, err := uc.Execute(context.Background(), usecase.CreateBookInput{
		ISBN:       "978-001",
		Title:      "Duplicate",
		Author:     "Author",
		TotalStock: 1,
	})
	if err == nil {
		t.Fatal("expected error for duplicate ISBN")
	}
}

func TestStockDecreaseSuccess(t *testing.T) {
	repo := newFakeBookRepo()
	book := seedBook(repo, "978-001", "Go Programming", "John Doe", 3)

	uc := usecase.NewStockUsecase(repo)
	event, err := uc.DecreaseStock(context.Background(), book.ID, 1, "txn-1")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if event.EventType != domain.StockEventDecrease {
		t.Errorf("expected event type DECREASE, got %s", event.EventType)
	}

	updated, _ := repo.FindByID(context.Background(), book.ID)
	if updated.AvailableStock != 2 {
		t.Errorf("expected available_stock 2, got %d", updated.AvailableStock)
	}
}

func TestStockDecreaseInsufficient(t *testing.T) {
	repo := newFakeBookRepo()
	book := seedBook(repo, "978-001", "Rare Book", "Author", 0)

	uc := usecase.NewStockUsecase(repo)
	event, err := uc.DecreaseStock(context.Background(), book.ID, 1, "txn-1")
	if err != nil {
		t.Fatalf("expected rejected stock event to be recorded, got: %v", err)
	}
	if event.Status != domain.StockEventFailed {
		t.Fatalf("expected failed stock event, got %s", event.Status)
	}
}

func TestStockIncreaseSuccess(t *testing.T) {
	repo := newFakeBookRepo()
	book := seedBook(repo, "978-001", "Go Programming", "John Doe", 1)
	if err := book.DecreaseStock(1); err != nil {
		t.Fatalf("failed to arrange stock decrease: %v", err)
	}

	uc := usecase.NewStockUsecase(repo)
	event, err := uc.IncreaseStock(context.Background(), book.ID, 1, "txn-1")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if event.EventType != domain.StockEventIncrease {
		t.Errorf("expected event type INCREASE, got %s", event.EventType)
	}

	updated, _ := repo.FindByID(context.Background(), book.ID)
	if updated.AvailableStock != 1 {
		t.Errorf("expected available_stock 1, got %d", updated.AvailableStock)
	}
}

func TestStockIncreaseExceedsTotal(t *testing.T) {
	repo := newFakeBookRepo()
	book := seedBook(repo, "978-001", "Go Programming", "John Doe", 1)

	uc := usecase.NewStockUsecase(repo)
	event, err := uc.IncreaseStock(context.Background(), book.ID, 1, "txn-1")
	if err != nil {
		t.Fatalf("expected rejected stock event to be recorded, got: %v", err)
	}
	if event.Status != domain.StockEventFailed {
		t.Fatalf("expected failed stock event, got %s", event.Status)
	}

	updated, _ := repo.FindByID(context.Background(), book.ID)
	if updated.AvailableStock != 1 {
		t.Errorf("expected available_stock unchanged at 1, got %d", updated.AvailableStock)
	}
}

func TestAvailability(t *testing.T) {
	repo := newFakeBookRepo()
	book := seedBook(repo, "978-001", "Available Book", "Author", 3)

	uc := usecase.NewStockUsecase(repo)
	output, err := uc.CheckAvailability(context.Background(), book.ID)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !output.CanBorrow {
		t.Error("expected CanBorrow true")
	}
	if output.AvailableStock != 3 {
		t.Errorf("expected available_stock 3, got %d", output.AvailableStock)
	}
}

func TestAvailabilityZeroStock(t *testing.T) {
	repo := newFakeBookRepo()
	book := seedBook(repo, "978-001", "Empty Book", "Author", 0)

	uc := usecase.NewStockUsecase(repo)
	output, err := uc.CheckAvailability(context.Background(), book.ID)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if output.CanBorrow {
		t.Error("expected CanBorrow false for zero stock")
	}
}

func TestStockDecreaseIdempotency(t *testing.T) {
	repo := newFakeBookRepo()
	book := seedBook(repo, "978-001", "Go Programming", "John Doe", 3)

	uc := usecase.NewStockUsecase(repo)

	// First decrease
	event1, err := uc.DecreaseStock(context.Background(), book.ID, 1, "txn-1")
	if err != nil {
		t.Fatalf("first decrease failed: %v", err)
	}

	// Second decrease with same transaction ID (idempotency check)
	event2, err := uc.DecreaseStock(context.Background(), book.ID, 1, "txn-1")
	if err != nil {
		t.Fatalf("second decrease failed (should be idempotent): %v", err)
	}

	if event1.EventID != event2.EventID {
		t.Errorf("expected same event to be returned, got different event IDs: %s vs %s", event1.EventID, event2.EventID)
	}

	updated, _ := repo.FindByID(context.Background(), book.ID)
	if updated.AvailableStock != 2 {
		t.Errorf("expected available_stock to be 2, but got %d (decreased twice)", updated.AvailableStock)
	}
}

func TestStockIncreaseIdempotency(t *testing.T) {
	repo := newFakeBookRepo()
	book := seedBook(repo, "978-001", "Go Programming", "John Doe", 1)
	if err := book.DecreaseStock(1); err != nil {
		t.Fatalf("failed to arrange stock decrease: %v", err)
	}

	uc := usecase.NewStockUsecase(repo)

	// First increase
	event1, err := uc.IncreaseStock(context.Background(), book.ID, 1, "txn-1")
	if err != nil {
		t.Fatalf("first increase failed: %v", err)
	}

	// Second increase with same transaction ID
	event2, err := uc.IncreaseStock(context.Background(), book.ID, 1, "txn-1")
	if err != nil {
		t.Fatalf("second increase failed: %v", err)
	}

	if event1.EventID != event2.EventID {
		t.Errorf("expected same event to be returned, got different event IDs: %s vs %s", event1.EventID, event2.EventID)
	}

	updated, _ := repo.FindByID(context.Background(), book.ID)
	if updated.AvailableStock != 1 {
		t.Errorf("expected available_stock to be 1, but got %d (increased twice)", updated.AvailableStock)
	}
}

func TestStockDecreaseEventIDIdempotency(t *testing.T) {
	repo := newFakeBookRepo()
	book := seedBook(repo, "978-001", "Go Programming", "John Doe", 3)

	uc := usecase.NewStockUsecase(repo)
	event1, err := uc.DecreaseStockEvent(context.Background(), book.ID, 1, "txn-1", "evt-1")
	if err != nil {
		t.Fatalf("first decrease failed: %v", err)
	}
	event2, err := uc.DecreaseStockEvent(context.Background(), book.ID, 1, "txn-2", "evt-1")
	if err != nil {
		t.Fatalf("duplicate event failed: %v", err)
	}

	if event1.ID != event2.ID {
		t.Errorf("expected duplicate event_id to return original event")
	}
	updated, _ := repo.FindByID(context.Background(), book.ID)
	if updated.AvailableStock != 2 {
		t.Errorf("expected available_stock 2 after duplicate event, got %d", updated.AvailableStock)
	}
}

func TestStockIncreaseEventIDIdempotency(t *testing.T) {
	repo := newFakeBookRepo()
	book := seedBook(repo, "978-001", "Go Programming", "John Doe", 1)
	if err := book.DecreaseStock(1); err != nil {
		t.Fatalf("failed to arrange stock decrease: %v", err)
	}

	uc := usecase.NewStockUsecase(repo)
	event1, err := uc.IncreaseStockEvent(context.Background(), book.ID, 1, "txn-1", "evt-2")
	if err != nil {
		t.Fatalf("first increase failed: %v", err)
	}
	event2, err := uc.IncreaseStockEvent(context.Background(), book.ID, 1, "txn-2", "evt-2")
	if err != nil {
		t.Fatalf("duplicate event failed: %v", err)
	}

	if event1.ID != event2.ID {
		t.Errorf("expected duplicate event_id to return original event")
	}
	updated, _ := repo.FindByID(context.Background(), book.ID)
	if updated.AvailableStock != 1 {
		t.Errorf("expected available_stock 1 after duplicate event, got %d", updated.AvailableStock)
	}
}
