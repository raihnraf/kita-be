package usecase

import (
	"context"

	domain "kita-be/internal/book/domain"
	"kita-be/internal/platform/pagination"
)

// Ambil banyak buku (paginated)
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
	input.Page, input.PerPage = pagination.Normalize(input.Page, input.PerPage)

	books, total, err := uc.bookRepo.List(ctx, input)
	if err != nil {
		return nil, err
	}

	return &ListBooksOutput{Books: books, Total: total}, nil
}
