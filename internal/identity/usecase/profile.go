package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	domain "kita-be/internal/identity/domain"
	"kita-be/internal/platform/apperror"
)

type ProfileUsecase struct {
	userRepo UserRepository
}

func NewProfileUsecase(userRepo UserRepository) *ProfileUsecase {
	return &ProfileUsecase{userRepo: userRepo}
}

type ProfileOutput struct {
	User *domain.User
}

func (uc *ProfileUsecase) Execute(ctx context.Context, userID string) (*ProfileOutput, error) {
	user, err := uc.userRepo.FindByID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.NotFound("user not found")
		}
		return nil, fmt.Errorf("failed to get user profile: %w", err)
	}

	return &ProfileOutput{User: user}, nil
}
