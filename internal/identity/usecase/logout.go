package usecase

import (
	"context"
	"fmt"

	domain "kita-be/internal/identity/domain"
	"kita-be/internal/platform/apperror"
)

type LogoutUsecase struct {
	userRepo         UserRepository
	refreshTokenRepo RefreshTokenRepository
}

func NewLogoutUsecase(userRepo UserRepository, refreshTokenRepo RefreshTokenRepository) *LogoutUsecase {
	return &LogoutUsecase{
		userRepo:         userRepo,
		refreshTokenRepo: refreshTokenRepo,
	}
}

type LogoutInput struct {
	RefreshToken string
}

func (uc *LogoutUsecase) Execute(ctx context.Context, input LogoutInput) error {
	tokenHash := hashToken(input.RefreshToken)

	storedToken, err := uc.refreshTokenRepo.FindByTokenHash(ctx, tokenHash)
	if err != nil {
		return apperror.Unauthorized("invalid refresh token")
	}

	if err := uc.refreshTokenRepo.RevokeByID(ctx, storedToken.ID); err != nil {
		return fmt.Errorf("failed to revoke refresh token: %w", err)
	}

	storedToken.Revoke()
	_ = domain.RefreshToken{}
	return nil
}
