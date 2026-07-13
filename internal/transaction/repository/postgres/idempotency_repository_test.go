package postgres_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	postgres "kita-be/internal/transaction/repository/postgres"
)

func TestIdempotencyRepositoryConcurrentCheckOrCreatePostgres(t *testing.T) {
	pool := newTestPool(t)
	repo := postgres.NewIdempotencyRepository(pool)
	ctx := context.Background()
	scope := fmt.Sprintf("test-borrow-%d", time.Now().UnixNano())
	key := "same-key"
	hash := "same-hash"

	const workers = 12
	var wg sync.WaitGroup
	duplicates := make(chan bool, workers)
	errs := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			duplicate, err := repo.CheckOrCreate(ctx, scope, key, hash)
			if err != nil {
				errs <- err
				return
			}
			duplicates <- duplicate
		}()
	}
	wg.Wait()
	close(duplicates)
	close(errs)

	for err := range errs {
		t.Fatalf("unexpected error: %v", err)
	}

	created := 0
	duplicateCount := 0
	for duplicate := range duplicates {
		if duplicate {
			duplicateCount++
		} else {
			created++
		}
	}
	if created != 1 || duplicateCount != workers-1 {
		t.Fatalf("expected 1 create and %d duplicates, got %d creates and %d duplicates", workers-1, created, duplicateCount)
	}
}

func TestIdempotencyRepositoryRejectsConflictingPostgres(t *testing.T) {
	pool := newTestPool(t)
	repo := postgres.NewIdempotencyRepository(pool)
	ctx := context.Background()
	scope := fmt.Sprintf("test-return-%d", time.Now().UnixNano())
	key := "same-key"

	duplicate, err := repo.CheckOrCreate(ctx, scope, key, "hash-1")
	if err != nil {
		t.Fatalf("expected first request to succeed, got: %v", err)
	}
	if duplicate {
		t.Fatal("expected first request not to be duplicate")
	}

	_, err = repo.CheckOrCreate(ctx, scope, key, "hash-2")
	if err == nil {
		t.Fatal("expected conflicting idempotency key to fail")
	}
}

func newTestPool(t *testing.T) *pgxpool.Pool {
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

	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS idempotency_records (
			id UUID PRIMARY KEY,
			scope VARCHAR(100) NOT NULL,
			idempotency_key VARCHAR(255) NOT NULL,
			request_hash VARCHAR(255) NOT NULL,
			response_payload JSONB,
			status VARCHAR(20) NOT NULL DEFAULT 'PROCESSING',
			expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_idempotency_scope_key ON idempotency_records(scope, idempotency_key);
		CREATE INDEX IF NOT EXISTS idx_idempotency_expires ON idempotency_records(expires_at);
		CREATE INDEX IF NOT EXISTS idx_idempotency_status ON idempotency_records(status);
	`); err != nil {
		t.Fatalf("failed to prepare idempotency table: %v", err)
	}

	return pool
}
