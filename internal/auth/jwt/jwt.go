package jwt

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Service struct {
	secret    string
	expiry    time.Duration
	refExpiry time.Duration
}

func NewService(secret string, expiry, refExpiry time.Duration) *Service {
	return &Service{
		secret:    secret,
		expiry:    expiry,
		refExpiry: refExpiry,
	}
}

func (s *Service) Expiry() time.Duration {
	return s.expiry
}

type TokenClaims struct {
	UserID    string `json:"user_id"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	TokenType string `json:"token_type"`
	jwt.RegisteredClaims
}

type RefreshClaims struct {
	TokenType string `json:"token_type"`
	jwt.RegisteredClaims
}

func (s *Service) GenerateAccessToken(userID, email, role string) (string, error) {
	now := time.Now()
	claims := TokenClaims{
		UserID:    userID,
		Email:     email,
		Role:      role,
		TokenType: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.expiry)),
			Issuer:    "kita-identity-service",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.secret))
}

func (s *Service) GenerateRefreshToken() (string, time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(s.refExpiry)

	claims := RefreshClaims{
		TokenType: "refresh",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        fmt.Sprintf("%d", now.UnixNano()),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			Issuer:    "kita-identity-service",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(s.secret))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	return tokenStr, expiresAt, nil
}

func (s *Service) ValidateAccessToken(tokenStr string) (*TokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &TokenClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(s.secret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(*TokenClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}
	if claims.TokenType != "access" {
		return nil, fmt.Errorf("invalid token type")
	}
	if claims.UserID == "" {
		return nil, fmt.Errorf("missing user id")
	}
	if claims.Issuer != "kita-identity-service" {
		return nil, fmt.Errorf("invalid issuer")
	}

	return claims, nil
}
