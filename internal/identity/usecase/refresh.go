package usecase

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	jwtsvc "kita-be/internal/auth/jwt"
	domain "kita-be/internal/identity/domain"
	"kita-be/internal/platform/apperror"
)

type RefreshUsecase struct {
	userRepo         UserRepository
	refreshTokenRepo RefreshTokenRepository
	jwtSvc           *jwtsvc.Service
}

func NewRefreshUsecase(userRepo UserRepository, refreshTokenRepo RefreshTokenRepository, jwtSvc *jwtsvc.Service) *RefreshUsecase {
	return &RefreshUsecase{
		userRepo:         userRepo,
		refreshTokenRepo: refreshTokenRepo,
		jwtSvc:           jwtSvc,
	}
}

type RefreshInput struct {
	RefreshToken string
}

type RefreshOutput struct {
	AccessToken  string
	RefreshToken string
	TokenType    string
	ExpiresIn    int64
}

func (uc *RefreshUsecase) Execute(ctx context.Context, input RefreshInput) (*RefreshOutput, error) {
	tokenHash := hashToken(input.RefreshToken)

	storedToken, err := uc.refreshTokenRepo.FindByTokenHash(ctx, tokenHash)
	if err != nil {
		return nil, apperror.Unauthorized("invalid refresh token")
	}

	if !storedToken.IsValid() {
		return nil, apperror.Unauthorized("refresh token expired or revoked")
	}

	user, err := uc.userRepo.FindByID(ctx, storedToken.UserID)
	if err != nil {
		return nil, apperror.Unauthorized("user not found")
	}

	accessToken, err := uc.jwtSvc.GenerateAccessToken(user.ID, user.Email, string(user.Role))
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	newRefreshTokenStr, expiresAt, err := uc.jwtSvc.GenerateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	newTokenHash := hashToken(newRefreshTokenStr)
	newRefreshToken := domain.NewRefreshToken(uuid.New().String(), user.ID, newTokenHash, expiresAt)

	if err := uc.refreshTokenRepo.Rotate(ctx, storedToken.ID, newRefreshToken); err != nil {
		return nil, fmt.Errorf("failed to rotate refresh token: %w", err)
	}

	return &RefreshOutput{
		AccessToken:  accessToken,
		RefreshToken: newRefreshTokenStr,
		TokenType:    "Bearer",
		ExpiresIn:    int64(uc.jwtSvc.Expiry().Seconds()),
	}, nil
}
