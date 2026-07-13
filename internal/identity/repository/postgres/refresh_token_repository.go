package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	domain "kita-be/internal/identity/domain"
)

type RefreshTokenRepository struct {
	pool *pgxpool.Pool
}

func NewRefreshTokenRepository(pool *pgxpool.Pool) *RefreshTokenRepository {
	return &RefreshTokenRepository{pool: pool}
}

func (r *RefreshTokenRepository) Create(ctx context.Context, token *domain.RefreshToken) error {
	query := `
		INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, revoked_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err := r.pool.Exec(ctx, query,
		token.ID, token.UserID, token.TokenHash,
		token.ExpiresAt, token.RevokedAt, token.CreatedAt, token.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create refresh token: %w", err)
	}
	return nil
}

func (r *RefreshTokenRepository) FindByTokenHash(ctx context.Context, hash string) (*domain.RefreshToken, error) {
	query := `
		SELECT id, user_id, token_hash, expires_at, revoked_at, created_at, updated_at
		FROM refresh_tokens
		WHERE token_hash = $1
	`

	var token domain.RefreshToken
	err := r.pool.QueryRow(ctx, query, hash).Scan(
		&token.ID, &token.UserID, &token.TokenHash,
		&token.ExpiresAt, &token.RevokedAt, &token.CreatedAt, &token.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find refresh token: %w", err)
	}

	return &token, nil
}

func (r *RefreshTokenRepository) RevokeByID(ctx context.Context, id string) error {
	query := `
		UPDATE refresh_tokens
		SET revoked_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`
	_, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to revoke refresh token: %w", err)
	}
	return nil
}

func (r *RefreshTokenRepository) Rotate(ctx context.Context, oldTokenID string, newToken *domain.RefreshToken) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin refresh token rotation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, oldTokenID); err != nil {
		return fmt.Errorf("failed to revoke refresh token: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, revoked_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, newToken.ID, newToken.UserID, newToken.TokenHash, newToken.ExpiresAt, newToken.RevokedAt, newToken.CreatedAt, newToken.UpdatedAt); err != nil {
		return fmt.Errorf("failed to create refresh token: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit refresh token rotation: %w", err)
	}
	return nil
}

func (r *RefreshTokenRepository) RevokeByUserID(ctx context.Context, userID string) error {
	query := `
		UPDATE refresh_tokens
		SET revoked_at = NOW(), updated_at = NOW()
		WHERE user_id = $1 AND revoked_at IS NULL
	`
	_, err := r.pool.Exec(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to revoke refresh tokens by user: %w", err)
	}
	return nil
}
