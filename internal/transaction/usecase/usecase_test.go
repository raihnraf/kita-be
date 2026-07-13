package usecase_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	domain "kita-be/internal/transaction/domain"
	"kita-be/internal/transaction/usecase"
)

type fakeTxnRepo struct {
	txns map[string]*domain.BorrowTransaction
}

func newFakeTxnRepo() *fakeTxnRepo {
	return &fakeTxnRepo{txns: make(map[string]*domain.BorrowTransaction)}
}

func (r *fakeTxnRepo) Create(ctx context.Context, tx *domain.BorrowTransaction) error {
	r.txns[tx.ID] = tx
	return nil
}

func (r *fakeTxnRepo) CreateIfUserBelowActiveLimit(ctx context.Context, tx *domain.BorrowTransaction, maxActive int) error {
	activeCount, err := r.CountActiveByUser(ctx, tx.UserID)
	if err != nil {
		return err
	}
	if activeCount >= maxActive {
		return domain.ErrActiveBorrowLimitReached
	}
	r.txns[tx.ID] = tx
	return nil
}

func (r *fakeTxnRepo) FindByID(ctx context.Context, id string) (*domain.BorrowTransaction, error) {
	tx, ok := r.txns[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	txCopy := *tx
	return &txCopy, nil
}

func (r *fakeTxnRepo) FindByRef(ctx context.Context, ref string) (*domain.BorrowTransaction, error) {
	for _, tx := range r.txns {
		if tx.TransactionRef == ref {
			txCopy := *tx
			return &txCopy, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func (r *fakeTxnRepo) Update(ctx context.Context, tx *domain.BorrowTransaction) error {
	r.txns[tx.ID] = tx
	return nil
}

func (r *fakeTxnRepo) ReturnIfActive(ctx context.Context, tx *domain.BorrowTransaction) error {
	existing, ok := r.txns[tx.ID]
	if !ok || existing.UserID != tx.UserID || existing.Status != domain.TransactionActive {
		return domain.ErrTransactionNotActive
	}
	r.txns[tx.ID] = tx
	return nil
}

func (r *fakeTxnRepo) FindActiveByUser(ctx context.Context, userID string) ([]domain.BorrowTransaction, error) {
	var result []domain.BorrowTransaction
	for _, tx := range r.txns {
		if tx.UserID == userID && tx.Status == domain.TransactionActive {
			result = append(result, *tx)
		}
	}
	return result, nil
}

func (r *fakeTxnRepo) CountActiveByUser(ctx context.Context, userID string) (int, error) {
	count := 0
	for _, tx := range r.txns {
		if tx.UserID == userID && tx.Status == domain.TransactionActive {
			count++
		}
	}
	return count, nil
}

func (r *fakeTxnRepo) GetHistory(ctx context.Context, userID string, page, perPage int) ([]domain.BorrowTransaction, int64, error) {
	var result []domain.BorrowTransaction
	for _, tx := range r.txns {
		if tx.UserID == userID {
			result = append(result, *tx)
		}
	}
	total := int64(len(result))
	start := (page - 1) * perPage
	if start >= len(result) {
		return nil, total, nil
	}
	end := start + perPage
	if end > len(result) {
		end = len(result)
	}
	return result[start:end], total, nil
}

func (r *fakeTxnRepo) ListAll(ctx context.Context, page, perPage int) ([]domain.BorrowTransaction, int64, error) {
	var result []domain.BorrowTransaction
	for _, tx := range r.txns {
		result = append(result, *tx)
	}
	total := int64(len(result))
	start := (page - 1) * perPage
	if start >= len(result) {
		return nil, total, nil
	}
	end := start + perPage
	if end > len(result) {
		end = len(result)
	}
	return result[start:end], total, nil
}

type fakeAuditRepo struct {
	audits map[string][]domain.TransactionAudit
}

func newFakeAuditRepo() *fakeAuditRepo {
	return &fakeAuditRepo{audits: make(map[string][]domain.TransactionAudit)}
}

func (r *fakeAuditRepo) Create(ctx context.Context, audit *domain.TransactionAudit) error {
	r.audits[audit.TransactionID] = append(r.audits[audit.TransactionID], *audit)
	return nil
}

func (r *fakeAuditRepo) FindByTransaction(ctx context.Context, txnID string) ([]domain.TransactionAudit, error) {
	return r.audits[txnID], nil
}

type fakeIdempotencyRepo struct {
	records map[string]*domain.IdempotencyRecord
}

func newFakeIdempotencyRepo() *fakeIdempotencyRepo {
	return &fakeIdempotencyRepo{records: make(map[string]*domain.IdempotencyRecord)}
}

func (r *fakeIdempotencyRepo) CheckOrCreate(ctx context.Context, scope, key, hash string) (bool, error) {
	k := scope + ":" + key
	existing, ok := r.records[k]
	if ok {
		if existing.RequestHash != hash {
			return false, fmt.Errorf("idempotency key conflict: different request body")
		}
		return true, nil
	}
	r.records[k] = &domain.IdempotencyRecord{
		ID:             uuid.New().String(),
		Scope:          scope,
		IdempotencyKey: key,
		RequestHash:    hash,
		Status:         "PROCESSING",
	}
	return false, nil
}

func (r *fakeIdempotencyRepo) SaveResponse(ctx context.Context, scope, key string, payload []byte) error {
	k := scope + ":" + key
	rec, ok := r.records[k]
	if ok {
		rec.Status = "COMPLETED"
		rec.ResponsePayload = payload
	}
	return nil
}

func (r *fakeIdempotencyRepo) GetRecord(ctx context.Context, scope, key string) (*domain.IdempotencyRecord, error) {
	k := scope + ":" + key
	rec, ok := r.records[k]
	if !ok {
		return nil, nil
	}
	return rec, nil
}

type fakeBookClient struct {
	stock map[string]int
}

func newFakeBookClient() *fakeBookClient {
	return &fakeBookClient{stock: make(map[string]int)}
}

func (c *fakeBookClient) DecreaseStock(ctx context.Context, bookID string, qty int, txnID string) (string, error) {
	current := c.stock[bookID]
	if current < qty {
		return "", fmt.Errorf("insufficient stock")
	}
	c.stock[bookID] = current - qty
	return uuid.New().String(), nil
}

func (c *fakeBookClient) GetBook(ctx context.Context, bookID string) (*domain.BookSnapshot, error) {
	return &domain.BookSnapshot{
		ISBN:   "isbn-" + bookID,
		Title:  "Book " + bookID,
		Author: "Author",
	}, nil
}

func (c *fakeBookClient) IncreaseStock(ctx context.Context, bookID string, qty int, txnID string) (string, error) {
	c.stock[bookID] += qty
	return uuid.New().String(), nil
}

func (c *fakeBookClient) setStock(bookID string, qty int) {
	c.stock[bookID] = qty
}

func TestBorrowSuccess(t *testing.T) {
	txnRepo := newFakeTxnRepo()
	auditRepo := newFakeAuditRepo()
	idempRepo := newFakeIdempotencyRepo()
	bookClient := newFakeBookClient()
	bookClient.setStock("book-1", 3)

	uc := usecase.NewBorrowUsecase(txnRepo, auditRepo, idempRepo, bookClient, 3, 7)

	output, err := uc.Execute(context.Background(), usecase.BorrowInput{
		UserID: "user-1",
		BookID: "book-1",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if output.Transaction.Status != domain.TransactionActive {
		t.Errorf("expected status ACTIVE, got %s", output.Transaction.Status)
	}
	if output.Transaction.UserID != "user-1" {
		t.Errorf("expected user_id user-1, got %s", output.Transaction.UserID)
	}

	active, _ := txnRepo.CountActiveByUser(context.Background(), "user-1")
	if active != 1 {
		t.Errorf("expected 1 active borrow, got %d", active)
	}
}

func TestBorrowMaxLimitReached(t *testing.T) {
	txnRepo := newFakeTxnRepo()
	auditRepo := newFakeAuditRepo()
	idempRepo := newFakeIdempotencyRepo()
	bookClient := newFakeBookClient()
	bookClient.setStock("book-1", 5)
	bookClient.setStock("book-2", 5)
	bookClient.setStock("book-3", 5)
	bookClient.setStock("book-4", 5)

	uc := usecase.NewBorrowUsecase(txnRepo, auditRepo, idempRepo, bookClient, 3, 7)

	if _, err := uc.Execute(context.Background(), usecase.BorrowInput{UserID: "user-1", BookID: "book-1"}); err != nil {
		t.Fatalf("expected first borrow to succeed, got: %v", err)
	}
	if _, err := uc.Execute(context.Background(), usecase.BorrowInput{UserID: "user-1", BookID: "book-2"}); err != nil {
		t.Fatalf("expected second borrow to succeed, got: %v", err)
	}
	if _, err := uc.Execute(context.Background(), usecase.BorrowInput{UserID: "user-1", BookID: "book-3"}); err != nil {
		t.Fatalf("expected third borrow to succeed, got: %v", err)
	}

	_, err := uc.Execute(context.Background(), usecase.BorrowInput{
		UserID: "user-1",
		BookID: "book-4",
	})
	if err == nil {
		t.Fatal("expected error for max borrows reached")
	}
}

func TestBorrowInsufficientStock(t *testing.T) {
	txnRepo := newFakeTxnRepo()
	auditRepo := newFakeAuditRepo()
	idempRepo := newFakeIdempotencyRepo()
	bookClient := newFakeBookClient()
	bookClient.setStock("book-1", 0)

	uc := usecase.NewBorrowUsecase(txnRepo, auditRepo, idempRepo, bookClient, 3, 7)

	_, err := uc.Execute(context.Background(), usecase.BorrowInput{
		UserID: "user-1",
		BookID: "book-1",
	})
	if err == nil {
		t.Fatal("expected error for insufficient stock")
	}
}

func TestBorrowIdempotencyReplaysCompleted(t *testing.T) {
	txnRepo := newFakeTxnRepo()
	auditRepo := newFakeAuditRepo()
	idempRepo := newFakeIdempotencyRepo()
	bookClient := newFakeBookClient()
	bookClient.setStock("book-1", 5)

	uc := usecase.NewBorrowUsecase(txnRepo, auditRepo, idempRepo, bookClient, 3, 7)

	out1, err := uc.Execute(context.Background(), usecase.BorrowInput{
		UserID:         "user-1",
		BookID:         "book-1",
		IdempotencyKey: "key-1",
	})
	if err != nil {
		t.Fatalf("expected no error on first call: %v", err)
	}

	out2, err := uc.Execute(context.Background(), usecase.BorrowInput{
		UserID:         "user-1",
		BookID:         "book-1",
		IdempotencyKey: "key-1",
	})
	if err != nil {
		t.Fatalf("expected no error on duplicate completed call (should replay): %v", err)
	}

	if out1.Transaction.ID != out2.Transaction.ID {
		t.Errorf("expected same transaction ID replayed, got %s and %s", out1.Transaction.ID, out2.Transaction.ID)
	}
}

func TestBorrowIdempotencyPreventsDuplicateProcessing(t *testing.T) {
	txnRepo := newFakeTxnRepo()
	auditRepo := newFakeAuditRepo()
	idempRepo := newFakeIdempotencyRepo()
	bookClient := newFakeBookClient()
	bookClient.setStock("book-1", 5)

	uc := usecase.NewBorrowUsecase(txnRepo, auditRepo, idempRepo, bookClient, 3, 7)

	// Manually insert a PROCESSING record
	ctx := context.Background()
	_, _ = idempRepo.CheckOrCreate(ctx, "borrow", "key-1", "some-hash")

	// This next request should fail with duplicate/conflict since it is still PROCESSING
	_, err := uc.Execute(ctx, usecase.BorrowInput{
		UserID:         "user-1",
		BookID:         "book-1",
		IdempotencyKey: "key-1",
	})
	if err == nil {
		t.Fatal("expected error for duplicate idempotency key in PROCESSING state")
	}
}

func TestBorrowIdempotencyDifferentBodyRejected(t *testing.T) {
	txnRepo := newFakeTxnRepo()
	auditRepo := newFakeAuditRepo()
	idempRepo := newFakeIdempotencyRepo()
	bookClient := newFakeBookClient()
	bookClient.setStock("book-1", 5)
	bookClient.setStock("book-2", 5)

	uc := usecase.NewBorrowUsecase(txnRepo, auditRepo, idempRepo, bookClient, 3, 7)

	if _, err := uc.Execute(context.Background(), usecase.BorrowInput{
		UserID:         "user-1",
		BookID:         "book-1",
		IdempotencyKey: "key-1",
	}); err != nil {
		t.Fatalf("expected initial borrow to succeed, got: %v", err)
	}

	_, err := uc.Execute(context.Background(), usecase.BorrowInput{
		UserID:         "user-1",
		BookID:         "book-2",
		IdempotencyKey: "key-1",
	})
	if err == nil {
		t.Fatal("expected error for same key different body")
	}
}

func TestReturnSuccess(t *testing.T) {
	txnRepo := newFakeTxnRepo()
	auditRepo := newFakeAuditRepo()
	idempRepo := newFakeIdempotencyRepo()
	bookClient := newFakeBookClient()
	bookClient.setStock("book-1", 3)

	borrowUC := usecase.NewBorrowUsecase(txnRepo, auditRepo, idempRepo, bookClient, 3, 7)
	borrowOutput, _ := borrowUC.Execute(context.Background(), usecase.BorrowInput{
		UserID: "user-1",
		BookID: "book-1",
	})
	if bookClient.stock["book-1"] != 2 {
		t.Fatalf("expected stock to decrease to 2 after borrow, got %d", bookClient.stock["book-1"])
	}

	time.Sleep(1 * time.Millisecond)

	fineCalc := usecase.NewFineCalculator(50000)
	returnUC := usecase.NewReturnUsecase(txnRepo, auditRepo, idempRepo, bookClient, fineCalc)

	output, err := returnUC.Execute(context.Background(), usecase.ReturnInput{
		TransactionID: borrowOutput.Transaction.ID,
		UserID:        "user-1",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if output.Transaction.Status != domain.TransactionReturned {
		t.Errorf("expected status RETURNED, got %s", output.Transaction.Status)
	}
	if bookClient.stock["book-1"] != 3 {
		t.Errorf("expected stock to be restored to 3 after return, got %d", bookClient.stock["book-1"])
	}
	active, err := txnRepo.CountActiveByUser(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("failed to count active loans: %v", err)
	}
	if active != 0 {
		t.Errorf("expected 0 active loans after return, got %d", active)
	}
}

func TestReturnLateCreatesFine(t *testing.T) {
	txnRepo := newFakeTxnRepo()
	auditRepo := newFakeAuditRepo()
	idempRepo := newFakeIdempotencyRepo()
	bookClient := newFakeBookClient()
	bookClient.setStock("book-1", 3)

	borrowUC := usecase.NewBorrowUsecase(txnRepo, auditRepo, idempRepo, bookClient, 3, 7)
	borrowOutput, _ := borrowUC.Execute(context.Background(), usecase.BorrowInput{
		UserID: "user-1",
		BookID: "book-1",
	})

	txn, _ := txnRepo.FindByID(context.Background(), borrowOutput.Transaction.ID)
	pastDue := time.Now().AddDate(0, 0, -10)
	txn.DueAt = pastDue
	txn.BorrowedAt = pastDue.AddDate(0, 0, -7)
	_ = txnRepo.Update(context.Background(), txn)

	fineCalc := usecase.NewFineCalculator(50000)
	returnUC := usecase.NewReturnUsecase(txnRepo, auditRepo, idempRepo, bookClient, fineCalc)

	output, err := returnUC.Execute(context.Background(), usecase.ReturnInput{
		TransactionID: borrowOutput.Transaction.ID,
		UserID:        "user-1",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if output.Transaction.Status != domain.TransactionReturnedLate {
		t.Errorf("expected status RETURNED_LATE, got %s", output.Transaction.Status)
	}
	if output.Transaction.LateDays <= 0 {
		t.Errorf("expected late_days > 0, got %d", output.Transaction.LateDays)
	}
	if output.Transaction.FineAmountCents <= 0 {
		t.Errorf("expected fine cents > 0, got %d", output.Transaction.FineAmountCents)
	}
}

func TestReturnWrongUser(t *testing.T) {
	txnRepo := newFakeTxnRepo()
	auditRepo := newFakeAuditRepo()
	idempRepo := newFakeIdempotencyRepo()
	bookClient := newFakeBookClient()
	bookClient.setStock("book-1", 3)

	borrowUC := usecase.NewBorrowUsecase(txnRepo, auditRepo, idempRepo, bookClient, 3, 7)
	borrowOutput, _ := borrowUC.Execute(context.Background(), usecase.BorrowInput{
		UserID: "user-1",
		BookID: "book-1",
	})

	fineCalc := usecase.NewFineCalculator(50000)
	returnUC := usecase.NewReturnUsecase(txnRepo, auditRepo, idempRepo, bookClient, fineCalc)

	_, err := returnUC.Execute(context.Background(), usecase.ReturnInput{
		TransactionID: borrowOutput.Transaction.ID,
		UserID:        "user-2",
	})
	if err == nil {
		t.Fatal("expected error for wrong user")
	}
}

func TestHistoryPaginated(t *testing.T) {
	txnRepo := newFakeTxnRepo()
	auditRepo := newFakeAuditRepo()
	idempRepo := newFakeIdempotencyRepo()
	bookClient := newFakeBookClient()

	for i := 0; i < 5; i++ {
		bookClient.setStock("book-"+string(rune('a'+i)), 5)
	}

	borrowUC := usecase.NewBorrowUsecase(txnRepo, auditRepo, idempRepo, bookClient, 10, 7)
	for i := 0; i < 5; i++ {
		if _, err := borrowUC.Execute(context.Background(), usecase.BorrowInput{
			UserID: "user-1",
			BookID: "book-" + string(rune('a'+i)),
		}); err != nil {
			t.Fatalf("expected borrow %d to succeed, got: %v", i, err)
		}
	}

	historyUC := usecase.NewHistoryUsecase(txnRepo, auditRepo)
	output, err := historyUC.GetHistory(context.Background(), usecase.HistoryInput{
		UserID:  "user-1",
		Page:    1,
		PerPage: 2,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(output.Transactions) != 2 {
		t.Errorf("expected 2 transactions on page 1, got %d", len(output.Transactions))
	}
	if output.Total != 5 {
		t.Errorf("expected total 5, got %d", output.Total)
	}
}

func TestActiveLoans(t *testing.T) {
	txnRepo := newFakeTxnRepo()
	auditRepo := newFakeAuditRepo()
	idempRepo := newFakeIdempotencyRepo()
	bookClient := newFakeBookClient()
	bookClient.setStock("book-1", 5)
	bookClient.setStock("book-2", 5)

	borrowUC := usecase.NewBorrowUsecase(txnRepo, auditRepo, idempRepo, bookClient, 10, 7)
	if _, err := borrowUC.Execute(context.Background(), usecase.BorrowInput{UserID: "user-1", BookID: "book-1"}); err != nil {
		t.Fatalf("expected first borrow to succeed, got: %v", err)
	}
	if _, err := borrowUC.Execute(context.Background(), usecase.BorrowInput{UserID: "user-1", BookID: "book-2"}); err != nil {
		t.Fatalf("expected second borrow to succeed, got: %v", err)
	}

	historyUC := usecase.NewHistoryUsecase(txnRepo, auditRepo)
	txs, err := historyUC.GetActive(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(txs) != 2 {
		t.Errorf("expected 2 active loans, got %d", len(txs))
	}
}
