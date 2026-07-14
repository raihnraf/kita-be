package http

import (
	domain "kita-be/internal/transaction/domain"
)

type BorrowRequest struct {
	BookID         string `json:"book_id"`
	IdempotencyKey string `json:"idempotency_key"`
}

type ReturnRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
}

type TransactionResponse struct {
	ID              string                `json:"id"`
	TransactionRef  string                `json:"transaction_ref"`
	UserID          string                `json:"user_id"`
	BookID          string                `json:"book_id"`
	Book            *BookSnapshotResponse `json:"book,omitempty"`
	BorrowedAt      string                `json:"borrowed_at"`
	DueAt           string                `json:"due_at"`
	ReturnedAt      *string               `json:"returned_at"`
	Status          string                `json:"status"`
	FineAmountCents int64                 `json:"fine_amount_cents"`
	LateDays        int                   `json:"late_days"`
}

type BookSnapshotResponse struct {
	ISBN   string `json:"isbn"`
	Title  string `json:"title"`
	Author string `json:"author"`
}

type TransactionAuditResponse struct {
	ID            string  `json:"id"`
	TransactionID string  `json:"transaction_id"`
	FromStatus    *string `json:"from_status"`
	ToStatus      string  `json:"to_status"`
	Reason        string  `json:"reason"`
	Metadata      *string `json:"metadata"`
	CreatedAt     string  `json:"created_at"`
}

func FromDomain(tx domain.BorrowTransaction) TransactionResponse {
	resp := TransactionResponse{
		ID:              tx.ID,
		TransactionRef:  tx.TransactionRef,
		UserID:          tx.UserID,
		BookID:          tx.BookID,
		BorrowedAt:      tx.BorrowedAt.Format("2006-01-02T15:04:05Z07:00"),
		DueAt:           tx.DueAt.Format("2006-01-02T15:04:05Z07:00"),
		Status:          string(tx.Status),
		FineAmountCents: tx.FineAmountCents,
		LateDays:        tx.LateDays,
	}
	if tx.ReturnedAt != nil {
		s := tx.ReturnedAt.Format("2006-01-02T15:04:05Z07:00")
		resp.ReturnedAt = &s
	}
	if tx.BookISBN != nil || tx.BookTitle != nil || tx.BookAuthor != nil {
		resp.Book = &BookSnapshotResponse{
			ISBN:   stringValue(tx.BookISBN),
			Title:  stringValue(tx.BookTitle),
			Author: stringValue(tx.BookAuthor),
		}
	}
	return resp
}

func AuditFromDomain(a domain.TransactionAudit) TransactionAuditResponse {
	return TransactionAuditResponse{
		ID:            a.ID,
		TransactionID: a.TransactionID,
		FromStatus:    a.FromStatus,
		ToStatus:      a.ToStatus,
		Reason:        a.Reason,
		Metadata:      a.Metadata,
		CreatedAt:     a.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
