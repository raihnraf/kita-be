package http

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"kita-be/internal/identity/usecase"
	"kita-be/internal/platform/response"
	"kita-be/internal/platform/validation"
)

type IdentityHandler struct {
	register *usecase.RegisterUsecase
	login    *usecase.LoginUsecase
	refresh  *usecase.RefreshUsecase
	logout   *usecase.LogoutUsecase
	profile  *usecase.ProfileUsecase
}

func NewIdentityHandler(
	register *usecase.RegisterUsecase,
	login *usecase.LoginUsecase,
	refresh *usecase.RefreshUsecase,
	logout *usecase.LogoutUsecase,
	profile *usecase.ProfileUsecase,
) *IdentityHandler {
	return &IdentityHandler{
		register: register,
		login:    login,
		refresh:  refresh,
		logout:   logout,
		profile:  profile,
	}
}

func (h *IdentityHandler) Register(c *fiber.Ctx) error {
	var req RegisterRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "INVALID_REQUEST", "invalid request body")
	}

	if req.FullName == "" {
		return response.BadRequest(c, "VALIDATION_ERROR", "full_name is required")
	}
	if req.Email == "" {
		return response.BadRequest(c, "VALIDATION_ERROR", "email is required")
	}
	if !validation.Email(req.Email) {
		return response.BadRequest(c, "VALIDATION_ERROR", "email must be valid")
	}
	if req.Password == "" || len(req.Password) < 6 {
		return response.BadRequest(c, "VALIDATION_ERROR", "password must be at least 6 characters")
	}

	output, err := h.register.Execute(c.UserContext(), usecase.RegisterInput{
		FullName: req.FullName,
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		return err
	}

	return response.Created(c, RegisterResponse{
		User: UserResponse{
			ID:        output.User.ID,
			FullName:  output.User.FullName,
			Email:     output.User.Email,
			Role:      string(output.User.Role),
			Status:    string(output.User.Status),
			CreatedAt: output.User.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		},
		AccessToken:  output.AccessToken,
		RefreshToken: output.RefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(h.register.Expiry().Seconds()),
	})
}

func (h *IdentityHandler) Token(c *fiber.Ctx) error {
	var req TokenRequest
	if strings.HasPrefix(c.Get("Content-Type"), "application/json") {
		_ = c.BodyParser(&req)
	}

	grantType := req.GrantType
	if grantType == "" {
		grantType = c.FormValue("grant_type")
	}
	if grantType == "" {
		grantType = c.Query("grant_type")
	}

	switch grantType {
	case "password":
		return h.handlePasswordGrant(c, req)
	case "refresh_token":
		return h.handleRefreshGrant(c, req)
	default:
		return response.BadRequest(c, "UNSUPPORTED_GRANT_TYPE", "grant_type must be password or refresh_token")
	}
}

func (h *IdentityHandler) handlePasswordGrant(c *fiber.Ctx, req TokenRequest) error {
	email := req.Email
	if email == "" {
		email = c.FormValue("email")
	}
	password := req.Password
	if password == "" {
		password = c.FormValue("password")
	}

	if email == "" || password == "" {
		return response.BadRequest(c, "VALIDATION_ERROR", "email and password are required")
	}
	if !validation.Email(email) {
		return response.BadRequest(c, "VALIDATION_ERROR", "email must be valid")
	}

	output, err := h.login.Execute(c.UserContext(), usecase.LoginInput{
		Email:    email,
		Password: password,
	})
	if err != nil {
		return err
	}

	return response.OK(c, TokenResponse{
		AccessToken:  output.AccessToken,
		RefreshToken: output.RefreshToken,
		TokenType:    output.TokenType,
		ExpiresIn:    output.ExpiresIn,
	})
}

func (h *IdentityHandler) handleRefreshGrant(c *fiber.Ctx, req TokenRequest) error {
	refreshToken := req.RefreshToken
	if refreshToken == "" {
		refreshToken = c.FormValue("refresh_token")
	}

	if refreshToken == "" {
		return response.BadRequest(c, "VALIDATION_ERROR", "refresh_token is required")
	}

	output, err := h.refresh.Execute(c.UserContext(), usecase.RefreshInput{
		RefreshToken: refreshToken,
	})
	if err != nil {
		return err
	}

	return response.OK(c, TokenResponse{
		AccessToken:  output.AccessToken,
		RefreshToken: output.RefreshToken,
		TokenType:    output.TokenType,
		ExpiresIn:    output.ExpiresIn,
	})
}

func (h *IdentityHandler) Logout(c *fiber.Ctx) error {
	var req LogoutRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "INVALID_REQUEST", "invalid request body")
	}

	if req.RefreshToken == "" {
		return response.BadRequest(c, "VALIDATION_ERROR", "refresh_token is required")
	}

	if err := h.logout.Execute(c.UserContext(), usecase.LogoutInput{
		RefreshToken: req.RefreshToken,
	}); err != nil {
		return err
	}

	return response.OK(c, fiber.Map{"message": "logged out successfully"})
}

func (h *IdentityHandler) Profile(c *fiber.Ctx) error {
	userID, ok := c.Locals("user_id").(string)
	if !ok {
		return response.Unauthorized(c, "UNAUTHORIZED", "invalid or missing token")
	}

	output, err := h.profile.Execute(c.UserContext(), userID)
	if err != nil {
		return err
	}

	return response.OK(c, UserResponse{
		ID:        output.User.ID,
		FullName:  output.User.FullName,
		Email:     output.User.Email,
		Role:      string(output.User.Role),
		Status:    string(output.User.Status),
		CreatedAt: output.User.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}
