package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	domain "kita-be/internal/transaction/domain"
)

type StockEventOutboxRepository struct {
	pool *pgxpool.Pool
}

func NewStockEventOutboxRepository(pool *pgxpool.Pool) *StockEventOutboxRepository {
	return &StockEventOutboxRepository{pool: pool}
}

func (r *StockEventOutboxRepository) ClaimDue(ctx context.Context, limit int) ([]domain.StockEventOutbox, error) {
	if limit <= 0 {
		limit = 10
	}

	dbtx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin outbox claim: %w", err)
	}
	defer func() { _ = dbtx.Rollback(ctx) }()

	query := `
		WITH due AS (
			SELECT id
			FROM stock_event_outbox
			WHERE (status IN ('PENDING', 'FAILED') AND next_attempt_at <= NOW())
				OR (status = 'PROCESSING' AND updated_at <= NOW() - INTERVAL '5 minutes')
			ORDER BY created_at ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE stock_event_outbox o
		SET status = 'PROCESSING', attempts = attempts + 1, updated_at = NOW()
		FROM due
		WHERE o.id = due.id
		RETURNING o.id, o.event_type, o.transaction_id, o.transaction_ref, o.user_id, o.book_id, o.quantity,
			o.status, o.attempts, o.last_error, o.next_attempt_at, o.published_at, o.created_at, o.updated_at
	`
	rows, err := dbtx.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to claim due outbox events: %w", err)
	}

	events, err := scanOutboxRows(rows)
	if err != nil {
		return nil, err
	}
	rows.Close()

	if err := dbtx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit outbox claim: %w", err)
	}
	return events, nil
}

func (r *StockEventOutboxRepository) MarkPublished(ctx context.Context, id string) error {
	query := `
		UPDATE stock_event_outbox
		SET status = 'PUBLISHED', published_at = NOW(), last_error = NULL, updated_at = NOW()
		WHERE id = $1
	`
	_, err := r.pool.Exec(ctx, query, id)
	return err
}

func (r *StockEventOutboxRepository) MarkFailed(ctx context.Context, id string, publishErr error, nextAttemptAt time.Time) error {
	message := publishErr.Error()
	query := `
		UPDATE stock_event_outbox
		SET status = 'FAILED', last_error = $2, next_attempt_at = $3, updated_at = NOW()
		WHERE id = $1
	`
	_, err := r.pool.Exec(ctx, query, id, message, nextAttemptAt)
	return err
}

func scanOutboxRows(rows pgx.Rows) ([]domain.StockEventOutbox, error) {
	var events []domain.StockEventOutbox
	for rows.Next() {
		var event domain.StockEventOutbox
		var status string
		if err := rows.Scan(
			&event.ID, &event.EventType, &event.TransactionID, &event.TransactionRef, &event.UserID,
			&event.BookID, &event.Quantity, &status, &event.Attempts, &event.LastError,
			&event.NextAttemptAt, &event.PublishedAt, &event.CreatedAt, &event.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan outbox event: %w", err)
		}
		event.Status = domain.StockEventOutboxStatus(status)
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate outbox rows: %w", err)
	}
	return events, nil
}
