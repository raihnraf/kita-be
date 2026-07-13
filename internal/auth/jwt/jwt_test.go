package jwt_test

import (
	"testing"
	"time"

	jwtsvc "kita-be/internal/auth/jwt"
)

func TestGenerateAndValidateAccessToken(t *testing.T) {
	svc := jwtsvc.NewService("test-secret", 15*time.Minute, 7*24*time.Hour)

	token, err := svc.GenerateAccessToken("user-1", "test@example.com", "MEMBER")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	if token == "" {
		t.Fatal("expected non-empty token")
	}

	claims, err := svc.ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("failed to validate token: %v", err)
	}

	if claims.UserID != "user-1" {
		t.Errorf("expected user_id user-1, got %s", claims.UserID)
	}
	if claims.Email != "test@example.com" {
		t.Errorf("expected email test@example.com, got %s", claims.Email)
	}
	if claims.Role != "MEMBER" {
		t.Errorf("expected role MEMBER, got %s", claims.Role)
	}
	if claims.TokenType != "access" {
		t.Errorf("expected token_type access, got %s", claims.TokenType)
	}
}

func TestValidateAccessTokenRejectsRefreshToken(t *testing.T) {
	svc := jwtsvc.NewService("test-secret", 15*time.Minute, 7*24*time.Hour)

	refreshToken, _, err := svc.GenerateRefreshToken()
	if err != nil {
		t.Fatalf("failed to generate refresh token: %v", err)
	}

	_, err = svc.ValidateAccessToken(refreshToken)
	if err == nil {
		t.Fatal("expected refresh token to be rejected as access token")
	}
}

func TestValidateInvalidToken(t *testing.T) {
	svc := jwtsvc.NewService("test-secret", 15*time.Minute, 7*24*time.Hour)

	_, err := svc.ValidateAccessToken("invalid-token")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

func TestValidateExpiredToken(t *testing.T) {
	svc := jwtsvc.NewService("test-secret", -1*time.Minute, 7*24*time.Hour)

	token, err := svc.GenerateAccessToken("user-1", "test@example.com", "MEMBER")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	_, err = svc.ValidateAccessToken(token)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestValidateTokenWrongSecret(t *testing.T) {
	svc1 := jwtsvc.NewService("secret-1", 15*time.Minute, 7*24*time.Hour)
	svc2 := jwtsvc.NewService("secret-2", 15*time.Minute, 7*24*time.Hour)

	token, err := svc1.GenerateAccessToken("user-1", "test@example.com", "MEMBER")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	_, err = svc2.ValidateAccessToken(token)
	if err == nil {
		t.Fatal("expected error when validating with wrong secret")
	}
}

func TestGenerateRefreshToken(t *testing.T) {
	svc := jwtsvc.NewService("test-secret", 15*time.Minute, 7*24*time.Hour)

	refreshToken, expiresAt, err := svc.GenerateRefreshToken()
	if err != nil {
		t.Fatalf("failed to generate refresh token: %v", err)
	}

	if refreshToken == "" {
		t.Fatal("expected non-empty refresh token")
	}
	if time.Now().After(expiresAt) {
		t.Fatal("expected expiry in the future")
	}
}
