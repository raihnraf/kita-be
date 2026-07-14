package http_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	bookhttp "kita-be/internal/book/delivery/http"
	domain "kita-be/internal/book/domain"
	"kita-be/internal/book/usecase"
)

func TestBookHandlerCreateRejectsMissingRequiredFields(t *testing.T) {
	app, _ := newBookTestApp()

	req := httptest.NewRequest(fiber.MethodPost, "/books", strings.NewReader(`{"isbn":"978-001","total_stock":1}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", fiber.StatusBadRequest, resp.StatusCode)
	}
}

func TestBookHandlerCreateSuccess(t *testing.T) {
	app, deps := newBookTestApp()

	req := httptest.NewRequest(fiber.MethodPost, "/books", strings.NewReader(`{"isbn":"978-001","title":"Go Programming","author":"John Doe","total_stock":3}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("expected status %d, got %d", fiber.StatusCreated, resp.StatusCode)
	}
	if len(deps.repo.books) != 1 {
		t.Fatalf("expected 1 created book, got %d", len(deps.repo.books))
	}

	var body struct {
		Success bool `json:"success"`
		Data    struct {
			ISBN           string `json:"isbn"`
			Title          string `json:"title"`
			AvailableStock int    `json:"available_stock"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !body.Success || body.Data.ISBN != "978-001" || body.Data.Title != "Go Programming" || body.Data.AvailableStock != 3 {
		t.Fatalf("unexpected response body: %+v", body)
	}
}

func TestBookHandlerListNormalizesPagination(t *testing.T) {
	app, deps := newBookTestApp()
	deps.repo.seedBook("978-001", "Go Programming", "John Doe", 5)

	req := httptest.NewRequest(fiber.MethodGet, "/books?page=0&per_page=200", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected status %d, got %d", fiber.StatusOK, resp.StatusCode)
	}

	var body struct {
		Success bool `json:"success"`
		Data    []struct {
			ISBN string `json:"isbn"`
		} `json:"data"`
		Meta struct {
			Page       int   `json:"page"`
			PerPage    int   `json:"per_page"`
			Total      int64 `json:"total"`
			TotalPages int64 `json:"total_pages"`
		} `json:"meta"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !body.Success || len(body.Data) != 1 || body.Data[0].ISBN != "978-001" {
		t.Fatalf("unexpected list response: %+v", body)
	}
	if body.Meta.Page != 1 || body.Meta.PerPage != 20 || body.Meta.Total != 1 || body.Meta.TotalPages != 1 {
		t.Fatalf("unexpected pagination meta: %+v", body.Meta)
	}
}

func TestBookHandlerInternalDecreaseStock(t *testing.T) {
	app, deps := newBookTestApp()
	book := deps.repo.seedBook("978-001", "Go Programming", "John Doe", 2)
	txnID := uuid.NewString()

	req := httptest.NewRequest(fiber.MethodPost, "/internal/books/"+book.ID+"/stock/decrease", strings.NewReader(`{"quantity":1,"transaction_id":"`+txnID+`"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected status %d, got %d", fiber.StatusOK, resp.StatusCode)
	}
	if book.AvailableStock != 1 {
		t.Fatalf("expected available stock 1, got %d", book.AvailableStock)
	}
}

type bookTestDeps struct {
	repo *handlerFakeBookRepo
}

func newBookTestApp() (*fiber.App, *bookTestDeps) {
	deps := &bookTestDeps{repo: newHandlerFakeBookRepo()}
	handler := bookhttp.NewBookHandler(
		usecase.NewListBooksUsecase(deps.repo),
		usecase.NewGetBookUsecase(deps.repo),
		usecase.NewCreateBookUsecase(deps.repo),
		usecase.NewUpdateBookUsecase(deps.repo),
		usecase.NewStockUsecase(deps.repo),
	)

	app := fiber.New()
	app.Get("/books", handler.List)
	app.Post("/books", handler.Create)
	app.Post("/internal/books/:id/stock/decrease", handler.InternalDecreaseStock)
	return app, deps
}

type handlerFakeBookRepo struct {
	books       map[string]*domain.Book
	stockEvents map[string]*domain.BookStockEvent
}

func newHandlerFakeBookRepo() *handlerFakeBookRepo {
	return &handlerFakeBookRepo{
		books:       make(map[string]*domain.Book),
		stockEvents: make(map[string]*domain.BookStockEvent),
	}
}

func (r *handlerFakeBookRepo) List(ctx context.Context, input usecase.ListBooksInput) ([]domain.Book, int64, error) {
	var result []domain.Book
	for _, book := range r.books {
		if input.Search != "" && !strings.Contains(book.Title, input.Search) && !strings.Contains(book.Author, input.Search) && !strings.Contains(book.ISBN, input.Search) {
			continue
		}
		if input.Category != "" && (book.Category == nil || *book.Category != input.Category) {
			continue
		}
		result = append(result, *book)
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

func (r *handlerFakeBookRepo) FindByID(ctx context.Context, id string) (*domain.Book, error) {
	book, ok := r.books[id]
	if !ok {
		return nil, fmt.Errorf("book not found")
	}
	return book, nil
}

func (r *handlerFakeBookRepo) FindByISBN(ctx context.Context, isbn string) (*domain.Book, error) {
	for _, book := range r.books {
		if book.ISBN == isbn {
			return book, nil
		}
	}
	return nil, fmt.Errorf("book not found")
}

func (r *handlerFakeBookRepo) Create(ctx context.Context, book *domain.Book) error {
	r.books[book.ID] = book
	return nil
}

func (r *handlerFakeBookRepo) Update(ctx context.Context, book *domain.Book) error {
	r.books[book.ID] = book
	return nil
}

func (r *handlerFakeBookRepo) DecreaseStock(ctx context.Context, id string, qty int) error {
	book, err := r.FindByID(ctx, id)
	if err != nil {
		return err
	}
	return book.DecreaseStock(qty)
}

func (r *handlerFakeBookRepo) IncreaseStock(ctx context.Context, id string, qty int) error {
	book, err := r.FindByID(ctx, id)
	if err != nil {
		return err
	}
	return book.IncreaseStock(qty)
}

func (r *handlerFakeBookRepo) ApplyStockEvent(ctx context.Context, event *domain.BookStockEvent) (*domain.BookStockEvent, error) {
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

func (r *handlerFakeBookRepo) RecordStockEvent(ctx context.Context, event *domain.BookStockEvent) error {
	r.stockEvents[event.ID] = event
	return nil
}

func (r *handlerFakeBookRepo) FindStockEventByEventID(ctx context.Context, eventID string) (*domain.BookStockEvent, error) {
	for _, event := range r.stockEvents {
		if event.EventID == eventID {
			return event, nil
		}
	}
	return nil, domain.ErrStockEventNotFound
}

func (r *handlerFakeBookRepo) FindStockEventByTransactionID(ctx context.Context, txnID string, eventType string) (*domain.BookStockEvent, error) {
	for _, event := range r.stockEvents {
		if event.TransactionID == txnID && string(event.EventType) == eventType {
			return event, nil
		}
	}
	return nil, domain.ErrStockEventNotFound
}

func (r *handlerFakeBookRepo) seedBook(isbn, title, author string, stock int) *domain.Book {
	book := domain.NewBook(uuid.NewString(), isbn, title, author, stock)
	r.books[book.ID] = book
	return book
}
