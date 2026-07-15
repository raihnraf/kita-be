package usecase

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5"

	domain "kita-be/internal/book/domain"
	"kita-be/internal/platform/apperror"
)

var isbnRegexUpdate = regexp.MustCompile(`^[0-9Xx-]{10,17}$`)

const maxTotalStockUpdate = 99999

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
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.NotFound("book not found")
		}
		return nil, fmt.Errorf("failed to get book for update: %w", err)
	}

	if input.ISBN != "" && input.ISBN != book.ISBN {
		if !isbnRegexUpdate.MatchString(input.ISBN) {
			return nil, apperror.BadRequest("invalid ISBN format")
		}
		existing, err := uc.bookRepo.FindByISBN(ctx, input.ISBN)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("failed to check existing book: %w", err)
		}
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
		if *input.TotalStock < 0 {
			return nil, apperror.BadRequest("total stock must be non-negative")
		}
		if *input.TotalStock > maxTotalStockUpdate {
			return nil, apperror.BadRequestf("total stock must not exceed %d", maxTotalStockUpdate)
		}
		if *input.TotalStock < book.AvailableStock {
			return nil, apperror.Conflict("cannot reduce total stock below available stock; return borrowed copies first")
		}
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
