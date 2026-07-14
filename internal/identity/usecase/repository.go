package usecase

import (
	"context"
	"time"

	domain "kita-be/internal/identity/domain"
)

type UserRepository interface {
	Create(ctx context.Context, user *domain.User) error
	FindByEmail(ctx context.Context, email string) (*domain.User, error)
	FindByID(ctx context.Context, id string) (*domain.User, error)
}

type RefreshTokenRepository interface {
	Create(ctx context.Context, token *domain.RefreshToken) error
	FindByTokenHash(ctx context.Context, hash string) (*domain.RefreshToken, error)
	RevokeByID(ctx context.Context, id string) error
	Rotate(ctx context.Context, oldTokenID string, newToken *domain.RefreshToken) error
	RevokeByUserID(ctx context.Context, userID string) error
}

type PasswordService interface {
	Hash(password string) (string, error)
	Verify(password, hash string) bool
}

type TokenService interface {
	GenerateAccessToken(userID, email, role string) (string, error)
	GenerateRefreshToken() (string, time.Time, error)
	Expiry() time.Duration
}
