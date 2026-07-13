package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	domain "kita-be/internal/identity/domain"
)

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

func (r *UserRepository) Create(ctx context.Context, user *domain.User) error {
	query := `
		INSERT INTO users (id, full_name, email, password_hash, role, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := r.pool.Exec(ctx, query,
		user.ID, user.FullName, user.Email, user.PasswordHash,
		string(user.Role), string(user.Status), user.CreatedAt, user.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}
	return nil
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	query := `
		SELECT id, full_name, email, password_hash, role, status, created_at, updated_at
		FROM users
		WHERE email = $1
	`

	var user domain.User
	var role, status string

	err := r.pool.QueryRow(ctx, query, email).Scan(
		&user.ID, &user.FullName, &user.Email, &user.PasswordHash,
		&role, &status, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find user by email: %w", err)
	}

	user.Role = domain.UserRole(role)
	user.Status = domain.UserStatus(status)
	return &user, nil
}

func (r *UserRepository) FindByID(ctx context.Context, id string) (*domain.User, error) {
	query := `
		SELECT id, full_name, email, password_hash, role, status, created_at, updated_at
		FROM users
		WHERE id = $1
	`

	var user domain.User
	var role, status string

	err := r.pool.QueryRow(ctx, query, id).Scan(
		&user.ID, &user.FullName, &user.Email, &user.PasswordHash,
		&role, &status, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find user by id: %w", err)
	}

	user.Role = domain.UserRole(role)
	user.Status = domain.UserStatus(status)
	return &user, nil
}
