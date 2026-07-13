package usecase

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/google/uuid"

	jwtsvc "kita-be/internal/auth/jwt"
	pwdsvc "kita-be/internal/auth/password"
	domain "kita-be/internal/identity/domain"
	"kita-be/internal/platform/apperror"
)

type RegisterUsecase struct {
	userRepo UserRepository
	pwdSvc   *pwdsvc.Service
	jwtSvc   *jwtsvc.Service
}

func NewRegisterUsecase(userRepo UserRepository, pwdSvc *pwdsvc.Service, jwtSvc *jwtsvc.Service) *RegisterUsecase {
	return &RegisterUsecase{userRepo: userRepo, pwdSvc: pwdSvc, jwtSvc: jwtSvc}
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
	existing, _ := uc.userRepo.FindByEmail(ctx, input.Email)
	if existing != nil {
		return nil, apperror.Conflict("email already registered")
	}

	hashedPassword, err := uc.pwdSvc.Hash(input.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	user := domain.NewUser(uuid.New().String(), input.FullName, input.Email, hashedPassword)

	if err := uc.userRepo.Create(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	accessToken, err := uc.jwtSvc.GenerateAccessToken(user.ID, user.Email, string(user.Role))
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	return &RegisterOutput{
		User:         user,
		AccessToken:  accessToken,
		RefreshToken: "",
	}, nil
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", h)
}
