package http

import (
	domain "kita-be/internal/book/domain"
)

type CreateBookRequest struct {
	ISBN        string  `json:"isbn"`
	Title       string  `json:"title"`
	Author      string  `json:"author"`
	Publisher   *string `json:"publisher"`
	Category    *string `json:"category"`
	Description *string `json:"description"`
	TotalStock  int     `json:"total_stock"`
}

type UpdateBookRequest struct {
	ISBN        string  `json:"isbn"`
	Title       string  `json:"title"`
	Author      string  `json:"author"`
	Publisher   *string `json:"publisher"`
	Category    *string `json:"category"`
	Description *string `json:"description"`
	TotalStock  *int    `json:"total_stock"`
}

type BookResponse struct {
	ID             string  `json:"id"`
	ISBN           string  `json:"isbn"`
	Title          string  `json:"title"`
	Author         string  `json:"author"`
	Publisher      *string `json:"publisher"`
	Category       *string `json:"category"`
	Description    *string `json:"description"`
	TotalStock     int     `json:"total_stock"`
	AvailableStock int     `json:"available_stock"`
	Status         string  `json:"status"`
	CanBorrow      bool    `json:"can_borrow"`
	CreatedAt      string  `json:"created_at"`
}

type StockChangeRequest struct {
	Quantity      int    `json:"quantity"`
	TransactionID string `json:"transaction_id"`
}

type AvailabilityResponse struct {
	BookID         string `json:"book_id"`
	AvailableStock int    `json:"available_stock"`
	CanBorrow      bool   `json:"can_borrow"`
}

func FromDomain(b domain.Book) BookResponse {
	return BookResponse{
		ID:             b.ID,
		ISBN:           b.ISBN,
		Title:          b.Title,
		Author:         b.Author,
		Publisher:      b.Publisher,
		Category:       b.Category,
		Description:    b.Description,
		TotalStock:     b.TotalStock,
		AvailableStock: b.AvailableStock,
		Status:         string(b.Status),
		CanBorrow:      b.CanBorrow(),
		CreatedAt:      b.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
