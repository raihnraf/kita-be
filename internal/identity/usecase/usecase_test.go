package usecase_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	jwtsvc "kita-be/internal/auth/jwt"
	pwdsvc "kita-be/internal/auth/password"
	domain "kita-be/internal/identity/domain"
	"kita-be/internal/identity/usecase"
)

type fakeUserRepo struct {
	users map[string]*domain.User
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{users: make(map[string]*domain.User)}
}

func (r *fakeUserRepo) Create(ctx context.Context, user *domain.User) error {
	r.users[user.Email] = user
	r.users[user.ID] = user
	return nil
}

func (r *fakeUserRepo) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	u, ok := r.users[email]
	if !ok {
		return nil, fmt.Errorf("user not found")
	}
	return u, nil
}

func (r *fakeUserRepo) FindByID(ctx context.Context, id string) (*domain.User, error) {
	u, ok := r.users[id]
	if !ok {
		return nil, fmt.Errorf("user not found")
	}
	return u, nil
}

type fakeRefreshTokenRepo struct {
	tokens map[string]*domain.RefreshToken
}

func newFakeRefreshTokenRepo() *fakeRefreshTokenRepo {
	return &fakeRefreshTokenRepo{tokens: make(map[string]*domain.RefreshToken)}
}

func (r *fakeRefreshTokenRepo) Create(ctx context.Context, token *domain.RefreshToken) error {
	r.tokens[token.TokenHash] = token
	return nil
}

func (r *fakeRefreshTokenRepo) FindByTokenHash(ctx context.Context, hash string) (*domain.RefreshToken, error) {
	t, ok := r.tokens[hash]
	if !ok {
		return nil, fmt.Errorf("token not found")
	}
	return t, nil
}

func (r *fakeRefreshTokenRepo) RevokeByID(ctx context.Context, id string) error {
	for _, t := range r.tokens {
		if t.ID == id {
			t.Revoke()
			return nil
		}
	}
	return nil
}

func (r *fakeRefreshTokenRepo) Rotate(ctx context.Context, oldTokenID string, newToken *domain.RefreshToken) error {
	if err := r.RevokeByID(ctx, oldTokenID); err != nil {
		return err
	}
	return r.Create(ctx, newToken)
}

func (r *fakeRefreshTokenRepo) RevokeByUserID(ctx context.Context, userID string) error {
	for _, t := range r.tokens {
		if t.UserID == userID && !t.IsRevoked() {
			t.Revoke()
		}
	}
	return nil
}

func newTestJWTService() *jwtsvc.Service {
	return jwtsvc.NewService("test-secret", 15*time.Minute, 7*24*time.Hour)
}

func TestRegisterSuccess(t *testing.T) {
	userRepo := newFakeUserRepo()
	pwdSvc := pwdsvc.NewService()
	jwtSvc := newTestJWTService()

	uc := usecase.NewRegisterUsecase(userRepo, pwdSvc, jwtSvc)

	output, err := uc.Execute(context.Background(), usecase.RegisterInput{
		FullName: "John Doe",
		Email:    "john@example.com",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if output.User.Email != "john@example.com" {
		t.Errorf("expected email john@example.com, got %s", output.User.Email)
	}
	if output.AccessToken == "" {
		t.Fatal("expected non-empty access token")
	}
}

func TestRegisterDuplicateEmail(t *testing.T) {
	userRepo := newFakeUserRepo()
	pwdSvc := pwdsvc.NewService()
	jwtSvc := newTestJWTService()

	uc := usecase.NewRegisterUsecase(userRepo, pwdSvc, jwtSvc)

	if _, err := uc.Execute(context.Background(), usecase.RegisterInput{
		FullName: "John Doe",
		Email:    "john@example.com",
		Password: "password123",
	}); err != nil {
		t.Fatalf("expected first registration to succeed, got: %v", err)
	}

	_, err := uc.Execute(context.Background(), usecase.RegisterInput{
		FullName: "Jane Doe",
		Email:    "john@example.com",
		Password: "password456",
	})
	if err == nil {
		t.Fatal("expected error for duplicate email")
	}
}

func TestLoginSuccess(t *testing.T) {
	userRepo := newFakeUserRepo()
	refreshTokenRepo := newFakeRefreshTokenRepo()
	pwdSvc := pwdsvc.NewService()
	jwtSvc := newTestJWTService()

	registerUC := usecase.NewRegisterUsecase(userRepo, pwdSvc, jwtSvc)
	if _, err := registerUC.Execute(context.Background(), usecase.RegisterInput{
		FullName: "John Doe",
		Email:    "john@example.com",
		Password: "password123",
	}); err != nil {
		t.Fatalf("expected registration to succeed, got: %v", err)
	}

	loginUC := usecase.NewLoginUsecase(userRepo, refreshTokenRepo, pwdSvc, jwtSvc)
	output, err := loginUC.Execute(context.Background(), usecase.LoginInput{
		Email:    "john@example.com",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if output.AccessToken == "" {
		t.Fatal("expected non-empty access token")
	}
	if output.RefreshToken == "" {
		t.Fatal("expected non-empty refresh token")
	}
	if output.TokenType != "Bearer" {
		t.Errorf("expected token type Bearer, got %s", output.TokenType)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	userRepo := newFakeUserRepo()
	refreshTokenRepo := newFakeRefreshTokenRepo()
	pwdSvc := pwdsvc.NewService()
	jwtSvc := newTestJWTService()

	registerUC := usecase.NewRegisterUsecase(userRepo, pwdSvc, jwtSvc)
	if _, err := registerUC.Execute(context.Background(), usecase.RegisterInput{
		FullName: "John Doe",
		Email:    "john@example.com",
		Password: "password123",
	}); err != nil {
		t.Fatalf("expected registration to succeed, got: %v", err)
	}

	loginUC := usecase.NewLoginUsecase(userRepo, refreshTokenRepo, pwdSvc, jwtSvc)
	_, err := loginUC.Execute(context.Background(), usecase.LoginInput{
		Email:    "john@example.com",
		Password: "wrong-password",
	})
	if err == nil {
		t.Fatal("expected error for wrong password")
	}
}

func TestLoginNonexistentUser(t *testing.T) {
	userRepo := newFakeUserRepo()
	refreshTokenRepo := newFakeRefreshTokenRepo()
	pwdSvc := pwdsvc.NewService()
	jwtSvc := newTestJWTService()

	loginUC := usecase.NewLoginUsecase(userRepo, refreshTokenRepo, pwdSvc, jwtSvc)
	_, err := loginUC.Execute(context.Background(), usecase.LoginInput{
		Email:    "nonexistent@example.com",
		Password: "password",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent user")
	}
}

func TestRefreshTokenSuccess(t *testing.T) {
	userRepo := newFakeUserRepo()
	refreshTokenRepo := newFakeRefreshTokenRepo()
	pwdSvc := pwdsvc.NewService()
	jwtSvc := newTestJWTService()

	registerUC := usecase.NewRegisterUsecase(userRepo, pwdSvc, jwtSvc)
	if _, err := registerUC.Execute(context.Background(), usecase.RegisterInput{
		FullName: "John Doe",
		Email:    "john@example.com",
		Password: "password123",
	}); err != nil {
		t.Fatalf("expected registration to succeed, got: %v", err)
	}

	loginUC := usecase.NewLoginUsecase(userRepo, refreshTokenRepo, pwdSvc, jwtSvc)
	loginOutput, _ := loginUC.Execute(context.Background(), usecase.LoginInput{
		Email:    "john@example.com",
		Password: "password123",
	})

	refreshUC := usecase.NewRefreshUsecase(userRepo, refreshTokenRepo, jwtSvc)
	output, err := refreshUC.Execute(context.Background(), usecase.RefreshInput{
		RefreshToken: loginOutput.RefreshToken,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if output.AccessToken == "" {
		t.Fatal("expected non-empty access token")
	}
	if output.RefreshToken == "" {
		t.Fatal("expected non-empty refresh token")
	}

	if output.RefreshToken == loginOutput.RefreshToken {
		t.Fatal("expected new refresh token, got same as old")
	}
}

func TestRefreshTokenReuse(t *testing.T) {
	userRepo := newFakeUserRepo()
	refreshTokenRepo := newFakeRefreshTokenRepo()
	pwdSvc := pwdsvc.NewService()
	jwtSvc := newTestJWTService()

	registerUC := usecase.NewRegisterUsecase(userRepo, pwdSvc, jwtSvc)
	if _, err := registerUC.Execute(context.Background(), usecase.RegisterInput{
		FullName: "John Doe",
		Email:    "john@example.com",
		Password: "password123",
	}); err != nil {
		t.Fatalf("expected registration to succeed, got: %v", err)
	}

	loginUC := usecase.NewLoginUsecase(userRepo, refreshTokenRepo, pwdSvc, jwtSvc)
	loginOutput, _ := loginUC.Execute(context.Background(), usecase.LoginInput{
		Email:    "john@example.com",
		Password: "password123",
	})

	refreshUC := usecase.NewRefreshUsecase(userRepo, refreshTokenRepo, jwtSvc)
	if _, err := refreshUC.Execute(context.Background(), usecase.RefreshInput{
		RefreshToken: loginOutput.RefreshToken,
	}); err != nil {
		t.Fatalf("expected first refresh to succeed, got: %v", err)
	}

	_, err := refreshUC.Execute(context.Background(), usecase.RefreshInput{
		RefreshToken: loginOutput.RefreshToken,
	})
	if err == nil {
		t.Fatal("expected error when reusing old refresh token")
	}
}

func TestLogout(t *testing.T) {
	userRepo := newFakeUserRepo()
	refreshTokenRepo := newFakeRefreshTokenRepo()
	pwdSvc := pwdsvc.NewService()
	jwtSvc := newTestJWTService()

	registerUC := usecase.NewRegisterUsecase(userRepo, pwdSvc, jwtSvc)
	if _, err := registerUC.Execute(context.Background(), usecase.RegisterInput{
		FullName: "John Doe",
		Email:    "john@example.com",
		Password: "password123",
	}); err != nil {
		t.Fatalf("expected registration to succeed, got: %v", err)
	}

	loginUC := usecase.NewLoginUsecase(userRepo, refreshTokenRepo, pwdSvc, jwtSvc)
	loginOutput, _ := loginUC.Execute(context.Background(), usecase.LoginInput{
		Email:    "john@example.com",
		Password: "password123",
	})

	logoutUC := usecase.NewLogoutUsecase(userRepo, refreshTokenRepo)
	err := logoutUC.Execute(context.Background(), usecase.LogoutInput{
		RefreshToken: loginOutput.RefreshToken,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	refreshUC := usecase.NewRefreshUsecase(userRepo, refreshTokenRepo, jwtSvc)
	_, err = refreshUC.Execute(context.Background(), usecase.RefreshInput{
		RefreshToken: loginOutput.RefreshToken,
	})
	if err == nil {
		t.Fatal("expected error when using revoked refresh token")
	}
}

func TestLogoutRevokesOnlySubmittedRefreshToken(t *testing.T) {
	userRepo := newFakeUserRepo()
	refreshTokenRepo := newFakeRefreshTokenRepo()
	pwdSvc := pwdsvc.NewService()
	jwtSvc := newTestJWTService()

	registerUC := usecase.NewRegisterUsecase(userRepo, pwdSvc, jwtSvc)
	_, _ = registerUC.Execute(context.Background(), usecase.RegisterInput{
		FullName: "John Doe",
		Email:    "john@example.com",
		Password: "password123",
	})

	loginUC := usecase.NewLoginUsecase(userRepo, refreshTokenRepo, pwdSvc, jwtSvc)
	firstLogin, _ := loginUC.Execute(context.Background(), usecase.LoginInput{
		Email:    "john@example.com",
		Password: "password123",
	})
	secondLogin, _ := loginUC.Execute(context.Background(), usecase.LoginInput{
		Email:    "john@example.com",
		Password: "password123",
	})

	logoutUC := usecase.NewLogoutUsecase(userRepo, refreshTokenRepo)
	if err := logoutUC.Execute(context.Background(), usecase.LogoutInput{RefreshToken: firstLogin.RefreshToken}); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	refreshUC := usecase.NewRefreshUsecase(userRepo, refreshTokenRepo, jwtSvc)
	if _, err := refreshUC.Execute(context.Background(), usecase.RefreshInput{RefreshToken: firstLogin.RefreshToken}); err == nil {
		t.Fatal("expected logged-out refresh token to be revoked")
	}
	if _, err := refreshUC.Execute(context.Background(), usecase.RefreshInput{RefreshToken: secondLogin.RefreshToken}); err != nil {
		t.Fatalf("expected other session refresh token to remain valid, got: %v", err)
	}
}

func TestProfileSuccess(t *testing.T) {
	userRepo := newFakeUserRepo()
	pwdSvc := pwdsvc.NewService()
	jwtSvc := newTestJWTService()

	registerUC := usecase.NewRegisterUsecase(userRepo, pwdSvc, jwtSvc)
	output, _ := registerUC.Execute(context.Background(), usecase.RegisterInput{
		FullName: "John Doe",
		Email:    "john@example.com",
		Password: "password123",
	})

	profileUC := usecase.NewProfileUsecase(userRepo)
	profileOutput, err := profileUC.Execute(context.Background(), output.User.ID)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if profileOutput.User.Email != "john@example.com" {
		t.Errorf("expected email john@example.com, got %s", profileOutput.User.Email)
	}
}
