package http

import (
	"strconv"

	"github.com/gofiber/fiber/v2"

	"kita-be/internal/platform/pagination"
	"kita-be/internal/platform/response"
	"kita-be/internal/platform/validation"
	"kita-be/internal/transaction/usecase"
)

type TransactionHandler struct {
	borrow  *usecase.BorrowUsecase
	return_ *usecase.ReturnUsecase
	history *usecase.HistoryUsecase
}

func NewTransactionHandler(
	borrow *usecase.BorrowUsecase,
	return_ *usecase.ReturnUsecase,
	history *usecase.HistoryUsecase,
) *TransactionHandler {
	return &TransactionHandler{
		borrow:  borrow,
		return_: return_,
		history: history,
	}
}

func (h *TransactionHandler) Borrow(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(string)
	if !ok {
		return response.Unauthorized(c, "UNAUTHORIZED", "invalid token")
	}

	var req BorrowRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "INVALID_REQUEST", "invalid request body")
	}
	if !validation.UUID(req.BookID) {
		return response.BadRequest(c, "VALIDATION_ERROR", "book_id must be a valid UUID")
	}
	if req.IdempotencyKey == "" {
		return response.BadRequest(c, "VALIDATION_ERROR", "idempotency_key is required")
	}

	output, err := h.borrow.Execute(c.Context(), usecase.BorrowInput{
		UserID:         userID,
		BookID:         req.BookID,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		return err
	}

	return response.Created(c, FromDomain(*output.Transaction))
}

func (h *TransactionHandler) Return(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(string)
	if !ok {
		return response.Unauthorized(c, "UNAUTHORIZED", "invalid token")
	}

	txnID := c.Params("id")
	if !validation.UUID(txnID) {
		return response.BadRequest(c, "VALIDATION_ERROR", "transaction id must be a valid UUID")
	}

	output, err := h.return_.Execute(c.Context(), usecase.ReturnInput{
		TransactionID: txnID,
		UserID:        userID,
	})
	if err != nil {
		return err
	}

	return response.OK(c, FromDomain(*output.Transaction))
}

func (h *TransactionHandler) History(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(string)
	if !ok {
		return response.Unauthorized(c, "UNAUTHORIZED", "invalid token")
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("per_page", "20"))
	page, perPage = pagination.Normalize(page, perPage)

	output, err := h.history.GetHistory(c.Context(), usecase.HistoryInput{
		UserID:  userID,
		Page:    page,
		PerPage: perPage,
	})
	if err != nil {
		return err
	}

	txs := make([]TransactionResponse, len(output.Transactions))
	for i, tx := range output.Transactions {
		txs[i] = FromDomain(tx)
	}

	totalPages := output.Total / int64(perPage)
	if output.Total%int64(perPage) != 0 {
		totalPages++
	}

	return response.Paginated(c, txs, response.Meta{
		Page:       page,
		PerPage:    perPage,
		Total:      output.Total,
		TotalPages: totalPages,
	})
}

func (h *TransactionHandler) Active(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(string)
	if !ok {
		return response.Unauthorized(c, "UNAUTHORIZED", "invalid token")
	}

	txs, err := h.history.GetActive(c.Context(), userID)
	if err != nil {
		return err
	}

	resp := make([]TransactionResponse, len(txs))
	for i, tx := range txs {
		resp[i] = FromDomain(tx)
	}

	return response.OK(c, resp)
}

func (h *TransactionHandler) Detail(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(string)
	if !ok {
		return response.Unauthorized(c, "UNAUTHORIZED", "invalid token")
	}

	txnID := c.Params("id")
	if !validation.UUID(txnID) {
		return response.BadRequest(c, "VALIDATION_ERROR", "transaction id must be a valid UUID")
	}

	tx, err := h.history.GetDetail(c.Context(), txnID, userID)
	if err != nil {
		return err
	}

	return response.OK(c, FromDomain(*tx))
}

func (h *TransactionHandler) InternalTransactions(c *fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("per_page", "20"))
	page, perPage = pagination.Normalize(page, perPage)

	output, err := h.history.GetAll(c.Context(), usecase.HistoryInput{
		Page:    page,
		PerPage: perPage,
	})
	if err != nil {
		return err
	}

	txs := make([]TransactionResponse, len(output.Transactions))
	for i, tx := range output.Transactions {
		txs[i] = FromDomain(tx)
	}

	totalPages := output.Total / int64(perPage)
	if output.Total%int64(perPage) != 0 {
		totalPages++
	}

	return response.Paginated(c, txs, response.Meta{
		Page:       page,
		PerPage:    perPage,
		Total:      output.Total,
		TotalPages: totalPages,
	})
}

func (h *TransactionHandler) InternalTransactionDetail(c *fiber.Ctx) error {
	txnID := c.Params("id")
	if !validation.UUID(txnID) {
		return response.BadRequest(c, "VALIDATION_ERROR", "transaction id must be a valid UUID")
	}

	tx, err := h.history.GetInternalDetail(c.Context(), txnID)
	if err != nil {
		return err
	}

	return response.OK(c, FromDomain(*tx))
}

func (h *TransactionHandler) InternalTransactionAudits(c *fiber.Ctx) error {
	txnID := c.Params("id")
	if !validation.UUID(txnID) {
		return response.BadRequest(c, "VALIDATION_ERROR", "transaction id must be a valid UUID")
	}

	audits, err := h.history.GetInternalAudits(c.Context(), txnID)
	if err != nil {
		return err
	}

	resp := make([]TransactionAuditResponse, len(audits))
	for i, audit := range audits {
		resp[i] = AuditFromDomain(audit)
	}

	return response.OK(c, resp)
}
