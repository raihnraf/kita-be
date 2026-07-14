package domain

import (
	"errors"
	"time"
)

var ErrStockEventNotFound = errors.New("stock event not found")

type StockEventType string

const (
	StockEventDecrease StockEventType = "DECREASE"
	StockEventIncrease StockEventType = "INCREASE"
)

type StockEventStatus string

const (
	StockEventPending   StockEventStatus = "PENDING"
	StockEventProcessed StockEventStatus = "PROCESSED"
	StockEventFailed    StockEventStatus = "FAILED"
)

type BookStockEvent struct {
	ID            string
	EventID       string
	BookID        string
	TransactionID string
	EventType     StockEventType
	Quantity      int
	Status        StockEventStatus
	ErrorMessage  *string
	ProcessedAt   *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
