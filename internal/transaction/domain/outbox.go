package domain

import "time"

type StockEventOutboxStatus string

const (
	StockEventOutboxPending    StockEventOutboxStatus = "PENDING"
	StockEventOutboxProcessing StockEventOutboxStatus = "PROCESSING"
	StockEventOutboxPublished  StockEventOutboxStatus = "PUBLISHED"
	StockEventOutboxFailed     StockEventOutboxStatus = "FAILED"
	StockEventOutboxSkipped    StockEventOutboxStatus = "SKIPPED"
)

type StockEventOutbox struct {
	ID                       string
	EventType                string
	TransactionID            string
	TransactionRef           string
	CompensationForEventType *string
	CompensationReason       *string
	UserID                   string
	BookID                   string
	Quantity                 int
	Status                   StockEventOutboxStatus
	Attempts                 int
	LastError                *string
	NextAttemptAt            time.Time
	PublishedAt              *time.Time
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

func NewStockEventOutbox(id, eventType string, tx *BorrowTransaction) *StockEventOutbox {
	now := time.Now().UTC()
	return &StockEventOutbox{
		ID:             id,
		EventType:      eventType,
		TransactionID:  tx.ID,
		TransactionRef: tx.TransactionRef,
		UserID:         tx.UserID,
		BookID:         tx.BookID,
		Quantity:       1,
		Status:         StockEventOutboxPending,
		NextAttemptAt:  now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func (o *StockEventOutbox) SetCompensationMetadata(compensatesEventType, reason string) {
	o.CompensationForEventType = &compensatesEventType
	o.CompensationReason = &reason
}
