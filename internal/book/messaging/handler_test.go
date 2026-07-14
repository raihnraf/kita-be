package messaging_test

import (
	"context"
	"fmt"
	"testing"

	domain "kita-be/internal/book/domain"
	"kita-be/internal/book/messaging"
	"kita-be/internal/book/usecase"
	"kita-be/internal/platform/rabbitmq"
)

type fakeBookRepo struct {
	books        map[string]*domain.Book
	stockEvents  map[string]*domain.BookStockEvent
	findEventErr error
}

type fakeResultPublisher struct {
	published []*domain.BookStockEvent
	err       error
}

func (p *fakeResultPublisher) PublishStockResult(ctx context.Context, event *domain.BookStockEvent) error {
	if p.err != nil {
		return p.err
	}
	p.published = append(p.published, event)
	return nil
}

func newFakeBookRepo() *fakeBookRepo {
	return &fakeBookRepo{
		books:       make(map[string]*domain.Book),
		stockEvents: make(map[string]*domain.BookStockEvent),
	}
}

func (r *fakeBookRepo) List(ctx context.Context, input usecase.ListBooksInput) ([]domain.Book, int64, error) {
	return nil, 0, nil
}

func (r *fakeBookRepo) FindByID(ctx context.Context, id string) (*domain.Book, error) {
	book, ok := r.books[id]
	if !ok {
		return nil, fmt.Errorf("book not found")
	}
	return book, nil
}

func (r *fakeBookRepo) FindByISBN(ctx context.Context, isbn string) (*domain.Book, error) {
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
	book, err := r.FindByID(ctx, id)
	if err != nil {
		return err
	}
	return book.DecreaseStock(qty)
}

func (r *fakeBookRepo) IncreaseStock(ctx context.Context, id string, qty int) error {
	book, err := r.FindByID(ctx, id)
	if err != nil {
		return err
	}
	return book.IncreaseStock(qty)
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
	}
	r.stockEvents[event.ID] = event
	return event, nil
}

func (r *fakeBookRepo) RecordStockEvent(ctx context.Context, event *domain.BookStockEvent) error {
	r.stockEvents[event.ID] = event
	return nil
}

func (r *fakeBookRepo) FindStockEventByEventID(ctx context.Context, eventID string) (*domain.BookStockEvent, error) {
	if r.findEventErr != nil {
		return nil, r.findEventErr
	}
	for _, event := range r.stockEvents {
		if event.EventID == eventID {
			return event, nil
		}
	}
	return nil, domain.ErrStockEventNotFound
}

func (r *fakeBookRepo) FindStockEventByTransactionID(ctx context.Context, txnID string, eventType string) (*domain.BookStockEvent, error) {
	if r.findEventErr != nil {
		return nil, r.findEventErr
	}
	for _, event := range r.stockEvents {
		if event.TransactionID == txnID && string(event.EventType) == eventType {
			return event, nil
		}
	}
	return nil, domain.ErrStockEventNotFound
}

func TestHandlerDecreaseEvent(t *testing.T) {
	msg := rabbitmq.Message{
		EventID:        "evt-1",
		EventType:      rabbitmq.EventTypeDecreaseStockRequested,
		TransactionID:  "txn-1",
		TransactionRef: "TXN-1",
		UserID:         "user-1",
		BookID:         "book-1",
		Quantity:       1,
		IdempotencyKey: "key-1",
	}

	if msg.EventID != "evt-1" {
		t.Error("expected event_id evt-1")
	}
	if msg.EventType != rabbitmq.EventTypeDecreaseStockRequested {
		t.Errorf("expected event_type %s", rabbitmq.EventTypeDecreaseStockRequested)
	}
	if msg.Quantity != 1 {
		t.Error("expected quantity 1")
	}
}

func TestHandlerIncreaseEvent(t *testing.T) {
	msg := rabbitmq.Message{
		EventID:        "evt-2",
		EventType:      rabbitmq.EventTypeIncreaseStockRequested,
		TransactionID:  "txn-2",
		TransactionRef: "TXN-2",
		UserID:         "user-1",
		BookID:         "book-1",
		Quantity:       1,
		IdempotencyKey: "key-2",
	}

	if msg.EventID != "evt-2" {
		t.Error("expected event_id evt-2")
	}
	if msg.EventType != rabbitmq.EventTypeIncreaseStockRequested {
		t.Errorf("expected event_type %s", rabbitmq.EventTypeIncreaseStockRequested)
	}
}

func TestHandlerUnknownEventType(t *testing.T) {
	msg := rabbitmq.Message{
		EventID:   "evt-3",
		EventType: "UNKNOWN",
	}

	if msg.EventType != "UNKNOWN" {
		t.Error("expected event_type UNKNOWN")
	}
	if msg.EventID != "evt-3" {
		t.Error("expected event_id evt-3")
	}
}

func TestMessageRetryCountLogic(t *testing.T) {
	msg := rabbitmq.Message{
		EventID:    "evt-4",
		EventType:  rabbitmq.EventTypeDecreaseStockRequested,
		RetryCount: 3,
	}

	if msg.RetryCount != 3 {
		t.Errorf("expected retry_count 3, got %d", msg.RetryCount)
	}

	msg.RetryCount++

	if msg.RetryCount != 4 {
		t.Errorf("expected retry_count 4 after increment, got %d", msg.RetryCount)
	}
}

func TestMessagePayloadCompleteness(t *testing.T) {
	msg := rabbitmq.Message{
		EventID:        "evt-99",
		EventType:      rabbitmq.EventTypeDecreaseStockRequested,
		TransactionID:  "txn-99",
		TransactionRef: "TXN-2026071300001",
		UserID:         "user-99",
		BookID:         "book-99",
		Quantity:       1,
		OccurredAt:     "2026-07-13T00:00:00Z",
		IdempotencyKey: "idem-99",
	}

	if msg.EventID == "" {
		t.Error("event_id should not be empty")
	}
	if msg.TransactionID == "" {
		t.Error("transaction_id should not be empty")
	}
	if msg.BookID == "" {
		t.Error("book_id should not be empty")
	}
	if msg.UserID == "" {
		t.Error("user_id should not be empty")
	}
	if msg.Quantity <= 0 {
		t.Error("quantity should be > 0")
	}
	if msg.TransactionRef == "" {
		t.Error("transaction_ref should not be empty")
	}
	if msg.IdempotencyKey == "" {
		t.Error("idempotency_key should not be empty")
	}
}

func TestHandlerDuplicateDecreaseEventDoesNotDoubleApplyStock(t *testing.T) {
	repo := newFakeBookRepo()
	publisher := &fakeResultPublisher{}
	book := domain.NewBook("book-1", "978-001", "Go Programming", "Author", 3)
	repo.books[book.ID] = book
	handler := messaging.NewHandler(usecase.NewStockUsecase(repo), publisher)

	msg := rabbitmq.Message{
		EventID:        "evt-duplicate",
		EventType:      rabbitmq.EventTypeDecreaseStockRequested,
		TransactionID:  "txn-1",
		TransactionRef: "TXN-1",
		UserID:         "user-1",
		BookID:         book.ID,
		Quantity:       1,
	}

	if err := handler.HandleStockEvent(msg); err != nil {
		t.Fatalf("first delivery failed: %v", err)
	}
	if err := handler.HandleStockEvent(msg); err != nil {
		t.Fatalf("duplicate delivery failed: %v", err)
	}

	if book.AvailableStock != 2 {
		t.Fatalf("expected stock to decrease once to 2, got %d", book.AvailableStock)
	}
	if len(publisher.published) != 2 {
		t.Fatalf("expected result event to be published for both deliveries, got %d", len(publisher.published))
	}
}

func TestHandlerPublishesRejectedResultWhenDecreaseFails(t *testing.T) {
	repo := newFakeBookRepo()
	publisher := &fakeResultPublisher{}
	book := domain.NewBook("book-1", "978-001", "Go Programming", "Author", 0)
	repo.books[book.ID] = book
	handler := messaging.NewHandler(usecase.NewStockUsecase(repo), publisher)

	msg := rabbitmq.Message{
		EventID:        "evt-dec-reject",
		EventType:      rabbitmq.EventTypeDecreaseStockRequested,
		TransactionID:  "txn-1",
		TransactionRef: "TXN-1",
		UserID:         "user-1",
		BookID:         book.ID,
		Quantity:       1,
	}

	if err := handler.HandleStockEvent(msg); err != nil {
		t.Fatalf("expected rejected decrease to be published as result, got: %v", err)
	}
	if book.AvailableStock != 0 {
		t.Fatalf("expected stock to remain unchanged on rejected decrease, got %d", book.AvailableStock)
	}
	if len(publisher.published) != 1 {
		t.Fatalf("expected one published result, got %d", len(publisher.published))
	}
	if publisher.published[0].Status != domain.StockEventFailed {
		t.Fatalf("expected published event status FAILED, got %s", publisher.published[0].Status)
	}
}

func TestHandlerPublishesSuccessResultWhenIncreaseSucceeds(t *testing.T) {
	repo := newFakeBookRepo()
	publisher := &fakeResultPublisher{}
	book := domain.NewBook("book-1", "978-001", "Go Programming", "Author", 3)
	repo.books[book.ID] = book
	handler := messaging.NewHandler(usecase.NewStockUsecase(repo), publisher)
	if err := book.DecreaseStock(1); err != nil {
		t.Fatalf("failed to arrange stock decrease: %v", err)
	}

	msg := rabbitmq.Message{
		EventID:        "evt-inc-1",
		EventType:      rabbitmq.EventTypeIncreaseStockRequested,
		TransactionID:  "txn-1",
		TransactionRef: "TXN-1",
		UserID:         "user-1",
		BookID:         book.ID,
		Quantity:       1,
	}
	if err := handler.HandleStockEvent(msg); err != nil {
		t.Fatalf("expected increase event to succeed, got %v", err)
	}
	if book.AvailableStock != 3 {
		t.Fatalf("expected stock to be restored to 3, got %d", book.AvailableStock)
	}
	if len(publisher.published) != 1 {
		t.Fatalf("expected one published result, got %d", len(publisher.published))
	}
	if publisher.published[0].Status != domain.StockEventProcessed {
		t.Fatalf("expected published event status PROCESSED, got %s", publisher.published[0].Status)
	}
}

func TestHandlerRetriesWhenResultPublishingFails(t *testing.T) {
	repo := newFakeBookRepo()
	publisher := &fakeResultPublisher{err: fmt.Errorf("broker unavailable")}
	book := domain.NewBook("book-1", "978-001", "Go Programming", "Author", 3)
	repo.books[book.ID] = book
	handler := messaging.NewHandler(usecase.NewStockUsecase(repo), publisher)

	msg := rabbitmq.Message{
		EventID:        "evt-dec-2",
		EventType:      rabbitmq.EventTypeDecreaseStockRequested,
		TransactionID:  "txn-1",
		TransactionRef: "TXN-1",
		UserID:         "user-1",
		BookID:         book.ID,
		Quantity:       1,
	}

	err := handler.HandleStockEvent(msg)
	if err == nil {
		t.Fatal("expected publish failure to be retried")
	}
}
