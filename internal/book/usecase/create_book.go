package usecase

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	domain "kita-be/internal/book/domain"
	"kita-be/internal/platform/apperror"
)

var isbnRegex = regexp.MustCompile(`^[0-9Xx-]{10,17}$`)

const maxTotalStock = 99999

type CreateBookUsecase struct {
	bookRepo BookRepository
}

func NewCreateBookUsecase(bookRepo BookRepository) *CreateBookUsecase {
	return &CreateBookUsecase{bookRepo: bookRepo}
}

type CreateBookInput struct {
	ISBN        string
	Title       string
	Author      string
	Publisher   *string
	Category    *string
	Description *string
	TotalStock  int
}

func (uc *CreateBookUsecase) Execute(ctx context.Context, input CreateBookInput) (*domain.Book, error) {
	if input.ISBN != "" && !isbnRegex.MatchString(input.ISBN) {
		return nil, apperror.BadRequest("invalid ISBN format")
	}
	if input.TotalStock < 0 {
		return nil, apperror.BadRequest("total stock must be non-negative")
	}
	if input.TotalStock > maxTotalStock {
		return nil, apperror.BadRequestf("total stock must not exceed %d", maxTotalStock)
	}

	existing, err := uc.bookRepo.FindByISBN(ctx, input.ISBN)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("failed to check existing book: %w", err)
	}
	if existing != nil {
		return nil, apperror.Conflict("book with this ISBN already exists")
	}

	book := domain.NewBook(uuid.NewString(), input.ISBN, input.Title, input.Author, input.TotalStock)
	book.Publisher = input.Publisher
	book.Category = input.Category
	book.Description = input.Description

	if err := uc.bookRepo.Create(ctx, book); err != nil {
		return nil, fmt.Errorf("failed to create book: %w", err)
	}

	return book, nil
}
