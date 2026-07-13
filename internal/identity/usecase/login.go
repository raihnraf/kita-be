package usecase

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	jwtsvc "kita-be/internal/auth/jwt"
	pwdsvc "kita-be/internal/auth/password"
	domain "kita-be/internal/identity/domain"
	"kita-be/internal/platform/apperror"
)

type LoginUsecase struct {
	userRepo         UserRepository
	refreshTokenRepo RefreshTokenRepository
	pwdSvc           *pwdsvc.Service
	jwtSvc           *jwtsvc.Service
}

func NewLoginUsecase(
	userRepo UserRepository,
	refreshTokenRepo RefreshTokenRepository,
	pwdSvc *pwdsvc.Service,
	jwtSvc *jwtsvc.Service,
) *LoginUsecase {
	return &LoginUsecase{
		userRepo:         userRepo,
		refreshTokenRepo: refreshTokenRepo,
		pwdSvc:           pwdSvc,
		jwtSvc:           jwtSvc,
	}
}

type LoginInput struct {
	Email    string
	Password string
}

type LoginOutput struct {
	AccessToken  string
	RefreshToken string
	TokenType    string
	ExpiresIn    int64
}

func (uc *LoginUsecase) Execute(ctx context.Context, input LoginInput) (*LoginOutput, error) {
	user, err := uc.userRepo.FindByEmail(ctx, input.Email)
	if err != nil {
		return nil, apperror.Unauthorized("invalid email or password")
	}

	if !uc.pwdSvc.Verify(input.Password, user.PasswordHash) {
		return nil, apperror.Unauthorized("invalid email or password")
	}

	accessToken, err := uc.jwtSvc.GenerateAccessToken(user.ID, user.Email, string(user.Role))
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	refreshTokenStr, expiresAt, err := uc.jwtSvc.GenerateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	tokenHash := hashToken(refreshTokenStr)
	refreshToken := domain.NewRefreshToken(uuid.New().String(), user.ID, tokenHash, expiresAt)

	if err := uc.refreshTokenRepo.Create(ctx, refreshToken); err != nil {
		return nil, fmt.Errorf("failed to store refresh token: %w", err)
	}

	return &LoginOutput{
		AccessToken:  accessToken,
		RefreshToken: refreshTokenStr,
		TokenType:    "Bearer",
		ExpiresIn:    int64(uc.jwtSvc.Expiry().Seconds()),
	}, nil
}
