package usecase

import (
	"context"

	domain "kita-be/internal/book/domain"
)

type ListBooksUsecase struct {
	bookRepo BookRepository
}

func NewListBooksUsecase(bookRepo BookRepository) *ListBooksUsecase {
	return &ListBooksUsecase{bookRepo: bookRepo}
}

type ListBooksOutput struct {
	Books []domain.Book
	Total int64
}

func (uc *ListBooksUsecase) Execute(ctx context.Context, input ListBooksInput) (*ListBooksOutput, error) {
	if input.Page < 1 {
		input.Page = 1
	}
	if input.PerPage < 1 || input.PerPage > 100 {
		input.PerPage = 20
	}

	books, total, err := uc.bookRepo.List(ctx, input)
	if err != nil {
		return nil, err
	}

	return &ListBooksOutput{Books: books, Total: total}, nil
}
