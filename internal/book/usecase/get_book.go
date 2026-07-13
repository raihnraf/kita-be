package usecase

import (
	"context"

	domain "kita-be/internal/book/domain"
	"kita-be/internal/platform/apperror"
)

type GetBookUsecase struct {
	bookRepo BookRepository
}

func NewGetBookUsecase(bookRepo BookRepository) *GetBookUsecase {
	return &GetBookUsecase{bookRepo: bookRepo}
}

func (uc *GetBookUsecase) Execute(ctx context.Context, id string) (*domain.Book, error) {
	book, err := uc.bookRepo.FindByID(ctx, id)
	if err != nil {
		return nil, apperror.NotFound("book not found")
	}
	return book, nil
}
