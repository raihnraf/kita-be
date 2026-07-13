package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	domain "kita-be/internal/book/domain"
	"kita-be/internal/book/usecase"
)

type BookRepository struct {
	pool *pgxpool.Pool
}

func (r *BookRepository) ApplyStockEvent(ctx context.Context, event *domain.BookStockEvent) (*domain.BookStockEvent, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin stock event transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	insertQuery := `
		INSERT INTO book_stock_events (id, event_id, book_id, transaction_id, event_type, quantity, status, error_message, processed_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), $9, $10)
		ON CONFLICT DO NOTHING
		RETURNING id
	`
	var insertedID string
	err = tx.QueryRow(ctx, insertQuery,
		event.ID, event.EventID, event.BookID, event.TransactionID,
		string(event.EventType), event.Quantity, string(event.Status),
		event.ErrorMessage, event.CreatedAt, event.UpdatedAt,
	).Scan(&insertedID)
	if err == pgx.ErrNoRows {
		existing, findErr := r.FindStockEventByEventID(ctx, event.EventID)
		if findErr == nil {
			if commitErr := tx.Commit(ctx); commitErr != nil {
				return nil, fmt.Errorf("failed to commit duplicate stock event check: %w", commitErr)
			}
			return existing, nil
		}
		existing, findErr = r.FindStockEventByTransactionID(ctx, event.TransactionID, string(event.EventType))
		if findErr != nil {
			return nil, findErr
		}
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return nil, fmt.Errorf("failed to commit duplicate stock event check: %w", commitErr)
		}
		return existing, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to record stock event: %w", err)
	}

	switch event.EventType {
	case domain.StockEventDecrease:
		result, err := tx.Exec(ctx, `UPDATE books SET available_stock = available_stock - $2, updated_at = NOW() WHERE id = $1 AND available_stock >= $2`, event.BookID, event.Quantity)
		if err != nil {
			return nil, fmt.Errorf("failed to decrease stock: %w", err)
		}
		if result.RowsAffected() == 0 {
			return nil, domain.ErrInsufficientStock
		}
		if _, err := tx.Exec(ctx, `UPDATE books SET status = 'OUT_OF_STOCK', updated_at = NOW() WHERE id = $1 AND available_stock = 0`, event.BookID); err != nil {
			return nil, fmt.Errorf("failed to update book status: %w", err)
		}
	case domain.StockEventIncrease:
		result, err := tx.Exec(ctx, `UPDATE books SET available_stock = available_stock + $2, updated_at = NOW() WHERE id = $1 AND available_stock + $2 <= total_stock`, event.BookID, event.Quantity)
		if err != nil {
			return nil, fmt.Errorf("failed to increase stock: %w", err)
		}
		if result.RowsAffected() == 0 {
			return nil, domain.ErrStockExceedsTotal
		}
		if _, err := tx.Exec(ctx, `UPDATE books SET status = 'AVAILABLE', updated_at = NOW() WHERE id = $1 AND available_stock > 0 AND status = 'OUT_OF_STOCK'`, event.BookID); err != nil {
			return nil, fmt.Errorf("failed to update book status: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported stock event type: %s", event.EventType)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit stock event transaction: %w", err)
	}

	return event, nil
}

func NewBookRepository(pool *pgxpool.Pool) *BookRepository {
	return &BookRepository{pool: pool}
}

func (r *BookRepository) List(ctx context.Context, input usecase.ListBooksInput) ([]domain.Book, int64, error) {
	var conditions []string
	var args []interface{}
	argIdx := 1

	if input.Search != "" {
		pattern := "%" + input.Search + "%"
		conditions = append(conditions, fmt.Sprintf(
			"(title ILIKE $%d OR author ILIKE $%d OR isbn ILIKE $%d OR COALESCE(category, '') ILIKE $%d)",
			argIdx, argIdx+1, argIdx+2, argIdx+3,
		))
		args = append(args, pattern, pattern, pattern, pattern)
		argIdx += 4
	}

	if input.Category != "" {
		conditions = append(conditions, fmt.Sprintf("category = $%d", argIdx))
		args = append(args, input.Category)
		argIdx++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM books %s", whereClause)
	err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count books: %w", err)
	}

	offset := (input.Page - 1) * input.PerPage
	dataQuery := fmt.Sprintf(
		`SELECT id, isbn, title, author, publisher, category, description, total_stock, available_stock, status, created_at, updated_at
		 FROM books %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		whereClause, argIdx, argIdx+1,
	)
	args = append(args, input.PerPage, offset)

	rows, err := r.pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list books: %w", err)
	}
	defer rows.Close()

	var books []domain.Book
	for rows.Next() {
		var b domain.Book
		var status string
		if err := rows.Scan(
			&b.ID, &b.ISBN, &b.Title, &b.Author, &b.Publisher, &b.Category,
			&b.Description, &b.TotalStock, &b.AvailableStock, &status,
			&b.CreatedAt, &b.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan book: %w", err)
		}
		b.Status = domain.BookStatus(status)
		books = append(books, b)
	}

	return books, total, nil
}

func (r *BookRepository) FindByID(ctx context.Context, id string) (*domain.Book, error) {
	query := `
		SELECT id, isbn, title, author, publisher, category, description, total_stock, available_stock, status, created_at, updated_at
		FROM books WHERE id = $1
	`

	var b domain.Book
	var status string
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&b.ID, &b.ISBN, &b.Title, &b.Author, &b.Publisher, &b.Category,
		&b.Description, &b.TotalStock, &b.AvailableStock, &status,
		&b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find book by id: %w", err)
	}
	b.Status = domain.BookStatus(status)
	return &b, nil
}

func (r *BookRepository) FindByISBN(ctx context.Context, isbn string) (*domain.Book, error) {
	query := `
		SELECT id, isbn, title, author, publisher, category, description, total_stock, available_stock, status, created_at, updated_at
		FROM books WHERE isbn = $1
	`

	var b domain.Book
	var status string
	err := r.pool.QueryRow(ctx, query, isbn).Scan(
		&b.ID, &b.ISBN, &b.Title, &b.Author, &b.Publisher, &b.Category,
		&b.Description, &b.TotalStock, &b.AvailableStock, &status,
		&b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find book by isbn: %w", err)
	}
	b.Status = domain.BookStatus(status)
	return &b, nil
}

func (r *BookRepository) Create(ctx context.Context, book *domain.Book) error {
	query := `
		INSERT INTO books (id, isbn, title, author, publisher, category, description, total_stock, available_stock, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`
	_, err := r.pool.Exec(ctx, query,
		book.ID, book.ISBN, book.Title, book.Author, book.Publisher, book.Category,
		book.Description, book.TotalStock, book.AvailableStock, string(book.Status),
		book.CreatedAt, book.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create book: %w", err)
	}
	return nil
}

func (r *BookRepository) Update(ctx context.Context, book *domain.Book) error {
	query := `
		UPDATE books SET isbn = $2, title = $3, author = $4, publisher = $5, category = $6,
		description = $7, total_stock = $8, available_stock = $9, status = $10, updated_at = $11
		WHERE id = $1
	`
	_, err := r.pool.Exec(ctx, query,
		book.ID, book.ISBN, book.Title, book.Author, book.Publisher, book.Category,
		book.Description, book.TotalStock, book.AvailableStock, string(book.Status),
		book.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to update book: %w", err)
	}
	return nil
}

func (r *BookRepository) DecreaseStock(ctx context.Context, id string, qty int) error {
	query := `
		UPDATE books SET available_stock = available_stock - $2, updated_at = NOW()
		WHERE id = $1 AND available_stock >= $2
	`
	result, err := r.pool.Exec(ctx, query, id, qty)
	if err != nil {
		return fmt.Errorf("failed to decrease stock: %w", err)
	}
	if result.RowsAffected() == 0 {
		return domain.ErrInsufficientStock
	}

	_, err = r.pool.Exec(ctx,
		`UPDATE books SET status = 'OUT_OF_STOCK', updated_at = NOW() WHERE id = $1 AND available_stock = 0`,
		id,
	)
	if err != nil {
		return fmt.Errorf("failed to update book status: %w", err)
	}

	return nil
}

func (r *BookRepository) IncreaseStock(ctx context.Context, id string, qty int) error {
	query := `
		UPDATE books SET available_stock = available_stock + $2, updated_at = NOW()
		WHERE id = $1 AND available_stock + $2 <= total_stock
	`
	result, err := r.pool.Exec(ctx, query, id, qty)
	if err != nil {
		return fmt.Errorf("failed to increase stock: %w", err)
	}
	if result.RowsAffected() == 0 {
		return domain.ErrStockExceedsTotal
	}

	_, err = r.pool.Exec(ctx,
		`UPDATE books SET status = 'AVAILABLE', updated_at = NOW() WHERE id = $1 AND available_stock > 0 AND status = 'OUT_OF_STOCK'`,
		id,
	)
	if err != nil {
		return fmt.Errorf("failed to update book status: %w", err)
	}

	return nil
}

func (r *BookRepository) RecordStockEvent(ctx context.Context, event *domain.BookStockEvent) error {
	query := `
		INSERT INTO book_stock_events (id, event_id, book_id, transaction_id, event_type, quantity, status, error_message, processed_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	_, err := r.pool.Exec(ctx, query,
		event.ID, event.EventID, event.BookID, event.TransactionID,
		string(event.EventType), event.Quantity, string(event.Status),
		event.ErrorMessage, event.ProcessedAt, event.CreatedAt, event.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to record stock event: %w", err)
	}
	return nil
}

func (r *BookRepository) FindStockEventByEventID(ctx context.Context, eventID string) (*domain.BookStockEvent, error) {
	query := `
		SELECT id, event_id, book_id, transaction_id, event_type, quantity, status, error_message, processed_at, created_at, updated_at
		FROM book_stock_events WHERE event_id = $1
	`
	var ev domain.BookStockEvent
	var evType, evStatus string
	err := r.pool.QueryRow(ctx, query, eventID).Scan(
		&ev.ID, &ev.EventID, &ev.BookID, &ev.TransactionID,
		&evType, &ev.Quantity, &evStatus,
		&ev.ErrorMessage, &ev.ProcessedAt, &ev.CreatedAt, &ev.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find stock event: %w", err)
	}
	ev.EventType = domain.StockEventType(evType)
	ev.Status = domain.StockEventStatus(evStatus)
	return &ev, nil
}

func (r *BookRepository) FindStockEventByTransactionID(ctx context.Context, txnID string, eventType string) (*domain.BookStockEvent, error) {
	query := `
		SELECT id, event_id, book_id, transaction_id, event_type, quantity, status, error_message, processed_at, created_at, updated_at
		FROM book_stock_events WHERE transaction_id = $1 AND event_type = $2
	`
	var ev domain.BookStockEvent
	var evType, evStatus string
	err := r.pool.QueryRow(ctx, query, txnID, eventType).Scan(
		&ev.ID, &ev.EventID, &ev.BookID, &ev.TransactionID,
		&evType, &ev.Quantity, &evStatus,
		&ev.ErrorMessage, &ev.ProcessedAt, &ev.CreatedAt, &ev.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find stock event by transaction: %w", err)
	}
	ev.EventType = domain.StockEventType(evType)
	ev.Status = domain.StockEventStatus(evStatus)
	return &ev, nil
}
