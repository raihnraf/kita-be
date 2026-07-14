package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	domain "kita-be/internal/transaction/domain"
	postgres "kita-be/internal/transaction/repository/postgres"
)

func TestTransactionRepositoryConcurrencyAdvisoryLock(t *testing.T) {
	pool := newTestPoolForTxn(t)
	repo := postgres.NewTransactionRepository(pool)
	ctx := context.Background()

	userID := uuid.New().String()
	bookID := uuid.New().String()
	maxActive := 3

	const workers = 10
	var wg sync.WaitGroup
	errs := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			txn := domain.NewBorrowTransaction(
				uuid.New().String(),
				fmt.Sprintf("REF-%s-%d", userID[:8], workerID),
				userID,
				bookID,
				time.Now(),
				time.Now().AddDate(0, 0, 7),
			)
			outbox := domain.NewStockEventOutbox(uuid.New().String(), "DECREASE", txn)
			err := repo.CreateBorrowWithOutbox(ctx, txn, maxActive, outbox)
			if err != nil {
				errs <- err
			} else {
				errs <- nil
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	successCount := 0
	limitErrCount := 0
	var otherErrs []error

	for err := range errs {
		if err == nil {
			successCount++
		} else if errors.Is(err, domain.ErrActiveBorrowLimitReached) {
			limitErrCount++
		} else {
			otherErrs = append(otherErrs, err)
		}
	}

	if len(otherErrs) > 0 {
		t.Fatalf("encountered unexpected errors: %v", otherErrs)
	}

	if successCount != maxActive {
		t.Errorf("expected exactly %d successful borrows, got %d", maxActive, successCount)
	}

	if limitErrCount != workers-maxActive {
		t.Errorf("expected %d rejects due to active borrow limit, got %d", workers-maxActive, limitErrCount)
	}

	// Verify the DB state: count active transactions for the user
	count, err := repo.CountActiveByUser(ctx, userID)
	if err != nil {
		t.Fatalf("failed to count active transactions: %v", err)
	}
	if count != maxActive {
		t.Errorf("expected CountActiveByUser to return %d, got %d", maxActive, count)
	}
}

func TestTransactionRepositoryReturnIfActiveConcurrency(t *testing.T) {
	pool := newTestPoolForTxn(t)
	repo := postgres.NewTransactionRepository(pool)
	ctx := context.Background()

	userID := uuid.New().String()
	bookID := uuid.New().String()
	txnID := uuid.New().String()
	ref := "REF-RETURN-CONC"

	txn := domain.NewBorrowTransaction(txnID, ref, userID, bookID, time.Now(), time.Now().AddDate(0, 0, 7))
	if err := repo.Create(ctx, txn); err != nil {
		t.Fatalf("failed to seed transaction: %v", err)
	}

	const workers = 5
	var wg sync.WaitGroup
	errs := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			txToReturn, err := repo.FindByID(ctx, txnID)
			if err != nil {
				errs <- err
				return
			}
			now := time.Now()
			txToReturn.ReturnedAt = &now
			txToReturn.Status = domain.TransactionReturned
			txToReturn.UpdatedAt = now

			outbox := domain.NewStockEventOutbox(uuid.New().String(), "INCREASE", txToReturn)
			err = repo.ReturnIfActiveWithOutbox(ctx, txToReturn, outbox)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	successCount := 0
	notActiveCount := 0
	var otherErrs []error

	for err := range errs {
		if err == nil {
			successCount++
		} else if errors.Is(err, domain.ErrTransactionNotActive) {
			notActiveCount++
		} else {
			otherErrs = append(otherErrs, err)
		}
	}

	if len(otherErrs) > 0 {
		t.Fatalf("encountered unexpected errors: %v", otherErrs)
	}

	if successCount != 1 {
		t.Errorf("expected exactly 1 successful return, got %d", successCount)
	}

	if notActiveCount != workers-1 {
		t.Errorf("expected %d rejects due to transaction not active, got %d", workers-1, notActiveCount)
	}
}

func newTestPoolForTxn(t *testing.T) *pgxpool.Pool {
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

	// Clean up and prepare schema
	_, _ = pool.Exec(ctx, `DROP TABLE IF EXISTS stock_event_outbox; DROP TABLE IF EXISTS borrow_transactions;`)

	if _, err := pool.Exec(ctx, `
		CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

		CREATE TABLE IF NOT EXISTS borrow_transactions (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			transaction_ref VARCHAR(50) NOT NULL,
			user_id UUID NOT NULL,
			book_id UUID NOT NULL,
			book_isbn VARCHAR(50),
			book_title VARCHAR(255),
			book_author VARCHAR(255),
			borrowed_at TIMESTAMP WITH TIME ZONE NOT NULL,
			due_at TIMESTAMP WITH TIME ZONE NOT NULL,
			returned_at TIMESTAMP WITH TIME ZONE,
			status VARCHAR(20) NOT NULL DEFAULT 'ACTIVE',
			fine_amount_cents BIGINT NOT NULL DEFAULT 0,
			late_days INT NOT NULL DEFAULT 0,
			stock_event_id UUID,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
		);

		CREATE UNIQUE INDEX IF NOT EXISTS idx_borrow_transactions_ref ON borrow_transactions(transaction_ref);

		CREATE TABLE IF NOT EXISTS stock_event_outbox (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			event_type VARCHAR(20) NOT NULL,
			transaction_id UUID NOT NULL,
			transaction_ref VARCHAR(50) NOT NULL,
			user_id UUID NOT NULL,
			book_id UUID NOT NULL,
			quantity INT NOT NULL DEFAULT 1,
			status VARCHAR(20) NOT NULL DEFAULT 'PENDING',
			attempts INT NOT NULL DEFAULT 0,
			last_error TEXT,
			next_attempt_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			published_at TIMESTAMP WITH TIME ZONE,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			compensation_for_event_type VARCHAR(20),
			compensation_reason TEXT
		);

		CREATE UNIQUE INDEX IF NOT EXISTS idx_stock_event_outbox_transaction_type ON stock_event_outbox(transaction_id, event_type);
	`); err != nil {
		t.Fatalf("failed to prepare database tables: %v", err)
	}

	return pool
}
