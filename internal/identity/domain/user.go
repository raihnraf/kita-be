package domain

import (
	"time"
)

type UserStatus string

const (
	UserStatusActive   UserStatus = "ACTIVE"
	UserStatusInactive UserStatus = "INACTIVE"
)

type UserRole string

const (
	UserRoleMember UserRole = "MEMBER"
	UserRoleAdmin  UserRole = "ADMIN"
)

type User struct {
	ID           string
	FullName     string
	Email        string
	PasswordHash string
	Role         UserRole
	Status       UserStatus
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func NewUser(id, fullName, email, passwordHash string) *User {
	now := time.Now()
	return &User{
		ID:           id,
		FullName:     fullName,
		Email:        email,
		PasswordHash: passwordHash,
		Role:         UserRoleMember,
		Status:       UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}
