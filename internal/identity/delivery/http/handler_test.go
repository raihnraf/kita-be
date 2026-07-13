package http_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	jwtsvc "kita-be/internal/auth/jwt"
	pwdsvc "kita-be/internal/auth/password"
	identityhttp "kita-be/internal/identity/delivery/http"
	domain "kita-be/internal/identity/domain"
	"kita-be/internal/identity/usecase"
)

func TestIdentityHandlerRegisterRejectsInvalidEmail(t *testing.T) {
	app, _ := newIdentityTestApp("")

	req := httptest.NewRequest(fiber.MethodPost, "/auth/register", strings.NewReader(`{"full_name":"John Doe","email":"bad-email","password":"password123"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", fiber.StatusBadRequest, resp.StatusCode)
	}
}

func TestIdentityHandlerRegisterSuccess(t *testing.T) {
	app, _ := newIdentityTestApp("")

	req := httptest.NewRequest(fiber.MethodPost, "/auth/register", strings.NewReader(`{"full_name":"John Doe","email":"john@example.com","password":"password123"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("expected status %d, got %d", fiber.StatusCreated, resp.StatusCode)
	}

	var body struct {
		Success bool `json:"success"`
		Data    struct {
			User struct {
				Email string `json:"email"`
			} `json:"user"`
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			TokenType    string `json:"token_type"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !body.Success || body.Data.User.Email != "john@example.com" || body.Data.AccessToken == "" || body.Data.RefreshToken == "" || body.Data.TokenType != "Bearer" {
		t.Fatalf("unexpected response body: %+v", body)
	}
}

func TestIdentityHandlerTokenPasswordGrantSuccess(t *testing.T) {
	app, deps := newIdentityTestApp("")
	deps.seedUser(t, "Jane Doe", "jane@example.com", "password123")

	req := httptest.NewRequest(fiber.MethodPost, "/auth/token", strings.NewReader("grant_type=password&email=jane@example.com&password=password123"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected status %d, got %d", fiber.StatusOK, resp.StatusCode)
	}

	var body struct {
		Success bool `json:"success"`
		Data    struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			TokenType    string `json:"token_type"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !body.Success || body.Data.AccessToken == "" || body.Data.RefreshToken == "" || body.Data.TokenType != "Bearer" {
		t.Fatalf("unexpected response body: %+v", body)
	}
}

func TestIdentityHandlerProfileRequiresUser(t *testing.T) {
	app, _ := newIdentityTestApp("")

	req := httptest.NewRequest(fiber.MethodGet, "/users/me", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", fiber.StatusUnauthorized, resp.StatusCode)
	}
}

type identityTestDeps struct {
	userRepo         *handlerFakeUserRepo
	refreshTokenRepo *handlerFakeRefreshTokenRepo
	pwdSvc           *pwdsvc.Service
}

func newIdentityTestApp(userID string) (*fiber.App, *identityTestDeps) {
	deps := &identityTestDeps{
		userRepo:         newHandlerFakeUserRepo(),
		refreshTokenRepo: newHandlerFakeRefreshTokenRepo(),
		pwdSvc:           pwdsvc.NewService(),
	}
	jwtSvc := jwtsvc.NewService("test-secret", 15*time.Minute, 7*24*time.Hour)
	handler := identityhttp.NewIdentityHandler(
		usecase.NewRegisterUsecase(deps.userRepo, deps.refreshTokenRepo, deps.pwdSvc, jwtSvc),
		usecase.NewLoginUsecase(deps.userRepo, deps.refreshTokenRepo, deps.pwdSvc, jwtSvc),
		usecase.NewRefreshUsecase(deps.userRepo, deps.refreshTokenRepo, jwtSvc),
		usecase.NewLogoutUsecase(deps.userRepo, deps.refreshTokenRepo),
		usecase.NewProfileUsecase(deps.userRepo),
	)

	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		if userID != "" {
			c.Locals("user_id", userID)
		}
		return c.Next()
	})
	app.Post("/auth/register", handler.Register)
	app.Post("/auth/token", handler.Token)
	app.Get("/users/me", handler.Profile)
	return app, deps
}

func (d *identityTestDeps) seedUser(t *testing.T, fullName, email, password string) *domain.User {
	t.Helper()
	hash, err := d.pwdSvc.Hash(password)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	user := domain.NewUser("user-1", fullName, email, hash)
	if err := d.userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("failed to seed user: %v", err)
	}
	return user
}

type handlerFakeUserRepo struct {
	users map[string]*domain.User
}

func newHandlerFakeUserRepo() *handlerFakeUserRepo {
	return &handlerFakeUserRepo{users: make(map[string]*domain.User)}
}

func (r *handlerFakeUserRepo) Create(ctx context.Context, user *domain.User) error {
	r.users[user.ID] = user
	r.users[user.Email] = user
	return nil
}

func (r *handlerFakeUserRepo) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	user, ok := r.users[email]
	if !ok {
		return nil, fmt.Errorf("user not found")
	}
	return user, nil
}

func (r *handlerFakeUserRepo) FindByID(ctx context.Context, id string) (*domain.User, error) {
	user, ok := r.users[id]
	if !ok {
		return nil, fmt.Errorf("user not found")
	}
	return user, nil
}

type handlerFakeRefreshTokenRepo struct {
	tokens map[string]*domain.RefreshToken
}

func newHandlerFakeRefreshTokenRepo() *handlerFakeRefreshTokenRepo {
	return &handlerFakeRefreshTokenRepo{tokens: make(map[string]*domain.RefreshToken)}
}

func (r *handlerFakeRefreshTokenRepo) Create(ctx context.Context, token *domain.RefreshToken) error {
	r.tokens[token.TokenHash] = token
	return nil
}

func (r *handlerFakeRefreshTokenRepo) FindByTokenHash(ctx context.Context, hash string) (*domain.RefreshToken, error) {
	token, ok := r.tokens[hash]
	if !ok {
		return nil, fmt.Errorf("token not found")
	}
	return token, nil
}

func (r *handlerFakeRefreshTokenRepo) RevokeByID(ctx context.Context, id string) error {
	for _, token := range r.tokens {
		if token.ID == id {
			token.Revoke()
			return nil
		}
	}
	return nil
}

func (r *handlerFakeRefreshTokenRepo) Rotate(ctx context.Context, oldTokenID string, newToken *domain.RefreshToken) error {
	if err := r.RevokeByID(ctx, oldTokenID); err != nil {
		return err
	}
	return r.Create(ctx, newToken)
}

func (r *handlerFakeRefreshTokenRepo) RevokeByUserID(ctx context.Context, userID string) error {
	for _, token := range r.tokens {
		if token.UserID == userID && !token.IsRevoked() {
			token.Revoke()
		}
	}
	return nil
}
