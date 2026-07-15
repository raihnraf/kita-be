package domain

import (
	"errors"
	"time"
)

var (
	ErrActiveBorrowLimitReached = errors.New("active borrow limit reached")
	ErrTransactionNotActive     = errors.New("transaction is not active")
	ErrBookAlreadyBorrowed      = errors.New("book is already borrowed")
	ErrTransactionNotPending    = errors.New("transaction is not pending")
)

type TransactionStatus string

const (
	TransactionActive        TransactionStatus = "ACTIVE"
	TransactionReturned      TransactionStatus = "RETURNED"
	TransactionReturnedLate  TransactionStatus = "RETURNED_LATE"
	TransactionPending       TransactionStatus = "PENDING"
	TransactionReturnPending TransactionStatus = "RETURN_PENDING"
	TransactionCancelled     TransactionStatus = "CANCELLED"
)

type BorrowTransaction struct {
	ID              string
	TransactionRef  string
	UserID          string
	BookID          string
	BookISBN        *string
	BookTitle       *string
	BookAuthor      *string
	BorrowedAt      time.Time
	DueAt           time.Time
	ReturnedAt      *time.Time
	Status          TransactionStatus
	FineAmountCents int64
	LateDays        int
	StockEventID    *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type BookSnapshot struct {
	ISBN           string
	Title          string
	Author         string
	AvailableStock int
	CanBorrow      bool
}

func NewBorrowTransaction(id, ref, userID, bookID string, borrowedAt, dueAt time.Time) *BorrowTransaction {
	now := time.Now()
	return &BorrowTransaction{
		ID:             id,
		TransactionRef: ref,
		UserID:         userID,
		BookID:         bookID,
		BorrowedAt:     borrowedAt,
		DueAt:          dueAt,
		Status:         TransactionActive,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func NewPendingBorrowTransaction(id, ref, userID, bookID string, borrowedAt, dueAt time.Time) *BorrowTransaction {
	now := time.Now()
	return &BorrowTransaction{
		ID:             id,
		TransactionRef: ref,
		UserID:         userID,
		BookID:         bookID,
		BorrowedAt:     borrowedAt,
		DueAt:          dueAt,
		Status:         TransactionPending,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func (tx *BorrowTransaction) SetBookSnapshot(snapshot *BookSnapshot) {
	if snapshot == nil {
		return
	}
	tx.BookISBN = &snapshot.ISBN
	tx.BookTitle = &snapshot.Title
	tx.BookAuthor = &snapshot.Author
}

func (tx *BorrowTransaction) IsActive() bool {
	return tx.Status == TransactionActive
}

func (tx *BorrowTransaction) IsPending() bool {
	return tx.Status == TransactionPending
}

func (tx *BorrowTransaction) IsReturnPending() bool {
	return tx.Status == TransactionReturnPending
}

func (tx *BorrowTransaction) IsCancelable() bool {
	return tx.Status == TransactionPending
}

func (tx *BorrowTransaction) BelongsTo(userID string) bool {
	return tx.UserID == userID
}
