package usecase

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	domain "kita-be/internal/book/domain"
	"kita-be/internal/platform/apperror"
)

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
	existing, _ := uc.bookRepo.FindByISBN(ctx, input.ISBN)
	if existing != nil {
		return nil, apperror.Conflict("book with this ISBN already exists")
	}

	book := domain.NewBook(uuid.New().String(), input.ISBN, input.Title, input.Author, input.TotalStock)
	book.Publisher = input.Publisher
	book.Category = input.Category
	book.Description = input.Description

	if err := uc.bookRepo.Create(ctx, book); err != nil {
		return nil, fmt.Errorf("failed to create book: %w", err)
	}

	return book, nil
}
