package usecase

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	domain "kita-be/internal/identity/domain"
	"kita-be/internal/platform/apperror"
)

type RegisterUsecase struct {
	userRepo         UserRepository
	refreshTokenRepo RefreshTokenRepository
	pwdSvc           PasswordService
	jwtSvc           TokenService
}

func NewRegisterUsecase(userRepo UserRepository, refreshTokenRepo RefreshTokenRepository, pwdSvc PasswordService, jwtSvc TokenService) *RegisterUsecase {
	return &RegisterUsecase{userRepo: userRepo, refreshTokenRepo: refreshTokenRepo, pwdSvc: pwdSvc, jwtSvc: jwtSvc}
}

func (uc *RegisterUsecase) Expiry() time.Duration {
	return uc.jwtSvc.Expiry()
}

type RegisterInput struct {
	FullName string
	Email    string
	Password string
}

type RegisterOutput struct {
	User         *domain.User
	AccessToken  string
	RefreshToken string
}

func (uc *RegisterUsecase) Execute(ctx context.Context, input RegisterInput) (*RegisterOutput, error) {
	email := strings.ToLower(strings.TrimSpace(input.Email))

	existing, err := uc.userRepo.FindByEmail(ctx, email)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("failed to check existing user: %w", err)
	}
	if existing != nil {
		return nil, apperror.Conflict("email already registered")
	}

	hashedPassword, err := uc.pwdSvc.Hash(input.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	user := domain.NewUser(uuid.NewString(), input.FullName, email, hashedPassword)

	if err := uc.userRepo.Create(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	accessToken, err := uc.jwtSvc.GenerateAccessToken(user.ID, user.Email, string(user.Role))
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	refreshTokenStr, expiresAt, err := uc.jwtSvc.GenerateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	refreshToken := domain.NewRefreshToken(uuid.NewString(), user.ID, hashToken(refreshTokenStr), expiresAt)
	if err := uc.refreshTokenRepo.Create(ctx, refreshToken); err != nil {
		return nil, fmt.Errorf("failed to store refresh token: %w", err)
	}

	return &RegisterOutput{
		User:         user,
		AccessToken:  accessToken,
		RefreshToken: refreshTokenStr,
	}, nil
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", h)
}
