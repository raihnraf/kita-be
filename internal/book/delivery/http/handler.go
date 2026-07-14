package http

import (
	"strconv"

	"github.com/gofiber/fiber/v2"

	"kita-be/internal/book/usecase"
	"kita-be/internal/platform/pagination"
	"kita-be/internal/platform/response"
	"kita-be/internal/platform/validation"
)

type BookHandler struct {
	listBooks  *usecase.ListBooksUsecase
	getBook    *usecase.GetBookUsecase
	createBook *usecase.CreateBookUsecase
	updateBook *usecase.UpdateBookUsecase
	stock      *usecase.StockUsecase
}

func NewBookHandler(
	listBooks *usecase.ListBooksUsecase,
	getBook *usecase.GetBookUsecase,
	createBook *usecase.CreateBookUsecase,
	updateBook *usecase.UpdateBookUsecase,
	stock *usecase.StockUsecase,
) *BookHandler {
	return &BookHandler{
		listBooks:  listBooks,
		getBook:    getBook,
		createBook: createBook,
		updateBook: updateBook,
		stock:      stock,
	}
}

func (h *BookHandler) List(c *fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("per_page", "20"))
	page, perPage = pagination.Normalize(page, perPage)
	search := c.Query("search", "")
	category := c.Query("category", "")

	output, err := h.listBooks.Execute(c.UserContext(), usecase.ListBooksInput{
		Search:   search,
		Category: category,
		Page:     page,
		PerPage:  perPage,
	})
	if err != nil {
		return err
	}

	books := make([]BookResponse, len(output.Books))
	for i, b := range output.Books {
		books[i] = FromDomain(b)
	}

	totalPages := output.Total / int64(perPage)
	if output.Total%int64(perPage) != 0 {
		totalPages++
	}

	return response.Paginated(c, books, response.Meta{
		Page:       page,
		PerPage:    perPage,
		Total:      output.Total,
		TotalPages: totalPages,
	})
}

func (h *BookHandler) Get(c *fiber.Ctx) error {
	id := c.Params("id")
	if !validation.UUID(id) {
		return response.BadRequest(c, "VALIDATION_ERROR", "book id must be a valid UUID")
	}

	book, err := h.getBook.Execute(c.UserContext(), id)
	if err != nil {
		return err
	}

	return response.OK(c, FromDomain(*book))
}

func (h *BookHandler) Create(c *fiber.Ctx) error {
	var req CreateBookRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "INVALID_REQUEST", "invalid request body")
	}

	if req.ISBN == "" || req.Title == "" || req.Author == "" {
		return response.BadRequest(c, "VALIDATION_ERROR", "isbn, title, and author are required")
	}
	if req.TotalStock < 0 {
		return response.BadRequest(c, "VALIDATION_ERROR", "total_stock must be non-negative")
	}

	book, err := h.createBook.Execute(c.UserContext(), usecase.CreateBookInput{
		ISBN:        req.ISBN,
		Title:       req.Title,
		Author:      req.Author,
		Publisher:   req.Publisher,
		Category:    req.Category,
		Description: req.Description,
		TotalStock:  req.TotalStock,
	})
	if err != nil {
		return err
	}

	return response.Created(c, FromDomain(*book))
}

func (h *BookHandler) Update(c *fiber.Ctx) error {
	id := c.Params("id")
	if !validation.UUID(id) {
		return response.BadRequest(c, "VALIDATION_ERROR", "book id must be a valid UUID")
	}

	var req UpdateBookRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "INVALID_REQUEST", "invalid request body")
	}
	if req.TotalStock != nil && *req.TotalStock < 0 {
		return response.BadRequest(c, "VALIDATION_ERROR", "total_stock must be non-negative")
	}

	book, err := h.updateBook.Execute(c.UserContext(), usecase.UpdateBookInput{
		ID:          id,
		ISBN:        req.ISBN,
		Title:       req.Title,
		Author:      req.Author,
		Publisher:   req.Publisher,
		Category:    req.Category,
		Description: req.Description,
		TotalStock:  req.TotalStock,
	})
	if err != nil {
		return err
	}

	return response.OK(c, FromDomain(*book))
}

func (h *BookHandler) Availability(c *fiber.Ctx) error {
	id := c.Params("id")
	if !validation.UUID(id) {
		return response.BadRequest(c, "VALIDATION_ERROR", "book id must be a valid UUID")
	}

	output, err := h.stock.CheckAvailability(c.UserContext(), id)
	if err != nil {
		return err
	}

	return response.OK(c, AvailabilityResponse{
		BookID:         output.BookID,
		AvailableStock: output.AvailableStock,
		CanBorrow:      output.CanBorrow,
	})
}

func (h *BookHandler) InternalDecreaseStock(c *fiber.Ctx) error {
	id := c.Params("id")
	if !validation.UUID(id) {
		return response.BadRequest(c, "VALIDATION_ERROR", "book id must be a valid UUID")
	}

	var req StockChangeRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "INVALID_REQUEST", "invalid request body")
	}

	if req.Quantity <= 0 || req.Quantity > 100 {
		return response.BadRequest(c, "VALIDATION_ERROR", "quantity must be between 1 and 100")
	}
	if req.TransactionID != "" && !validation.UUID(req.TransactionID) {
		return response.BadRequest(c, "VALIDATION_ERROR", "transaction_id must be a valid UUID")
	}

	event, err := h.stock.DecreaseStock(c.UserContext(), id, req.Quantity, req.TransactionID)
	if err != nil {
		return err
	}

	return response.OK(c, fiber.Map{
		"event_id": event.EventID,
		"status":   string(event.Status),
	})
}

func (h *BookHandler) InternalIncreaseStock(c *fiber.Ctx) error {
	id := c.Params("id")
	if !validation.UUID(id) {
		return response.BadRequest(c, "VALIDATION_ERROR", "book id must be a valid UUID")
	}

	var req StockChangeRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "INVALID_REQUEST", "invalid request body")
	}

	if req.Quantity <= 0 || req.Quantity > 100 {
		return response.BadRequest(c, "VALIDATION_ERROR", "quantity must be between 1 and 100")
	}
	if req.TransactionID != "" && !validation.UUID(req.TransactionID) {
		return response.BadRequest(c, "VALIDATION_ERROR", "transaction_id must be a valid UUID")
	}

	event, err := h.stock.IncreaseStock(c.UserContext(), id, req.Quantity, req.TransactionID)
	if err != nil {
		return err
	}

	return response.OK(c, fiber.Map{
		"event_id": event.EventID,
		"status":   string(event.Status),
	})
}
