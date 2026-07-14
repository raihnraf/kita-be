package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	domain "kita-be/internal/transaction/domain"
)

type TransactionRepository struct {
	pool *pgxpool.Pool
}

func NewTransactionRepository(pool *pgxpool.Pool) *TransactionRepository {
	return &TransactionRepository{pool: pool}
}

type dbExecer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

func (r *TransactionRepository) Create(ctx context.Context, tx *domain.BorrowTransaction) error {
	query := `
		INSERT INTO borrow_transactions (id, transaction_ref, user_id, book_id, book_isbn, book_title, book_author, borrowed_at, due_at, returned_at, status, fine_amount, late_days, stock_event_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
	`
	_, err := r.pool.Exec(ctx, query,
		tx.ID, tx.TransactionRef, tx.UserID, tx.BookID,
		tx.BookISBN, tx.BookTitle, tx.BookAuthor,
		tx.BorrowedAt, tx.DueAt, tx.ReturnedAt, string(tx.Status),
		fineAmountDecimal(tx.FineAmountCents), tx.LateDays, tx.StockEventID,
		tx.CreatedAt, tx.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create transaction: %w", err)
	}
	return nil
}

func (r *TransactionRepository) CreateIfUserBelowActiveLimit(ctx context.Context, tx *domain.BorrowTransaction, maxActive int) error {
	return r.CreateBorrowWithOutbox(ctx, tx, maxActive, nil)
}

func (r *TransactionRepository) CreateBorrowWithOutbox(ctx context.Context, tx *domain.BorrowTransaction, maxActive int, outbox *domain.StockEventOutbox) error {
	dbtx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = dbtx.Rollback(ctx) }()

	if _, err := dbtx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, tx.UserID); err != nil {
		return fmt.Errorf("failed to lock user active borrows: %w", err)
	}

	var count int
	if err := dbtx.QueryRow(ctx, `SELECT COUNT(*) FROM borrow_transactions WHERE user_id = $1 AND status = 'ACTIVE'`, tx.UserID).Scan(&count); err != nil {
		return fmt.Errorf("failed to count active transactions: %w", err)
	}
	if count >= maxActive {
		return domain.ErrActiveBorrowLimitReached
	}

	query := `
		INSERT INTO borrow_transactions (id, transaction_ref, user_id, book_id, book_isbn, book_title, book_author, borrowed_at, due_at, returned_at, status, fine_amount, late_days, stock_event_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
	`
	if _, err := dbtx.Exec(ctx, query,
		tx.ID, tx.TransactionRef, tx.UserID, tx.BookID,
		tx.BookISBN, tx.BookTitle, tx.BookAuthor,
		tx.BorrowedAt, tx.DueAt, tx.ReturnedAt, string(tx.Status),
		fineAmountDecimal(tx.FineAmountCents), tx.LateDays, tx.StockEventID,
		tx.CreatedAt, tx.UpdatedAt,
	); err != nil {
		return fmt.Errorf("failed to create transaction: %w", err)
	}
	if outbox != nil {
		if err := insertStockEventOutbox(ctx, dbtx, outbox); err != nil {
			return fmt.Errorf("failed to enqueue stock event outbox: %w", err)
		}
	}

	if err := dbtx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

func (r *TransactionRepository) FindByID(ctx context.Context, id string) (*domain.BorrowTransaction, error) {
	query := `
		SELECT id, transaction_ref, user_id, book_id, book_isbn, book_title, book_author, borrowed_at, due_at, returned_at, status, (fine_amount * 100)::bigint, late_days, stock_event_id, created_at, updated_at
		FROM borrow_transactions WHERE id = $1
	`

	var tx domain.BorrowTransaction
	var status string
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&tx.ID, &tx.TransactionRef, &tx.UserID, &tx.BookID,
		&tx.BookISBN, &tx.BookTitle, &tx.BookAuthor,
		&tx.BorrowedAt, &tx.DueAt, &tx.ReturnedAt, &status,
		&tx.FineAmountCents, &tx.LateDays, &tx.StockEventID,
		&tx.CreatedAt, &tx.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find transaction: %w", err)
	}
	tx.Status = domain.TransactionStatus(status)
	return &tx, nil
}

func (r *TransactionRepository) FindByRef(ctx context.Context, ref string) (*domain.BorrowTransaction, error) {
	query := `
		SELECT id, transaction_ref, user_id, book_id, book_isbn, book_title, book_author, borrowed_at, due_at, returned_at, status, (fine_amount * 100)::bigint, late_days, stock_event_id, created_at, updated_at
		FROM borrow_transactions WHERE transaction_ref = $1
	`

	var tx domain.BorrowTransaction
	var status string
	err := r.pool.QueryRow(ctx, query, ref).Scan(
		&tx.ID, &tx.TransactionRef, &tx.UserID, &tx.BookID,
		&tx.BookISBN, &tx.BookTitle, &tx.BookAuthor,
		&tx.BorrowedAt, &tx.DueAt, &tx.ReturnedAt, &status,
		&tx.FineAmountCents, &tx.LateDays, &tx.StockEventID,
		&tx.CreatedAt, &tx.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find transaction: %w", err)
	}
	tx.Status = domain.TransactionStatus(status)
	return &tx, nil
}

func (r *TransactionRepository) Update(ctx context.Context, tx *domain.BorrowTransaction) error {
	query := `
		UPDATE borrow_transactions SET returned_at = $2, status = $3, fine_amount = $4, late_days = $5, stock_event_id = $6, updated_at = $7
		WHERE id = $1
	`
	_, err := r.pool.Exec(ctx, query,
		tx.ID, tx.ReturnedAt, string(tx.Status), fineAmountDecimal(tx.FineAmountCents),
		tx.LateDays, tx.StockEventID, tx.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to update transaction: %w", err)
	}
	return nil
}

func (r *TransactionRepository) ReturnIfActive(ctx context.Context, tx *domain.BorrowTransaction) error {
	return r.ReturnIfActiveWithOutbox(ctx, tx, nil)
}

func (r *TransactionRepository) ReturnIfActiveWithOutbox(ctx context.Context, tx *domain.BorrowTransaction, outbox *domain.StockEventOutbox) error {
	dbtx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = dbtx.Rollback(ctx) }()

	query := `
		UPDATE borrow_transactions SET returned_at = $3, status = $4, fine_amount = $5, late_days = $6, stock_event_id = $7, updated_at = $8
		WHERE id = $1 AND user_id = $2 AND status = 'ACTIVE'
	`
	result, err := dbtx.Exec(ctx, query,
		tx.ID, tx.UserID, tx.ReturnedAt, string(tx.Status), fineAmountDecimal(tx.FineAmountCents),
		tx.LateDays, tx.StockEventID, tx.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to return transaction: %w", err)
	}
	if result.RowsAffected() == 0 {
		return domain.ErrTransactionNotActive
	}
	if outbox != nil {
		if err := insertStockEventOutbox(ctx, dbtx, outbox); err != nil {
			return fmt.Errorf("failed to enqueue stock event outbox: %w", err)
		}
	}

	if err := dbtx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

func insertStockEventOutbox(ctx context.Context, execer dbExecer, outbox *domain.StockEventOutbox) error {
	query := `
		INSERT INTO stock_event_outbox (id, event_type, transaction_id, transaction_ref, user_id, book_id, quantity, status, attempts, next_attempt_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (transaction_id, event_type) DO NOTHING
	`
	_, err := execer.Exec(ctx, query,
		outbox.ID, outbox.EventType, outbox.TransactionID, outbox.TransactionRef, outbox.UserID, outbox.BookID,
		outbox.Quantity, string(outbox.Status), outbox.Attempts, outbox.NextAttemptAt, outbox.CreatedAt, outbox.UpdatedAt,
	)
	return err
}

func (r *TransactionRepository) FindActiveByUser(ctx context.Context, userID string) ([]domain.BorrowTransaction, error) {
	query := `
		SELECT id, transaction_ref, user_id, book_id, book_isbn, book_title, book_author, borrowed_at, due_at, returned_at, status, (fine_amount * 100)::bigint, late_days, stock_event_id, created_at, updated_at
		FROM borrow_transactions WHERE user_id = $1 AND status = 'ACTIVE'
		ORDER BY borrowed_at DESC
	`

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to find active transactions: %w", err)
	}
	defer rows.Close()

	var txs []domain.BorrowTransaction
	for rows.Next() {
		var tx domain.BorrowTransaction
		var status string
		if err := rows.Scan(
			&tx.ID, &tx.TransactionRef, &tx.UserID, &tx.BookID,
			&tx.BookISBN, &tx.BookTitle, &tx.BookAuthor,
			&tx.BorrowedAt, &tx.DueAt, &tx.ReturnedAt, &status,
			&tx.FineAmountCents, &tx.LateDays, &tx.StockEventID,
			&tx.CreatedAt, &tx.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan transaction: %w", err)
		}
		tx.Status = domain.TransactionStatus(status)
		txs = append(txs, tx)
	}

	return txs, nil
}

func (r *TransactionRepository) CountActiveByUser(ctx context.Context, userID string) (int, error) {
	query := `SELECT COUNT(*) FROM borrow_transactions WHERE user_id = $1 AND status = 'ACTIVE'`

	var count int
	err := r.pool.QueryRow(ctx, query, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count active transactions: %w", err)
	}
	return count, nil
}

func (r *TransactionRepository) GetHistory(ctx context.Context, userID string, page, perPage int) ([]domain.BorrowTransaction, int64, error) {
	var total int64
	countQuery := `SELECT COUNT(*) FROM borrow_transactions WHERE user_id = $1`
	if err := r.pool.QueryRow(ctx, countQuery, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count history: %w", err)
	}

	offset := (page - 1) * perPage
	query := `
		SELECT id, transaction_ref, user_id, book_id, book_isbn, book_title, book_author, borrowed_at, due_at, returned_at, status, (fine_amount * 100)::bigint, late_days, stock_event_id, created_at, updated_at
		FROM borrow_transactions WHERE user_id = $1
		ORDER BY created_at DESC LIMIT $2 OFFSET $3
	`

	rows, err := r.pool.Query(ctx, query, userID, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get history: %w", err)
	}
	defer rows.Close()

	var txs []domain.BorrowTransaction
	for rows.Next() {
		var tx domain.BorrowTransaction
		var status string
		if err := rows.Scan(
			&tx.ID, &tx.TransactionRef, &tx.UserID, &tx.BookID,
			&tx.BookISBN, &tx.BookTitle, &tx.BookAuthor,
			&tx.BorrowedAt, &tx.DueAt, &tx.ReturnedAt, &status,
			&tx.FineAmountCents, &tx.LateDays, &tx.StockEventID,
			&tx.CreatedAt, &tx.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan transaction: %w", err)
		}
		tx.Status = domain.TransactionStatus(status)
		txs = append(txs, tx)
	}

	return txs, total, nil
}

func (r *TransactionRepository) ListAll(ctx context.Context, page, perPage int) ([]domain.BorrowTransaction, int64, error) {
	var total int64
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM borrow_transactions`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count transactions: %w", err)
	}

	offset := (page - 1) * perPage
	query := `
		SELECT id, transaction_ref, user_id, book_id, book_isbn, book_title, book_author, borrowed_at, due_at, returned_at, status, (fine_amount * 100)::bigint, late_days, stock_event_id, created_at, updated_at
		FROM borrow_transactions
		ORDER BY created_at DESC LIMIT $1 OFFSET $2
	`
	rows, err := r.pool.Query(ctx, query, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list transactions: %w", err)
	}
	defer rows.Close()

	var txs []domain.BorrowTransaction
	for rows.Next() {
		var tx domain.BorrowTransaction
		var status string
		if err := rows.Scan(
			&tx.ID, &tx.TransactionRef, &tx.UserID, &tx.BookID,
			&tx.BookISBN, &tx.BookTitle, &tx.BookAuthor,
			&tx.BorrowedAt, &tx.DueAt, &tx.ReturnedAt, &status,
			&tx.FineAmountCents, &tx.LateDays, &tx.StockEventID,
			&tx.CreatedAt, &tx.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan transaction: %w", err)
		}
		tx.Status = domain.TransactionStatus(status)
		txs = append(txs, tx)
	}

	return txs, total, nil
}

func fineAmountDecimal(cents int64) string {
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}
