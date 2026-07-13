package usecase

import (
	"context"
	"fmt"

	domain "kita-be/internal/book/domain"
	"kita-be/internal/platform/apperror"
)

type UpdateBookUsecase struct {
	bookRepo BookRepository
}

func NewUpdateBookUsecase(bookRepo BookRepository) *UpdateBookUsecase {
	return &UpdateBookUsecase{bookRepo: bookRepo}
}

type UpdateBookInput struct {
	ID          string
	ISBN        string
	Title       string
	Author      string
	Publisher   *string
	Category    *string
	Description *string
	TotalStock  *int
}

func (uc *UpdateBookUsecase) Execute(ctx context.Context, input UpdateBookInput) (*domain.Book, error) {
	book, err := uc.bookRepo.FindByID(ctx, input.ID)
	if err != nil {
		return nil, apperror.NotFound("book not found")
	}

	if input.ISBN != "" && input.ISBN != book.ISBN {
		existing, _ := uc.bookRepo.FindByISBN(ctx, input.ISBN)
		if existing != nil && existing.ID != book.ID {
			return nil, apperror.Conflict("book with this ISBN already exists")
		}
		book.ISBN = input.ISBN
	}
	if input.Title != "" {
		book.Title = input.Title
	}
	if input.Author != "" {
		book.Author = input.Author
	}
	book.Publisher = input.Publisher
	book.Category = input.Category
	book.Description = input.Description

	if input.TotalStock != nil {
		diff := *input.TotalStock - book.TotalStock
		book.TotalStock = *input.TotalStock
		book.AvailableStock += diff
		if book.AvailableStock < 0 {
			book.AvailableStock = 0
		}
		if book.AvailableStock == 0 {
			book.Status = domain.BookStatusOutOfStock
		} else {
			book.Status = domain.BookStatusAvailable
		}
	}

	if err := uc.bookRepo.Update(ctx, book); err != nil {
		return nil, fmt.Errorf("failed to update book: %w", err)
	}

	return book, nil
}
