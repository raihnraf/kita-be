package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	domain "kita-be/internal/transaction/domain"
)

type IdempotencyRepository struct {
	pool *pgxpool.Pool
}

func NewIdempotencyRepository(pool *pgxpool.Pool) *IdempotencyRepository {
	return &IdempotencyRepository{pool: pool}
}

func (r *IdempotencyRepository) CheckOrCreate(ctx context.Context, scope, key, hash string) (bool, error) {
	now := time.Now()
	expiresAt := now.Add(24 * time.Hour)

	query := `
		INSERT INTO idempotency_records (id, scope, idempotency_key, request_hash, status, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'PROCESSING', $5, $6, $7)
		ON CONFLICT (scope, idempotency_key) DO NOTHING
		RETURNING id
	`

	var id string
	err := r.pool.QueryRow(ctx, query,
		uuid.New().String(), scope, key, hash, expiresAt, now, now,
	).Scan(&id)

	if err != nil {
		if err != pgx.ErrNoRows {
			return false, fmt.Errorf("idempotency check failed: %w", err)
		}
		existing, checkErr := r.findExisting(ctx, scope, key)
		if checkErr != nil {
			return false, fmt.Errorf("idempotency check failed: %w", checkErr)
		}
		if existing != hash {
			return false, fmt.Errorf("idempotency key conflict: different request body")
		}
		return true, nil
	}

	return false, nil
}

func (r *IdempotencyRepository) SaveResponse(ctx context.Context, scope, key string, payload []byte) error {
	query := `
		UPDATE idempotency_records SET response_payload = $1, status = 'COMPLETED', updated_at = $2
		WHERE scope = $3 AND idempotency_key = $4
	`
	_, err := r.pool.Exec(ctx, query, payload, time.Now(), scope, key)
	return err
}

func (r *IdempotencyRepository) findExisting(ctx context.Context, scope, key string) (string, error) {
	query := `SELECT request_hash FROM idempotency_records WHERE scope = $1 AND idempotency_key = $2`
	var hash string
	err := r.pool.QueryRow(ctx, query, scope, key).Scan(&hash)
	return hash, err
}

func (r *IdempotencyRepository) GetRecord(ctx context.Context, scope, key string) (*domain.IdempotencyRecord, error) {
	query := `
		SELECT id, scope, idempotency_key, request_hash, response_payload, status
		FROM idempotency_records WHERE scope = $1 AND idempotency_key = $2
	`
	var rec domain.IdempotencyRecord
	var payload []byte
	err := r.pool.QueryRow(ctx, query, scope, key).Scan(
		&rec.ID, &rec.Scope, &rec.IdempotencyKey, &rec.RequestHash, &payload, &rec.Status,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get idempotency record: %w", err)
	}
	rec.ResponsePayload = payload
	return &rec, nil
}
