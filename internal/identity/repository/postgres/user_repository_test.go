package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	domain "kita-be/internal/identity/domain"
	postgres "kita-be/internal/identity/repository/postgres"
)

func TestUserRepositoryCreateAndFindByEmail(t *testing.T) {
	pool := newTestPoolForIdentity(t)
	repo := postgres.NewUserRepository(pool)
	ctx := context.Background()

	user := domain.NewUser(uuid.NewString(), "Test User", "test@example.com", "hashedpw")
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	found, err := repo.FindByEmail(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("failed to find user by email: %v", err)
	}
	if found.ID != user.ID {
		t.Errorf("expected ID %s, got %s", user.ID, found.ID)
	}
	if found.FullName != "Test User" {
		t.Errorf("expected FullName 'Test User', got %s", found.FullName)
	}
	if found.Email != "test@example.com" {
		t.Errorf("expected Email 'test@example.com', got %s", found.Email)
	}
}

func TestUserRepositoryFindByID(t *testing.T) {
	pool := newTestPoolForIdentity(t)
	repo := postgres.NewUserRepository(pool)
	ctx := context.Background()

	user := domain.NewUser(uuid.NewString(), "Find By ID User", "findbyid@example.com", "hashedpw")
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	found, err := repo.FindByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("failed to find user by ID: %v", err)
	}
	if found.ID != user.ID {
		t.Errorf("expected ID %s, got %s", user.ID, found.ID)
	}
	if found.Email != "findbyid@example.com" {
		t.Errorf("expected Email 'findbyid@example.com', got %s", found.Email)
	}
}

func TestUserRepositoryFindByEmailNotFound(t *testing.T) {
	pool := newTestPoolForIdentity(t)
	repo := postgres.NewUserRepository(pool)
	ctx := context.Background()

	_, err := repo.FindByEmail(ctx, "nonexistent@example.com")
	if err == nil {
		t.Fatal("expected error for non-existent user")
	}
}

func TestRefreshTokenRepositoryCreateAndFindByTokenHash(t *testing.T) {
	pool := newTestPoolForIdentity(t)
	userRepo := postgres.NewUserRepository(pool)
	tokenRepo := postgres.NewRefreshTokenRepository(pool)
	ctx := context.Background()

	user := domain.NewUser(uuid.NewString(), "Token User", "token@example.com", "hashedpw")
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	token := domain.NewRefreshToken(uuid.NewString(), user.ID, "token-hash-123", expiresAt)
	if err := tokenRepo.Create(ctx, token); err != nil {
		t.Fatalf("failed to create refresh token: %v", err)
	}

	found, err := tokenRepo.FindByTokenHash(ctx, "token-hash-123")
	if err != nil {
		t.Fatalf("failed to find refresh token: %v", err)
	}
	if found.ID != token.ID {
		t.Errorf("expected ID %s, got %s", token.ID, found.ID)
	}
	if found.UserID != user.ID {
		t.Errorf("expected UserID %s, got %s", user.ID, found.UserID)
	}
}

func TestRefreshTokenRepositoryRevokeByID(t *testing.T) {
	pool := newTestPoolForIdentity(t)
	userRepo := postgres.NewUserRepository(pool)
	tokenRepo := postgres.NewRefreshTokenRepository(pool)
	ctx := context.Background()

	user := domain.NewUser(uuid.NewString(), "Revoke User", "revoke@example.com", "hashedpw")
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	token := domain.NewRefreshToken(uuid.NewString(), user.ID, "revoke-hash", expiresAt)
	if err := tokenRepo.Create(ctx, token); err != nil {
		t.Fatalf("failed to create refresh token: %v", err)
	}

	if err := tokenRepo.RevokeByID(ctx, token.ID); err != nil {
		t.Fatalf("failed to revoke token: %v", err)
	}

	found, err := tokenRepo.FindByTokenHash(ctx, "revoke-hash")
	if err != nil {
		t.Fatalf("failed to find revoked token: %v", err)
	}
	if found.RevokedAt == nil {
		t.Error("expected RevokedAt to be set after revoke")
	}
}

func TestRefreshTokenRepositoryRotate(t *testing.T) {
	pool := newTestPoolForIdentity(t)
	userRepo := postgres.NewUserRepository(pool)
	tokenRepo := postgres.NewRefreshTokenRepository(pool)
	ctx := context.Background()

	user := domain.NewUser(uuid.NewString(), "Rotate User", "rotate@example.com", "hashedpw")
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	oldToken := domain.NewRefreshToken(uuid.NewString(), user.ID, "old-hash", expiresAt)
	if err := tokenRepo.Create(ctx, oldToken); err != nil {
		t.Fatalf("failed to create old token: %v", err)
	}

	newToken := domain.NewRefreshToken(uuid.NewString(), user.ID, "new-hash", time.Now().Add(48*time.Hour))
	if err := tokenRepo.Rotate(ctx, oldToken.ID, newToken); err != nil {
		t.Fatalf("failed to rotate token: %v", err)
	}

	oldFound, err := tokenRepo.FindByTokenHash(ctx, "old-hash")
	if err != nil {
		t.Fatalf("failed to find old token: %v", err)
	}
	if oldFound.RevokedAt == nil {
		t.Error("expected old token to be revoked after rotation")
	}

	newFound, err := tokenRepo.FindByTokenHash(ctx, "new-hash")
	if err != nil {
		t.Fatalf("failed to find new token: %v", err)
	}
	if newFound.ID != newToken.ID {
		t.Errorf("expected new token ID %s, got %s", newToken.ID, newFound.ID)
	}
}

func newTestPoolForIdentity(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to connect to postgres: %v", err)
	}
	t.Cleanup(pool.Close)

	if _, err := pool.Exec(ctx, `DROP TABLE IF EXISTS refresh_tokens; DROP TABLE IF EXISTS users;`); err != nil {
		t.Fatalf("failed to drop tables: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

		CREATE TABLE users (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			full_name VARCHAR(255) NOT NULL,
			email VARCHAR(255) NOT NULL,
			password_hash VARCHAR(255) NOT NULL,
			role VARCHAR(50) NOT NULL DEFAULT 'MEMBER',
			status VARCHAR(50) NOT NULL DEFAULT 'ACTIVE',
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
		);

		CREATE UNIQUE INDEX idx_users_email ON users(LOWER(email));

		CREATE TABLE refresh_tokens (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			user_id UUID NOT NULL,
			token_hash VARCHAR(255) NOT NULL,
			expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
			revoked_at TIMESTAMP WITH TIME ZONE,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			CONSTRAINT fk_refresh_tokens_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);

		CREATE UNIQUE INDEX idx_refresh_tokens_token_hash ON refresh_tokens(token_hash);
	`); err != nil {
		t.Fatalf("failed to create tables: %v", err)
	}

	return pool
}

func TestUserRepositoryCaseInsensitiveEmailConstraint(t *testing.T) {
	pool := newTestPoolForIdentity(t)
	repo := postgres.NewUserRepository(pool)
	ctx := context.Background()

	// Insert first user
	user1 := domain.NewUser(uuid.NewString(), "User One", "test@example.com", "hash")
	if err := repo.Create(ctx, user1); err != nil {
		t.Fatalf("failed to create user1: %v", err)
	}

	// Insert second user with different email casing - should fail at DB level due to LOWER(email) index
	user2 := domain.NewUser(uuid.NewString(), "User Two", "TEST@EXAMPLE.COM", "hash")
	err := repo.Create(ctx, user2)
	if err == nil {
		t.Fatal("expected database-level duplicate key conflict error, got nil")
	}
}
