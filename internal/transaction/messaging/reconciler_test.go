package messaging_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	domain "kita-be/internal/transaction/domain"
	"kita-be/internal/transaction/messaging"
)

type mockReconciliationRepo struct {
	mu     sync.Mutex
	txns   map[string]*domain.BorrowTransaction
	outbox map[string]*domain.StockEventOutbox
}

func (m *mockReconciliationRepo) Create(ctx context.Context, tx *domain.BorrowTransaction) error {
	return nil
}
func (m *mockReconciliationRepo) CreateIfUserBelowActiveLimit(ctx context.Context, tx *domain.BorrowTransaction, maxActive int) error {
	return nil
}
func (m *mockReconciliationRepo) CreateBorrowWithOutbox(ctx context.Context, tx *domain.BorrowTransaction, maxActive int, outbox *domain.StockEventOutbox) error {
	return nil
}
func (m *mockReconciliationRepo) EnqueueStockEvent(ctx context.Context, outbox *domain.StockEventOutbox) error {
	return nil
}
func (m *mockReconciliationRepo) SkipOutboxByTransactionID(ctx context.Context, transactionID string) error {
	return nil
}
func (m *mockReconciliationRepo) FindByID(ctx context.Context, id string) (*domain.BorrowTransaction, error) {
	return nil, nil
}
func (m *mockReconciliationRepo) FindByRef(ctx context.Context, ref string) (*domain.BorrowTransaction, error) {
	return nil, nil
}
func (m *mockReconciliationRepo) Update(ctx context.Context, tx *domain.BorrowTransaction) error {
	return nil
}
func (m *mockReconciliationRepo) UpdateStockEventID(ctx context.Context, id, stockEventID string) error {
	return nil
}
func (m *mockReconciliationRepo) ActivateBorrow(ctx context.Context, id, stockEventID string) error {
	return nil
}
func (m *mockReconciliationRepo) CancelBorrow(ctx context.Context, id string) error {
	return nil
}
func (m *mockReconciliationRepo) StartReturnWithOutbox(ctx context.Context, tx *domain.BorrowTransaction, outbox *domain.StockEventOutbox) error {
	return nil
}
func (m *mockReconciliationRepo) FinalizeReturn(ctx context.Context, id, stockEventID string) error {
	return nil
}
func (m *mockReconciliationRepo) RejectReturn(ctx context.Context, id string) error {
	return nil
}
func (m *mockReconciliationRepo) ReturnIfActive(ctx context.Context, tx *domain.BorrowTransaction) error {
	return nil
}
func (m *mockReconciliationRepo) ReturnIfActiveWithOutbox(ctx context.Context, tx *domain.BorrowTransaction, outbox *domain.StockEventOutbox) error {
	return nil
}
func (m *mockReconciliationRepo) FindActiveByUser(ctx context.Context, userID string) ([]domain.BorrowTransaction, error) {
	return nil, nil
}
func (m *mockReconciliationRepo) CountActiveByUser(ctx context.Context, userID string) (int, error) {
	return 0, nil
}
func (m *mockReconciliationRepo) GetHistory(ctx context.Context, userID string, page, perPage int) ([]domain.BorrowTransaction, int64, error) {
	return nil, 0, nil
}
func (m *mockReconciliationRepo) ListAll(ctx context.Context, page, perPage int) ([]domain.BorrowTransaction, int64, error) {
	return nil, 0, nil
}

func (m *mockReconciliationRepo) FindPendingOlderThan(ctx context.Context, threshold time.Time) ([]domain.BorrowTransaction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []domain.BorrowTransaction
	for _, tx := range m.txns {
		if (tx.Status == domain.TransactionPending || tx.Status == domain.TransactionReturnPending) && tx.CreatedAt.Before(threshold) {
			result = append(result, *tx)
		}
	}
	return result, nil
}

func (m *mockReconciliationRepo) ReconcileCancelBorrow(ctx context.Context, id string, outbox *domain.StockEventOutbox) error {
	return nil
}

func (m *mockReconciliationRepo) RequeueStockCommand(ctx context.Context, transactionID, eventType string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, event := range m.outbox {
		if event.TransactionID == transactionID && event.EventType == eventType {
			event.Status = domain.StockEventOutboxPending
			return nil
		}
	}
	return nil
}

func TestReconciliationWorker(t *testing.T) {
	repo := &mockReconciliationRepo{
		txns:   make(map[string]*domain.BorrowTransaction),
		outbox: make(map[string]*domain.StockEventOutbox),
	}

	now := time.Now().UTC()
	stuckTx := &domain.BorrowTransaction{
		ID:             uuid.NewString(),
		TransactionRef: "STUCK-1",
		UserID:         "user-1",
		BookID:         "book-1",
		Status:         domain.TransactionPending,
		CreatedAt:      now.Add(-2 * time.Minute),
	}
	repo.txns[stuckTx.ID] = stuckTx
	repo.outbox[uuid.NewString()] = &domain.StockEventOutbox{TransactionID: stuckTx.ID, EventType: "DECREASE", Status: domain.StockEventOutboxPublished}

	freshTx := &domain.BorrowTransaction{
		ID:             uuid.NewString(),
		TransactionRef: "FRESH-1",
		UserID:         "user-1",
		BookID:         "book-1",
		Status:         domain.TransactionPending,
		CreatedAt:      now.Add(-10 * time.Second),
	}
	repo.txns[freshTx.ID] = freshTx

	worker := messaging.NewReconciliationWorker(repo, 50*time.Millisecond, 1*time.Minute)
	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		worker.Run(ctx)
	}()
	time.Sleep(100 * time.Millisecond)

	// Stop the worker before reading shared state to avoid data races.
	cancel()
	wg.Wait()

	if stuckTx.Status != domain.TransactionPending {
		t.Errorf("expected stuck transaction to remain pending for retry, got status: %s", stuckTx.Status)
	}
	if freshTx.Status != domain.TransactionPending {
		t.Errorf("expected fresh transaction to remain pending, got status: %s", freshTx.Status)
	}

	repo.mu.Lock()
	defer repo.mu.Unlock()

	if len(repo.outbox) != 1 {
		t.Fatalf("expected exactly 1 command event in outbox, got: %d", len(repo.outbox))
	}

	var pendingEvent *domain.StockEventOutbox
	for _, event := range repo.outbox {
		pendingEvent = event
	}

	if pendingEvent.EventType != "DECREASE" {
		t.Errorf("expected requeued event type DECREASE, got: %s", pendingEvent.EventType)
	}
	if pendingEvent.Status != domain.StockEventOutboxPending {
		t.Errorf("expected requeued event status PENDING, got: %s", pendingEvent.Status)
	}
}
