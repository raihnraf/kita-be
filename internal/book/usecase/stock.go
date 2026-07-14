package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"time"

	domain "kita-be/internal/book/domain"
	"kita-be/internal/platform/apperror"
)

type StockUsecase struct {
	bookRepo BookRepository
}

func NewStockUsecase(bookRepo BookRepository) *StockUsecase {
	return &StockUsecase{bookRepo: bookRepo}
}

type StockAvailabilityOutput struct {
	BookID         string
	AvailableStock int
	CanBorrow      bool
}

func (uc *StockUsecase) CheckAvailability(ctx context.Context, bookID string) (*StockAvailabilityOutput, error) {
	book, err := uc.bookRepo.FindByID(ctx, bookID)
	if err != nil {
		return nil, apperror.NotFound("book not found")
	}

	return &StockAvailabilityOutput{
		BookID:         book.ID,
		AvailableStock: book.AvailableStock,
		CanBorrow:      book.CanBorrow(),
	}, nil
}

func (uc *StockUsecase) DecreaseStock(ctx context.Context, bookID string, qty int, transactionID string) (*domain.BookStockEvent, error) {
	return uc.DecreaseStockEvent(ctx, bookID, qty, transactionID, "")
}

func (uc *StockUsecase) DecreaseStockEvent(ctx context.Context, bookID string, qty int, transactionID string, eventID string) (*domain.BookStockEvent, error) {
	if eventID != "" {
		existingEvent, err := uc.bookRepo.FindStockEventByEventID(ctx, eventID)
		if err == nil && existingEvent != nil {
			if existingEvent.Status == domain.StockEventProcessed || existingEvent.Status == domain.StockEventFailed {
				return existingEvent, nil
			}
			return nil, apperror.Conflict("insufficient stock")
		}
	}

	if transactionID != "" {
		existingEvent, err := uc.bookRepo.FindStockEventByTransactionID(ctx, transactionID, string(domain.StockEventDecrease))
		if err == nil && existingEvent != nil {
			if existingEvent.Status == domain.StockEventProcessed || existingEvent.Status == domain.StockEventFailed {
				return existingEvent, nil
			}
			return nil, apperror.Conflict("insufficient stock")
		}
	}

	event := &domain.BookStockEvent{
		ID:            uuid.New().String(),
		EventID:       eventIDOrNew(eventID),
		BookID:        bookID,
		TransactionID: transactionID,
		EventType:     domain.StockEventDecrease,
		Quantity:      qty,
		Status:        domain.StockEventProcessed,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	appliedEvent, err := uc.bookRepo.ApplyStockEvent(ctx, event)
	if err != nil {
		if errors.Is(err, domain.ErrInsufficientStock) {
			failedEvent := newFailedStockEvent(bookID, qty, transactionID, event.EventID, domain.StockEventDecrease, err.Error())
			if recordErr := uc.bookRepo.RecordStockEvent(ctx, failedEvent); recordErr != nil {
				return nil, fmt.Errorf("failed to record rejected decrease stock event: %w", recordErr)
			}
			return failedEvent, nil
		}
		return nil, fmt.Errorf("failed to decrease stock: %w", err)
	}

	return appliedEvent, nil
}

func (uc *StockUsecase) IncreaseStock(ctx context.Context, bookID string, qty int, transactionID string) (*domain.BookStockEvent, error) {
	return uc.IncreaseStockEvent(ctx, bookID, qty, transactionID, "")
}

func (uc *StockUsecase) IncreaseStockEvent(ctx context.Context, bookID string, qty int, transactionID string, eventID string) (*domain.BookStockEvent, error) {
	if eventID != "" {
		existingEvent, err := uc.bookRepo.FindStockEventByEventID(ctx, eventID)
		if err == nil && existingEvent != nil {
			if existingEvent.Status == domain.StockEventProcessed || existingEvent.Status == domain.StockEventFailed {
				return existingEvent, nil
			}
			return nil, fmt.Errorf("failed to increase stock: previously failed")
		}
	}

	if transactionID != "" {
		existingEvent, err := uc.bookRepo.FindStockEventByTransactionID(ctx, transactionID, string(domain.StockEventIncrease))
		if err == nil && existingEvent != nil {
			if existingEvent.Status == domain.StockEventProcessed || existingEvent.Status == domain.StockEventFailed {
				return existingEvent, nil
			}
			return nil, fmt.Errorf("failed to increase stock: previously failed")
		}
	}

	event := &domain.BookStockEvent{
		ID:            uuid.New().String(),
		EventID:       eventIDOrNew(eventID),
		BookID:        bookID,
		TransactionID: transactionID,
		EventType:     domain.StockEventIncrease,
		Quantity:      qty,
		Status:        domain.StockEventProcessed,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	appliedEvent, err := uc.bookRepo.ApplyStockEvent(ctx, event)
	if err != nil {
		if errors.Is(err, domain.ErrStockExceedsTotal) {
			failedEvent := newFailedStockEvent(bookID, qty, transactionID, event.EventID, domain.StockEventIncrease, err.Error())
			if recordErr := uc.bookRepo.RecordStockEvent(ctx, failedEvent); recordErr != nil {
				return nil, fmt.Errorf("failed to record rejected increase stock event: %w", recordErr)
			}
			return failedEvent, nil
		}
		return nil, fmt.Errorf("failed to increase stock: %w", err)
	}

	return appliedEvent, nil
}

func eventIDOrNew(eventID string) string {
	if eventID != "" {
		return eventID
	}
	return uuid.New().String()
}

func newFailedStockEvent(bookID string, qty int, transactionID string, eventID string, eventType domain.StockEventType, errMessage string) *domain.BookStockEvent {
	now := time.Now()
	message := errMessage
	return &domain.BookStockEvent{
		ID:            uuid.New().String(),
		EventID:       eventIDOrNew(eventID),
		BookID:        bookID,
		TransactionID: transactionID,
		EventType:     eventType,
		Quantity:      qty,
		Status:        domain.StockEventFailed,
		ErrorMessage:  &message,
		ProcessedAt:   &now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func (uc *StockUsecase) HasProcessedDecreaseEvent(ctx context.Context, transactionID string) (bool, error) {
	if transactionID == "" {
		return false, fmt.Errorf("transaction ID is required to verify processed decrease event")
	}
	event, err := uc.bookRepo.FindStockEventByTransactionID(ctx, transactionID, string(domain.StockEventDecrease))
	if err != nil {
		if errors.Is(err, domain.ErrStockEventNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("failed to verify processed decrease event: %w", err)
	}
	return event != nil && event.Status == domain.StockEventProcessed, nil
}
