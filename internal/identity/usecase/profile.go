package usecase

import (
	"context"

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
		return nil, apperror.NotFound("user not found")
	}

	return &ProfileOutput{User: user}, nil
}
