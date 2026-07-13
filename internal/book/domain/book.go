package domain

import (
	"time"
)

type BookStatus string

const (
	BookStatusAvailable  BookStatus = "AVAILABLE"
	BookStatusOutOfStock BookStatus = "OUT_OF_STOCK"
	BookStatusInactive   BookStatus = "INACTIVE"
)

type Book struct {
	ID             string
	ISBN           string
	Title          string
	Author         string
	Publisher      *string
	Category       *string
	Description    *string
	TotalStock     int
	AvailableStock int
	Status         BookStatus
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func NewBook(id, isbn, title, author string, totalStock int) *Book {
	now := time.Now()
	status := BookStatusAvailable
	if totalStock <= 0 {
		status = BookStatusOutOfStock
	}
	return &Book{
		ID:             id,
		ISBN:           isbn,
		Title:          title,
		Author:         author,
		TotalStock:     totalStock,
		AvailableStock: totalStock,
		Status:         status,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func (b *Book) CanBorrow() bool {
	return b.AvailableStock > 0 && b.Status == BookStatusAvailable
}

func (b *Book) DecreaseStock(qty int) error {
	if b.AvailableStock < qty {
		return ErrInsufficientStock
	}
	b.AvailableStock -= qty
	if b.AvailableStock == 0 {
		b.Status = BookStatusOutOfStock
	}
	b.UpdatedAt = time.Now()
	return nil
}

func (b *Book) IncreaseStock(qty int) error {
	if b.AvailableStock+qty > b.TotalStock {
		return ErrStockExceedsTotal
	}
	b.AvailableStock += qty
	if b.AvailableStock > 0 && b.Status == BookStatusOutOfStock {
		b.Status = BookStatusAvailable
	}
	b.UpdatedAt = time.Now()
	return nil
}

type ErrInsufficientStockType struct{}

func (e ErrInsufficientStockType) Error() string {
	return "insufficient stock"
}

var ErrInsufficientStock = ErrInsufficientStockType{}

type ErrStockExceedsTotalType struct{}

func (e ErrStockExceedsTotalType) Error() string {
	return "stock increase would exceed total stock"
}

var ErrStockExceedsTotal = ErrStockExceedsTotalType{}
