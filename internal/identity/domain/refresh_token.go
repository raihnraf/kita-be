package domain

import (
	"time"
)

type RefreshToken struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

func NewRefreshToken(id, userID, tokenHash string, expiresAt time.Time) *RefreshToken {
	now := time.Now()
	return &RefreshToken{
		ID:        id,
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func (rt *RefreshToken) IsExpired() bool {
	return time.Now().After(rt.ExpiresAt)
}

func (rt *RefreshToken) IsRevoked() bool {
	return rt.RevokedAt != nil
}

func (rt *RefreshToken) IsValid() bool {
	return !rt.IsExpired() && !rt.IsRevoked()
}

func (rt *RefreshToken) Revoke() {
	now := time.Now()
	rt.RevokedAt = &now
	rt.UpdatedAt = now
}
