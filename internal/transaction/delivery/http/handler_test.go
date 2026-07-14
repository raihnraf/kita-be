package http_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	transactionhttp "kita-be/internal/transaction/delivery/http"
	domain "kita-be/internal/transaction/domain"
	"kita-be/internal/transaction/usecase"
)

const (
	testUserID = "user-1"
	testBookID = "11111111-1111-1111-1111-111111111111"
)

func TestTransactionHandlerBorrowRequiresUser(t *testing.T) {
	app, _ := newTransactionTestApp("")

	req := httptest.NewRequest(fiber.MethodPost, "/transactions/borrow", strings.NewReader(`{"book_id":"`+testBookID+`","idempotency_key":"borrow-1"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", fiber.StatusUnauthorized, resp.StatusCode)
	}
}

func TestTransactionHandlerBorrowRequiresIdempotencyKey(t *testing.T) {
	app, _ := newTransactionTestApp(testUserID)

	req := httptest.NewRequest(fiber.MethodPost, "/transactions/borrow", strings.NewReader(`{"book_id":"`+testBookID+`"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", fiber.StatusBadRequest, resp.StatusCode)
	}
}

func TestTransactionHandlerBorrowSuccess(t *testing.T) {
	app, deps := newTransactionTestApp(testUserID)
	deps.bookClient.setStock(testBookID, 2)

	req := httptest.NewRequest(fiber.MethodPost, "/transactions/borrow", strings.NewReader(`{"book_id":"`+testBookID+`","idempotency_key":"borrow-1"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("expected status %d, got %d", fiber.StatusCreated, resp.StatusCode)
	}

	var body struct {
		Success bool `json:"success"`
		Data    struct {
			UserID string `json:"user_id"`
			BookID string `json:"book_id"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !body.Success || body.Data.UserID != testUserID || body.Data.BookID != testBookID || body.Data.Status != string(domain.TransactionActive) {
		t.Fatalf("unexpected response body: %+v", body)
	}
	if got := deps.bookClient.stock[testBookID]; got != 1 {
		t.Fatalf("expected stock 1 after borrow, got %d", got)
	}
}

func TestTransactionHandlerBorrowThenReturnFlow(t *testing.T) {
	app, deps := newTransactionTestApp(testUserID)
	deps.bookClient.setStock(testBookID, 2)

	borrowReq := httptest.NewRequest(fiber.MethodPost, "/transactions/borrow", strings.NewReader(`{"book_id":"`+testBookID+`","idempotency_key":"borrow-return-1"}`))
	borrowReq.Header.Set("Content-Type", "application/json")

	borrowResp, err := app.Test(borrowReq)
	if err != nil {
		t.Fatalf("expected no borrow error, got %v", err)
	}
	if borrowResp.StatusCode != fiber.StatusCreated {
		t.Fatalf("expected borrow status %d, got %d", fiber.StatusCreated, borrowResp.StatusCode)
	}

	var borrowBody struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(borrowResp.Body).Decode(&borrowBody); err != nil {
		t.Fatalf("failed to decode borrow response: %v", err)
	}
	if borrowBody.Data.ID == "" {
		t.Fatal("expected transaction id")
	}
	if got := deps.bookClient.stock[testBookID]; got != 1 {
		t.Fatalf("expected stock 1 after borrow, got %d", got)
	}

	returnReq := httptest.NewRequest(fiber.MethodPost, "/transactions/"+borrowBody.Data.ID+"/return", strings.NewReader(`{"idempotency_key":"return-1"}`))
	returnReq.Header.Set("Content-Type", "application/json")
	returnResp, err := app.Test(returnReq)
	if err != nil {
		t.Fatalf("expected no return error, got %v", err)
	}
	if returnResp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected return status %d, got %d", fiber.StatusOK, returnResp.StatusCode)
	}

	var returnBody struct {
		Success bool `json:"success"`
		Data    struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(returnResp.Body).Decode(&returnBody); err != nil {
		t.Fatalf("failed to decode return response: %v", err)
	}
	if !returnBody.Success || returnBody.Data.ID != borrowBody.Data.ID || returnBody.Data.Status != string(domain.TransactionReturned) {
		t.Fatalf("unexpected return response: %+v", returnBody)
	}
	if got := deps.bookClient.stock[testBookID]; got != 2 {
		t.Fatalf("expected stock restored to 2, got %d", got)
	}
}

func TestTransactionHandlerReturnRequiresIdempotencyKey(t *testing.T) {
	app, deps := newTransactionTestApp(testUserID)
	txn := deps.txnRepo.addTransaction(testUserID, testBookID)

	returnReq := httptest.NewRequest(fiber.MethodPost, "/transactions/"+txn.ID+"/return", strings.NewReader(`{}`))
	returnReq.Header.Set("Content-Type", "application/json")

	returnResp, err := app.Test(returnReq)
	if err != nil {
		t.Fatalf("expected no return error, got %v", err)
	}
	if returnResp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected return status %d, got %d", fiber.StatusBadRequest, returnResp.StatusCode)
	}
}

func TestTransactionHandlerReturnReplaysCompletedIdempotencyKey(t *testing.T) {
	app, deps := newTransactionTestApp(testUserID)
	deps.bookClient.setStock(testBookID, 1)
	txn := deps.txnRepo.addTransaction(testUserID, testBookID)

	body := `{"idempotency_key":"return-replay-1"}`
	returnReq1 := httptest.NewRequest(fiber.MethodPost, "/transactions/"+txn.ID+"/return", strings.NewReader(body))
	returnReq1.Header.Set("Content-Type", "application/json")
	returnResp1, err := app.Test(returnReq1)
	if err != nil {
		t.Fatalf("expected no first return error, got %v", err)
	}
	if returnResp1.StatusCode != fiber.StatusOK {
		t.Fatalf("expected first return status %d, got %d", fiber.StatusOK, returnResp1.StatusCode)
	}

	returnReq2 := httptest.NewRequest(fiber.MethodPost, "/transactions/"+txn.ID+"/return", strings.NewReader(body))
	returnReq2.Header.Set("Content-Type", "application/json")
	returnResp2, err := app.Test(returnReq2)
	if err != nil {
		t.Fatalf("expected no replay return error, got %v", err)
	}
	if returnResp2.StatusCode != fiber.StatusOK {
		t.Fatalf("expected replay return status %d, got %d", fiber.StatusOK, returnResp2.StatusCode)
	}
	if got := deps.bookClient.stock[testBookID]; got != 2 {
		t.Fatalf("expected stock increased exactly once, got %d", got)
	}
}

func TestTransactionHandlerHistoryUsesCurrentUserAndNormalizesPagination(t *testing.T) {
	app, deps := newTransactionTestApp(testUserID)
	deps.txnRepo.addTransaction(testUserID, testBookID)
	deps.txnRepo.addTransaction("other-user", testBookID)

	req := httptest.NewRequest(fiber.MethodGet, "/transactions/history?page=0&per_page=101", nil)

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
			UserID string `json:"user_id"`
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
	if !body.Success || len(body.Data) != 1 || body.Data[0].UserID != testUserID {
		t.Fatalf("expected only current user's transaction, got %+v", body.Data)
	}
	if body.Meta.Page != 1 || body.Meta.PerPage != 20 || body.Meta.Total != 1 || body.Meta.TotalPages != 1 {
		t.Fatalf("unexpected pagination meta: %+v", body.Meta)
	}
}

type transactionTestDeps struct {
	txnRepo    *handlerFakeTxnRepo
	auditRepo  *handlerFakeAuditRepo
	idempRepo  *handlerFakeIdempotencyRepo
	bookClient *handlerFakeBookClient
}

func newTransactionTestApp(userID string) (*fiber.App, *transactionTestDeps) {
	deps := &transactionTestDeps{
		txnRepo:    newHandlerFakeTxnRepo(),
		auditRepo:  newHandlerFakeAuditRepo(),
		idempRepo:  newHandlerFakeIdempotencyRepo(),
		bookClient: newHandlerFakeBookClient(),
	}
	borrowUC := usecase.NewBorrowUsecase(deps.txnRepo, deps.auditRepo, deps.idempRepo, deps.bookClient, 3, 7)
	returnUC := usecase.NewReturnUsecase(deps.txnRepo, deps.auditRepo, deps.idempRepo, deps.bookClient, usecase.NewFineCalculator(500))
	historyUC := usecase.NewHistoryUsecase(deps.txnRepo, deps.auditRepo)
	handler := transactionhttp.NewTransactionHandler(borrowUC, returnUC, historyUC)

	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		if userID != "" {
			c.Locals("user_id", userID)
		}
		return c.Next()
	})
	app.Post("/transactions/borrow", handler.Borrow)
	app.Post("/transactions/:id/return", handler.Return)
	app.Get("/transactions/history", handler.History)
	return app, deps
}

type handlerFakeTxnRepo struct {
	txns   map[string]*domain.BorrowTransaction
	outbox map[string]*domain.StockEventOutbox
}

func newHandlerFakeTxnRepo() *handlerFakeTxnRepo {
	return &handlerFakeTxnRepo{txns: make(map[string]*domain.BorrowTransaction), outbox: make(map[string]*domain.StockEventOutbox)}
}

func (r *handlerFakeTxnRepo) Create(ctx context.Context, tx *domain.BorrowTransaction) error {
	r.txns[tx.ID] = tx
	return nil
}

func (r *handlerFakeTxnRepo) CreateIfUserBelowActiveLimit(ctx context.Context, tx *domain.BorrowTransaction, maxActive int) error {
	return r.CreateBorrowWithOutbox(ctx, tx, maxActive, nil)
}

func (r *handlerFakeTxnRepo) CreateBorrowWithOutbox(ctx context.Context, tx *domain.BorrowTransaction, maxActive int, outbox *domain.StockEventOutbox) error {
	activeCount, err := r.CountActiveByUser(ctx, tx.UserID)
	if err != nil {
		return err
	}
	if activeCount >= maxActive {
		return domain.ErrActiveBorrowLimitReached
	}
	r.txns[tx.ID] = tx
	if outbox != nil {
		r.outbox[outbox.ID] = outbox
	}
	return nil
}

func (r *handlerFakeTxnRepo) FindByID(ctx context.Context, id string) (*domain.BorrowTransaction, error) {
	tx, ok := r.txns[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	txCopy := *tx
	return &txCopy, nil
}

func (r *handlerFakeTxnRepo) FindByRef(ctx context.Context, ref string) (*domain.BorrowTransaction, error) {
	for _, tx := range r.txns {
		if tx.TransactionRef == ref {
			txCopy := *tx
			return &txCopy, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func (r *handlerFakeTxnRepo) Update(ctx context.Context, tx *domain.BorrowTransaction) error {
	r.txns[tx.ID] = tx
	return nil
}

func (r *handlerFakeTxnRepo) ReturnIfActive(ctx context.Context, tx *domain.BorrowTransaction) error {
	return r.ReturnIfActiveWithOutbox(ctx, tx, nil)
}

func (r *handlerFakeTxnRepo) ReturnIfActiveWithOutbox(ctx context.Context, tx *domain.BorrowTransaction, outbox *domain.StockEventOutbox) error {
	existing, ok := r.txns[tx.ID]
	if !ok || existing.UserID != tx.UserID || existing.Status != domain.TransactionActive {
		return domain.ErrTransactionNotActive
	}
	r.txns[tx.ID] = tx
	if outbox != nil {
		r.outbox[outbox.ID] = outbox
	}
	return nil
}

func (r *handlerFakeTxnRepo) FindActiveByUser(ctx context.Context, userID string) ([]domain.BorrowTransaction, error) {
	var result []domain.BorrowTransaction
	for _, tx := range r.txns {
		if tx.UserID == userID && tx.Status == domain.TransactionActive {
			result = append(result, *tx)
		}
	}
	return result, nil
}

func (r *handlerFakeTxnRepo) CountActiveByUser(ctx context.Context, userID string) (int, error) {
	count := 0
	for _, tx := range r.txns {
		if tx.UserID == userID && tx.Status == domain.TransactionActive {
			count++
		}
	}
	return count, nil
}

func (r *handlerFakeTxnRepo) GetHistory(ctx context.Context, userID string, page, perPage int) ([]domain.BorrowTransaction, int64, error) {
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

func (r *handlerFakeTxnRepo) ListAll(ctx context.Context, page, perPage int) ([]domain.BorrowTransaction, int64, error) {
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

func (r *handlerFakeTxnRepo) addTransaction(userID, bookID string) *domain.BorrowTransaction {
	now := time.Now()
	tx := domain.NewBorrowTransaction(uuid.NewString(), "TXN-"+uuid.NewString(), userID, bookID, now, now.AddDate(0, 0, 7))
	r.txns[tx.ID] = tx
	return tx
}

type handlerFakeAuditRepo struct {
	audits map[string][]domain.TransactionAudit
}

func newHandlerFakeAuditRepo() *handlerFakeAuditRepo {
	return &handlerFakeAuditRepo{audits: make(map[string][]domain.TransactionAudit)}
}

func (r *handlerFakeAuditRepo) Create(ctx context.Context, audit *domain.TransactionAudit) error {
	r.audits[audit.TransactionID] = append(r.audits[audit.TransactionID], *audit)
	return nil
}

func (r *handlerFakeAuditRepo) FindByTransaction(ctx context.Context, txnID string) ([]domain.TransactionAudit, error) {
	return r.audits[txnID], nil
}

type handlerFakeIdempotencyRepo struct {
	records map[string]*domain.IdempotencyRecord
}

func newHandlerFakeIdempotencyRepo() *handlerFakeIdempotencyRepo {
	return &handlerFakeIdempotencyRepo{records: make(map[string]*domain.IdempotencyRecord)}
}

func (r *handlerFakeIdempotencyRepo) CheckOrCreate(ctx context.Context, scope, key, hash string) (bool, error) {
	recordKey := scope + ":" + key
	record, ok := r.records[recordKey]
	if ok {
		if record.RequestHash != hash {
			return false, fmt.Errorf("idempotency key conflict: different request body")
		}
		return true, nil
	}
	r.records[recordKey] = &domain.IdempotencyRecord{
		ID:             uuid.NewString(),
		Scope:          scope,
		IdempotencyKey: key,
		RequestHash:    hash,
		Status:         "PROCESSING",
	}
	return false, nil
}

func (r *handlerFakeIdempotencyRepo) SaveResponse(ctx context.Context, scope, key string, payload []byte) error {
	record, ok := r.records[scope+":"+key]
	if ok {
		record.Status = "COMPLETED"
		record.ResponsePayload = payload
	}
	return nil
}

func (r *handlerFakeIdempotencyRepo) GetRecord(ctx context.Context, scope, key string) (*domain.IdempotencyRecord, error) {
	return r.records[scope+":"+key], nil
}

type handlerFakeBookClient struct {
	stock map[string]int
}

func newHandlerFakeBookClient() *handlerFakeBookClient {
	return &handlerFakeBookClient{stock: make(map[string]int)}
}

func (c *handlerFakeBookClient) GetBook(ctx context.Context, bookID string) (*domain.BookSnapshot, error) {
	return &domain.BookSnapshot{
		ISBN:   "isbn-" + bookID,
		Title:  "Book " + bookID,
		Author: "Author",
	}, nil
}

func (c *handlerFakeBookClient) DecreaseStock(ctx context.Context, bookID string, qty int, txnID string) (string, error) {
	current := c.stock[bookID]
	if current < qty {
		return "", fmt.Errorf("insufficient stock")
	}
	c.stock[bookID] = current - qty
	return uuid.NewString(), nil
}

func (c *handlerFakeBookClient) IncreaseStock(ctx context.Context, bookID string, qty int, txnID string) (string, error) {
	c.stock[bookID] += qty
	return uuid.NewString(), nil
}

func (c *handlerFakeBookClient) setStock(bookID string, qty int) {
	c.stock[bookID] = qty
}
